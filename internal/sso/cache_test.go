package sso

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/rudian/aws_sso_hub/internal/api"
)

// requireCLIPython returns the python bundled with an AWS CLI install, taken from AWS_CLI_PYTHON, or skips the test.
// Set it to the python inside the AWS CLI installation, e.g. AWS_CLI_PYTHON=<install dir>/libexec/bin/python.
func requireCLIPython(t *testing.T) string {
	t.Helper()
	p := os.Getenv("AWS_CLI_PYTHON")
	if p == "" {
		t.Skip("AWS_CLI_PYTHON is not set; point it at the python bundled with the AWS CLI to run the botocore compatibility tests")
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("AWS_CLI_PYTHON=%q is not usable: %v", p, err)
	}
	return p
}

func TestKeys(t *testing.T) {
	if got := TokenCacheKey("initech", "https://d-1234567890.awsapps.com/start"); got != "278165aca35d4262a51ebb5800d9feefa5e24a78" {
		t.Fatal(got)
	}
	if got := TokenCacheKey("", "https://d-x.awsapps.com/start"); got == TokenCacheKey("x", "") {
		t.Fatal("legacy key must hash the start URL")
	}
	if got := RegistrationCacheKey("https://d-1234567890.awsapps.com/start", "us-west-2", []string{"sso:account:access"}, "initech"); got != "6d7d8c382750cc6de7ea0592b5910fb371ff185f" {
		t.Fatal(got)
	}
	if got := RegistrationCacheKey("https://d-1234567890.awsapps.com/start", "us-west-2", nil, "initech"); got != "efb9407125073ca5de7fed738aac998468fc9a60" {
		t.Fatal(got)
	}
}

func TestRoleCacheKey(t *testing.T) {
	if got := RoleCacheKey("777788889999", "AdministratorAccess", "initech", ""); got != "fc20f8c0fc97e889384bb1e9f0299f28491ab46a" {
		t.Fatal(got)
	}
	if got := RoleCacheKey("111111111111", "ReadOnlyAccess", "fake-portal", ""); got != "52437bae7b93ccbe150cfa53b736e6dcdf91262c" {
		t.Fatal(got)
	}
	if RoleCacheKey("1", "R", "", "https://x/start") == RoleCacheKey("1", "R", "s", "") {
		t.Fatal("legacy and session keys must differ")
	}
}

func TestTokenExactFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "cache")
	tok := goodToken(2 * time.Hour)
	if err := WriteToken(dir, "k", tok); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(filepath.Join(dir, "k.json"))
	di, _ := os.Stat(dir)
	if fi.Mode().Perm() != 0o600 || di.Mode().Perm() != 0o700 {
		t.Fatal(fi.Mode(), di.Mode())
	}
	b, _ := os.ReadFile(filepath.Join(dir, "k.json"))
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"startUrl", "region", "accessToken", "expiresAt", "clientId", "clientSecret", "registrationExpiresAt", "refreshToken"} {
		if m[k] == "" {
			t.Errorf("missing %s", k)
		}
	}
	if len(m) != 8 {
		t.Errorf("%d keys", len(m))
	}
	for _, k := range []string{"expiresAt", "registrationExpiresAt"} {
		if len(m[k]) != 20 || !strings.HasSuffix(m[k], "Z") {
			t.Errorf("%s format %q", k, m[k])
		}
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, "*.tmp-*"))
	if len(leftovers) != 0 {
		t.Fatal("temp files left")
	}
	got, err := ReadToken(dir, "k")
	if err != nil || got.AccessToken != tok.AccessToken || !got.ExpiresAt.Equal(tok.ExpiresAt) {
		t.Fatal(err)
	}
}

func TestUnknownKeysPreserved(t *testing.T) {
	dir := t.TempDir()
	raw := `{"startUrl":"u","region":"r","accessToken":"a","expiresAt":"2030-01-01T00:00:00Z","mystery":{"x":1},"other":"y"}`
	os.WriteFile(filepath.Join(dir, "k.json"), []byte(raw), 0o600)
	tok, err := ReadToken(dir, "k")
	if err != nil {
		t.Fatal(err)
	}
	WriteToken(dir, "k", tok)
	b, _ := os.ReadFile(filepath.Join(dir, "k.json"))
	var m map[string]any
	json.Unmarshal(b, &m)
	if m["other"] != "y" || m["mystery"] == nil {
		t.Fatalf("unknown keys dropped: %s", b)
	}
	if _, ok := m["clientId"]; ok {
		t.Fatal("clientId must not be invented")
	}
}

func TestRefreshTokenOmittedWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	tok := goodToken(time.Hour)
	tok.RefreshToken = ""
	tok.Extra = map[string]json.RawMessage{"refreshToken": json.RawMessage(`"stale"`)}
	WriteToken(dir, "k", tok)
	b, _ := os.ReadFile(filepath.Join(dir, "k.json"))
	if strings.Contains(string(b), "refreshToken") {
		t.Fatalf("refreshToken must be omitted: %s", b)
	}
}

func TestClassifyToken(t *testing.T) {
	now := time.Now()
	mk := func(left time.Duration, mut func(*CachedToken)) *CachedToken {
		tok := goodToken(0)
		tok.ExpiresAt = now.Add(left)
		tok.RegistrationExpiresAt = now.Add(24 * time.Hour)
		if mut != nil {
			mut(tok)
		}
		return tok
	}
	cases := []struct {
		name string
		tok  *CachedToken
		want TokenState
	}{
		{"valid", mk(time.Hour, nil), TokenValid},
		{"boundary 15m", mk(15*time.Minute+time.Second, nil), TokenValid},
		{"low refreshable", mk(10*time.Minute, nil), TokenRefreshable},
		{"expired refreshable", mk(-time.Minute, nil), TokenRefreshable},
		{"low no refresh token", mk(10*time.Minute, func(c *CachedToken) { c.RefreshToken = "" }), TokenNeedsLogin},
		{"registration expired", mk(-time.Minute, func(c *CachedToken) { c.RegistrationExpiresAt = now.Add(-time.Hour) }), TokenNeedsLogin},
		{"no client", mk(-time.Minute, func(c *CachedToken) { c.ClientID = "" }), TokenNeedsLogin},
	}
	for _, c := range cases {
		if got := ClassifyToken(c.tok, now); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

type fakeCreate struct{ calls int }

func (f *fakeCreate) CreateToken(ctx context.Context, in *ssooidc.CreateTokenInput, _ ...func(*ssooidc.Options)) (*ssooidc.CreateTokenOutput, error) {
	f.calls++
	return &ssooidc.CreateTokenOutput{AccessToken: aws.String("NEW-AT"), ExpiresIn: 3600, RefreshToken: aws.String("NEW-RT")}, nil
}

func TestSDKReadsAndRefreshesOurFile(t *testing.T) {
	dir := t.TempDir()
	key := TokenCacheKey("s1", "")
	tok := goodToken(-time.Minute)
	if err := WriteToken(dir, key, tok); err != nil {
		t.Fatal(err)
	}
	fc := &fakeCreate{}
	p := ssocreds.NewSSOTokenProvider(fc, filepath.Join(dir, key+".json"))
	bt, err := p.RetrieveBearerToken(context.Background())
	if err != nil || bt.Value != "NEW-AT" || fc.calls != 1 {
		t.Fatal(err, fc.calls)
	}
	b, _ := os.ReadFile(filepath.Join(dir, key+".json"))
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"startUrl", "region", "registrationExpiresAt", "clientId", "clientSecret", "refreshToken", "expiresAt", "accessToken"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("SDK refresh dropped %s", k)
		}
	}
	got, err := ReadToken(dir, key)
	if err != nil || got.AccessToken != "NEW-AT" || got.StartURL != tok.StartURL {
		t.Fatal(err)
	}
	if st := ClassifyToken(got, time.Now()); st != TokenValid {
		t.Fatalf("our classifier on the SDK-written file: %v", st)
	}
}

func TestBotocoreReadsOurFile(t *testing.T) {
	cliPython := requireCLIPython(t)
	dir := t.TempDir()
	key := TokenCacheKey("s1", "")
	if err := WriteToken(dir, key, goodToken(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	script := `
import sys, awscli
from botocore.utils import SSOTokenLoader, JSONFileCache
import dateutil.parser
c = JSONFileCache(sys.argv[1])
t = SSOTokenLoader(cache=c)("https://d-1234567890.awsapps.com/start", session_name="s1")
print("loaded", sorted(t.keys()), dateutil.parser.parse(t["expiresAt"]).isoformat())
`
	out, err := exec.Command(cliPython, "-I", "-c", script, dir).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !strings.Contains(string(out), "loaded") {
		t.Fatalf("%s", out)
	}
}

func TestAuthorizeURLGolden(t *testing.T) {
	got := AuthorizeURL("https://oidc.us-east-1.amazonaws.com", "CLIENT-ID_x", "http://127.0.0.1:54321/oauth/callback",
		"8c3e5f0e-1111-4222-8333-444455556666", "CHALLENGE", []string{"sso:account:access"})
	want := "https://oidc.us-east-1.amazonaws.com/authorize?response_type=code&client_id=CLIENT-ID_x" +
		"&redirect_uri=http%3A%2F%2F127.0.0.1%3A54321%2Foauth%2Fcallback&state=8c3e5f0e-1111-4222-8333-444455556666" +
		"&code_challenge_method=S256&scopes=sso%3Aaccount%3Aaccess&code_challenge=CHALLENGE"
	if got != want {
		t.Fatalf("\n%s\n%s", got, want)
	}
	multi := AuthorizeURL("b", "c", "r", "s", "x", []string{"a:b", "c"})
	if !strings.Contains(multi, "&scopes=a%3Ab%20c&") {
		t.Fatal(multi)
	}
	if !strings.Contains(AuthorizeURL("b", "c", "r", "s", "x", nil), "scopes=sso%3Aaccount%3Aaccess") {
		t.Fatal("default scope")
	}
}

func TestAuthorizeURLMatchesBotocore(t *testing.T) {
	cliPython := requireCLIPython(t)
	got := AuthorizeURL("https://oidc.us-east-1.amazonaws.com", "CLIENT-ID_x", "http://127.0.0.1:54321/oauth/callback",
		"8c3e5f0e-1111-4222-8333-444455556666", "CHALLENGE", []string{"sso:account:access"})
	script := `
import awscli, uuid
from botocore.utils import SSOTokenFetcherAuth
class F:
    def redirect_uri_with_port(self): return "http://127.0.0.1:54321/oauth/callback"
f = SSOTokenFetcherAuth.__new__(SSOTokenFetcherAuth)
f._auth_code_fetcher = F()
f.code_challenge = "CHALLENGE="
f._get_base_authorization_uri = lambda: "https://oidc.us-east-1.amazonaws.com"
print(f._get_authorization_uri("CLIENT-ID_x", ["sso:account:access"], uuid.UUID("8c3e5f0e-1111-4222-8333-444455556666")), end="")
`
	out, err := exec.Command(cliPython, "-I", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if string(out) != got {
		t.Fatalf("\nbotocore=%s\ngo      =%s", out, got)
	}
}

func TestPKCEShape(t *testing.T) {
	v, c, err := NewPKCE()
	if err != nil || len(v) != 64 || len(c) != 43 || strings.ContainsAny(c, "=+/") {
		t.Fatal(len(v), len(c))
	}
	for _, r := range v {
		if !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~", r) {
			t.Fatalf("bad verifier char %q", r)
		}
	}
	s := NewState()
	if len(s) != 36 || s[14] != '4' {
		t.Fatal(s)
	}
}

func TestStatusClientIDHash(t *testing.T) {
	e := newEnv(t)
	tok := goodToken(time.Hour)
	e.writeToken(t, tok)
	st := e.m.Status(e.sess)
	if len(st.ClientIDHash) != 16 || st.ClientIDHash != ClientIDHash("CID-old") {
		t.Fatal(st.ClientIDHash)
	}
	if strings.Contains(st.ClientIDHash, "CID") {
		t.Fatal("hash must not contain the client id")
	}
	tok.ClientID, tok.ClientSecret = "", ""
	e.writeToken(t, tok)
	if h := e.m.Status(e.sess).ClientIDHash; h != "" {
		t.Fatalf("no clientId must give empty hash, got %q", h)
	}
	_ = api.SessionActive
}

func TestShortcutURL(t *testing.T) {
	got := ShortcutURL("https://d-1234567890.awsapps.com/start", "777788889999", "Administrator Access", "https://console.aws.amazon.com/iam/home#/users")
	want := "https://d-1234567890.awsapps.com/start/#/console?account_id=777788889999&role_name=Administrator+Access&destination=https%3A%2F%2Fconsole.aws.amazon.com%2Fiam%2Fhome%23%2Fusers"
	if got != want {
		t.Fatalf("\n%s\n%s", got, want)
	}
	if got := ShortcutURL("https://d-1.awsapps.com/start/", "1", "R", ""); got != "https://d-1.awsapps.com/start/#/console?account_id=1&role_name=R" {
		t.Fatal(got)
	}
}

func TestWriteTokenLeavesNoTempFilesAndTightensDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := WriteToken(dir, "k", goodToken(2*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 || ents[0].Name() != "k.json" {
		for _, e := range ents {
			t.Log(e.Name())
		}
		t.Fatalf("only the token file may remain, got %d entries", len(ents))
	}
	for _, e := range ents {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("temp file left: %s", e.Name())
		}
	}
	if di, _ := os.Stat(dir); di.Mode().Perm() != 0o700 {
		t.Fatalf("a cache directory that others could write to must become 0700, got %v", di.Mode().Perm())
	}
	if fi, _ := os.Stat(filepath.Join(dir, "k.json")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("token file mode %v", fi.Mode().Perm())
	}
}

func TestWriteTokenTightensLooseExistingFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "k.json")
	if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chmod(p, 0o644)
	if err := WriteToken(dir, "k", goodToken(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Fatalf("%v", fi.Mode().Perm())
	}
	if _, err := ReadToken(dir, "k"); err != nil {
		t.Fatal(err)
	}
}
