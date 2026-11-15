package toolinstall

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/execx"
	"github.com/rudian/codepass/internal/netx"
	"github.com/rudian/codepass/internal/redact"
)

// ErrRunning is returned by Start while a run is going.
var ErrRunning = errors.New("toolinstall: a setup run is already going")

const logCap = 200

// Config wires the installer to the machine. The zero value of every optional field is the real thing.
type Config struct {
	GOOS     string // default runtime.GOOS
	Net      Net
	CacheDir string // the app's private download folder; Homebrew.pkg is saved here
	Emit     func(api.InstallState)
	Lang     func() string // "zh-CN" | "en", for the sentence in the macOS password window

	Runner      Runner // brew and winget; default RealRunner
	OSAScript   string // default OSAScriptPath
	Pkgutil     string // default PkgutilPath
	Team        string // default HomebrewTeamID
	APIURL      string // default ReleaseAPIURL
	FallbackURL string // default FallbackURL
	PrefixDir   string // default /opt/homebrew
	BrewPath    string // default <PrefixDir>/bin/brew
	WingetPath  string // default: the path the gate found
	BaseEnv     func() []string
	// Recheck runs the gate's local checks again. After each install step the row of that tool must be ready;
	// when it still blocks, the run fails with a cause instead of finishing "done". nil: no check.
	Recheck func(ctx context.Context) api.SetupState
}

func (c *Config) defaults() {
	if c.GOOS == "" {
		c.GOOS = goos()
	}
	if c.Runner == nil {
		c.Runner = RealRunner()
	}
	if c.OSAScript == "" {
		c.OSAScript = OSAScriptPath
	}
	if c.Pkgutil == "" {
		c.Pkgutil = PkgutilPath
	}
	if c.Team == "" {
		c.Team = HomebrewTeamID
	}
	if c.APIURL == "" {
		c.APIURL = ReleaseAPIURL
	}
	if c.FallbackURL == "" {
		c.FallbackURL = FallbackURL
	}
	if c.PrefixDir == "" {
		c.PrefixDir = "/opt/homebrew"
	}
	if c.BrewPath == "" {
		c.BrewPath = c.PrefixDir + "/bin/brew"
	}
	if c.BaseEnv == nil {
		c.BaseEnv = execx.ChildEnv
	}
	if c.Lang == nil {
		c.Lang = func() string { return "en" }
	}
}

// Installer runs one setup at a time and reports every change as an api.InstallState.
type Installer struct {
	cfg Config

	mu        sync.Mutex
	st        api.InstallState
	cancel    context.CancelFunc
	cancelDL  bool // the running step can be stopped at once (the download)
	lastEmit  time.Time
	pending   *time.Timer
	done      chan struct{}
	setup     api.SetupState
	asset     Asset
	pkgPath   string
	startedAt time.Time
}

// New returns an installer. Net may be nil only when no run will download.
func New(cfg Config) *Installer {
	cfg.defaults()
	return &Installer{cfg: cfg, st: api.InstallState{Steps: []api.InstallStep{}, Log: []string{}}}
}

// State returns a copy of the current snapshot.
func (in *Installer) State() api.InstallState {
	in.mu.Lock()
	defer in.mu.Unlock()
	return copyState(in.st)
}

func copyState(s api.InstallState) api.InstallState {
	s.Steps = append([]api.InstallStep{}, s.Steps...)
	s.Log = append([]string{}, s.Log...)
	if s.Failure != nil {
		f := *s.Failure
		s.Failure = &f
	}
	return s
}

// Wait blocks until the current run (if any) has ended. Tests use it.
func (in *Installer) Wait() {
	in.mu.Lock()
	d := in.done
	in.mu.Unlock()
	if d != nil {
		<-d
	}
}

// Cancel asks the run to stop. A download stops at once; every other step finishes first ("stops after the
// current step; what is installed stays"), and the steps after it are skipped.
func (in *Installer) Cancel() {
	in.mu.Lock()
	defer in.mu.Unlock()
	if !in.st.Running {
		return
	}
	in.st.Cancelling = true
	if in.cancelDL && in.cancel != nil {
		in.cancel()
	}
	in.emitLocked(true)
}

// Start begins a run for every row of setup that blocks the gate. It returns at once; progress arrives through
// Config.Emit. The setup state is the one the screen showed, so "try again" re-detects first and continues from
// the first row that is still missing.
func (in *Installer) Start(parent context.Context, setup api.SetupState) error {
	in.mu.Lock()
	if in.st.Running {
		in.mu.Unlock()
		return ErrRunning
	}
	steps := in.plan(setup)
	in.setup = setup
	in.asset, in.pkgPath = Asset{}, ""
	in.st = api.InstallState{Running: true, Steps: steps, Log: []string{}}
	ctx, cancel := context.WithCancel(parent)
	in.cancel = cancel
	in.done = make(chan struct{})
	in.startedAt = time.Now()
	in.emitLocked(true)
	in.mu.Unlock()
	go in.run(ctx)
	return nil
}

// ---- plan ------------------------------------------------------------------------------------------------

func brewCommand(verb, formula string) string { return "brew " + verb + " " + formula }

func (in *Installer) plan(s api.SetupState) []api.InstallStep {
	steps := []api.InstallStep{}
	add := func(id string, tool api.SetupToolID, kind api.InstallStepKind, cmd string) {
		steps = append(steps, api.InstallStep{ID: id, Tool: tool, Kind: kind, State: api.StepQueued, Command: cmd})
	}
	for _, t := range s.Tools {
		if !t.Blocking || t.Fix == api.FixUseDetected {
			continue // use-detected: the service clears the chosen path, nothing is installed
		}
		switch t.ID {
		case api.SetupHomebrew:
			if t.Fix == api.FixRepairPrefix {
				add("homebrew:install", t.ID, api.StepInstall, "")
				continue
			}
			add("homebrew:download", t.ID, api.StepDownload, "Homebrew.pkg")
			add("homebrew:verify", t.ID, api.StepVerify, "pkgutil --check-signature")
			add("homebrew:install", t.ID, api.StepInstall, "installer -pkg Homebrew.pkg")
			steps[len(steps)-1].Indeterminate = true
		case api.SetupWinget:
			add("winget:install", t.ID, api.StepInstall, "")
		case api.SetupGit, api.SetupAWSCLI:
			add(string(t.ID)+":install", t.ID, api.StepInstall, in.toolCommand(t))
		}
	}
	return steps
}

func (in *Installer) toolCommand(t api.SetupTool) string {
	if in.cfg.GOOS == "windows" {
		// `winget install` upgrades an installed copy too, and works when the old one did not come from winget.
		return "winget " + strings.Join(wingetArgs(t.ID)[:wingetShown(t.ID)], " ")
	}
	formula := brewFormula(t.ID)
	verb := "install"
	// A copy that brew itself installed is upgraded; a copy found elsewhere is left alone and a newer one is
	// installed next to it (it comes first on the PATH).
	if t.Fix == api.FixUpgrade && !t.Elsewhere {
		verb = "upgrade"
	}
	return brewCommand(verb, formula)
}

func brewFormula(id api.SetupToolID) string {
	if id == api.SetupAWSCLI {
		return "awscli"
	}
	return "git"
}

// wingetArgs is the complete argument list of one winget call. The AWS CLI is installed machine-wide until the
// per-user question is settled (plan 4.1, owner decision 1, check B11).
func wingetArgs(id api.SetupToolID) []string {
	args := []string{"install", "--id", wingetID(id), "-e"}
	if id == api.SetupAWSCLI {
		args = append(args, "--scope", "machine")
	}
	return append(args, "--accept-package-agreements", "--accept-source-agreements", "--silent")
}

// wingetShown is how many leading arguments of wingetArgs the screen shows as the command (not the switches).
func wingetShown(id api.SetupToolID) int {
	if id == api.SetupAWSCLI {
		return 6
	}
	return 4
}

func wingetID(id api.SetupToolID) string {
	if id == api.SetupAWSCLI {
		return "Amazon.AWSCLI"
	}
	return "Git.Git"
}

// ---- state helpers ---------------------------------------------------------------------------------------

func (in *Installer) stepIdx(id string) int {
	for i := range in.st.Steps {
		if in.st.Steps[i].ID == id {
			return i
		}
	}
	return -1
}

func (in *Installer) setStep(id string, f func(*api.InstallStep), force bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	if i := in.stepIdx(id); i >= 0 {
		f(&in.st.Steps[i])
	}
	in.emitLocked(force)
}

func (in *Installer) addLog(line string) {
	line = redact.String(line)
	in.mu.Lock()
	defer in.mu.Unlock()
	in.st.Log = append(in.st.Log, line)
	if n := len(in.st.Log); n > logCap {
		in.st.Log = append([]string(nil), in.st.Log[n-logCap:]...)
	}
	in.emitLocked(false)
}

// emitLocked sends a snapshot. Log lines are coalesced to one event per 100 ms; a state change goes at once.
func (in *Installer) emitLocked(force bool) {
	if in.cfg.Emit == nil {
		return
	}
	const gap = 100 * time.Millisecond
	if !force && time.Since(in.lastEmit) < gap {
		if in.pending == nil {
			in.pending = time.AfterFunc(gap, func() {
				in.mu.Lock()
				defer in.mu.Unlock()
				in.pending = nil
				in.emitLocked(true)
			})
		}
		return
	}
	if in.pending != nil {
		in.pending.Stop()
		in.pending = nil
	}
	in.lastEmit = time.Now()
	in.cfg.Emit(copyState(in.st))
}

func (in *Installer) cancelRequested() bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.st.Cancelling
}

func (in *Installer) finish(fail *api.InstallFailure, cancelled bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.st.Running, in.st.Finished, in.st.Cancelling = false, true, false
	in.st.Cancelled = cancelled
	in.st.Failure = fail
	for i := range in.st.Steps {
		if s := in.st.Steps[i].State; s == api.StepQueued || s == api.StepRunning || s == api.StepWaitingOS {
			in.st.Steps[i].State = api.StepSkipped
		}
	}
	if fail != nil {
		if i := in.stepIdx(fail.StepID); i >= 0 {
			in.st.Steps[i].State = api.StepFailed
		}
	}
	in.emitLocked(true)
	if in.cancel != nil {
		in.cancel()
	}
	close(in.done)
}

// ---- the run ---------------------------------------------------------------------------------------------

func (in *Installer) run(ctx context.Context) {
	in.mu.Lock()
	ids := make([]string, len(in.st.Steps))
	for i, s := range in.st.Steps {
		ids[i] = s.ID
	}
	in.mu.Unlock()

	if len(ids) > 0 && !in.setup.Preflight.EnoughSpace {
		in.finish(&api.InstallFailure{Tool: in.stepTool(ids[0]), StepID: ids[0], Cause: api.CauseDiskFull, ExitCode: -1}, false)
		return
	}
	for _, id := range ids {
		if in.cancelRequested() || ctx.Err() != nil {
			in.finish(nil, true)
			return
		}
		in.setStepState(id, api.StepRunning)
		if f := in.runStep(ctx, id); f != nil {
			if f.Cause == api.CauseCancelled && in.cancelRequested() {
				in.finish(nil, true)
				return
			}
			in.finish(in.failure(id, f), false)
			return
		}
		if f := in.recheck(ctx, id); f != nil {
			in.finish(in.failure(id, f), false)
			return
		}
		in.setStepState(id, api.StepDone)
	}
	in.finish(nil, in.cancelRequested())
}

// recheck runs the gate again after an install step. The row of that step's tool must be ready now; a step that
// "succeeded" without changing the answer (a stale brew cache, a winget no-op, a copy that is not found first)
// ends the run with a cause instead of looping for ever.
func (in *Installer) recheck(ctx context.Context, id string) *Failure {
	if in.cfg.Recheck == nil || !strings.HasSuffix(id, ":install") {
		return nil
	}
	tool := api.SetupToolID(strings.TrimSuffix(id, ":install"))
	st := in.cfg.Recheck(ctx)
	for _, t := range st.Tools {
		if t.ID != tool || !t.Blocking {
			continue
		}
		what := string(t.Status)
		if t.Version != "" {
			what = t.Version + ", needs " + t.Min
		}
		return &Failure{Cause: api.CauseInstallerFailed, Code: -1, Detail: string(tool) + " is still not usable after the install (" + what + ")"}
	}
	return nil
}

func (in *Installer) setStepState(id string, st api.InstallStepState) {
	in.setStep(id, func(s *api.InstallStep) { s.State = st }, true)
}

func (in *Installer) stepTool(id string) api.SetupToolID {
	in.mu.Lock()
	defer in.mu.Unlock()
	if i := in.stepIdx(id); i >= 0 {
		return in.st.Steps[i].Tool
	}
	return ""
}

func (in *Installer) failure(id string, f *Failure) *api.InstallFailure {
	in.mu.Lock()
	defer in.mu.Unlock()
	out := &api.InstallFailure{StepID: id, Cause: f.Cause, ExitCode: f.Code, Line: lastLine(f.Detail)}
	if i := in.stepIdx(id); i >= 0 {
		out.Tool, out.Command = in.st.Steps[i].Tool, in.st.Steps[i].Command
	}
	if f.Cause == api.CauseAuthDenied && in.setup.Preflight.Admin == api.TriNo {
		out.Cause = api.CauseNotAdmin
	}
	if f.Cause == api.CauseNetwork || f.Cause == api.CauseProxyAuth {
		out.Proxy = in.setup.Preflight.Proxy.HostPort
	}
	out.Line = redact.String(out.Line)
	return out
}

func lastLine(s string) string {
	ls := splitLines(s)
	for i := len(ls) - 1; i >= 0; i-- {
		if l := CleanLine(ls[i]); l != "" {
			return l
		}
	}
	return ""
}

func (in *Installer) runStep(ctx context.Context, id string) *Failure {
	switch id {
	case "homebrew:download":
		return in.stepDownload(ctx)
	case "homebrew:verify":
		return in.stepVerify(ctx)
	case "homebrew:install":
		return in.stepPkgInstall(ctx)
	case "winget:install":
		return fail(api.CauseWingetMissing, "winget is not available")
	}
	tool, _, _ := strings.Cut(id, ":")
	in.mu.Lock()
	var row api.SetupTool
	for _, t := range in.setup.Tools {
		if string(t.ID) == tool {
			row = t
		}
	}
	in.mu.Unlock()
	if in.cfg.GOOS == "windows" {
		return in.stepWinget(ctx, id, row)
	}
	return in.stepBrew(ctx, id, row)
}

// ---- Homebrew.pkg ------------------------------------------------------------------------------------------

func (in *Installer) stepDownload(ctx context.Context) *Failure {
	if in.cfg.Net == nil {
		return fail(api.CauseInstallerFailed, "no network client")
	}
	// Cancel works from the first moment: the release lookup can take 30 s behind a slow proxy.
	in.mu.Lock()
	in.cancelDL = true
	cancelled := in.st.Cancelling
	in.mu.Unlock()
	defer func() {
		in.mu.Lock()
		in.cancelDL = false
		in.mu.Unlock()
	}()
	if cancelled {
		return fail(api.CauseCancelled, "cancelled")
	}
	a, f := FetchAsset(ctx, in.cfg.Net, in.cfg.APIURL, in.cfg.FallbackURL)
	if f != nil {
		if ctx.Err() != nil {
			return fail(api.CauseCancelled, "cancelled")
		}
		return f
	}
	in.mu.Lock()
	in.asset = a
	in.mu.Unlock()
	if in.cancelRequested() {
		return fail(api.CauseCancelled, "cancelled")
	}
	// Try again continues: a file from an earlier run that matches the published size and digest is reused.
	if p := filepath.Join(in.cfg.CacheDir, pkgAsset); a.Size > 0 && a.Sha256 != "" {
		if PreCheck0(p, a) {
			in.mu.Lock()
			in.pkgPath = p
			in.mu.Unlock()
			return nil
		}
	}
	last := time.Now()
	path, f := Download(ctx, in.cfg.Net, a, in.cfg.CacheDir, func(done, total int64) {
		if time.Since(last) < 150*time.Millisecond && done != total {
			return
		}
		last = time.Now()
		in.setStep("homebrew:download", func(s *api.InstallStep) { s.Done, s.Total = done, total }, false)
	})
	if f != nil {
		if ctx.Err() != nil {
			return fail(api.CauseCancelled, "cancelled")
		}
		return f
	}
	in.mu.Lock()
	in.pkgPath = path
	in.mu.Unlock()
	return nil
}

// PreCheck0 reports whether the file at p is a regular file of the expected size and digest (no pkgutil).
func PreCheck0(p string, a Asset) bool {
	st, err := os.Lstat(p)
	if err != nil || !st.Mode().IsRegular() || st.Size() != a.Size {
		return false
	}
	return fileSHA256(p) == a.Sha256
}

func (in *Installer) stepVerify(ctx context.Context) *Failure {
	in.mu.Lock()
	path, a := in.pkgPath, in.asset
	in.mu.Unlock()
	f := PreCheck(ctx, in.cfg.Pkgutil, PreCheckInput{Path: path, Team: in.cfg.Team, Size: a.Size, Sha256: a.Sha256})
	if f != nil && (f.Cause == api.CauseDamagedDownload || f.Cause == api.CauseBadSignature || f.Cause == api.CauseUnknownSigner) {
		in.removeDownload(path) // "try again" downloads it again
	}
	if f != nil {
		in.addLog("verify: " + f.Detail)
	}
	if f == nil {
		return nil
	}
	return f
}

// removeDownload deletes the one file this run saved, by its exact path, and only when it is inside CacheDir.
func (in *Installer) removeDownload(path string) {
	if path == "" || filepath.Dir(path) != filepath.Clean(in.cfg.CacheDir) || filepath.Base(path) != pkgAsset {
		return
	}
	os.Remove(path)
}

func prompt(lang string) string {
	if strings.HasPrefix(lang, "zh") {
		return "Codepass 需要你的密码来安装 Homebrew。"
	}
	return "Codepass needs your password to install Homebrew."
}

func (in *Installer) stepPkgInstall(ctx context.Context) *Failure {
	in.mu.Lock()
	setup, a, path := in.setup, in.asset, in.pkgPath
	in.mu.Unlock()
	for _, t := range setup.Tools {
		if t.ID == api.SetupHomebrew && t.Fix == api.FixRepairPrefix {
			return fail(api.CausePrefixExists, "%s already exists", in.cfg.PrefixDir)
		}
	}
	// A cheap look before the password window; the script repeats it as root.
	if _, err := os.Lstat(in.cfg.PrefixDir); err == nil {
		return fail(api.CausePrefixExists, "%s already exists", in.cfg.PrefixDir)
	}
	in.setStep("homebrew:install", func(s *api.InstallStep) { s.State = api.StepWaitingOS }, true)
	res, err := runOSAScript(ctx, in.cfg.OSAScript, PkgRequest{
		Path: path, Team: in.cfg.Team, Size: a.Size, Sha256: a.Sha256, Mode: "install", Prompt: prompt(in.cfg.Lang()),
	})
	if err != nil {
		if ctx.Err() != nil {
			return fail(api.CauseCancelled, "cancelled")
		}
		return fail(api.CauseInstallerFailed, "cannot start osascript: %v", err)
	}
	if res.Exit != 0 {
		cause := MacPrivileged(res.Exit, res.Stderr)
		code, ok := ScriptCode(res.Stderr)
		if !ok {
			code = res.Exit
		}
		lines := osaLines(res.Stderr)
		for _, l := range lines {
			in.addLog(l)
		}
		return &Failure{Cause: cause, Detail: strings.Join(lines, "\n"), Code: code}
	}
	for _, l := range osaLines(res.Stdout) {
		in.addLog(l) // "verified: size=... gatekeeper=off"
	}
	in.addLog("Homebrew is installed")
	in.removeDownload(path) // the package is no longer needed
	return nil
}

// ---- brew and winget ---------------------------------------------------------------------------------------

// toolEnv is the environment of brew and winget: the app's child environment without any proxy variable, then
// the proxy the app detected (upper and lower case), then the Homebrew switches for a quiet, private run.
func (in *Installer) toolEnv(extra ...string) []string {
	base := in.cfg.BaseEnv()
	env := make([]string, 0, len(base)+16)
	for _, kv := range netx.StripProxyEnv(base) {
		if strings.HasPrefix(strings.ToUpper(kv), "PATH=") {
			continue
		}
		env = append(env, kv)
	}
	if in.cfg.Net != nil {
		env = append(env, in.cfg.Net.Proxy().ChildEnv()...)
	}
	return append(env, extra...)
}

// wingetEnv gives winget, and every installer it starts, a PATH of its own: the system folders and the
// WindowsApps folder (where the winget alias lives). Without one an installer cannot find cmd, powershell or msiexec.
func (in *Installer) wingetEnv() []string {
	get := func(k string) string {
		for _, kv := range in.cfg.BaseEnv() {
			if n, v, ok := strings.Cut(kv, "="); ok && strings.EqualFold(n, k) {
				return v
			}
		}
		return ""
	}
	root := get("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	dirs := []string{root + `\System32`, root, root + `\System32\Wbem`, root + `\System32\WindowsPowerShell\v1.0`}
	if la := get("LOCALAPPDATA"); la != "" {
		dirs = append(dirs, la+`\Microsoft\WindowsApps`)
	}
	return in.toolEnv("PATH=" + strings.Join(dirs, ";"))
}

func (in *Installer) brewEnv() []string {
	p := in.cfg.PrefixDir
	return in.toolEnv(
		"PATH="+p+"/bin:"+p+"/sbin:/usr/bin:/bin:/usr/sbin:/sbin",
		"HOMEBREW_NO_ANALYTICS=1", "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ENV_HINTS=1",
		"HOMEBREW_NO_INSTALL_CLEANUP=1", "NONINTERACTIVE=1", "NO_COLOR=1",
	)
}

func (in *Installer) stepBrew(ctx context.Context, id string, row api.SetupTool) *Failure {
	if st, err := os.Stat(in.cfg.BrewPath); err != nil || st.IsDir() {
		return fail(api.CauseInstallerFailed, "brew is not installed at %s", in.cfg.BrewPath)
	}
	verb, formula := "install", brewFormula(row.ID)
	in.mu.Lock()
	if i := in.stepIdx(id); i >= 0 && strings.HasPrefix(in.st.Steps[i].Command, "brew upgrade") {
		verb = "upgrade"
	}
	in.mu.Unlock()
	return in.runTool(ctx, Cmd{Path: in.cfg.BrewPath, Args: []string{verb, formula}, Env: in.brewEnv()}, func(exit int, out string) *Failure {
		return &Failure{Cause: Brew(exit, out), Detail: errorLine(out), Code: exit}
	})
}

func (in *Installer) stepWinget(ctx context.Context, id string, row api.SetupTool) *Failure {
	path := in.cfg.WingetPath
	if path == "" {
		for _, t := range in.setup.Tools {
			if t.ID == api.SetupWinget {
				path = t.Path
			}
		}
	}
	if path == "" {
		return fail(api.CauseWingetMissing, "winget is not available")
	}
	return in.runTool(ctx, Cmd{Path: path, Args: wingetArgs(row.ID), Env: in.wingetEnv()}, func(exit int, out string) *Failure {
		cause := Winget(uint32(int32(exit)))
		if cause == "" {
			return nil // "no applicable upgrade" and "already installed" are success
		}
		return &Failure{Cause: cause, Detail: errorLine(out), Code: exit}
	})
}

// runTool runs one program, feeding the log, and classifies a non-zero exit with classify.
func (in *Installer) runTool(ctx context.Context, c Cmd, classify func(exit int, out string) *Failure) *Failure {
	var tail []string
	exit, err := in.cfg.Runner.Run(ctx, c, func(l string) {
		in.addLog(l)
		tail = append(tail, l)
		if len(tail) > 60 {
			tail = tail[len(tail)-60:]
		}
	})
	if err != nil {
		if ctx.Err() != nil {
			return fail(api.CauseCancelled, "cancelled")
		}
		cause := api.CauseInstallerFailed
		if in.cfg.GOOS == "windows" {
			if c := WingetStart(err.Error()); c != api.CauseUnknown {
				cause = c
			}
		}
		return fail(cause, "cannot start %s: %v", filepath.Base(c.Path), err)
	}
	if exit == 0 {
		return nil
	}
	f := classify(exit, strings.Join(tail, "\n"))
	if f == nil {
		return nil
	}
	if f.Cause == "" {
		f.Cause = api.CauseInstallerFailed
	}
	return f
}

// errorLine picks the line that explains a failure: the last "Error:" line, else the last line.
func errorLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "Error:") {
			return lines[i]
		}
	}
	return lines[len(lines)-1]
}

// splitLines splits on \n and on \r: the real osascript returns the script's lines joined by \r.
func splitLines(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' })
}

var (
	reOSAPrefix = regexp.MustCompile(`^(\d+:\d+: )?execution error: `)
	reOSASuffix = regexp.MustCompile(`\s*\((-?\d+)\)$`)
)

// osaLines turns osascript's output into clean log lines: split on \r and \n, control characters removed, the
// "[N:M: ]execution error: " prefix of the first line and the "(code)" tail of the last line dropped.
func osaLines(s string) []string {
	raw := splitLines(s)
	out := make([]string, 0, len(raw))
	for i, l := range raw {
		if i == 0 {
			l = reOSAPrefix.ReplaceAllString(l, "")
		}
		if i == len(raw)-1 {
			l = reOSASuffix.ReplaceAllString(l, "")
		}
		if l = CleanLine(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// RemoveStaleDownload deletes Homebrew.pkg from the download folder when no run is going (a cancelled password
// window, a wrong password or a later brew failure leaves the 154 MB file behind). The caller says so only when
// Homebrew is installed.
func (in *Installer) RemoveStaleDownload() {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.st.Running || in.cfg.CacheDir == "" {
		return
	}
	p := filepath.Join(in.cfg.CacheDir, pkgAsset)
	if st, err := os.Lstat(p); err == nil && st.Mode().IsRegular() {
		os.Remove(p)
	}
}
