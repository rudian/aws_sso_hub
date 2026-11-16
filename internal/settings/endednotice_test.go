package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/api"
)

// A settings file written before reminders.endedNoticeOff existed (0.0.1 and 0.0.2 write the same reminders
// block) reads as "notify"; so does a file with no reminders block at all.
func TestEndedNoticeDefaultsToNotify(t *testing.T) {
	for name, body := range map[string]string{
		"0.0.1/0.0.2 reminders": `{"schemaVersion": 1, "firstRunDone": true, "reminders": {"expiryNotice": false, "leadMinutes": 30, "autoRenew": true}}`,
		"no reminders":          `{"schemaVersion": 1, "firstRunDone": true}`,
		"fresh file":            "",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "settings.json")
			if body != "" {
				if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			s, _, err := Open(path, filepath.Join(dir, "code"), fixedNow)
			if err != nil {
				t.Fatal(err)
			}
			if !s.EndedNotice() {
				t.Fatal("a file without endedNoticeOff must notify")
			}
			if got := s.API().Reminders.EndedNoticeOff; got {
				t.Fatal("EndedNoticeOff must read false")
			}
		})
	}
}

// Turning the notice off is stored as endedNoticeOff and survives a reload; the other reminder fields stay.
func TestEndedNoticeOffRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	s, _, err := Open(path, filepath.Join(dir, "code"), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	r := s.API().Reminders
	r.EndedNoticeOff = true
	if _, err := s.Update(api.SettingsPatch{Reminders: &r}); err != nil {
		t.Fatal(err)
	}
	if s.EndedNotice() {
		t.Fatal("off")
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"endedNoticeOff": true`) {
		t.Fatalf("not stored as endedNoticeOff:\n%s", b)
	}
	again, _, err := Open(path, filepath.Join(dir, "code"), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if again.EndedNotice() || !again.ExpiryNotice() || again.ReminderLead() != 15*60*1e9 {
		t.Fatalf("%+v", again.API().Reminders)
	}
}
