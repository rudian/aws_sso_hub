package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/aitools"
	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/toolinstall"
)

type okRunner struct{ calls []toolinstall.Cmd }

func (r *okRunner) Run(ctx context.Context, c toolinstall.Cmd, onLine func(string)) (int, error) {
	r.calls = append(r.calls, c)
	onLine("==> Installing")
	return 0, nil
}

func fakeDetectRun(found map[string]string) aitools.RunFunc {
	return func(ctx context.Context, argv []string, dir string, env []string) ([]byte, int, error) {
		name := argv[len(argv)-2] // the tool name before --version
		if p, ok := found[name]; ok {
			return []byte("__CODEPASS_AT__" + p + "\n1.2.3 (" + name + ")\n"), 0, nil
		}
		return []byte("__CODEPASS_AT__\nzsh: command not found: " + name + "\n"), 127, nil
	}
}

func TestAIToolServiceDetectShowsTheExactInstallCommand(t *testing.T) {
	s := NewAIToolServiceWith(AIToolDeps{GOOS: "darwin", Detect: aitools.DetectOptions{
		Shell: "/bin/zsh", Env: []string{"HOME=/h"}, Run: fakeDetectRun(map[string]string{"claude": "/opt/homebrew/bin/claude"})}})
	st, err := s.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Tools) != 3 || st.GOOS != "darwin" {
		t.Fatalf("%+v", st)
	}
	c, x, o := st.Tools[0], st.Tools[1], st.Tools[2]
	if !c.Installed || c.Version != "1.2.3" || c.Path != "/opt/homebrew/bin/claude" || c.Via != api.AIViaFile || c.InstallCommand != "brew install --cask claude-code" {
		t.Fatalf("%+v", c)
	}
	if x.Installed || x.InstallCommand != "brew install --cask codex" || o.Installed || o.InstallCommand != "brew install opencode" || x.Via != api.AIViaNone {
		t.Fatalf("%+v %+v", x, o)
	}
}

func TestAIToolServiceInstallEmitsProgressAndRechecks(t *testing.T) {
	var mu sync.Mutex
	var last api.AIInstallState
	r := &okRunner{}
	found := map[string]string{}
	s := NewAIToolServiceWith(AIToolDeps{GOOS: "darwin",
		Detect: aitools.DetectOptions{Shell: "/bin/zsh", Env: []string{"HOME=/h"}, Run: func(ctx context.Context, a []string, d string, e []string) ([]byte, int, error) {
			return fakeDetectRun(found)(ctx, a, d, e)
		}},
		Install: aitools.InstallerConfig{Runner: r, BaseEnv: func() []string { return nil },
			Lookup: func(string) (string, error) { return "/opt/homebrew/bin/brew", nil }},
		Emit: func(name string, data any) {
			if name == api.EventAIToolInstall {
				mu.Lock()
				last = data.(api.AIInstallState)
				mu.Unlock()
			}
		}})
	if err := s.StartInstall(api.AIToolID("vim")); err == nil {
		t.Fatal("unknown tool")
	}
	found["codex"] = "/opt/homebrew/bin/codex" // what the install will have put there
	if err := s.StartInstall(api.AIToolCodex); err != nil {
		t.Fatal(err)
	}
	s.installer().Wait()
	st, _ := s.GetInstallState()
	if !st.Finished || st.Failure != nil || st.Installed == nil || !st.Installed.Installed || st.Installed.InstallCommand == "" || st.Log == nil {
		t.Fatalf("%+v", st)
	}
	mu.Lock()
	defer mu.Unlock()
	if !last.Finished || last.Command != "brew install --cask codex" {
		t.Fatalf("%+v", last)
	}
	if err := s.CancelInstall(); err != nil {
		t.Fatal(err)
	}
}

func TestAIToolServiceApproval(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
	os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755)
	os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte("approval_policy = \"never\"\nsandbox_mode = \"read-only\"\n"), 0o600)
	os.WriteFile(filepath.Join(home, ".config", "opencode", "opencode.json"), []byte(`{"permission":"allow"}`), 0o600)
	proj := t.TempDir()
	os.WriteFile(filepath.Join(proj, "opencode.json"), []byte(`{"permission":"ask"}`), 0o600)
	s := NewAIToolServiceWith(AIToolDeps{Home: home})
	a, err := s.GetApproval(api.AIToolClaude, "bypassPermissions", "")
	if err != nil || !a.Known || a.Mode != "bypassPermissions" || a.Kind != api.AIApprovalPermissionMode {
		t.Fatalf("%+v %v", a, err)
	}
	if a, _ := s.GetApproval(api.AIToolClaude, "", ""); a.Known {
		t.Fatal("before the first event the mode is unknown")
	}
	if a, _ := s.GetApproval(api.AIToolCodex, "", ""); a.Approval != "never" || a.Sandbox != "read-only" {
		t.Fatalf("%+v", a)
	}
	if a, _ := s.GetApproval(api.AIToolOpenCode, "", ""); a.Mode != "allow" {
		t.Fatalf("%+v", a)
	}
	if a, _ := s.GetApproval(api.AIToolOpenCode, "", proj); a.Mode != "ask" {
		t.Fatalf("%+v", a)
	}
	if _, err := s.GetApproval("vim", "", ""); err == nil {
		t.Fatal()
	}
}

func TestAIToolServiceNameTask(t *testing.T) {
	var argv []string
	s := NewAIToolServiceWith(AIToolDeps{GOOS: "darwin", Lang: func() string { return "zh-CN" },
		Namer: aitools.Namer{WorkRoot: t.TempDir(), BaseEnv: func() []string { return []string{"ANTHROPIC_API_KEY=k", "PATH=/x"} },
			Run: func(ctx context.Context, a []string, d string, e []string) ([]byte, int, error) {
				argv = a
				for _, kv := range e {
					if strings.HasPrefix(kv, "ANTHROPIC_API_KEY") {
						t.Error("API key in the naming environment")
					}
				}
				return []byte(`{"title":"修复登录","slug":"fix-login"}`), 0, nil
			}}})
	n, err := s.NameTask(context.Background(), api.AIToolClaude, "修复登录页", 2)
	if err != nil || !n.FromAI || n.Title != "修复登录" || n.Slug != "fix-login" || argv[6] != "claude" {
		t.Fatalf("%+v %v %q", n, err, argv)
	}
	n, _ = s.NameTask(context.Background(), api.AIToolClaude, "", 5)
	if n.Title != "新任务 5" || n.Slug != "new-task-5" || n.Reason != "empty" {
		t.Fatalf("%+v", n)
	}
	if _, err := s.NameTask(context.Background(), "vim", "x", 1); err == nil {
		t.Fatal()
	}
}

func TestAIToolServiceBuildLaunchFillsTheMachineDetails(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
	os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte("notify = [\"/usr/bin/say\"]\n"), 0o600)
	s := NewAIToolServiceWith(AIToolDeps{GOOS: "darwin", Home: home, Detect: aitools.DetectOptions{Shell: "/bin/bash"}})
	l, err := s.buildLaunch(aitools.LaunchRequest{Tool: aitools.Codex, CodexNotify: []string{"/app/codepass", "report"}, FirstMessage: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(l.Argv, "|"); got != "/bin/bash|-l|-i|-c|\"$@\"|codepass|codex|--|hi" || !l.NotifySkipped {
		t.Fatalf("%s skipped=%v", got, l.NotifySkipped)
	}
	// Windows takes the path the last detection found
	w := NewAIToolServiceWith(AIToolDeps{GOOS: "windows", Detect: aitools.DetectOptions{GOOS: "windows", Env: []string{"PATH=x"},
		Lookup: func(string) (string, error) { return `C:\t\claude.exe`, nil },
		Run:    func(context.Context, []string, string, []string) ([]byte, int, error) { return []byte("2.0.0"), 0, nil }}})
	if _, err := w.Detect(context.Background()); err != nil {
		t.Fatal(err)
	}
	l, err = w.buildLaunch(aitools.LaunchRequest{Tool: aitools.Claude, FirstMessage: "x"})
	if err != nil || l.Argv[0] != `C:\t\claude.exe` {
		t.Fatalf("%q %v", l.Argv, err)
	}
}

type blockRunner struct{ started chan struct{} }

func (r *blockRunner) Run(ctx context.Context, c toolinstall.Cmd, onLine func(string)) (int, error) {
	onLine("==> Downloading")
	close(r.started)
	<-ctx.Done()
	return -1, ctx.Err()
}

func TestAIToolServiceShutdownCancelsAnInstall(t *testing.T) {
	r := &blockRunner{started: make(chan struct{})}
	s := NewAIToolServiceWith(AIToolDeps{GOOS: "darwin",
		Install: aitools.InstallerConfig{Runner: r, BaseEnv: func() []string { return nil },
			Lookup: func(string) (string, error) { return "/opt/homebrew/bin/brew", nil }}})
	if err := s.StartInstall(api.AIToolCodex); err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.started:
	case <-time.After(2 * time.Second):
		t.Fatal("the install never started")
	}
	if st, _ := s.GetInstallState(); !st.Running {
		t.Fatalf("%+v", st)
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { s.installer().Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the install survived the shutdown")
	}
	if st, _ := s.GetInstallState(); st.Running || !st.Cancelled || !st.Finished {
		t.Fatalf("%+v", st)
	}
}

// Built the way NewAIToolService builds it, with no detect options: the login shell comes from SHELL, and the
// launch and the naming call use the same one (plan 4.7: $SHELL -l -i -c).
func TestAIToolServiceUsesTheUsersLoginShellEverywhere(t *testing.T) {
	for _, c := range []struct{ shell, want string }{
		{"/bin/bash", "/bin/bash"},
		{"/usr/bin/fish", "/bin/zsh"}, // a shell that cannot run -c '"$@"' falls back for all three
		{"", "/bin/zsh"},
	} {
		t.Setenv("SHELL", c.shell)
		var namingArgv []string
		s := NewAIToolServiceWith(AIToolDeps{GOOS: "darwin", Home: t.TempDir(),
			Namer: aitools.Namer{WorkRoot: t.TempDir(), BaseEnv: func() []string { return []string{"PATH=/x"} },
				Run: func(ctx context.Context, a []string, d string, e []string) ([]byte, int, error) {
					namingArgv = a
					return []byte(`{"title":"Fix","slug":"fix"}`), 0, nil
				}}})
		l, err := s.buildLaunch(aitools.LaunchRequest{Tool: aitools.Claude, FirstMessage: "hi there"})
		if err != nil {
			t.Fatal(err)
		}
		if l.Argv[0] != c.want {
			t.Errorf("SHELL=%q: launch argv[0] %q want %q", c.shell, l.Argv[0], c.want)
		}
		if _, err := s.NameTask(context.Background(), api.AIToolClaude, "Fix it", 1); err != nil {
			t.Fatal(err)
		}
		if namingArgv[0] != c.want {
			t.Errorf("SHELL=%q: naming argv[0] %q want %q", c.shell, namingArgv[0], c.want)
		}
		if got := s.detectOptions().Shell; got != c.want {
			t.Errorf("SHELL=%q: detect shell %q want %q", c.shell, got, c.want)
		}
	}
}

func TestAIToolServiceReadsMoreConfigFiles(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755)
	os.WriteFile(filepath.Join(home, ".config", "opencode", "opencode.jsonc"), []byte("{\n// comment\n\"permission\": \"allow\",\n}"), 0o600)
	proj := t.TempDir()
	os.MkdirAll(filepath.Join(proj, ".opencode"), 0o755)
	s := NewAIToolServiceWith(AIToolDeps{Home: home})
	if a, _ := s.GetApproval(api.AIToolOpenCode, "", proj); a.Mode != "allow" {
		t.Fatalf("opencode.jsonc: %+v", a)
	}
	os.WriteFile(filepath.Join(proj, ".opencode", "opencode.json"), []byte(`{"permission":"deny"}`), 0o600)
	if a, _ := s.GetApproval(api.AIToolOpenCode, "", proj); a.Mode != "deny" {
		t.Fatalf(".opencode/opencode.json: %+v", a)
	}
	// an account's own CODEX_HOME decides whether the person has a notify program
	acc := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
	os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte("notify = [\"/usr/bin/say\"]\n"), 0o600)
	l, err := s.buildLaunch(aitools.LaunchRequest{Tool: aitools.Codex, CodexNotify: []string{"/app/codepass", "report"}, AccountEnv: []string{"CODEX_HOME=" + acc}})
	if err != nil || l.NotifySkipped {
		t.Fatalf("the default folder's notify must not count for an account folder: %+v %v", l, err)
	}
	os.WriteFile(filepath.Join(acc, "config.toml"), []byte("notify = [\"x\"]\n"), 0o600)
	if l, _ = s.buildLaunch(aitools.LaunchRequest{Tool: aitools.Codex, CodexNotify: []string{"/app/codepass", "report"}, AccountEnv: []string{"CODEX_HOME=" + acc}}); !l.NotifySkipped {
		t.Fatal("the account folder's own notify must be seen")
	}
}
