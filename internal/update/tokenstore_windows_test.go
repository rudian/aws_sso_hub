//go:build windows

package update

import (
	"os"
	"strings"
	"testing"
)

// Opt-in: CODEPASS_CRED_TEST=1 uses the real Credential Manager under a throw-away target name.
func TestCredentialRoundTrip(t *testing.T) {
	if os.Getenv("CODEPASS_CRED_TEST") != "1" {
		t.Skip("set CODEPASS_CRED_TEST=1 to use the real Credential Manager")
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
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
}
