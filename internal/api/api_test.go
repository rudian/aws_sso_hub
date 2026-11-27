package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestAsAppError(t *testing.T) {
	if got := AsAppError(nil); got != nil {
		t.Errorf("AsAppError(nil) = %v, want nil", got)
	}

	plain := errors.New("boom aws_secret_access_key = SEKRET123")
	got := AsAppError(plain)
	if got == nil || got.Code != ErrInternal {
		t.Fatalf("plain error -> %+v, want E_INTERNAL", got)
	}
	if strings.Contains(got.Detail, "SEKRET123") || !strings.Contains(got.Detail, "boom") {
		t.Errorf("Detail not redacted properly: %q", got.Detail)
	}
	if strings.Contains(got.Message, "SEKRET123") {
		t.Errorf("Message leaks secret: %q", got.Message)
	}

	orig := NewError(ErrNetwork, "x").WithDetail("h")
	wrapped := fmt.Errorf("outer: %w", orig)
	if got := AsAppError(wrapped); got != orig {
		t.Errorf("wrapped *AppError must be returned as is, got %+v", got)
	}
}

func TestAppErrorJSON(t *testing.T) {
	e := NewError(ErrNetwork, "x").WithDetail("h")
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"code":"E_NETWORK","message":"x","detail":"h"}`; string(b) != want {
		t.Errorf("json = %s, want %s", b, want)
	}
	e = NewError(ErrInvalidInput, "bad %s", "field").WithParam("field", "name")
	b, _ = json.Marshal(e)
	if want := `{"code":"E_INVALID_INPUT","message":"bad field","params":{"field":"name"}}`; string(b) != want {
		t.Errorf("json = %s, want %s", b, want)
	}
}

func TestErrorStringAndNotImplemented(t *testing.T) {
	e := NotImplemented()
	if e.Code != ErrNotImplemented || e.Error() != "E_NOT_IMPLEMENTED: not implemented" {
		t.Errorf("NotImplemented() = %+v / %q", e, e.Error())
	}
	var _ error = e // *AppError must satisfy error
}

func TestParamsAreRedacted(t *testing.T) {
	e := NewError(ErrInternal, "x").WithParam("v", "password=hunter2")
	if strings.Contains(e.Params["v"], "hunter2") {
		t.Errorf("param leaks: %q", e.Params["v"])
	}
}

func TestEventNames(t *testing.T) {
	names := []string{
		EventAccountsChanged, EventSessionChanged, EventSessionRenewed,
		EventLoginProgress, EventMigrationProgress, EventReposChanged, EventScanProgress,
		EventCloneProgress, EventExternalChange, EventSettingsChanged, EventNavigate, EventSecondInstance,
	}
	if len(names) != 12 {
		t.Fatalf("want 13 events, got %d", len(names))
	}
	seen := map[string]bool{}
	for _, n := range names {
		if n == "" || seen[n] {
			t.Errorf("empty or duplicate event name %q", n)
		}
		seen[n] = true
	}
}

func TestSessionStatusJSONKeys(t *testing.T) {
	b, err := json.Marshal(SessionStatus{Portal: "p", State: SessionActive})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{`"sessionKnown":false`, `"sessionMinutes":0`, `"sessionOverrun":false`, `"progressPct":0`, `"canRefresh":false`, `"refreshing":false`} {
		if !strings.Contains(string(b), k) {
			t.Errorf("SessionStatus JSON %s lacks %s", b, k)
		}
	}
	b, _ = json.Marshal(SessionRecord{})
	if string(b) != "{}" {
		t.Errorf("empty SessionRecord JSON = %s, want {}", b)
	}
}
