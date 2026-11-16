package accounts

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/sso"
)

func signInTokenOf(t *testing.T, link string) string {
	t.Helper()
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("SigninToken")
}

func TestConsoleSignInURL(t *testing.T) {
	h := newHarness(t, true)
	_, err := h.eng.ConsoleSignInURL(bg, "initech-admin")
	if ae := wantErr(t, err, api.ErrNotSignedIn); ae.Params["portal"] != "initech" {
		t.Fatalf("%+v", ae)
	}

	h.login()
	got, err := h.eng.ConsoleSignInURL(bg, "initech-admin")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(got)
	q := u.Query()
	if u.Scheme+"://"+u.Host != h.aws.URL || u.Path != "/federation" || q.Get("Action") != "login" || q.Get("SigninToken") != "SIGNIN-ASIAIOSFODNN7EXAMPLE" ||
		q.Get("Issuer") != portalInitech.StartURL || q.Get("Destination") != "https://console.aws.amazon.com/console/home?region=us-west-2" {
		t.Fatalf("%q", got)
	}
	// the role credentials come from the portal with this app's own access token
	calls := h.aws.SignIns()
	if len(calls) != 1 || calls[0].SessionID != "ASIAIOSFODNN7EXAMPLE" || calls[0].SessionKey != "roleSecretNotReal" || calls[0].SessionToken != "roleSessionTokNotReal" {
		t.Fatalf("federation calls: %+v", calls)
	}
	tok, terr := sso.ReadToken(sso.TokenCacheDir(h.p.Home), sso.TokenCacheKey("initech", ""))
	if terr != nil || h.por.CredCalls() != 1 || h.por.LastToken() != tok.AccessToken {
		t.Fatalf("role credentials: %d calls, %v", h.por.CredCalls(), terr)
	}
	// each role signs in with its own credentials
	h.por.SetKeyID("ASIAREADONLY0EXAMPLE")
	got, err = h.eng.ConsoleSignInURL(bg, "initech-readonly")
	if err != nil || signInTokenOf(t, got) != "SIGNIN-ASIAREADONLY0EXAMPLE" {
		t.Fatalf("readonly: %v", err)
	}

	_, err = h.eng.ConsoleSignInURL(bg, "acme")
	wantErr(t, err, api.ErrInvalidInput)
	_, err = h.eng.ConsoleSignInURL(bg, "zzz")
	wantErr(t, err, api.ErrProfileNotFound)

	h.aws.FailSignIn(400)
	got, err = h.eng.ConsoleSignInURL(bg, "initech-admin")
	if ae := wantErr(t, err, api.ErrAWSAPI); ae.Detail != "getSigninToken: HTTP 400" || got != "" {
		t.Fatalf("%+v %q", ae, got)
	}
}

// Partitions without a known federation endpoint (here an ISO region) keep the access portal link.
func TestConsoleSignInURLFallsBackToThePortalLink(t *testing.T) {
	h := newHarness(t, true)
	fixture := string(readTestdata(t, "config.fixture"))
	cfg := strings.Replace(fixture, "sso_region     = us-west-2", "sso_region     = us-iso-east-1", 1)
	if cfg == fixture {
		t.Fatal("the fixture has no sso_region to replace")
	}
	if err := os.WriteFile(h.p.AWSConfigFile, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	h.writeToken(time.Hour, "CID-1")
	got, err := h.eng.ConsoleSignInURL(bg, "initech-admin")
	if err != nil || got != "https://d-1234567890.awsapps.com/start/#/console?account_id=777788889999&role_name=AdministratorAccess" {
		t.Fatalf("%q %v", got, err)
	}
	if n := len(h.aws.SignIns()); n != 0 {
		t.Fatalf("%d federation calls", n)
	}
	if n := h.por.CredCalls(); n != 0 {
		t.Fatalf("no role credentials are fetched for a portal link: %d", n)
	}
	// it needs no AWS call, so it works signed out too
	os.Remove(filepath.Join(sso.TokenCacheDir(h.p.Home), sso.TokenCacheKey("initech", "")+".json"))
	if got, err := h.eng.ConsoleSignInURL(bg, "initech-admin"); err != nil || !strings.Contains(got, "/#/console?") {
		t.Fatalf("%q %v", got, err)
	}
}
