package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// A zero Highest must not switch the version floor off.
func TestZeroHighestFailsClosed(t *testing.T) {
	if v, err := (Highest{}).Get(); !errors.Is(err, ErrHighestUnreadable) || v != "" {
		t.Errorf("Get: %q %v", v, err)
	}
	if err := (Highest{}).Record("1.0.0"); !errors.Is(err, ErrHighestUnreadable) {
		t.Errorf("Record: %v", err)
	}
}

// A missing wire is a visible refusal, never a rule that silently does not apply.
func TestControllerRefusesWithoutItsWiring(t *testing.T) {
	good := ControllerConfig{
		Running:      "1.0.0-rc.1",
		Gate:         Gate{BundleID: BundleID, Requirement: "anchor apple"},
		Highest:      Highest{Path: filepath.Join(t.TempDir(), "highest")},
		Channel:      func() (Channel, error) { return Channel{}, errors.New("not reached") },
		HelperLog:    filepath.Join(t.TempDir(), "helperlog"),
		StagedRecord: filepath.Join(t.TempDir(), "staged"),
	}
	cases := map[string]func(*ControllerConfig){
		"no highest file":  func(c *ControllerConfig) { c.Highest = Highest{} },
		"no channel":       func(c *ControllerConfig) { c.Channel = nil },
		"no requirement":   func(c *ControllerConfig) { c.Gate.Requirement = "" },
		"no helper record": func(c *ControllerConfig) { c.HelperLog = "" },
		"no staged record": func(c *ControllerConfig) { c.StagedRecord = "" },
	}
	for name, mut := range cases {
		cfg := good
		mut(&cfg)
		c := NewController(cfg)
		st, err := c.Check(context.Background())
		if err == nil || !IsRefusal(err) || st.Phase != PhaseRefused || st.Code != CodeConfig || st.Step != StepCheck {
			t.Errorf("%s: %+v %v", name, st, err)
		}
	}
}

func TestRestartThatDoesNotQuitEndsInAnError(t *testing.T) {
	c := NewController(ControllerConfig{RestartWait: 20 * time.Millisecond})
	s := &session{rel: &updater.Release{Version: "1.0.0-rc.2"}, target: t.TempDir() + "/Codepass.app"}
	c.mu.Lock()
	c.session = s
	c.state = State{Phase: PhaseRestarting}
	c.mu.Unlock()
	c.watchRestart(s, time.Now())
	deadline := time.Now().Add(2 * time.Second)
	for c.State().Phase == PhaseRestarting && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if st := c.State(); st.Phase != PhaseError || st.Code != CodeRestartTimeout || st.Step != StepRestart {
		t.Fatalf("state %+v", st)
	}
	// a flow that moved on in the meantime is left alone
	c2 := NewController(ControllerConfig{RestartWait: 20 * time.Millisecond})
	c2.mu.Lock()
	c2.session = s
	c2.state = State{Phase: PhaseUpToDate}
	c2.mu.Unlock()
	c2.watchRestart(s, time.Now())
	time.Sleep(100 * time.Millisecond)
	if c2.State().Phase != PhaseUpToDate {
		t.Errorf("state %+v", c2.State())
	}
}

// The swap deletes the old bundle, so every folder inside it must be writable, not only the folder around it.
func TestPreflightBlocksABundleThatCannotBeDeleted(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	dir := t.TempDir()
	app := filepath.Join(dir, "Codepass.app")
	locked := filepath.Join(app, "Contents", "Frameworks")
	exe := filepath.Join(app, "Contents", "MacOS", "codepass")
	for _, d := range []string{filepath.Dir(exe), locked} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	err := Preflight(exe, "darwin")
	var pe *PreflightError
	if !errors.As(err, &pe) || pe.Kind != BlockNotWritable || pe.Folder != app {
		t.Fatalf("err = %v", err)
	}
	if err := os.Chmod(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Preflight(exe, "darwin"); err != nil {
		t.Errorf("writable bundle: %v", err)
	}
}

func TestTestChannelUsesPinnedKeysOnly(t *testing.T) {
	old := testKeyHex
	t.Cleanup(func() { testKeyHex = old })
	testKeyHex = [2]string{}
	if _, err := TestChannel("owner/repo", "tok"); err == nil {
		t.Error("the test channel must refuse while no test key is pinned")
	}
	if _, err := TestChannel("owner/repo", ""); err == nil {
		t.Error("the test channel must refuse without a token")
	}
	testKeyHex = [2]string{strings.Repeat("ab", 32), ""}
	ch, err := TestChannel("owner/repo", "tok")
	if err != nil || ch.Name != "test" || !ch.Prerelease || len(ch.Keys) != 1 || ch.Token != "tok" {
		t.Fatalf("%+v %v", ch, err)
	}
	for _, bad := range []string{"", "owner", "a/b/c", "o/r?x=1"} {
		if _, err := TestChannel(bad, ""); err == nil {
			t.Errorf("repository %q accepted", bad)
		}
	}
}

func TestSandboxTokenStoreNeverTouchesTheKeychain(t *testing.T) {
	s := NewTokenStore(true)
	if _, err := s.Get(); !errors.Is(err, ErrTokenStoreUnsupported) {
		t.Errorf("Get: %v", err)
	}
	if err := s.Set(strings.Repeat("a", 30)); !errors.Is(err, ErrTokenStoreUnsupported) {
		t.Errorf("Set: %v", err)
	}
}
