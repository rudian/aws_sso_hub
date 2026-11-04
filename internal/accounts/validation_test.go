package accounts

import (
	"crypto/sha256"
	"os"
	"strings"
	"testing"

	"github.com/rudian/aws_sso_hub/internal/api"
)

func fileSum(t *testing.T, path string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

func wantErr(t *testing.T, err error, code api.ErrorCode) *api.AppError {
	t.Helper()
	ae := api.AsAppError(err)
	if err == nil || ae == nil || ae.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
	return ae
}

func TestInjectedStartURLNeverReachesTheConfig(t *testing.T) {
	h := newHarness(t, true)
	before := fileSum(t, h.p.AWSConfigFile)
	evil := "https://acme.awsapps.com/start\n[profile evil]\ncredential_process = /usr/bin/id"

	c := h.eng.CheckPortalInput(api.PortalInput{Name: "acme", StartURL: evil, Region: "us-east-1"})
	if c.Errors["startUrl"] != api.ErrInvalidStartURL {
		t.Fatalf("check: %+v", c.Errors)
	}
	_, err := h.eng.AddDraft(api.PortalInput{Name: "acme", StartURL: evil, Region: "us-east-1"})
	wantErr(t, err, api.ErrInvalidStartURL)
	if h.eng.IsDraft("acme") {
		t.Fatal("no draft may be created")
	}
	if fileSum(t, h.p.AWSConfigFile) != before {
		t.Fatal("config changed")
	}
}

func TestCreateProfilesRejectsUnsafeFields(t *testing.T) {
	h := newHarness(t, true)
	before := fileSum(t, h.p.AWSConfigFile)
	base := api.NewProfileSpec{Name: "globex-admin", AccountID: "444455556666", Role: "AdministratorAccess"}
	cases := map[string]struct {
		mod   func(s *api.NewProfileSpec)
		field string
	}{
		"role newline":    {func(s *api.NewProfileSpec) { s.Role = "AdministratorAccess\ncredential_process = /usr/bin/id" }, "role"},
		"role crlf":       {func(s *api.NewProfileSpec) { s.Role = "AdministratorAccess\r\n[profile x]" }, "role"},
		"role bracket":    {func(s *api.NewProfileSpec) { s.Role = "AdministratorAccess]" }, "role"},
		"role tab":        {func(s *api.NewProfileSpec) { s.Role = "AdministratorAccess\tx" }, "role"},
		"role empty":      {func(s *api.NewProfileSpec) { s.Role = "" }, "role"},
		"role too long":   {func(s *api.NewProfileSpec) { s.Role = strings.Repeat("a", 65) }, "role"},
		"account letter":  {func(s *api.NewProfileSpec) { s.AccountID = "44445555666a" }, "accountId"},
		"account newline": {func(s *api.NewProfileSpec) { s.AccountID = "44445555666\n" }, "accountId"},
	}
	for name, c := range cases {
		spec := base
		c.mod(&spec)
		req := api.CreateProfilesRequest{Portal: "initech", Profiles: []api.NewProfileSpec{spec}}
		_, perr := h.eng.PreviewCreate(req)
		ae := wantErr(t, perr, api.ErrInvalidInput)
		if ae.Params["field"] != c.field {
			t.Errorf("%s: field %q, want %q", name, ae.Params["field"], c.field)
		}
		_, cerr := h.eng.Create(req, "")
		wantErr(t, cerr, api.ErrInvalidInput)
	}
	nameReq := api.CreateProfilesRequest{Portal: "initech", Profiles: []api.NewProfileSpec{{Name: "x]\n[profile y", AccountID: "444455556666", Role: "AdministratorAccess"}}}
	_, nerr := h.eng.PreviewCreate(nameReq)
	wantErr(t, nerr, api.ErrInvalidName)
	if fileSum(t, h.p.AWSConfigFile) != before {
		t.Fatal("config changed")
	}

	for _, role := range []string{"AdministratorAccess", "acme.Admin+ops@x", "Power_User=1,2"} {
		spec := base
		spec.Role = role
		if _, err := h.eng.PreviewCreate(api.CreateProfilesRequest{Portal: "initech", Profiles: []api.NewProfileSpec{spec}}); err != nil {
			t.Errorf("role %q must pass: %v", role, err)
		}
	}
}

func TestRenamePortalRejectsUnsafeName(t *testing.T) {
	h := newHarness(t, true)
	before := fileSum(t, h.p.AWSConfigFile)
	for _, name := range []string{"x]\n[profile y", "x]", "a b", "a\nb"} {
		_, err := h.eng.PreviewRenamePortal("initech", name)
		wantErr(t, err, api.ErrInvalidName)
		_, err = h.eng.RenamePortal("initech", name, "")
		wantErr(t, err, api.ErrInvalidName)
	}
	if fileSum(t, h.p.AWSConfigFile) != before {
		t.Fatal("config changed")
	}
}

func TestPortalURLValidatesTheStoredStartURL(t *testing.T) {
	h := newHarness(t, true)
	cfg := string(readTestdata(t, "config.fixture")) + "\n[sso-session evil]\nsso_start_url = https://evil.example\nsso_region = us-east-1\n" +
		"\n[sso-session sneaky]\nsso_start_url = https://acme.awsapps.com/start?next=https://evil.example\nsso_region = us-east-1\n"
	if err := os.WriteFile(h.p.AWSConfigFile, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, portal := range []string{"evil", "sneaky"} {
		u, err := h.eng.PortalURL(portal)
		wantErr(t, err, api.ErrInvalidStartURL)
		if u != "" {
			t.Fatalf("%s: nothing may be returned, got %q", portal, u)
		}
	}
	if u, err := h.eng.PortalURL("initech"); err != nil || u != portalInitech.StartURL {
		t.Fatalf("%q %v", u, err)
	}
}

func TestConsoleURLValidatesTheProfile(t *testing.T) {
	h := newHarness(t, true)
	cfg := string(readTestdata(t, "config.fixture")) +
		"\n[profile evil-host]\nsso_start_url = https://evil.example\nsso_region = us-east-1\nsso_account_id = 111122223333\nsso_role_name = AdministratorAccess\n" +
		"\n[profile odd-role]\nsso_session = initech\nsso_account_id = 111122223333\nsso_role_name = AdministratorAccess&x=y#\n" +
		"\n[profile odd-account]\nsso_session = initech\nsso_account_id = 1111\nsso_role_name = AdministratorAccess\n"
	if err := os.WriteFile(h.p.AWSConfigFile, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := h.eng.ConsoleURL("evil-host", "")
	wantErr(t, err, api.ErrInvalidStartURL)
	_, err = h.eng.ConsoleURL("odd-role", "")
	wantErr(t, err, api.ErrInvalidInput)
	_, err = h.eng.ConsoleURL("odd-account", "")
	wantErr(t, err, api.ErrInvalidInput)

	u, err := h.eng.ConsoleURL("initech-admin", "")
	if err != nil || u != "https://d-1234567890.awsapps.com/start/#/console?account_id=777788889999&role_name=AdministratorAccess" {
		t.Fatalf("%q %v", u, err)
	}
}
