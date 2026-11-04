package awsconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudian/aws_sso_hub/internal/api"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func loadFixtures(t *testing.T) *Snapshot {
	t.Helper()
	dir := t.TempDir()
	f := Files{Config: filepath.Join(dir, "config"), Credentials: filepath.Join(dir, "credentials")}
	if err := os.WriteFile(f.Config, readFixture(t, "config.fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.Credentials, readFixture(t, "credentials.fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(f)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLoadClassifiesRealShape(t *testing.T) {
	s := loadFixtures(t)
	want := map[string]api.ProfileKind{
		"acme": api.ProfileKindStatic, "globex": api.ProfileKindStatic,
		"initech-admin": api.ProfileKindSSO, "initech-readonly": api.ProfileKindSSO,
	}
	ps := s.Profiles()
	if len(ps) != 4 {
		t.Fatalf("%d profiles", len(ps))
	}
	for _, p := range ps {
		if p.Kind != want[p.Name] {
			t.Errorf("%s kind %s", p.Name, p.Kind)
		}
	}
	sk, _ := s.Profile("acme")
	if !sk.HasStaticKeys || sk.StaticKeysIn != "credentials" || !sk.InConfig || !sk.InCredentials || sk.Region != "us-east-1" {
		t.Errorf("acme %+v", sk)
	}
	wm, _ := s.Profile("initech-admin")
	if wm.SSOSession != "initech" || wm.AccountID != "777788889999" || wm.Role != "AdministratorAccess" || wm.SSOStartURL != "https://d-1234567890.awsapps.com/start" {
		t.Errorf("initech-admin %+v", wm)
	}
	if wm.Order <= sk.Order {
		t.Error("order")
	}
	ses, ok := s.Session("initech")
	if !ok || ses.Region != "us-west-2" || len(ses.Scopes) != 1 || ses.Scopes[0] != "sso:account:access" || !ses.ScopesSet {
		t.Errorf("session %+v", ses)
	}
	k, ok := s.StaticKey("acme")
	if !ok || k.AccessKeyID != "AKIAIOSFODNN7EXAMPLE" {
		t.Fatal("static key")
	}
	if strings.Contains(k.String(), "fakeSecret") || !strings.Contains(k.String(), "AKIA••••••••••••MPLE") {
		t.Fatal(k.String())
	}
	if s.ReadOnly() || len(s.Issues(nil)) != 0 {
		t.Fatal("unexpected issues")
	}
	if s.ConfigHash() == "" || s.CredentialsHash() == "" {
		t.Fatal("hash")
	}
}

func TestLoadMissingAndBroken(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(Files{Config: filepath.Join(dir, "none"), Credentials: filepath.Join(dir, "none2")})
	if err != nil || len(s.Profiles()) != 0 || s.ConfigHash() != "" || s.ReadOnly() {
		t.Fatal("missing files must be empty", err)
	}
	p := filepath.Join(dir, "bad")
	os.WriteFile(p, []byte("[profile a]\ngarbage\n"), 0o600)
	s, _ = Load(Files{Config: p})
	if !s.ReadOnly() {
		t.Fatal("must be read only")
	}
	is := s.Issues(func(x string) string { return "~/bad" })
	if len(is) != 1 || is[0].Line != 2 || is[0].File != "~/bad" {
		t.Fatalf("%+v", is)
	}
	p2 := filepath.Join(dir, "dangling")
	os.WriteFile(p2, []byte("[profile a]\nsso_session = nope\n"), 0o600)
	s, _ = Load(Files{Config: p2})
	if s.ReadOnly() || len(s.Issues(nil)) != 1 {
		t.Fatalf("dangling reference: %+v", s.Issues(nil))
	}
}

func TestGoldenMigrateAcme(t *testing.T) {
	cfg := readFixture(t, "config.fixture")
	out, err := MigrateProfile(cfg, "acme", nil, "initech", "111122223333", "AdministratorAccess", "")
	if err != nil {
		t.Fatal(err)
	}
	old := "[profile acme]\nregion = us-east-1\noutput = json\n"
	neu := "[profile acme]\nsso_session    = initech\nsso_account_id = 111122223333\nsso_role_name  = AdministratorAccess\nregion = us-east-1\noutput = json\n"
	want := strings.Replace(string(cfg), old, neu, 1)
	if string(out) != want {
		t.Fatalf("got:\n%s", out)
	}
}

func TestGoldenMigrateWithNewSession(t *testing.T) {
	cfg := readFixture(t, "config.fixture")
	ses := &Session{Name: "acme-sso", StartURL: "https://d-0000000000.awsapps.com/start", Region: "us-east-1", Scopes: []string{"sso:account:access"}, ScopesSet: true}
	out, err := MigrateProfile(cfg, "acme", ses, "acme-sso", "111122223333", "AdministratorAccess", "")
	if err != nil {
		t.Fatal(err)
	}
	base, _ := MigrateProfile(cfg, "acme", nil, "acme-sso", "111122223333", "AdministratorAccess", "")
	add := "\n[sso-session acme-sso]\nsso_start_url           = https://d-0000000000.awsapps.com/start\nsso_region              = us-east-1\nsso_registration_scopes = sso:account:access\n"
	if string(out) != string(base)+add {
		t.Fatalf("got:\n%s", out)
	}
}

func TestMigrateRegionChange(t *testing.T) {
	cfg := readFixture(t, "config.fixture")
	out, _ := MigrateProfile(cfg, "globex", nil, "initech", "444455556666", "ReadOnlyAccess", "us-east-1")
	if !strings.Contains(string(out), "[profile globex]\nsso_session    = initech\nsso_account_id = 444455556666\nsso_role_name  = ReadOnlyAccess\nregion = us-east-1\n") {
		t.Fatalf("got:\n%s", out)
	}
}

func TestGoldenAddProfiles(t *testing.T) {
	cfg := readFixture(t, "config.fixture")
	out, err := AddProfiles(cfg, nil, []NewProfile{{Name: "initech-prod", Session: "initech", AccountID: "777788889999", Role: "ReadOnlyAccess", Region: "us-west-2"}})
	if err != nil {
		t.Fatal(err)
	}
	add := "\n[profile initech-prod]\nsso_session    = initech\nsso_account_id = 777788889999\nsso_role_name  = ReadOnlyAccess\nregion         = us-west-2\noutput         = json\n"
	if string(out) != string(cfg)+add {
		t.Fatalf("got:\n%s", out)
	}
	ses := &Session{Name: "n", StartURL: "https://d-1.awsapps.com/start", Region: "us-east-1", Scopes: []string{"sso:account:access"}, ScopesSet: true}
	out, _ = AddProfiles(cfg, ses, []NewProfile{{Name: "a", Session: "n", AccountID: "1", Role: "R", Region: "us-east-1"}})
	i, j := strings.Index(string(out), "[sso-session n]"), strings.Index(string(out), "[profile a]")
	if i < 0 || j < i {
		t.Fatal("session must precede the new profiles")
	}
	if _, err := AddProfiles(cfg, nil, []NewProfile{{Name: "acme"}}); err == nil || api.AsAppError(err).Code != api.ErrProfileExists {
		t.Fatalf("duplicate profile: %v", err)
	}
}

func TestRemoveStaticKeysExact(t *testing.T) {
	cred := readFixture(t, "credentials.fixture")
	out, changed, err := RemoveStaticKeys(cred, "acme")
	if err != nil || !changed {
		t.Fatal(err, changed)
	}
	want := "[globex]\naws_access_key_id = AKIAI44QH8DHBEXAMPLE\naws_secret_access_key = fakeSecretValueDoNotUse1111111111111111\n"
	if string(out) != want {
		t.Fatalf("got %q", out)
	}
	if _, changed, _ := RemoveStaticKeys(cred, "nobody"); changed {
		t.Fatal("unknown section must not change")
	}
}

func TestRemoveStaticKeysKeepsCommentOnlyFile(t *testing.T) {
	in := []byte("# my keys\n[acme]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\naws_secret_access_key = fakeSecretValueDoNotUse0000000000000000\n")
	out, changed, err := RemoveStaticKeys(in, "acme")
	if err != nil || !changed || string(out) != "# my keys\n" {
		t.Fatalf("%q %v %v", out, changed, err)
	}
}

func TestKeysInConfigSection(t *testing.T) {
	cfg := []byte("[profile x]\nregion = us-east-1\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\naws_secret_access_key = fakeSecretValueDoNotUse0000000000000000\n")
	dir := t.TempDir()
	p := filepath.Join(dir, "config")
	os.WriteFile(p, cfg, 0o600)
	s, _ := Load(Files{Config: p})
	x, _ := s.Profile("x")
	if x.Kind != api.ProfileKindStatic || x.StaticKeysIn != "config" {
		t.Fatalf("%+v", x)
	}
	if _, ok := s.StaticKey("x"); !ok {
		t.Fatal("key in config section must be found")
	}
	out, changed, _ := RemoveStaticKeys(cfg, "profile x")
	if !changed || string(out) != "[profile x]\nregion = us-east-1\n" {
		t.Fatalf("%q", out)
	}
}

func TestMaskForPreview(t *testing.T) {
	out := string(MaskForPreview(readFixture(t, "credentials.fixture")))
	if strings.Contains(out, "fakeSecret") || strings.Contains(out, "AKIAIOSFODNN7EXAMPLE") {
		t.Fatal(out)
	}
	if !strings.Contains(out, "aws_access_key_id = AKIA••••••••••••MPLE") || !strings.Contains(out, "aws_secret_access_key = ••••••••") {
		t.Fatal(out)
	}
}

func TestPreviewNeverContainsSecrets(t *testing.T) {
	before := readFixture(t, "credentials.fixture")
	after, _, _ := RemoveStaticKeys(before, "acme")
	pv := Preview("/x/credentials", "~/.aws/credentials", "~/Backups", before, after)
	if pv.BaseHash != Hash(before) || pv.Removed != 4 || pv.Added != 0 {
		t.Fatalf("%+v", pv)
	}
	for _, l := range pv.Lines {
		if strings.Contains(l.Text, "fakeSecret") || strings.Contains(l.Text, "AKIAIOSF") {
			t.Fatalf("secret in preview line %q", l.Text)
		}
	}
	empty := Preview("p", "d", "b", nil, nil)
	if empty.Lines == nil || empty.BaseHash != "" {
		t.Fatal("empty preview")
	}
}

func TestNormalizeStartURL(t *testing.T) {
	cases := []struct {
		in, want, region string
		code             api.ErrorCode
	}{
		{"https://d-1234567890.awsapps.com/start#/", "https://d-1234567890.awsapps.com/start", "", ""},
		{"  https://d-1234567890.awsapps.com/start# ", "https://d-1234567890.awsapps.com/start", "", ""},
		{"https://d-1234567890.awsapps.com", "https://d-1234567890.awsapps.com/start", "", ""},
		{"https://ssoins-7223a1b2c3d4e5f6.portal.us-east-1.app.aws", "https://ssoins-7223a1b2c3d4e5f6.portal.us-east-1.app.aws", "us-east-1", ""},
		{"https://identitycenter.amazonaws.com/ssoins-1", "https://identitycenter.amazonaws.com/ssoins-1", "", ""},
		{"http://d-1.awsapps.com/start", "", "", api.ErrInvalidStartURL},
		{"https://sso.example.com", "", "", api.ErrInvalidStartURL},
		{"https://awsapps.com/start", "", "", api.ErrInvalidStartURL},
		{"d-1.awsapps.com", "", "", api.ErrInvalidStartURL},
	}
	for _, c := range cases {
		got, reg, err := NormalizeStartURL(c.in)
		if c.code != "" {
			if err == nil || err.Code != c.code {
				t.Errorf("%q: want %s got %v", c.in, c.code, err)
			}
			continue
		}
		if err != nil || got != c.want || reg != c.region {
			t.Errorf("%q: got %q %q %v", c.in, got, reg, err)
		}
	}
}

func TestSuggestNames(t *testing.T) {
	if got := SuggestSessionName("https://d-1234567890.awsapps.com/start"); got != "d-1234567890" {
		t.Fatal(got)
	}
	if got := SuggestSessionName("https://ssoins-7223a1b2c3d4e5f6.portal.us-east-1.app.aws"); got != "ssoins-7223a1b2c3d4e5f6" {
		t.Fatal(got)
	}
	taken := map[string]bool{}
	if got := SuggestProfileName("acme", "AdministratorAccess", taken); got != "acme-admin" {
		t.Fatal(got)
	}
	taken["acme-admin"] = true
	if got := SuggestProfileName("acme", "AdministratorAccess", taken); got != "acme-admin-2" {
		t.Fatal(got)
	}
	if got := SuggestProfileName("initech platform (Main)", "ReadOnlyAccess", taken); got != "initech-platform-main-readonly" {
		t.Fatal(got)
	}
	if got := SuggestProfileName("", "PowerUserAccess", taken); got != "profile-poweruser" {
		t.Fatal(got)
	}
}

func TestProfileNameRE(t *testing.T) {
	for _, bad := range []string{"a'b", "a b", "", strings.Repeat("a", 65), "a/b", "a\"b", "a]b"} {
		if ProfileNameRE.MatchString(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
	for _, good := range []string{"acme", "initech-admin", "a.b_c@d+e=f,g"} {
		if !ProfileNameRE.MatchString(good) {
			t.Errorf("%q rejected", good)
		}
	}
	for _, r := range []string{"us-east-1", "us-gov-west-1", "us-iso-east-1", "eu-central-1"} {
		if !RegionRE.MatchString(r) {
			t.Errorf("region %q rejected", r)
		}
	}
	if RegionRE.MatchString("mars") {
		t.Error("mars accepted")
	}
}

func TestRemoveProfileAndSessionOps(t *testing.T) {
	cfg := readFixture(t, "config.fixture")
	out, err := RemoveProfile(cfg, "globex")
	if err != nil || strings.Contains(string(out), "globex") {
		t.Fatal(err)
	}
	if _, err := RemoveProfile(cfg, "zzz"); err == nil || api.AsAppError(err).Code != api.ErrProfileNotFound {
		t.Fatal(err)
	}
	out, err = RenameSession(cfg, "initech", "wl")
	if err != nil || strings.Count(string(out), "sso_session    = wl") != 2 || !strings.Contains(string(out), "[sso-session wl]") {
		t.Fatalf("%v\n%s", err, out)
	}
	out, err = RemoveSession(cfg, "initech")
	if err != nil || strings.Contains(string(out), "[sso-session initech]") {
		t.Fatal(err)
	}
}

func wantUnsafe(t *testing.T, err error, field string) {
	t.Helper()
	ae := api.AsAppError(err)
	if err == nil || ae == nil || ae.Code != api.ErrUnsafeValue {
		t.Fatalf("want E_UNSAFE_VALUE, got %v", err)
	}
	if field != "" && ae.Params["value"] != field {
		t.Fatalf("param value %q, want %q", ae.Params["value"], field)
	}
	if strings.Contains(err.Error(), "credential_process") || strings.Contains(ae.Params["value"], "\n") {
		t.Fatalf("the error must name the field, not echo the input: %v", err)
	}
}

const injectedRole = "AdministratorAccess\ncredential_process = /usr/bin/id"

func TestWritesRefuseControlCharacters(t *testing.T) {
	cfg := readFixture(t, "config.fixture")
	sess := &Session{Name: "globex", StartURL: "https://globex.awsapps.com/start", Region: "us-east-1", Scopes: []string{"sso:account:access"}, ScopesSet: true}

	t.Run("AddProfiles role", func(t *testing.T) {
		out, err := AddProfiles(cfg, nil, []NewProfile{{Name: "x", Session: "initech", AccountID: "111122223333", Role: injectedRole, Region: "us-east-1"}})
		wantUnsafe(t, err, "sso_role_name")
		if out != nil {
			t.Fatal("no bytes may be returned")
		}
	})
	t.Run("AddProfiles start url", func(t *testing.T) {
		bad := *sess
		bad.StartURL = "https://acme.awsapps.com/start\n[profile evil]\ncredential_process = /usr/bin/id"
		_, err := AddProfiles(cfg, &bad, []NewProfile{{Name: "x", Session: "globex", AccountID: "111122223333", Role: "AdministratorAccess", Region: "us-east-1"}})
		wantUnsafe(t, err, "sso_start_url")
	})
	t.Run("AddProfiles profile name", func(t *testing.T) {
		_, err := AddProfiles(cfg, nil, []NewProfile{{Name: "x]\n[profile y", Session: "initech", AccountID: "111122223333", Role: "AdministratorAccess", Region: "us-east-1"}})
		wantUnsafe(t, err, "section")
	})
	t.Run("MigrateProfile role", func(t *testing.T) {
		_, err := MigrateProfile(cfg, "acme", nil, "initech", "111122223333", injectedRole, "")
		wantUnsafe(t, err, "sso_role_name")
	})
	t.Run("MigrateProfile new profile region", func(t *testing.T) {
		_, err := MigrateProfile(cfg, "brand-new", nil, "initech", "111122223333", "AdministratorAccess", "us-east-1\nx = y")
		wantUnsafe(t, err, "region")
	})
	t.Run("MigrateProfile new session", func(t *testing.T) {
		bad := *sess
		bad.Region = "us-east-1\r\ncredential_process = /usr/bin/id"
		_, err := MigrateProfile(cfg, "acme", &bad, "globex", "111122223333", "AdministratorAccess", "")
		wantUnsafe(t, err, "sso_region")
	})
	t.Run("RenameSession", func(t *testing.T) {
		_, err := RenameSession(cfg, "initech", "x]\n[profile y")
		wantUnsafe(t, err, "name")
	})
	t.Run("RenameSession bracket only", func(t *testing.T) {
		_, err := RenameSession(cfg, "initech", "x]y")
		wantUnsafe(t, err, "section")
	})
	t.Run("RemoveProfile", func(t *testing.T) {
		_, err := RemoveProfile(cfg, "a\nb")
		wantUnsafe(t, err, "name")
	})
	t.Run("RemoveSession", func(t *testing.T) {
		_, err := RemoveSession(cfg, "a b")
		wantUnsafe(t, err, "name")
	})
	t.Run("RemoveStaticKeys", func(t *testing.T) {
		_, _, err := RemoveStaticKeys(cfg, "acme\n[x]")
		wantUnsafe(t, err, "section")
	})
}

func TestWritesAcceptOrdinaryValues(t *testing.T) {
	cfg := readFixture(t, "config.fixture")
	out, err := AddProfiles(cfg, nil, []NewProfile{{Name: "globex-admin", Session: "initech", AccountID: "444455556666", Role: "acme.Admin+ops@x", Region: "us-east-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "= acme.Admin+ops@x") {
		t.Fatalf("%s", out)
	}
}

func TestNormalizeStartURLRejectsTrickyHosts(t *testing.T) {
	for _, raw := range []string{
		`https://evil.example\.awsapps.com/start`,
		`https://evil.example%5c.awsapps.com/start`,
		`https://acme.awsapps.com/start?x=1`,
		`https://acme.awsapps.com/st art`,
		`https://u@acme.awsapps.com/start`,
		`https://u:p@acme.awsapps.com/start`,
		`https://acme.awsapps.com:8443/start`,
		`https://acme.awsapps.com/start#frag`,
		`https://acme.awsapps.com/start/%2e%2e/x`,
		`https://acme.awsapps.com/../start`,
		`https://acme.awsapps.com/start\n[profile evil]`,
		"https://acme.awsapps.com/start\n[profile evil]\ncredential_process = /usr/bin/id",
		"https://acme.awsapps.com/start\x00",
		`https://.awsapps.com/start`,
		`https://-acme.awsapps.com/start`,
		`https://acme..awsapps.com/start`,
		`https://evil.example`,
		`http://acme.awsapps.com/start`,
		`https://acme.awsapps.com.evil.example/start`,
		`https://awsapps.com/start`,
	} {
		if _, _, err := NormalizeStartURL(raw); err == nil || err.Code != api.ErrInvalidStartURL {
			t.Errorf("%q must be rejected, got %v", raw, err)
		}
	}
}

func TestNormalizeStartURLAcceptsPortals(t *testing.T) {
	cases := map[string]string{
		"https://acme.awsapps.com/start/#/":                     "https://acme.awsapps.com/start",
		"https://ACME.awsapps.com":                              "https://acme.awsapps.com/start",
		"https://acme.awsapps.com/start#":                       "https://acme.awsapps.com/start",
		"https://ssoins-1111.portal.us-east-1.app.aws":          "https://ssoins-1111.portal.us-east-1.app.aws",
		"https://ssoins-1111.portal.us-east-1.app.aws/":         "https://ssoins-1111.portal.us-east-1.app.aws",
		"https://identitycenter.amazonaws.com/ssoins-1111":      "https://identitycenter.amazonaws.com/ssoins-1111",
		"https://identitycenter.amazonaws.com/ssoins-1111/":     "https://identitycenter.amazonaws.com/ssoins-1111",
		"https://d-1234567890.awsapps.com/start":                "https://d-1234567890.awsapps.com/start",
		"  https://globex.awsapps.com/directory/d-1234567890  ": "https://globex.awsapps.com/directory/d-1234567890",
	}
	for raw, want := range cases {
		got, _, err := NormalizeStartURL(raw)
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", raw, got, err, want)
		}
	}
	if _, region, err := NormalizeStartURL("https://ssoins-1111.portal.eu-west-1.app.aws"); err != nil || region != "eu-west-1" {
		t.Fatalf("region guess: %q %v", region, err)
	}
}

func TestCredentialsKeySections(t *testing.T) {
	dir := t.TempDir()
	f := Files{Config: filepath.Join(dir, "config"), Credentials: filepath.Join(dir, "credentials")}
	creds := "[acme]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\naws_secret_access_key = fakeSecret\n\n[profile globex]\naws_access_key_id = AKIAI44QH8DHBEXAMPLE\naws_secret_access_key = fakeSecret2\n"
	if err := os.WriteFile(f.Credentials, []byte(creds), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(f)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.CredentialsKeySections("acme"); len(got) != 1 || got[0] != "acme" {
		t.Fatalf("%v", got)
	}
	if got := s.CredentialsKeySections("globex"); len(got) != 1 || got[0] != "profile globex" {
		t.Fatalf("%v", got)
	}
	if got := s.CredentialsKeySections("initech"); len(got) != 0 {
		t.Fatalf("%v", got)
	}
}
