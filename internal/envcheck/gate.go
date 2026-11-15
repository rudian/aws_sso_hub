package envcheck

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/execx"
	"github.com/rudian/codepass/internal/netx"
)

// The tools gate (plan 4.1, design 5.1). Everything here is local: files, `--version`, `id`, `profiles`, the
// system proxy settings. Nothing opens a network connection, so an offline Mac with the tools goes straight in.

// NeedBytes is the free space the setup run asks for (Homebrew about 0.5 GB, git and awscli about 0.5 GB, bottles).
const NeedBytes int64 = 1_200_000_000

// BrewPrefix is where Homebrew.pkg installs on Apple silicon.
const BrewPrefix = "/opt/homebrew"

// Runner runs one read-only probe and returns its stdout. Tests replace it.
type Runner func(ctx context.Context, name string, args ...string) (stdout string, exit int, err error)

// GateOptions configures Gate. The zero value is the real machine.
type GateOptions struct {
	AWSPath string // the user's chosen AWS CLI path ("" = detect)
	GitPath string // the user's chosen git path ("" = detect)
	GOOS    string // default runtime.GOOS

	BrewPaths   []string // macOS: where brew may be (default /opt/homebrew/bin/brew only: the installer runs nothing else)
	WingetPaths []string // Windows: where winget may be (default the WindowsApps alias)
	PrefixDir   string   // macOS: default /opt/homebrew
	SpaceDir    string   // the folder whose volume is measured (default the home folder)

	Run   Runner                                // default: a child with a fixed environment and a 5 s limit
	Proxy func(ctx context.Context) netx.Config // default netx.Detect
	Free  func(dir string) (int64, error)       // default: the volume's free bytes
	Now   func() time.Time
}

func (o *GateOptions) defaults() {
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.BrewPaths == nil {
		// Only the arm64 prefix the package installs. A /usr/local brew is an Intel Homebrew (Rosetta): the
		// installer would not run it, and its bottles are x86.
		o.BrewPaths = []string{BrewPrefix + "/bin/brew"}
	}
	if o.WingetPaths == nil {
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			o.WingetPaths = []string{filepath.Join(la, "Microsoft", "WindowsApps", "winget.exe")}
		}
	}
	if o.PrefixDir == "" {
		o.PrefixDir = BrewPrefix
	}
	if o.SpaceDir == "" {
		o.SpaceDir, _ = os.UserHomeDir()
	}
	if o.Run == nil {
		o.Run = runProbe
	}
	if o.Proxy == nil {
		o.Proxy = netx.Detect
	}
	if o.Free == nil {
		o.Free = freeBytes
	}
	if o.Now == nil {
		o.Now = time.Now
	}
}

// probeEnv is the environment of the read-only probes: nothing inherited that could change what they do.
var probeEnv = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LC_ALL=C"}

func runProbe(ctx context.Context, name string, args ...string) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = probeEnv
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err == nil {
		return out.String(), 0, nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return out.String(), ee.ExitCode(), nil
	}
	return out.String(), -1, err
}

// Gate runs every local check and decides whether the setup screen must replace the window. A row blocks only
// when its tool is missing or below the hard minimum (git 2.44, AWS CLI 2.9.0, or AWS CLI v1); "older than
// recommended" and "could not read the version" never block (R5).
func Gate(ctx context.Context, o GateOptions) api.SetupState {
	o.defaults()
	st := api.SetupState{Platform: o.GOOS, Tools: make([]api.SetupTool, 0, 3), CheckedAt: o.Now().UTC().Format(time.RFC3339)}
	var mgr api.SetupTool
	if o.GOOS == "windows" {
		mgr = checkWinget(ctx, o)
	} else {
		mgr = checkBrew(ctx, o)
	}
	git := gateGit(ctx, o, mgr)
	aws := gateAWS(ctx, o)
	st.Tools = append(st.Tools, mgr, git, aws)
	for _, t := range st.Tools {
		st.Total++
		if t.Status == api.ToolOK || t.Status == api.ToolOutdated {
			st.Ready++
		}
		if t.Blocking {
			st.Blocked = true
		}
	}
	st.Preflight = preflight(ctx, o)
	return st
}

var reBrewVersion = regexp.MustCompile(`Homebrew >?=?\s*(\d+)\.(\d+)\.(\d+)`)
var reWingetVersion = regexp.MustCompile(`v?(\d+)\.(\d+)\.(\d+)`)

// firstExecutable returns the first candidate that is a regular, executable file (a Windows alias counts).
func firstExecutable(paths []string, goos string) string {
	for _, p := range paths {
		st, err := os.Lstat(p)
		if err != nil || st.IsDir() {
			continue
		}
		if goos == "windows" || st.Mode().Perm()&0o111 != 0 {
			return p
		}
	}
	return ""
}

// brewEnv keeps brew from phoning home or updating itself during a check.
var brewEnv = []string{"HOMEBREW_NO_ANALYTICS=1", "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ENV_HINTS=1"}

func versionOf(ctx context.Context, bin string, re *regexp.Regexp, extraEnv []string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := execx.Command(ctx, bin, args...)
	cmd.Env = append(cmd.Env, extraEnv...)
	res, err := execx.Run(cmd, nil)
	if err != nil || res.ExitCode != 0 {
		return "", false
	}
	m := re.FindStringSubmatch(string(res.Stdout) + "\n" + string(res.Stderr))
	if m == nil {
		return "", false
	}
	return versionString(atoi3(m)), true
}

func checkBrew(ctx context.Context, o GateOptions) api.SetupTool {
	t := api.SetupTool{ID: api.SetupHomebrew}
	path := firstExecutable(o.BrewPaths, o.GOOS)
	if path == "" {
		t.Status, t.Blocking, t.Fix = api.ToolMissing, true, api.FixInstall
		if _, err := os.Lstat(o.PrefixDir); err == nil {
			// Homebrew's folder is there without a working brew: never touched by Codepass.
			t.Fix = api.FixRepairPrefix
		}
		return t
	}
	t.Path = path
	if v, ok := versionOf(ctx, path, reBrewVersion, brewEnv, "--version"); ok {
		t.Status, t.Version = api.ToolOK, v
	} else {
		t.Status = api.ToolError
	}
	return t
}

func checkWinget(ctx context.Context, o GateOptions) api.SetupTool {
	t := api.SetupTool{ID: api.SetupWinget}
	path := firstExecutable(o.WingetPaths, o.GOOS)
	if path == "" {
		t.Status, t.Blocking, t.Fix = api.ToolMissing, true, api.FixOpenStore
		return t
	}
	t.Path = path
	if v, ok := versionOf(ctx, path, reWingetVersion, nil, "--version"); ok {
		t.Status, t.Version = api.ToolOK, v
	} else {
		t.Status = api.ToolError
	}
	return t
}

func gateGit(ctx context.Context, o GateOptions, mgr api.SetupTool) api.SetupTool {
	info := CheckGit(ctx, o.GitPath)
	t := api.SetupTool{ID: api.SetupGit, Status: info.Status, Version: info.Version, Min: GitMin, Path: info.Path, Override: o.GitPath != ""}
	switch info.Status {
	case api.ToolMissing:
		t.Blocking, t.Fix = true, api.FixInstall
	case api.ToolTooOld:
		t.Blocking, t.Fix = true, api.FixUpgrade
		t.Elsewhere = o.GOOS != "windows" && !strings.HasPrefix(info.Path, o.PrefixDir+"/")
	}
	if t.Blocking && t.Override {
		// The chosen path is what blocks. When automatic detection finds a usable git, the fix is to stop
		// pointing at the chosen one.
		if det := CheckGit(ctx, ""); det.Status == api.ToolOK {
			t.Fix, t.Elsewhere = api.FixUseDetected, false
		}
	}
	return t
}

func gateAWS(ctx context.Context, o GateOptions) api.SetupTool {
	info := CheckAWSCLI(ctx, o.AWSPath)
	t := api.SetupTool{ID: api.SetupAWSCLI, Status: info.Status, Version: info.Version, Min: AWSMinBlocking, Path: info.Path, Override: o.AWSPath != ""}
	switch info.Status {
	case api.ToolMissing:
		t.Blocking, t.Fix = true, api.FixInstall
	case api.ToolV1, api.ToolTooOld:
		t.Blocking, t.Fix = true, api.FixUpgrade
		t.Elsewhere = o.GOOS != "windows" && !strings.HasPrefix(info.Path, o.PrefixDir+"/")
	}
	if t.Blocking && t.Override {
		if det := CheckAWSCLI(ctx, ""); det.Status != api.ToolMissing && det.Status != api.ToolV1 && det.Status != api.ToolTooOld {
			t.Fix, t.Elsewhere = api.FixUseDetected, false
		}
	}
	return t
}

var reMDM = regexp.MustCompile(`(?im)^\s*MDM enrollment:\s*(\w+)`)

func preflight(ctx context.Context, o GateOptions) api.Preflight {
	pf := api.Preflight{Admin: api.TriUnknown, Managed: api.TriUnknown, FreeBytes: -1, NeedBytes: NeedBytes, EnoughSpace: true}
	if o.GOOS == "darwin" {
		// `id -Gn` lists the account's groups; an administrator is in "admin".
		if out, exit, err := o.Run(ctx, "/usr/bin/id", "-Gn"); err == nil && exit == 0 {
			pf.Admin = api.TriNo
			for _, g := range strings.Fields(out) {
				if g == "admin" {
					pf.Admin = api.TriYes
				}
			}
		}
		if out, exit, err := o.Run(ctx, "/usr/bin/profiles", "status", "-type", "enrollment"); err == nil && exit == 0 {
			if m := reMDM.FindStringSubmatch(out); m != nil {
				pf.Managed = api.TriNo
				if strings.EqualFold(m[1], "yes") {
					pf.Managed = api.TriYes
				}
			}
		}
		if _, err := os.Lstat(o.PrefixDir); err == nil {
			pf.PrefixExists = true
		}
	}
	if o.SpaceDir != "" {
		if n, err := o.Free(o.SpaceDir); err == nil {
			pf.FreeBytes = n
			pf.EnoughSpace = n >= NeedBytes
		}
	}
	c := o.Proxy(ctx)
	pf.Proxy = api.ProxyInfo{HostPort: c.HostPort(), Source: c.Source, PACOnly: c.PACOnly(), Unsupported: c.Unsupported}
	return pf
}
