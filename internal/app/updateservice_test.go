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
	"github.com/rudian/codepass/internal/settings"
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

func writeLegacyPrefs(t *testing.T, w *world, name, body string) string {
	t.Helper()
	path := filepath.Join(w.p.AppDataDir, "update", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// FU-17d: a settings file of 0.0.1 or 0.0.2 has no updateCheckOff field: the check is on, and an unrelated
// settings write does not add the field.
func TestUpdateCheckIsOnWithoutTheField(t *testing.T) {
	for name, body := range map[string]string{
		"0.0.1": `{"schemaVersion":1,"language":"en","theme":"system"}`,
		"0.0.2": `{"schemaVersion":1,"language":"en","theme":"system","privateSignIn":false}`,
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			if err := os.WriteFile(w.p.SettingsFile, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			st, _, err := settings.Open(w.p.SettingsFile, w.p.DefaultScanRoot, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			if !st.UpdateCheckOn() || st.API().UpdateCheckOff {
				t.Fatal("no field means the check is on")
			}
			dark := api.ThemeDark
			if _, err := st.Update(api.SettingsPatch{Theme: &dark}); err != nil {
				t.Fatal(err)
			}
			if b, _ := os.ReadFile(w.p.SettingsFile); strings.Contains(string(b), "updateCheckOff") {
				t.Fatalf("an unrelated write added the field: %s", b)
			}
		})
	}
}

// The preference lives in settings.json: SetAutoCheck writes it and tells the window.
func TestSetAutoCheckWritesSettings(t *testing.T) {
	w := newWorld(t)
	svc := NewUpdateService(w.c)
	if p, _ := svc.GetPreferences(context.Background()); !p.AutoCheck {
		t.Fatal("on by default")
	}
	if p, err := svc.SetAutoCheck(context.Background(), false); err != nil || p.AutoCheck {
		t.Fatalf("%v %v", p, err)
	}
	if b, _ := os.ReadFile(w.p.SettingsFile); !strings.Contains(string(b), `"updateCheckOff": true`) {
		t.Fatalf("not in settings.json: %s", b)
	}
	if w.c.Settings.UpdateCheckOn() || w.rt.count(api.EventSettingsChanged) == 0 {
		t.Fatal("off must be in effect and announced")
	}
	if p, _ := svc.GetPreferences(context.Background()); p.AutoCheck {
		t.Fatal("off was not kept")
	}
	if p, err := svc.SetAutoCheck(context.Background(), true); err != nil || !p.AutoCheck {
		t.Fatalf("%v %v", p, err)
	}
	if b, _ := os.ReadFile(w.p.SettingsFile); strings.Contains(string(b), "updateCheckOff") {
		t.Fatalf("on removes the field: %s", b)
	}
}

// FU-17d: an old update-prefs file moves into settings.json once and is removed.
func TestOldUpdatePrefsMoveIntoSettingsOnce(t *testing.T) {
	for _, name := range []string{"update-prefs.json", "prefs.json"} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			path := writeLegacyPrefs(t, w, name, `{"schemaVersion":1,"autoCheckOff":true}`)
			w.c.migrateUpdatePrefs()
			if w.c.Settings.UpdateCheckOn() {
				t.Fatal("off was not moved")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("the old file must be removed")
			}
			// the person turns it on again; a second run finds nothing to move
			if _, err := NewUpdateService(w.c).SetAutoCheck(context.Background(), true); err != nil {
				t.Fatal(err)
			}
			w.c.migrateUpdatePrefs()
			if !w.c.Settings.UpdateCheckOn() {
				t.Fatal("the move runs once")
			}
		})
	}
	// an old file that says on moves nothing but still goes away
	w := newWorld(t)
	path := writeLegacyPrefs(t, w, "update-prefs.json", `{"schemaVersion":1}`)
	w.c.migrateUpdatePrefs()
	if _, err := os.Stat(path); !os.IsNotExist(err) || !w.c.Settings.UpdateCheckOn() {
		t.Fatal("an old file that says on is removed and keeps the check on")
	}
}

// FU-17c: a damaged old file is set aside, never makes SetAutoCheck fail; a newer one is left alone.
func TestDamagedOldUpdatePrefsDoNotBreakTheSetting(t *testing.T) {
	w := newWorld(t)
	path := writeLegacyPrefs(t, w, "update-prefs.json", "{{{ not json")
	w.c.migrateUpdatePrefs()
	if m, _ := filepath.Glob(path + ".corrupt-*"); len(m) != 1 {
		t.Fatalf("set aside: %v", m)
	}
	if !w.c.Settings.UpdateCheckOn() {
		t.Fatal("a damaged file reads as the default")
	}
	if _, err := NewUpdateService(w.c).SetAutoCheck(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	newer := writeLegacyPrefs(t, w, "prefs.json", `{"schemaVersion":99,"autoCheckOff":true}`)
	w.c.migrateUpdatePrefs()
	if _, err := os.Stat(newer); err != nil {
		t.Fatal("a newer file is left alone")
	}
}

// FU-17d: building the container moves the old file once, without a hand call.
func TestContainerStartMovesOldUpdatePrefs(t *testing.T) {
	var path string
	w := newWorld(t, func(*ContainerOptions) {
		path = writeLegacyPrefs(t, active, "update-prefs.json", `{"schemaVersion":1,"autoCheckOff":true}`)
	})
	if w.c.Settings.UpdateCheckOn() {
		t.Fatal("the check must be off after start")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("the old file must be gone after start")
	}
}
