package aitools

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/toolinstall"
)

type fakeRunner struct {
	mu    sync.Mutex
	calls []toolinstall.Cmd
	lines []string
	exit  int
	err   error
	block chan struct{} // when set, wait for the context
}

func (f *fakeRunner) Run(ctx context.Context, c toolinstall.Cmd, onLine func(string)) (int, error) {
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
	for _, l := range f.lines {
		onLine(l)
	}
	if f.block != nil {
		close(f.block)
		<-ctx.Done()
		return -1, ctx.Err()
	}
	return f.exit, f.err
}

func testInstaller(f *fakeRunner, goos string, found map[string]string, rc func(ctx context.Context, id ID) Detection) (*Installer, *[]api.AIInstallState) {
	var mu sync.Mutex
	var seen []api.AIInstallState
	in := NewInstaller(InstallerConfig{
		GOOS: goos, Runner: f,
		Lookup: func(n string) (string, error) {
			if p, ok := found[n]; ok {
				return p, nil
			}
			return "", os.ErrNotExist
		},
		BaseEnv:  func() []string { return []string{"PATH=/x", "HTTPS_proxy=http://old", "HOME=/h"} },
		ProxyEnv: func() []string { return []string{"HTTPS_PROXY=http://new:8080"} },
		Recheck:  rc,
		Info: func(d Detection) api.AIToolInfo {
			return api.AIToolInfo{ID: api.AIToolID(d.ID), Installed: d.Installed, Version: d.Version}
		},
		Emit: func(s api.AIInstallState) { mu.Lock(); seen = append(seen, s); mu.Unlock() },
	})
	return in, &seen
}

func TestInstallBrewSucceeds(t *testing.T) {
	f := &fakeRunner{lines: []string{"==> Downloading", "==> Installing Cask claude-code"}}
	in, seen := testInstaller(f, "darwin", map[string]string{"brew": "/opt/homebrew/bin/brew"},
		func(ctx context.Context, id ID) Detection {
			return Detection{ID: id, Installed: true, Version: "2.1.325"}
		})
	if err := in.Start(context.Background(), Claude); err != nil {
		t.Fatal(err)
	}
	if st := in.State(); st.Command != "brew install --cask claude-code" || !st.Running {
		t.Fatalf("the exact command is shown first: %+v", st)
	}
	in.Wait()
	st := in.State()
	if st.Running || !st.Finished || st.Failure != nil || st.Installed == nil || st.Installed.Version != "2.1.325" || len(st.Log) != 2 {
		t.Fatalf("%+v", st)
	}
	c := f.calls[0]
	if c.Path != "/opt/homebrew/bin/brew" || strings.Join(c.Args, " ") != "install --cask claude-code" {
		t.Fatalf("%+v", c)
	}
	env := strings.Join(c.Env, "\n")
	if strings.Contains(env, "http://old") || !strings.Contains(env, "HTTPS_PROXY=http://new:8080") || !strings.Contains(env, "NONINTERACTIVE=1") || !strings.Contains(env, "PATH=/opt/homebrew/bin:") {
		t.Fatalf("%s", env)
	}
	if len(*seen) < 2 || !(*seen)[len(*seen)-1].Finished {
		t.Fatal("events")
	}
}

func TestInstallFailureCauses(t *testing.T) {
	// the installer is missing: the exact command is still in the state
	in, _ := testInstaller(&fakeRunner{}, "darwin", nil, nil)
	in.Start(context.Background(), Codex)
	in.Wait()
	if st := in.State(); st.Failure == nil || st.Failure.Cause != api.AICauseInstallerMissing || st.Command != "brew install --cask codex" {
		t.Fatalf("%+v", st)
	}
	// a non-zero exit: the installer's own error line
	f := &fakeRunner{lines: []string{"==> x", "Error: No available cask", "see above"}, exit: 1}
	in, _ = testInstaller(f, "darwin", map[string]string{"brew": "/opt/homebrew/bin/brew"}, nil)
	in.Start(context.Background(), OpenCode)
	in.Wait()
	if st := in.State(); st.Failure == nil || st.Failure.Cause != api.AICauseFailed || st.Failure.Detail != "Error: No available cask" || st.Failure.Exit != 1 {
		t.Fatalf("%+v", st)
	}
	// success but not found afterwards
	in, _ = testInstaller(&fakeRunner{}, "windows", map[string]string{"winget": `C:\w\winget.exe`},
		func(ctx context.Context, id ID) Detection { return Detection{ID: id} })
	in.Start(context.Background(), Claude)
	in.Wait()
	if st := in.State(); st.Failure == nil || st.Failure.Cause != api.AICauseStillMissing || st.Installed != nil {
		t.Fatalf("%+v", st)
	}
	// cannot start
	in, _ = testInstaller(&fakeRunner{err: errors.New("boom")}, "windows", map[string]string{"npm": `C:\n\npm.cmd`}, nil)
	in.Start(context.Background(), OpenCode)
	in.Wait()
	if st := in.State(); st.Failure == nil || st.Failure.Cause != api.AICauseFailed || !strings.Contains(st.Failure.Detail, "boom") {
		t.Fatalf("%+v", st)
	}
}

func TestInstallWindowsCommands(t *testing.T) {
	for id, want := range map[ID]string{Claude: "install --id Anthropic.ClaudeCode -e", Codex: "install --id OpenAI.Codex -e", OpenCode: "install -g opencode-ai"} {
		f := &fakeRunner{}
		in, _ := testInstaller(f, "windows", map[string]string{"winget": `C:\w`, "npm": `C:\n`}, nil)
		in.Start(context.Background(), id)
		in.Wait()
		if got := strings.Join(f.calls[0].Args, " "); got != want {
			t.Errorf("%s: %q", id, got)
		}
	}
}

func TestInstallCancelStopsTheRunAndOnlyOneRunsAtATime(t *testing.T) {
	f := &fakeRunner{block: make(chan struct{})}
	in, _ := testInstaller(f, "darwin", map[string]string{"brew": "/b/brew"}, nil)
	if err := in.Start(context.Background(), Claude); err != nil {
		t.Fatal(err)
	}
	<-f.block
	if err := in.Start(context.Background(), Codex); !errors.Is(err, ErrRunning) {
		t.Fatalf("%v", err)
	}
	in.Cancel()
	done := make(chan struct{})
	go func() { in.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not stop the run")
	}
	if st := in.State(); !st.Cancelled || st.Failure != nil || st.Running || !st.Finished {
		t.Fatalf("%+v", st)
	}
	// a new run can start afterwards
	f2 := &fakeRunner{}
	in2, _ := testInstaller(f2, "darwin", map[string]string{"brew": "/b/brew"}, nil)
	if err := in2.Start(context.Background(), Codex); err != nil {
		t.Fatal(err)
	}
	in2.Wait()
	if err := in2.Start(context.Background(), Codex); err != nil {
		t.Fatalf("second run: %v", err)
	}
	in2.Wait()
}

func TestInstallRefusesUnknownToolAndSystem(t *testing.T) {
	in, _ := testInstaller(&fakeRunner{}, "darwin", nil, nil)
	if err := in.Start(context.Background(), "vim"); !errors.Is(err, ErrBadTool) {
		t.Fatal(err)
	}
	in, _ = testInstaller(&fakeRunner{}, "linux", nil, nil)
	if err := in.Start(context.Background(), Claude); !errors.Is(err, ErrNoInstaller) {
		t.Fatal(err)
	}
}

// Lines that arrive inside the 100 ms gap are sent when the gap ends, not dropped: a download that prints two
// lines and then stays silent still shows both.
func TestInstallerFlushesLinesThatCameInsideTheGap(t *testing.T) {
	f := &fakeRunner{lines: []string{"==> Fetching", "==> Downloading https://example.com/x"}, block: make(chan struct{})}
	var mu sync.Mutex
	var lastLog []string
	in := NewInstaller(InstallerConfig{GOOS: "darwin", Runner: f,
		Lookup:  func(string) (string, error) { return "/opt/homebrew/bin/brew", nil },
		BaseEnv: func() []string { return []string{"PATH=/x"} },
		Emit:    func(s api.AIInstallState) { mu.Lock(); lastLog = s.Log; mu.Unlock() }})
	if err := in.Start(context.Background(), Claude); err != nil {
		t.Fatal(err)
	}
	<-f.block
	defer func() { in.Cancel(); in.Wait() }()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(lastLog)
		mu.Unlock()
		if n == 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the lines that arrived inside the gap were never sent")
}
