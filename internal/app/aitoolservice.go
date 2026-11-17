package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/codepass/internal/aitools"
	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/netx"
)

// AI tools (plan 4.7, 4.8): which of Claude Code, Codex and OpenCode are installed, the install command shown
// before it runs, the task's name from the tool, and the approval mode a task header shows.

// AIToolDeps is everything AIToolService needs from the rest of the app. NewAIToolService fills it from the
// Container; tests fill it by hand.
type AIToolDeps struct {
	GOOS     string
	Detect   aitools.DetectOptions // the shell, the environment and the runner; zero = the real thing
	Install  aitools.InstallerConfig
	Namer    aitools.Namer // WorkRoot, Run and BaseEnv may be set by tests
	Emit     func(name string, data any)
	Lang     func() string // "zh-CN" | "en"
	Home     string        // where the tools' own config files are read (read-only)
	ProxyEnv func() []string
}

// AIToolService is the bound Wails service of the AI tools list and the sheet's naming step.
//
// Registration (the wiring task): application.NewService(app.NewAIToolService(c)) and
// application.RegisterEvent[api.AIInstallState](api.EventAIToolInstall) in internal/app/events.go.
type AIToolService struct {
	d    AIToolDeps
	inst *aitools.Installer

	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	last   map[aitools.ID]aitools.Detection // the latest detection: the Windows program paths
}

// NewAIToolService wires the service to the container.
func NewAIToolService(c *Container) *AIToolService {
	return NewAIToolServiceWith(AIToolDeps{
		Emit: c.emit,
		Lang: c.Settings.Lang,
		Home: c.Paths.Home,
		Namer: aitools.Namer{
			WorkRoot: filepath.Join(c.Paths.AppDataDir, "naming"),
		},
		ProxyEnv: func() []string { return netx.Detect(context.Background()).ChildEnv() },
	})
}

// NewAIToolServiceWith builds the service from explicit dependencies.
func NewAIToolServiceWith(d AIToolDeps) *AIToolService {
	if d.GOOS == "" {
		d.GOOS = runtime.GOOS
	}
	if d.Emit == nil {
		d.Emit = func(string, any) {}
	}
	if d.Lang == nil {
		d.Lang = func() string { return "en" }
	}
	// One login shell for Detect, the naming call and every launch (plan 4.7: $SHELL -l -i -c).
	// (SHELL is not changed by ChildEnv, which would run the login shell to build PATH, so read the process env.)
	env := d.Detect.Env
	if env == nil {
		env = os.Environ()
	}
	d.Detect.Shell = aitools.LoginShell(d.Detect.Shell, env)
	s := &AIToolService{d: d, last: map[aitools.ID]aitools.Detection{}}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	return s
}

// ServiceStartup runs nothing: detection and the installer start on first use.
func (s *AIToolService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown stops an install that is still going (what is installed stays).
func (s *AIToolService) ServiceShutdown() error {
	s.cancel()
	return nil
}

func (s *AIToolService) detectOptions() aitools.DetectOptions {
	o := s.d.Detect
	if o.GOOS == "" {
		o.GOOS = s.d.GOOS
	}
	return o
}

func (s *AIToolService) info(d aitools.Detection) api.AIToolInfo {
	cmd := ""
	if ins, ok := aitools.InstallFor(d.ID, s.d.GOOS); ok {
		cmd = ins.Display()
	}
	via := api.AIViaNone
	switch d.Via {
	case "file":
		via = api.AIViaFile
	case "function":
		via = api.AIViaFunction
	}
	return api.AIToolInfo{
		ID: api.AIToolID(d.ID), Name: d.ID.Name(), Installed: d.Installed, Version: d.Version, Path: d.Path,
		Via: via, InstallCommand: cmd, Note: d.Note,
	}
}

func (s *AIToolService) remember(ds []aitools.Detection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range ds {
		s.last[d.ID] = d
	}
}

// Detect looks for the three tools (only `--version` runs) and returns a row for each, with the exact install
// command of this system. A missing tool is not an error.
func (s *AIToolService) Detect(ctx context.Context) (api.AIToolsState, error) {
	ds := aitools.Detect(ctx, s.detectOptions())
	s.remember(ds)
	st := api.AIToolsState{GOOS: s.d.GOOS}
	for _, d := range ds {
		st.Tools = append(st.Tools, s.info(d))
	}
	return nn(st), nil
}

func (s *AIToolService) installer() *aitools.Installer {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inst == nil {
		cfg := s.d.Install
		if cfg.GOOS == "" {
			cfg.GOOS = s.d.GOOS
		}
		if cfg.ProxyEnv == nil {
			cfg.ProxyEnv = s.d.ProxyEnv
		}
		if cfg.Recheck == nil {
			cfg.Recheck = func(ctx context.Context, id aitools.ID) aitools.Detection {
				d := aitools.DetectOne(ctx, s.detectOptions(), id)
				s.remember([]aitools.Detection{d})
				return d
			}
		}
		if cfg.Info == nil {
			cfg.Info = s.info
		}
		cfg.Emit = func(st api.AIInstallState) { s.d.Emit(api.EventAIToolInstall, st) }
		s.inst = aitools.NewInstaller(cfg)
	}
	return s.inst
}

// StartInstall installs one tool with the command Detect showed (brew on macOS; winget or npm on Windows). It
// returns at once; progress arrives as EventAIToolInstall. A run already going is E_INVALID_INPUT.
func (s *AIToolService) StartInstall(tool api.AIToolID) error {
	id := aitools.ID(tool)
	if !id.Valid() {
		return fail(invalidField("tool"))
	}
	if err := s.installer().Start(s.ctx, id); err != nil {
		return fail(invalidField("run"))
	}
	return nil
}

// CancelInstall stops the run; what the installer finished stays.
func (s *AIToolService) CancelInstall() error {
	s.installer().Cancel()
	return nil
}

// GetInstallState returns the latest snapshot, so a reloaded window can pick the run up again.
func (s *AIToolService) GetInstallState() (api.AIInstallState, error) {
	return nn(s.installer().State()), nil
}

// NameTask asks the chosen tool for the task's title and slug (plan 4.7 "Naming"): one headless call with a
// 5-second budget. It never fails: when no valid answer arrives the quiet fallback is returned with the reason.
// n is the number of "New task <n>" and "task-<n>".
func (s *AIToolService) NameTask(ctx context.Context, tool api.AIToolID, description string, n int) (api.AINaming, error) {
	id := aitools.ID(tool)
	if !id.Valid() {
		return api.AINaming{}, fail(invalidField("tool"))
	}
	nm := s.d.Namer
	nm.GOOS, nm.Lang = s.d.GOOS, s.d.Lang()
	if nm.Shell == "" {
		nm.Shell = s.d.Detect.Shell
	}
	s.mu.Lock()
	nm.ToolPath = s.last[id].Path
	s.mu.Unlock()
	r := nm.Name(ctx, id, description, n)
	return api.AINaming{Title: r.Title, Slug: r.Slug, FromAI: r.FromAI, Reason: r.Reason}, nil
}

// GetApproval reads what a task's header shows for a tool. permissionMode is the last permission_mode a Claude
// Code hook reported (A2-05; "" before the first event); dir is the task's folder (OpenCode reads its project
// config there). Codex and OpenCode are read from the tool's own config files, read-only.
func (s *AIToolService) GetApproval(tool api.AIToolID, permissionMode, dir string) (api.AIApproval, error) {
	var a aitools.Approval
	switch aitools.ID(tool) {
	case aitools.Claude:
		a = aitools.ClaudeApproval(permissionMode)
	case aitools.Codex:
		a = aitools.CodexApproval(filepath.Join(s.d.Home, ".codex", "config.toml"))
	case aitools.OpenCode:
		// the global files first, then the task folder's (later ones override earlier ones)
		var files []string
		for _, name := range []string{"opencode.json", "opencode.jsonc"} {
			files = append(files, filepath.Join(s.d.Home, ".config", "opencode", name))
		}
		if dir != "" && filepath.IsAbs(dir) {
			for _, name := range []string{"opencode.json", "opencode.jsonc", filepath.Join(".opencode", "opencode.json"), filepath.Join(".opencode", "opencode.jsonc")} {
				files = append(files, filepath.Join(dir, name))
			}
		}
		a = aitools.OpenCodeApproval(files...)
	default:
		return api.AIApproval{}, fail(invalidField("tool"))
	}
	return nn(api.AIApproval{Tool: tool, Kind: api.AIApprovalKind(a.Kind), Mode: a.Mode, Approval: a.Approval,
		Sandbox: a.Sandbox, Known: a.Known, Source: a.Source}), nil
}

// buildLaunch is for the task lifecycle (A2-08), in this package: it fills the login shell, the OS, the
// program path found by the last detection (Windows), and whether the person's own Codex config already has a
// notify program, then builds the launch. It is not a bound method: its argument is a Go struct of the
// package, not something the window can send.
func (s *AIToolService) buildLaunch(r aitools.LaunchRequest) (aitools.Launch, error) {
	if r.GOOS == "" {
		r.GOOS = s.d.GOOS
	}
	if r.Shell == "" {
		r.Shell = s.d.Detect.Shell
	}
	s.mu.Lock()
	if r.ToolPath == "" {
		r.ToolPath = s.last[r.Tool].Path
	}
	s.mu.Unlock()
	if r.Tool == aitools.Codex && len(r.CodexNotify) > 0 {
		// The account's own folder when the launch sets CODEX_HOME, else the default one.
		codexHome := filepath.Join(s.d.Home, ".codex")
		for _, kv := range r.AccountEnv {
			if v, ok := strings.CutPrefix(kv, "CODEX_HOME="); ok && v != "" {
				codexHome = v
			}
		}
		r.CodexHasNotify = aitools.CodexHasNotify(filepath.Join(codexHome, "config.toml"))
	}
	return aitools.BuildLaunch(r)
}
