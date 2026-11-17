package aitools

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/execx"
	"github.com/rudian/codepass/internal/toolinstall"
)

// ErrRunning is returned by Start while a run is going.
var ErrRunning = errors.New("aitools: an install is already running")

// ErrNoInstaller is returned for a system with no install command (not macOS or Windows).
var ErrNoInstaller = errors.New("aitools: no install command for this system")

const logCap = 200

// InstallerConfig wires the installer to the machine. The zero value of every optional field is the real thing.
type InstallerConfig struct {
	GOOS   string
	Runner toolinstall.Runner // brew, winget and npm; nil = toolinstall.RealRunner
	// Lookup finds brew, winget or npm; nil = execx.LookPath, then Homebrew's default folders on macOS.
	Lookup func(name string) (string, error)
	// BaseEnv is the app's child environment; nil = execx.ChildEnv. Any proxy variable in it is replaced by ProxyEnv.
	BaseEnv  func() []string
	ProxyEnv func() []string
	// Recheck detects the tool again after a successful run; nil = no check.
	Recheck func(ctx context.Context, id ID) Detection
	// Info turns a detection into the row of the screen (the service adds the install command).
	Info func(d Detection) api.AIToolInfo
	Emit func(api.AIInstallState)
}

// Installer runs one install at a time and reports every change as an api.AIInstallState. Like
// toolinstall.Installer, it shows the exact command first, streams the installer's own lines, and stops on
// Cancel.
type Installer struct {
	cfg InstallerConfig

	mu       sync.Mutex
	st       api.AIInstallState
	cancel   context.CancelFunc
	done     chan struct{}
	lastEmit time.Time
	pending  *time.Timer // the flush of log lines that came inside the gap
}

// NewInstaller returns an installer.
func NewInstaller(cfg InstallerConfig) *Installer {
	if cfg.GOOS == "" {
		cfg.GOOS = runtimeGOOS
	}
	if cfg.Runner == nil {
		cfg.Runner = toolinstall.RealRunner()
	}
	if cfg.Lookup == nil {
		cfg.Lookup = defaultLookup
	}
	if cfg.BaseEnv == nil {
		cfg.BaseEnv = execx.ChildEnv
	}
	return &Installer{cfg: cfg, st: api.AIInstallState{Log: []string{}}}
}

func defaultLookup(name string) (string, error) {
	if p, err := execx.LookPath(name); err == nil {
		return p, nil
	}
	if name == "brew" {
		for _, p := range []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"} {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p, nil
			}
		}
	}
	return "", execx.ErrNotFound
}

// State returns a copy of the current snapshot.
func (in *Installer) State() api.AIInstallState {
	in.mu.Lock()
	defer in.mu.Unlock()
	return copyInstallState(in.st)
}

func copyInstallState(s api.AIInstallState) api.AIInstallState {
	s.Log = append([]string{}, s.Log...)
	if s.Failure != nil {
		f := *s.Failure
		s.Failure = &f
	}
	if s.Installed != nil {
		i := *s.Installed
		s.Installed = &i
	}
	return s
}

// Wait blocks until the current run has ended (tests).
func (in *Installer) Wait() {
	in.mu.Lock()
	d := in.done
	in.mu.Unlock()
	if d != nil {
		<-d
	}
}

// Cancel stops the run: the installer is asked to stop (SIGINT; the hard kill follows after five seconds).
func (in *Installer) Cancel() {
	in.mu.Lock()
	defer in.mu.Unlock()
	if !in.st.Running {
		return
	}
	in.st.Cancelling = true
	if in.cancel != nil {
		in.cancel()
	}
	in.emitLocked(true)
}

// emitLocked sends a snapshot. Log lines are coalesced to one event per 100 ms: a line that arrives inside the
// gap is sent by a timer when the gap ends, so the last lines of a burst are never lost. A state change goes at
// once and cancels the timer.
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
	in.cfg.Emit(copyInstallState(in.st))
}

func (in *Installer) addLog(l string) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.st.Log = append(in.st.Log, l)
	if len(in.st.Log) > logCap {
		in.st.Log = in.st.Log[len(in.st.Log)-logCap:]
	}
	in.emitLocked(false)
}

// Start begins installing one tool. It returns at once; progress arrives through Config.Emit.
func (in *Installer) Start(parent context.Context, id ID) error {
	if !id.Valid() {
		return ErrBadTool
	}
	ins, ok := InstallFor(id, in.cfg.GOOS)
	if !ok {
		return ErrNoInstaller
	}
	in.mu.Lock()
	if in.st.Running {
		in.mu.Unlock()
		return ErrRunning
	}
	ctx, cancel := context.WithCancel(parent)
	in.cancel = cancel
	in.done = make(chan struct{})
	in.st = api.AIInstallState{Tool: api.AIToolID(id), Command: ins.Display(), Running: true, Log: []string{}}
	in.emitLocked(true)
	in.mu.Unlock()
	go in.run(ctx, id, ins)
	return nil
}

func (in *Installer) run(ctx context.Context, id ID, ins Install) {
	defer func() {
		in.mu.Lock()
		close(in.done)
		in.mu.Unlock()
	}()
	fail := func(cause api.AIInstallCause, detail string, exit int) {
		in.mu.Lock()
		defer in.mu.Unlock()
		in.st.Running, in.st.Finished, in.st.Cancelling = false, true, false
		in.st.Failure = &api.AIInstallFailure{Cause: cause, Detail: detail, Exit: exit}
		if cause == api.AICauseCancelled {
			in.st.Cancelled, in.st.Failure = true, nil
		}
		in.emitLocked(true)
	}
	exe, err := in.cfg.Lookup(ins.Exe)
	if err != nil {
		fail(api.AICauseInstallerMissing, ins.Exe+" is not installed", -1)
		return
	}
	var tail []string
	exit, err := in.cfg.Runner.Run(ctx, toolinstall.Cmd{Path: exe, Args: ins.Args, Env: in.env(ins.Exe, exe)}, func(l string) {
		in.addLog(l)
		tail = append(tail, l)
		if len(tail) > 60 {
			tail = tail[len(tail)-60:]
		}
	})
	switch {
	case ctx.Err() != nil:
		fail(api.AICauseCancelled, "", exit)
		return
	case err != nil:
		fail(api.AICauseFailed, "cannot start "+ins.Exe+": "+err.Error(), exit)
		return
	case exit != 0:
		fail(api.AICauseFailed, errorLine(tail), exit)
		return
	}
	var info *api.AIToolInfo
	if in.cfg.Recheck != nil {
		d := in.cfg.Recheck(ctx, id)
		if ctx.Err() != nil {
			fail(api.AICauseCancelled, "", 0)
			return
		}
		if !d.Installed {
			fail(api.AICauseStillMissing, "the installer finished but "+id.Name()+" was not found afterwards", 0)
			return
		}
		if in.cfg.Info != nil {
			i := in.cfg.Info(d)
			info = &i
		}
	}
	in.mu.Lock()
	in.st.Running, in.st.Finished, in.st.Cancelling = false, true, false
	in.st.Installed = info
	in.emitLocked(true)
	in.mu.Unlock()
}

// env is the environment of the installer: the app's child environment without its proxy variables, the proxy
// the app detected, and quiet, non-interactive switches.
func (in *Installer) env(exe, path string) []string {
	var env []string
	env = append(env, stripProxy(in.cfg.BaseEnv())...)
	if in.cfg.ProxyEnv != nil {
		env = append(env, in.cfg.ProxyEnv()...)
	}
	if exe == "brew" {
		dir := path
		if i := strings.LastIndex(dir, "/bin/"); i >= 0 {
			dir = dir[:i]
		}
		env = append(env, "PATH="+dir+"/bin:"+dir+"/sbin:/usr/bin:/bin:/usr/sbin:/sbin",
			"HOMEBREW_NO_ANALYTICS=1", "HOMEBREW_NO_ENV_HINTS=1", "HOMEBREW_NO_INSTALL_CLEANUP=1",
			"HOMEBREW_NO_AUTO_UPDATE=1", "NONINTERACTIVE=1", "NO_COLOR=1")
	} else {
		env = append(env, "NO_COLOR=1")
	}
	return env
}

func stripProxy(base []string) []string {
	out := make([]string, 0, len(base))
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(k) {
		case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "FTP_PROXY":
			continue
		}
		out = append(out, kv)
	}
	return out
}

// errorLine picks the line that explains a failure: the last "Error:" line, else the last line.
func errorLine(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "Error:") {
			return lines[i]
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return lines[len(lines)-1]
}
