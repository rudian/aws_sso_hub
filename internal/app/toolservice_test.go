package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/netx"
	"github.com/rudian/codepass/internal/toolinstall"
)

type toolRunner struct {
	mu    sync.Mutex
	calls []toolinstall.Cmd
	hold  chan struct{}
}

func (r *toolRunner) Run(ctx context.Context, c toolinstall.Cmd, onLine func(string)) (int, error) {
	r.mu.Lock()
	r.calls = append(r.calls, c)
	r.mu.Unlock()
	if r.hold != nil {
		<-r.hold
	}
	onLine("==> Pouring x")
	return 0, nil
}

type stubNet struct{ toolinstall.Net }

func (stubNet) Proxy() netx.Config { return netx.Config{Source: "none"} }

func (r *toolRunner) count() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.calls) }

func waitCalls(t *testing.T, r *toolRunner, n int) {
	t.Helper()
	for i := 0; i < 400 && r.count() < n; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if r.count() < n {
		t.Fatalf("the runner was called %d times, want %d", r.count(), n)
	}
}

func newToolSvc(t *testing.T, gate api.SetupState, run *toolRunner) (*ToolService, *[]string, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var events, opened []string
	svc := NewToolServiceWith(ToolDeps{
		GOOS: "darwin",
		Gate: func(context.Context) api.SetupState { return gate },
		Net:  stubNet{},
		Emit: func(name string, data any) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, name)
			if _, err := json.Marshal(data); err != nil {
				t.Errorf("event payload is not JSON: %v", err)
			}
		},
		OpenURL: func(u string) error { opened = append(opened, u); return nil },
		Install: toolinstall.Config{Runner: run, PrefixDir: t.TempDir() + "/none", BrewPath: "/usr/bin/true", CacheDir: t.TempDir(),
			Recheck: func(context.Context) api.SetupState { return api.SetupState{} }}, // the install "worked"
	})
	return svc, &events, &opened
}

func TestToolServiceSetupStateHasNoNullSlices(t *testing.T) {
	svc, _, _ := newToolSvc(t, api.SetupState{Platform: "darwin"}, &toolRunner{})
	st, err := svc.GetSetupState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(st)
	if strings.Contains(string(b), "null") {
		t.Errorf("null in %s", b)
	}
	is, _ := svc.GetInstallState()
	b, _ = json.Marshal(is)
	if strings.Contains(string(b), "null") {
		t.Errorf("null in %s", b)
	}
}

func TestToolServiceRunsAndEmits(t *testing.T) {
	run := &toolRunner{}
	gate := api.SetupState{Platform: "darwin", Preflight: api.Preflight{EnoughSpace: true}, Tools: []api.SetupTool{
		{ID: api.SetupHomebrew, Status: api.ToolOK},
		{ID: api.SetupGit, Status: api.ToolMissing, Blocking: true, Fix: api.FixInstall},
		{ID: api.SetupAWSCLI, Status: api.ToolOK},
	}}
	svc, events, _ := newToolSvc(t, gate, run)
	if err := svc.StartSetupInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc.installer().Wait()
	st, _ := svc.GetInstallState()
	if !st.Finished || st.Failure != nil || len(st.Steps) != 1 || st.Steps[0].State != api.StepDone {
		t.Fatalf("%+v", st)
	}
	if len(run.calls) != 1 || run.calls[0].Args[0] != "install" || run.calls[0].Args[1] != "git" {
		t.Errorf("calls: %+v", run.calls)
	}
	if len(*events) == 0 {
		t.Fatal("no events")
	}
	for _, e := range *events {
		if e != api.EventToolInstall {
			t.Errorf("event %q", e)
		}
	}
}

func TestToolServiceSecondStartAndCancel(t *testing.T) {
	run := &toolRunner{hold: make(chan struct{})}
	gate := api.SetupState{Platform: "darwin", Preflight: api.Preflight{EnoughSpace: true}, Tools: []api.SetupTool{
		{ID: api.SetupHomebrew, Status: api.ToolOK},
		{ID: api.SetupGit, Status: api.ToolMissing, Blocking: true, Fix: api.FixInstall},
		{ID: api.SetupAWSCLI, Status: api.ToolMissing, Blocking: true, Fix: api.FixInstall},
	}}
	svc, _, _ := newToolSvc(t, gate, run)
	if err := svc.StartSetupInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitCalls(t, run, 1)
	if err := svc.StartSetupInstall(context.Background()); err == nil {
		t.Error("a second start must be refused")
	}
	if err := svc.CancelSetupInstall(); err != nil {
		t.Fatal(err)
	}
	close(run.hold)
	svc.installer().Wait()
	st, _ := svc.GetInstallState()
	if !st.Cancelled || run.count() != 1 {
		t.Errorf("cancelled=%v calls=%d", st.Cancelled, run.count())
	}
}

func TestToolServiceOpensOnlyItsFixedPages(t *testing.T) {
	svc, _, opened := newToolSvc(t, api.SetupState{}, &toolRunner{})
	for _, id := range []api.SetupToolID{api.SetupHomebrew, api.SetupGit, api.SetupAWSCLI} {
		if err := svc.OpenInstallPage(id); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
	if err := svc.OpenInstallPage(api.SetupWinget); err == nil {
		t.Error("winget has no page on macOS")
	}
	if err := svc.OpenInstallPage("javascript:alert(1)"); err == nil {
		t.Error("an unknown tool must be refused")
	}
	for _, u := range *opened {
		if !strings.HasPrefix(u, "https://") {
			t.Errorf("not https: %s", u)
		}
	}
	if len(*opened) != 3 {
		t.Errorf("opened %v", *opened)
	}
}

func TestToolServiceShutdownStopsARun(t *testing.T) {
	run := &toolRunner{hold: make(chan struct{})}
	gate := api.SetupState{Platform: "darwin", Preflight: api.Preflight{EnoughSpace: true}, Tools: []api.SetupTool{
		{ID: api.SetupHomebrew, Status: api.ToolOK},
		{ID: api.SetupGit, Status: api.ToolMissing, Blocking: true, Fix: api.FixInstall},
		{ID: api.SetupAWSCLI, Status: api.ToolMissing, Blocking: true, Fix: api.FixInstall},
	}}
	svc, _, _ := newToolSvc(t, gate, run)
	svc.StartSetupInstall(context.Background())
	waitCalls(t, run, 1)
	svc.ServiceShutdown()
	close(run.hold)
	svc.installer().Wait()
	if st, _ := svc.GetInstallState(); st.Running || run.count() != 1 {
		t.Errorf("running=%v calls=%d", st.Running, run.count())
	}
}

// A tool path chosen in Settings that blocks the gate is cleared before the run (the setup screen cannot change
// it), and the gate is asked again.
func TestToolServiceClearsABlockingChosenPath(t *testing.T) {
	chosen := "/opt/old/git"
	var cleared []api.SetupToolID
	blocked := api.SetupState{Platform: "darwin", Blocked: true, Preflight: api.Preflight{EnoughSpace: true}, Tools: []api.SetupTool{
		{ID: api.SetupHomebrew, Status: api.ToolOK},
		{ID: api.SetupGit, Status: api.ToolTooOld, Version: "2.43.0", Min: "2.44.0", Blocking: true, Fix: api.FixUseDetected, Override: true},
		{ID: api.SetupAWSCLI, Status: api.ToolOK},
	}}
	run := &toolRunner{}
	svc := NewToolServiceWith(ToolDeps{
		GOOS: "darwin", Net: stubNet{},
		GitPath: func() string { return chosen },
		Gate: func(context.Context) api.SetupState {
			if chosen != "" {
				return blocked
			}
			return api.SetupState{Platform: "darwin", Tools: []api.SetupTool{{ID: api.SetupGit, Status: api.ToolOK}}}
		},
		ClearPath: func(id api.SetupToolID) error { cleared = append(cleared, id); chosen = ""; return nil },
		Install:   toolinstall.Config{Runner: run, CacheDir: t.TempDir()},
	})
	if err := svc.StartSetupInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc.installer().Wait()
	st, _ := svc.GetInstallState()
	if len(cleared) != 1 || cleared[0] != api.SetupGit {
		t.Fatalf("cleared %v", cleared)
	}
	if !st.Finished || st.Failure != nil || len(st.Steps) != 0 || run.count() != 0 {
		t.Errorf("a use-detected row installs nothing: %+v calls=%d", st, run.count())
	}
}

// A step that succeeds but leaves the row blocked ends the run with a cause, not "done".
func TestToolServiceRecheckFailsAStepThatChangedNothing(t *testing.T) {
	blocked := api.SetupState{Platform: "darwin", Blocked: true, Preflight: api.Preflight{EnoughSpace: true}, Tools: []api.SetupTool{
		{ID: api.SetupHomebrew, Status: api.ToolOK},
		{ID: api.SetupGit, Status: api.ToolMissing, Blocking: true, Fix: api.FixInstall},
		{ID: api.SetupAWSCLI, Status: api.ToolMissing, Blocking: true, Fix: api.FixInstall},
	}}
	run := &toolRunner{}
	svc := NewToolServiceWith(ToolDeps{
		GOOS: "darwin", Net: stubNet{},
		Gate:    func(context.Context) api.SetupState { return blocked },
		Install: toolinstall.Config{Runner: run, PrefixDir: t.TempDir() + "/none", BrewPath: "/usr/bin/true", CacheDir: t.TempDir()},
	})
	if err := svc.StartSetupInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc.installer().Wait()
	st, _ := svc.GetInstallState()
	if st.Failure == nil || st.Failure.Cause != api.CauseInstallerFailed || st.Failure.StepID != "git:install" || !strings.Contains(st.Failure.Line, "still not usable") {
		t.Fatalf("%+v", st.Failure)
	}
	if run.count() != 1 {
		t.Errorf("the next step must not run: %d", run.count())
	}
}
