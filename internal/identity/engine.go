package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/paths"
	"github.com/rudian/codepass/internal/projects"
)

// BackupPort is the part of backup.Store the engine uses.
type BackupPort interface {
	Snapshot(path, display string, kind api.BackupKind, reason api.BackupReason, params map[string]string) (api.BackupEntry, error)
	Get(id string) (api.BackupEntry, []byte, error)
}

// Deps wires the engine.
type Deps struct {
	Paths    paths.Paths
	Git      func(ctx context.Context) (*gitx.Git, error)
	Projects *projects.Store
	Backup   BackupPort
	// Shell returns the shell the terminals use; nil = $SHELL, else /bin/zsh on macOS.
	Shell func() string
	// Env returns the base environment of the terminal; nil = TerminalEnv(Paths.Home, Proxy()).
	Env func() []string
	// Proxy returns the proxy variables (netx Config.ChildEnv) the terminal gets; nil = none.
	Proxy func() []string
	// GOOS defaults to runtime.GOOS. On windows no shell is probed.
	GOOS string
	// ProbeTTL is how long a shell probe is reused (default 15 s). ProbeTimeout bounds one probe (default 5 s).
	ProbeTTL     time.Duration
	ProbeTimeout time.Duration
	Emit         func(name string, data any)
	// OnWrite is told about every global config write (the file watcher records it).
	OnWrite func(path string, content []byte)
}

// Engine implements the author identity backend.
type Engine struct {
	d Deps

	writeMu sync.Mutex // serialises every write and undo

	undoMu sync.Mutex
	undos  undoStore

	probeMu  sync.Mutex
	probeAt  time.Time
	probeVal termEnv
}

type termEnv struct {
	vars   map[string]string // probed identity variables
	shell  string
	probed bool
	err    api.ProbeErrorKind
	exit   int
}

// New creates the engine.
func New(d Deps) *Engine {
	if d.Emit == nil {
		d.Emit = func(string, any) {}
	}
	if d.GOOS == "" {
		d.GOOS = runtime.GOOS
	}
	if d.ProbeTTL <= 0 {
		d.ProbeTTL = 15 * time.Second
	}
	return &Engine{d: d}
}

func (e *Engine) git(ctx context.Context) (*gitx.Git, error) {
	if e.d.Git == nil {
		return nil, api.NewError(api.ErrGitMissing, "git is not configured")
	}
	g, err := e.d.Git(ctx)
	if err != nil {
		return nil, api.AsAppError(err)
	}
	return g, nil
}

func (e *Engine) shell() string {
	if e.d.Shell != nil {
		return e.d.Shell()
	}
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	if e.d.GOOS == "darwin" {
		return "/bin/zsh"
	}
	return ""
}

func (e *Engine) baseEnv() []string {
	if e.d.Env != nil {
		return e.d.Env()
	}
	var proxy []string
	if e.d.Proxy != nil {
		proxy = e.d.Proxy()
	}
	return TerminalEnv(e.d.Paths.Home, proxy)
}

// terminalEnv returns the identity variables the terminal gets, probing the login shell at most once per TTL.
func (e *Engine) terminalEnv(ctx context.Context, refresh bool) termEnv {
	e.probeMu.Lock()
	defer e.probeMu.Unlock()
	if !refresh && !e.probeAt.IsZero() && time.Since(e.probeAt) < e.d.ProbeTTL {
		return e.probeVal
	}
	base := e.baseEnv()
	t := termEnv{vars: map[string]string{}}
	if e.d.GOOS == "windows" {
		t.vars, t.probed = envFromList(base), true
	} else {
		sh := e.shell()
		t.shell = sh
		vars, err := ProbeShellEnv(ctx, ProbeOptions{Shell: sh, Env: base, Dir: e.d.Paths.Home, Timeout: e.d.ProbeTimeout})
		if err != nil {
			t.vars = envFromList(base) // what the app's own children have; the shell's exports are unknown
			t.err, t.exit = probeErrKind(err)
			if ctx.Err() != nil {
				return t // the caller gave up: that says nothing about the shell, so it is not remembered
			}
		} else {
			t.vars, t.probed = vars, true
		}
	}
	e.probeAt, e.probeVal = time.Now(), t
	return t
}

func probeErrKind(err error) (api.ProbeErrorKind, int) {
	var pf *ProbeFailure
	if errors.As(err, &pf) {
		return pf.Kind, pf.Exit
	}
	if errors.Is(err, ErrNoShell) {
		return api.ProbeNoShell, 0
	}
	return api.ProbeFailed, 0
}

// termGit returns git with the terminal's config-relevant variables applied.
func (e *Engine) termGit(g *gitx.Git, t termEnv) *gitx.Git {
	var extra []string
	for _, n := range forwardNames {
		if v, ok := t.vars[n]; ok {
			extra = append(extra, n+"="+v)
		}
	}
	return g.WithEnv(extra...)
}

func envOf(g *gitx.Git, name string) string {
	val := ""
	found := false
	for _, kv := range g.Env {
		if v, ok := strings.CutPrefix(kv, name+"="); ok {
			val, found = v, true
		}
	}
	if !found {
		val = os.Getenv(name)
	}
	return val
}

// TerminalEnv reports the identity variables exported in the terminal environment.
func (e *Engine) TerminalEnv(ctx context.Context, refresh bool) api.TerminalEnvOverride {
	g, _ := e.git(ctx)
	return e.envReport(g, e.terminalEnv(ctx, refresh))
}

func (e *Engine) envReport(g *gitx.Git, t termEnv) api.TerminalEnvOverride {
	out := api.TerminalEnvOverride{Vars: []api.EnvVar{}, Files: []api.IdentityFile{}, Probed: t.probed, ProbeError: t.err, ProbeExitCode: t.exit}
	if t.shell != "" {
		out.Shell = filepath.Base(t.shell)
	}
	have := map[string]bool{}
	for _, n := range identityVarNames {
		if v, ok := t.vars[n]; ok {
			out.Vars = append(out.Vars, api.EnvVar{Name: n, Value: v})
			have[n] = true
		}
	}
	_, an := t.vars["GIT_AUTHOR_NAME"]
	_, ae := t.vars["GIT_AUTHOR_EMAIL"]
	out.Active = an || ae
	_, cn := t.vars["GIT_COMMITTER_NAME"]
	_, ce := t.vars["GIT_COMMITTER_EMAIL"]
	out.CommitterVars = cn || ce
	_, out.FallbackEmail = t.vars["EMAIL"]
	if g != nil {
		for _, n := range []string{"GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM"} {
			if v, ok := t.vars[n]; ok && v != envOf(g, n) {
				out.ConfigRedirected = true
			}
		}
		if c, ok := t.vars["GIT_CONFIG_COUNT"]; ok && c != "" && c != "0" && envOf(g, "GIT_CONFIG_COUNT") == "" {
			out.ConfigRedirected = true
		}
	}
	// "Open that file": a text scan of the shell's startup files. Names the probe did not see exported (or the
	// whole list when the probe failed) are still scanned, so a plain assignment that does not reach git is
	// not reported as active but its file is.
	if len(out.Vars) > 0 || !t.probed {
		files := profileCandidates(t.shell, e.d.Paths.Home, e.baseEnv())
		hits := scanProfiles(files, nil)
		seen := map[string]bool{}
		var names []string
		for n := range hits {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			for _, f := range hits[n] {
				if !seen[f] {
					seen[f] = true
					out.Files = append(out.Files, api.IdentityFile{File: e.d.Paths.Display(f), Path: f})
				}
			}
		}
	}
	return out
}

// profileFileFor returns the first profile file whose text assigns variable name (hint only).
func (e *Engine) profileFileFor(t termEnv, name string) (display, abs string) {
	hits := scanProfiles(profileCandidates(t.shell, e.d.Paths.Home, e.baseEnv()), map[string]bool{name: true})
	if fs := hits[name]; len(fs) > 0 {
		return e.d.Paths.Display(fs[0]), fs[0]
	}
	return "", ""
}

func (e *Engine) emit() { e.d.Emit(api.EventIdentityChanged, struct{}{}) }

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func hashOf(b []byte, exists bool) string {
	if !exists {
		return ""
	}
	return sha(b)
}

func readFile(path string) (content []byte, exists bool, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, api.NewError(api.ErrFileIO, "cannot read a git config file").WithParam("file", path).WithDetail(err.Error())
	}
	return b, true, nil
}
