package app

import (
	"context"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/envcheck"
	"github.com/rudian/codepass/internal/netx"
	"github.com/rudian/codepass/internal/toolinstall"
)

// The tools gate (plan 4.1, design 5.1): what is missing, and the one-click setup that installs it.

// ToolDeps is everything ToolService needs from the rest of the app. NewToolService fills it from the Container;
// tests fill it by hand.
type ToolDeps struct {
	Gate    func(ctx context.Context) api.SetupState // default: envcheck.Gate with the user's tool paths
	Install toolinstall.Config                       // Net, CacheDir, Emit and Lang are filled in when empty
	Net     toolinstall.Net                          // default: a netx client with the detected proxy
	Emit    func(name string, data any)
	OpenURL func(url string) error
	Lang    func() string
	// AWSPath and GitPath are the user's chosen program paths ("" = detect).
	AWSPath, GitPath func() string
	// ClearPath empties the user's chosen path of a tool (git or AWS CLI) through the normal settings write
	// (events, environment re-check, repository rescan). The old path is not kept: settings.json is not backed
	// up, so the setup screen never promises a way back. The setup run uses it for a row whose chosen path
	// blocks the gate.
	ClearPath func(id api.SetupToolID) error
	CacheDir  string // where Homebrew.pkg is saved (the app's private folder)
	GOOS      string
}

// ToolService is the bound Wails service of the setup screen and the Health rows.
//
// Registration (the wiring task): application.NewService(app.NewToolService(c, nil)) and
// application.RegisterEvent[api.InstallState](api.EventToolInstall) in internal/app/events.go.
type ToolService struct {
	d    ToolDeps
	inst *toolinstall.Installer

	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
}

// NewToolService wires the service to the container. n is the app's netx client (nil: one is created here).
func NewToolService(c *Container, n toolinstall.Net) *ToolService {
	return NewToolServiceWith(ToolDeps{
		Net:      n,
		Emit:     c.emit,
		OpenURL:  c.RT.OpenURL,
		Lang:     c.Settings.Lang,
		AWSPath:  c.Settings.AWSCLIPath,
		GitPath:  c.Settings.GitPath,
		CacheDir: filepath.Join(c.Paths.AppDataDir, "downloads"),
		ClearPath: func(id api.SetupToolID) error {
			empty := ""
			var p api.SettingsPatch
			switch id {
			case api.SetupGit:
				p.GitPath = &empty
			case api.SetupAWSCLI:
				p.AWSCLIPath = &empty
			default:
				return nil
			}
			// The same path as the Settings screen: events, environment re-check, repository rescan.
			_, err := (&SystemService{c: c}).UpdateSettings(p)
			return err
		},
	})
}

// NewToolServiceWith builds the service from explicit dependencies.
func NewToolServiceWith(d ToolDeps) *ToolService {
	if d.GOOS == "" {
		d.GOOS = runtime.GOOS
	}
	if d.Emit == nil {
		d.Emit = func(string, any) {}
	}
	if d.Lang == nil {
		d.Lang = func() string { return "en" }
	}
	if d.AWSPath == nil {
		d.AWSPath = func() string { return "" }
	}
	if d.GitPath == nil {
		d.GitPath = func() string { return "" }
	}
	s := &ToolService{d: d}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	return s
}

// ServiceStartup runs nothing: the checks and the installer are created on first use.
func (s *ToolService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown stops a run that is still going (what is installed stays).
func (s *ToolService) ServiceShutdown() error {
	s.cancel()
	return nil
}

func (s *ToolService) net(ctx context.Context) toolinstall.Net {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.d.Net == nil {
		s.d.Net = netx.New(netx.Options{Proxy: netx.Detect(ctx), UserAgent: "Codepass"})
	}
	return s.d.Net
}

func (s *ToolService) installer() *toolinstall.Installer {
	n := s.net(context.Background())
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inst == nil {
		cfg := s.d.Install
		if cfg.GOOS == "" {
			cfg.GOOS = s.d.GOOS
		}
		if cfg.Net == nil {
			cfg.Net = n
		}
		if cfg.CacheDir == "" {
			cfg.CacheDir = s.d.CacheDir
		}
		if cfg.Lang == nil {
			cfg.Lang = s.d.Lang
		}
		if cfg.Recheck == nil {
			cfg.Recheck = func(ctx context.Context) api.SetupState {
				ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
				defer cancel()
				return s.gate(ctx)
			}
		}
		cfg.Emit = func(st api.InstallState) { s.d.Emit(api.EventToolInstall, st) }
		s.inst = toolinstall.New(cfg)
	}
	return s.inst
}

func (s *ToolService) gate(ctx context.Context) api.SetupState {
	if s.d.Gate != nil {
		return s.d.Gate(ctx)
	}
	n := s.net(ctx)
	return envcheck.Gate(ctx, envcheck.GateOptions{
		AWSPath: s.d.AWSPath(), GitPath: s.d.GitPath(), GOOS: s.d.GOOS,
		Proxy: func(ctx context.Context) netx.Config {
			if p := n.Proxy(); p.Source != "" {
				return p
			}
			return netx.Detect(ctx)
		},
	})
}

// GetSetupState runs the local checks (no network) and says whether the setup screen must replace the window.
func (s *ToolService) GetSetupState(ctx context.Context) (api.SetupState, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	st := s.gate(ctx)
	for _, t := range st.Tools {
		if t.ID == api.SetupHomebrew && t.Status == api.ToolOK {
			s.installer().RemoveStaleDownload() // Homebrew is there: the 154 MB package is not needed any more
		}
	}
	return nn(st), nil
}

// StartSetupInstall installs every row of the gate that blocks it, in order. It returns at once; progress
// arrives as EventToolInstall. The checks run again first, so "try again" continues from the first row that is
// still missing, and the proxy is read again in case it changed while the screen was open.
func (s *ToolService) StartSetupInstall(ctx context.Context) error {
	n := s.net(ctx)
	if sp, ok := n.(interface{ SetProxy(netx.Config) }); ok {
		sp.SetProxy(netx.Detect(ctx))
	}
	chk, cancel := context.WithTimeout(ctx, 60*time.Second)
	state := s.gate(chk)
	cancel()
	// A tool path chosen in Settings that blocks the gate cannot be changed from the setup screen, which
	// replaces the whole window: clear it first (through the normal settings write, which keeps no copy of the old path), then judge again.
	cleared := false
	for _, t := range state.Tools {
		if t.Blocking && t.Override && s.d.ClearPath != nil {
			if err := s.d.ClearPath(t.ID); err != nil {
				return fail(err)
			}
			cleared = true
		}
	}
	if cleared {
		chk, cancel := context.WithTimeout(ctx, 60*time.Second)
		state = s.gate(chk)
		cancel()
	}
	if err := s.installer().Start(s.ctx, state); err != nil {
		return fail(invalidField("run"))
	}
	return nil
}

// StartToolUpdate updates git or the AWS CLI with the installer that manages it (brew upgrade on macOS, winget
// upgrade on Windows): the Health page's "Update" action. It returns at once; progress and the failure cause
// arrive as EventToolInstall, exactly as for an install, and CancelSetupInstall stops it. Any other tool, or a
// run already going, is E_INVALID_INPUT.
func (s *ToolService) StartToolUpdate(ctx context.Context, tool api.SetupToolID) error {
	if tool != api.SetupGit && tool != api.SetupAWSCLI {
		return fail(invalidField("tool"))
	}
	n := s.net(ctx)
	if sp, ok := n.(interface{ SetProxy(netx.Config) }); ok {
		sp.SetProxy(netx.Detect(ctx))
	}
	chk, cancel := context.WithTimeout(ctx, 60*time.Second)
	state := s.gate(chk)
	cancel()
	if err := s.installer().StartUpdate(s.ctx, state, tool); err != nil {
		return fail(invalidField("run"))
	}
	return nil
}

// CancelSetupInstall stops the run after its current step (a download stops at once). What is installed stays.
func (s *ToolService) CancelSetupInstall() error {
	s.installer().Cancel()
	return nil
}

// GetInstallState returns the latest snapshot, so a reloaded window can pick the run up again.
func (s *ToolService) GetInstallState() (api.InstallState, error) {
	return nn(s.installer().State()), nil
}

// installPages are the official pages behind "I'll install them myself". They are constants.
var installPages = map[api.SetupToolID]map[string]string{
	api.SetupHomebrew: {"darwin": "https://brew.sh/"},
	api.SetupWinget:   {"windows": "https://apps.microsoft.com/detail/9nblggh4nns1"},
	api.SetupGit:      {"darwin": "https://git-scm.com/download/mac", "windows": "https://git-scm.com/download/win"},
	api.SetupAWSCLI: {
		"darwin":  "https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html",
		"windows": "https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html",
	},
}

// OpenInstallPage opens the official page of one tool in the browser. Only these fixed https pages can be opened.
func (s *ToolService) OpenInstallPage(tool api.SetupToolID) error {
	u, ok := installPages[tool][s.d.GOOS]
	if !ok {
		return fail(invalidField("tool"))
	}
	if s.d.OpenURL == nil {
		return nil
	}
	return fail(s.d.OpenURL(u))
}
