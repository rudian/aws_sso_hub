package accounts

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/rudian/aws_sso_hub/internal/api"
)

// Profiles that are neither SSO nor static keys must never be resolved through the SDK default chain, which
// would run credential_process programs and query the instance metadata service.
func TestProfileCredsDoesNotResolveOtherProfiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	h := newHarness(t, false)
	marker := filepath.Join(t.TempDir(), "ran")
	script := filepath.Join(t.TempDir(), "creds.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch '"+marker+"'\necho '{\"Version\":1,\"AccessKeyId\":\"AKIAIOSFODNN7EXAMPLE\",\"SecretAccessKey\":\"fake\"}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[profile tools]\ncredential_process = " + script + "\n\n[profile plain]\nregion = us-east-1\n\n" +
		"[profile acme]\nregion = us-east-1\n"
	creds := "[acme]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\naws_secret_access_key = fakeSecretValueDoNotUse0000000000000000\n"
	if err := os.MkdirAll(filepath.Dir(h.p.AWSConfigFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.p.AWSConfigFile, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.p.AWSCredentialsFile, []byte(creds), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAI44QH8DHBEXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "fakeEnvSecret")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	cs, err := h.eng.ProfileCreds(bg)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]ProfileCreds{}
	for _, c := range cs {
		seen[c.Name] = c
	}
	for _, name := range []string{"tools", "plain"} {
		c, ok := seen[name]
		if !ok {
			t.Fatalf("%s missing from %+v", name, cs)
		}
		if c.Kind != api.ProfileKindOther || c.Usable || c.Provider != nil {
			t.Errorf("%s must not be usable: %+v", name, c)
		}
	}
	if c := seen["acme"]; !c.Usable || c.Provider == nil || c.Kind != api.ProfileKindStatic {
		t.Errorf("a static-key profile keeps its own key: %+v", c)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("credential_process ran")
	}
}
