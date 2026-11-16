package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/update"
)

func TestUpdatePhasesMatchTheAPI(t *testing.T) {
	pairs := map[update.Phase]api.UpdatePhase{
		update.PhaseIdle: api.UpdateIdle, update.PhaseChecking: api.UpdateChecking, update.PhaseUpToDate: api.UpdateUpToDate,
		update.PhaseAvailable: api.UpdateAvailable, update.PhaseDownloading: api.UpdateDownloading, update.PhaseVerifying: api.UpdateVerifying,
		update.PhaseReady: api.UpdateReady, update.PhaseRestarting: api.UpdateRestarting, update.PhaseBlocked: api.UpdateBlocked,
		update.PhaseRefused: api.UpdateRefused, update.PhaseError: api.UpdateError,
	}
	for in, want := range pairs {
		if got := toAPIUpdateState(update.State{Phase: in}).Phase; got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestUpdateStateCarriesNoEnglishError(t *testing.T) {
	at := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	got := toAPIUpdateState(update.State{Phase: update.PhaseError, Error: "dial tcp 10.0.0.1: token=abc", Code: update.CodeNetwork, Step: update.StepDownload, Notes: "<b>x</b>", CheckedAt: at})
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "dial tcp") || strings.Contains(string(raw), `"error":`) {
		t.Errorf("the English error leaked: %s", raw)
	}
	if got.Code != "network" || got.Step != "download" || got.Notes != "<b>x</b>" || got.CheckedAt != "2026-11-16T09:00:00Z" {
		t.Errorf("state = %+v", got)
	}
}

func TestUpdatePreferences(t *testing.T) {
	c := &Container{}
	c.Paths.AppDataDir = t.TempDir()
	s := NewUpdateService(c)
	ctx := context.Background()
	if p, _ := s.GetPreferences(ctx); !p.AutoCheck {
		t.Fatal("the daily check must default to on")
	}
	if p, err := s.SetAutoCheck(ctx, false); err != nil || p.AutoCheck {
		t.Fatalf("SetAutoCheck(false) = %+v, %v", p, err)
	}
	if p, _ := s.GetPreferences(ctx); p.AutoCheck {
		t.Error("the choice was not kept")
	}
	if p, _ := s.SetAutoCheck(ctx, true); !p.AutoCheck {
		t.Error("turning it back on failed")
	}
}

func TestUpdateServiceWithoutController(t *testing.T) {
	s := NewUpdateService(&Container{})
	st, err := s.State(context.Background())
	if err != nil || st.Phase != api.UpdateIdle {
		t.Errorf("State = %+v, %v", st, err)
	}
	if _, err := s.Check(context.Background()); err == nil {
		t.Error("Check without a controller must fail")
	}
}

// The automatic check obeys the preference; this is the one place the schedule meets it.
func TestRunSkipsTheTickWhenAutoCheckIsOff(t *testing.T) {
	var on atomic.Bool
	ctl := update.NewController(update.ControllerConfig{
		Running: "1.0.0", Highest: update.Highest{Path: filepath.Join(t.TempDir(), "highest")},
		HelperLog: filepath.Join(t.TempDir(), "h"), StagedRecord: filepath.Join(t.TempDir(), "s"),
		AutoCheck: func() bool { return on.Load() },
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { ctl.Run(ctx, 5*time.Millisecond, 5*time.Millisecond); close(done) }()
	time.Sleep(80 * time.Millisecond)
	if ph := ctl.State().Phase; ph != update.PhaseIdle {
		t.Fatalf("with the check off the phase moved to %q", ph)
	}
	on.Store(true)
	deadline := time.Now().Add(2 * time.Second)
	for ctl.State().Phase == update.PhaseIdle && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if ctl.State().Phase == update.PhaseIdle {
		t.Error("with the check on, a tick must run")
	}
	cancel()
	<-done
}

func TestAutoCheckerFollowsThePreferenceAndTheDailyGap(t *testing.T) {
	on := true
	now := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	st := update.State{}
	a := &autoChecker{on: func() bool { return on }, state: func() update.State { return st }, now: func() time.Time { return now }}
	on = false
	if a.due() {
		t.Fatal("off: no check")
	}
	on = true
	if !a.due() {
		t.Fatal("the launch tick checks when the setting is on")
	}
	st = update.State{Phase: update.PhaseUpToDate, CheckedAt: now}
	now = now.Add(5 * time.Hour)
	if a.due() {
		t.Error("5 h after a check is too soon")
	}
	now = now.Add(19*time.Hour + time.Minute)
	if !a.due() {
		t.Error("over 24 h after a check is due")
	}
	st = update.State{Phase: update.PhaseError, CheckedAt: now}
	if !a.due() {
		t.Error("a failed check is retried on the next tick")
	}
	on = false
	if a.due() {
		t.Error("turning the setting off stops the ticks")
	}
}

func TestUpdatePrefsAreVersioned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update", "update-prefs.json")
	if err := writeUpdatePrefs(path, api.UpdatePreferences{AutoCheck: false}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "schemaVersion") {
		t.Errorf("no schemaVersion: %s", b)
	}
	if readUpdatePrefs(path).AutoCheck {
		t.Error("off was not kept")
	}
	for i := 0; i < 20; i++ { // quick toggles never fail
		if err := writeUpdatePrefs(path, api.UpdatePreferences{AutoCheck: i%2 == 0}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, []byte(`{"schemaVersion":99,"autoCheckOff":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !readUpdatePrefs(path).AutoCheck {
		t.Error("a newer file must read as the default")
	}
}
