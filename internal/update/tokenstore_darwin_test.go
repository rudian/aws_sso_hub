//go:build darwin

package update

import (
	"os"
	"strings"
	"testing"
)

// Runs against the real login Keychain under a throw-away service name, so it is opt-in:
// CODEPASS_KEYCHAIN_TEST=1 go test ./internal/update -run TestKeychainRoundTrip
func TestKeychainRoundTrip(t *testing.T) {
	if os.Getenv("CODEPASS_KEYCHAIN_TEST") != "1" {
		t.Skip("set CODEPASS_KEYCHAIN_TEST=1 to use the real Keychain")
	}
	s := secretStore{b: platformBackend("Codepass update test channel (unit test)", "read-only token")}
	t.Cleanup(func() { _ = s.Delete() })
	_ = s.Delete()
	if got, err := s.Get(); err != nil || got != "" {
		t.Fatalf("empty: %q %v", got, err)
	}
	tok := "github_pat_" + strings.Repeat("A1_-", 8)
	if err := s.Set(tok); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get(); err != nil || got != tok {
		t.Fatalf("get: %q %v", got, err)
	}
	tok2 := tok + "X"
	if err := s.Set(tok2); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(); got != tok2 {
		t.Fatalf("update: %q", got)
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get(); err != nil || got != "" {
		t.Fatalf("after delete: %q %v", got, err)
	}
}
