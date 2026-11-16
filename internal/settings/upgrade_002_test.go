package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rudian/codepass/internal/api"
)

// settings002 is a settings.json as AWS SSO Hub 0.0.2 writes it (fake data only): sessions carry the
// sessionEndsAt and newSession fields; one record holds an end that is not RFC 3339.
const settings002 = `{
  "schemaVersion": 1,
  "language": "en",
  "uiLanguage": "en",
  "privateSignIn": false,
  "firstRunDone": true,
  "accountNames": {"111122223333": "acme-prod"},
  "sessions": {
    "acme": {"loginAt": "2026-11-04T01:30:00Z", "clientIdHash": "3f9a0c7be21d4e58", "tokenIssuedAt": "2026-11-04T02:18:00Z",
             "sessionMinutes": 480, "sessionEndsAt": "2026-11-04T09:30:00Z", "newSession": true},
    "globex": {"loginAt": "2026-11-04T01:00:00Z", "sessionEndsAt": "tomorrow at nine", "newSession": true}
  }
}`

// A 0.0.2 data folder opens in Codepass keeping both new session fields; an end that is not RFC 3339 is
// dropped (the 0.0.2 rule); later writes keep what was loaded.
func TestUpgradeFrom002KeepsSessionFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(settings002), 0o600); err != nil {
		t.Fatal(err)
	}
	s, first, err := Open(path, filepath.Join(dir, "code"), fixedNow)
	if err != nil || first {
		t.Fatalf("first=%v err=%v", first, err)
	}
	acme, ok := s.SessionRecord("acme")
	want := api.SessionRecord{LoginAt: "2026-11-04T01:30:00Z", ClientIDHash: "3f9a0c7be21d4e58", TokenIssuedAt: "2026-11-04T02:18:00Z",
		SessionMinutes: 480, SessionEndsAt: "2026-11-04T09:30:00Z", NewSession: true}
	if !ok || acme != want {
		t.Fatalf("acme: %+v", acme)
	}
	globex, _ := s.SessionRecord("globex")
	if globex.SessionEndsAt != "" || !globex.NewSession || globex.LoginAt != "2026-11-04T01:00:00Z" {
		t.Fatalf("globex: %+v", globex)
	}
	if s.PrivateSignIn() {
		t.Fatal("the private sign-in choice must survive")
	}
	// an unrelated write rewrites the file: both fields are still there
	if err := s.SetUILanguage("zh-CN"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Sessions map[string]map[string]any `json:"sessions"`
		Names    map[string]string         `json:"accountNames"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Sessions["acme"]["sessionEndsAt"] != "2026-11-04T09:30:00Z" || out.Sessions["acme"]["newSession"] != true ||
		out.Sessions["globex"]["newSession"] != true || out.Names["111122223333"] != "acme-prod" {
		t.Fatalf("rewritten file lost data: %s", b)
	}
}
