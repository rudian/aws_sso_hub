package sso

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
)

func TestStatusStates(t *testing.T) {
	e := newEnv(t)
	legacy := Session{Name: "", StartURL: "https://d-legacy.awsapps.com/start", Region: "us-east-1"}

	if st := e.m.Status(e.sess); st.State != api.SessionSignedOut {
		t.Fatalf("no file: %v", st.State)
	}
	os.MkdirAll(TokenCacheDir(e.home), 0o700)
	os.WriteFile(filepath.Join(TokenCacheDir(e.home), TokenCacheKey("initech", "")+".json"), []byte("not json"), 0o600)
	if st := e.m.Status(e.sess); st.State != api.SessionError || st.Err == nil || st.Err.Code != api.ErrFileIO {
		t.Fatalf("invalid file: %+v", st)
	}
	cases := []struct {
		name string
		mut  func(*CachedToken)
		left time.Duration
		want api.SessionState
		can  bool
	}{
		{"active", nil, time.Hour, api.SessionActive, true},
		{"exactly 15 min", nil, 15*time.Minute + 2*time.Second, api.SessionActive, true},
		{"expiring", nil, 10 * time.Minute, api.SessionExpiring, true},
		{"expired refreshable", nil, -time.Minute, api.SessionRefreshable, true},
		{"expired no refresh token", func(c *CachedToken) { c.RefreshToken = "" }, -time.Minute, api.SessionExpired, false},
		{"expired registration over", func(c *CachedToken) { c.RegistrationExpiresAt = time.Now().Add(-time.Hour) }, -time.Minute, api.SessionExpired, false},
	}
	for _, c := range cases {
		tok := goodToken(c.left)
		if c.mut != nil {
			c.mut(tok)
		}
		e.writeToken(t, tok)
		st := e.m.Status(e.sess)
		if st.State != c.want || st.CanRefresh != c.can {
			t.Errorf("%s: state=%v can=%v", c.name, st.State, st.CanRefresh)
		}
		if !st.ExpiresAt.Equal(tok.ExpiresAt) {
			t.Errorf("%s: expiresAt", c.name)
		}
	}
	// legacy profiles never refresh
	tok := goodToken(-time.Minute)
	WriteToken(TokenCacheDir(e.home), TokenCacheKey("", legacy.StartURL), tok)
	if st := e.m.Status(legacy); st.State != api.SessionExpired || st.CanRefresh {
		t.Fatalf("legacy: %+v", st)
	}
	// failed refresh recorded for this expiresAt -> expired
	e.writeToken(t, goodToken(-time.Minute))
	e.oidc.queueToken(fakeResp{400, "InvalidGrantException", ""})
	if _, err := e.m.Refresh(context.Background(), e.sess); !errors.Is(err, ErrInvalidGrant) {
		t.Fatal(err)
	}
	if st := e.m.Status(e.sess); st.State != api.SessionExpired || st.CanRefresh {
		t.Fatalf("after failed refresh: %+v", st)
	}
}

func TestTokenRefreshesWhenLow(t *testing.T) {
	e := newEnv(t)
	e.writeToken(t, goodToken(3*time.Minute))
	tok, err := e.m.Token(context.Background(), e.sess)
	if err != nil || tok.AccessToken != "AT-1" {
		t.Fatal(err)
	}
	b := e.oidc.tokenCalls()
	if len(b) != 1 || b[0]["grantType"] != "refresh_token" || b[0]["refreshToken"] != "RT-old" || b[0]["clientId"] != "CID-old" || b[0]["clientSecret"] != "CS-old" {
		t.Fatalf("refresh body %v", b)
	}
	got := e.readToken(t)
	// clientId/secret/registration preserved, new access/refresh token written, startUrl kept
	if got.AccessToken != "AT-1" || got.RefreshToken != "RT-1" || got.ClientID != "CID-old" || got.ClientSecret != "CS-old" || got.StartURL != e.sess.StartURL {
		t.Fatalf("%+v", got)
	}
	if got.ExpiresAt.Before(time.Now().Add(50 * time.Minute)) {
		t.Fatal("expiresAt not moved")
	}
	// plenty of time left: no network
	if _, err := e.m.Token(context.Background(), e.sess); err != nil || len(e.oidc.tokenCalls()) != 1 {
		t.Fatal("second call must not refresh")
	}
	// refreshToken dropped when the response has none: covered by the format test; here the errors
	e2 := newEnv(t)
	if _, err := e2.m.Token(context.Background(), e2.sess); !errors.Is(err, ErrNotSignedIn) {
		t.Fatal(err)
	}
	tok2 := goodToken(-time.Minute)
	tok2.RefreshToken = ""
	e2.writeToken(t, tok2)
	if _, err := e2.m.Token(context.Background(), e2.sess); !errors.Is(err, ErrSessionExpired) {
		t.Fatal(err)
	}
}

func TestRefreshRereadsFileFirst(t *testing.T) {
	e := newEnv(t)
	stale := goodToken(2 * time.Minute)
	e.writeToken(t, stale)
	// another writer (aws sso login / the CLI) renews the file after the caller looked at it
	renewed := goodToken(time.Hour)
	renewed.AccessToken = "AT-from-cli"
	e.writeToken(t, renewed)
	tok, err := e.m.refresh(context.Background(), e.sess, stale)
	if err != nil || tok.AccessToken != "AT-from-cli" {
		t.Fatal(err)
	}
	if n := len(e.oidc.tokenCalls()); n != 0 {
		t.Fatalf("CreateToken called %d times", n)
	}
	// and via the public API when the file is already good: Token() never refreshes
	if tok, err := e.m.Token(context.Background(), e.sess); err != nil || tok.AccessToken != "AT-from-cli" || len(e.oidc.tokenCalls()) != 0 {
		t.Fatal("Token must reuse the renewed file")
	}
}

func TestRefreshInvalidGrantMarksExpired(t *testing.T) {
	e := newEnv(t)
	e.writeToken(t, goodToken(-time.Minute))
	e.oidc.queueToken(fakeResp{400, "InvalidGrantException", ""})
	_, err := e.m.Refresh(context.Background(), e.sess)
	if !errors.Is(err, ErrInvalidGrant) || ToAppError(err, "initech").Code != api.ErrSessionExpired {
		t.Fatal(err)
	}
	if !e.m.FailedRefresh(e.sess) {
		t.Fatal("failure must be remembered for this expiresAt")
	}
	// no second network attempt for the same token
	if _, err := e.m.Refresh(context.Background(), e.sess); !errors.Is(err, ErrInvalidGrant) || len(e.oidc.tokenCalls()) != 1 {
		t.Fatalf("calls=%d err=%v", len(e.oidc.tokenCalls()), err)
	}
	// a re-read that finds a renewed file wins over the rejection (the CLI renewed it while our request was in flight)
	e2 := newEnv(t)
	stale := goodToken(-time.Minute)
	e2.writeToken(t, stale)
	e2.oidc.queueToken(fakeResp{400, "InvalidGrantException", ""})
	renewed := goodToken(time.Hour)
	renewed.AccessToken = "AT-race"
	e2.oidc.onToken = func() { e2.writeToken(t, renewed) }
	// the caller looked at a file that is identical to what is on disk now, so only the post-failure re-read can save it
	tok, err := e2.m.refresh(context.Background(), e2.sess, stale)
	if err != nil || tok.AccessToken != "AT-race" {
		t.Fatalf("%v", err)
	}
	if e2.m.FailedRefresh(e2.sess) {
		t.Fatal("no failure may be recorded")
	}
}

func TestRefreshNetworkErrorKeepsState(t *testing.T) {
	e := newEnv(t)
	e.writeToken(t, goodToken(-time.Minute))
	e.oidc.srv.Close()
	_, err := e.m.Refresh(context.Background(), e.sess)
	if err == nil {
		t.Fatal("expected error")
	}
	st := e.m.Status(e.sess)
	if st.State != api.SessionRefreshable || st.Err == nil || st.Err.Code != api.ErrNetwork {
		t.Fatalf("%+v", st)
	}
	if ToAppError(err, "p").Code != api.ErrNetwork {
		t.Fatalf("%v", ToAppError(err, "p"))
	}
}

func TestLogoutDeletesRoleCache(t *testing.T) {
	e := newEnv(t)
	e.writeToken(t, goodToken(time.Hour))
	cache := CLICacheDir(e.home)
	os.MkdirAll(cache, 0o700)
	put := func(key, provider string) string {
		p := filepath.Join(cache, key+".json")
		os.WriteFile(p, []byte(`{"ProviderType":"`+provider+`","Credentials":{"AccessKeyId":"ASIAx"}}`), 0o600)
		return p
	}
	mine := put(RoleCacheKey("777788889999", "AdministratorAccess", "initech", ""), "sso")
	assume := put(RoleCacheKey("777788889999", "ReadOnlyAccess", "initech", ""), "assume-role")
	other := put(RoleCacheKey("111122223333", "ReadOnlyAccess", "other-portal", ""), "sso")
	n, server, err := e.m.Logout(context.Background(), e.sess, []AccountRole{
		{"777788889999", "AdministratorAccess"}, {"777788889999", "ReadOnlyAccess"}, {"777788889999", "NoCacheFile"}})
	if err != nil || n != 1 || !server {
		t.Fatalf("n=%d server=%v err=%v", n, server, err)
	}
	if _, err := os.Stat(mine); err == nil {
		t.Fatal("sso role file must be deleted")
	}
	for _, p := range []string{assume, other} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s must stay", p)
		}
	}
	if _, err := os.Stat(filepath.Join(TokenCacheDir(e.home), TokenCacheKey("initech", "")+".json")); err == nil {
		t.Fatal("token file must be deleted")
	}
	if e.por.logoutCalls != 1 || e.por.lastToken != "AT-old" {
		t.Fatalf("logout calls=%d token=%q", e.por.logoutCalls, e.por.lastToken)
	}
	if st := e.m.Status(e.sess); st.State != api.SessionSignedOut {
		t.Fatal(st.State)
	}
	// a second logout is harmless
	if n, server, err := e.m.Logout(context.Background(), e.sess, nil); err != nil || n != 0 || server {
		t.Fatal(n, server, err)
	}
}

func TestLogoutNeverSweepsOtherTokenFiles(t *testing.T) {
	e := newEnv(t)
	e.writeToken(t, goodToken(time.Hour))
	foreign := filepath.Join(TokenCacheDir(e.home), "kiro-auth-token.json")
	os.WriteFile(foreign, []byte(`{"accessToken":"x","region":"us-east-1"}`), 0o600)
	e.m.Logout(context.Background(), e.sess, nil)
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal("foreign token file deleted")
	}
}

func TestListAccountsPaginates(t *testing.T) {
	e := newEnv(t)
	e.writeToken(t, goodToken(time.Hour))
	e.por.accounts = [][]map[string]string{
		{{"accountId": "111122223333", "accountName": "acme", "emailAddress": "a@x@example.com"}},
		{{"accountId": "777788889999", "accountName": "initech", "emailAddress": "b@x@example.com"}},
	}
	e.por.roles["777788889999"] = []string{"AdministratorAccess", "ReadOnlyAccess"}
	accts, err := e.m.ListAccounts(context.Background(), e.sess)
	if err != nil || len(accts) != 2 || accts[1].ID != "777788889999" || accts[0].Name != "acme" || accts[0].Email != "a@x@example.com" {
		t.Fatalf("%v %v", accts, err)
	}
	if e.por.lastToken != "AT-old" {
		t.Fatal("bearer header")
	}
	roles, err := e.m.ListAccountRoles(context.Background(), e.sess, "777788889999")
	if err != nil || strings.Join(roles, ",") != "AdministratorAccess,ReadOnlyAccess" {
		t.Fatalf("%v %v", roles, err)
	}
	e.por.unauthorized = true
	if _, err := e.m.ListAccounts(context.Background(), e.sess); !errors.Is(err, ErrSessionExpired) {
		t.Fatal(err)
	}
	e3 := newEnv(t)
	if _, err := e3.m.ListAccounts(context.Background(), e3.sess); !errors.Is(err, ErrNotSignedIn) || ToAppError(err, "initech").Code != api.ErrNotSignedIn {
		t.Fatal(err)
	}
}

func TestRoleCredentialsMillis(t *testing.T) {
	e := newEnv(t)
	e.writeToken(t, goodToken(time.Hour))
	want := time.Now().Add(time.Hour).Truncate(time.Millisecond)
	e.por.expireMillis = want.UnixMilli()
	c, err := e.m.RoleCredentials(context.Background(), e.sess, "111122223333", "AdministratorAccess")
	if err != nil {
		t.Fatal(err)
	}
	if c.AccessKeyID != "ASIAIOSFODNN7EXAMPLE" || c.SessionToken != "roleSessionTok" || !c.CanExpire || !c.Expires.Equal(want) || c.AccountID != "111122223333" {
		t.Fatalf("%+v", c)
	}
}

func TestRoleCredentialsCached(t *testing.T) {
	e := newEnv(t)
	e.writeToken(t, goodToken(time.Hour))
	for i := 0; i < 3; i++ {
		if _, err := e.m.RoleCredentials(context.Background(), e.sess, "111122223333", "AdministratorAccess"); err != nil {
			t.Fatal(err)
		}
	}
	if e.por.credCalls != 1 {
		t.Fatalf("%d calls", e.por.credCalls)
	}
	if _, err := e.m.RoleCredentials(context.Background(), e.sess, "111122223333", "Other"); err != nil || e.por.credCalls != 2 {
		t.Fatal("a different role must fetch")
	}
	// credentials expiring within 5 minutes are refetched
	e.por.expireMillis = time.Now().Add(4 * time.Minute).UnixMilli()
	e.m.RoleCredentials(context.Background(), e.sess, "111122223333", "Short")
	e.m.RoleCredentials(context.Background(), e.sess, "111122223333", "Short")
	if e.por.credCalls != 4 {
		t.Fatalf("%d calls", e.por.credCalls)
	}
	// never written to ~/.aws/cli/cache
	if _, err := os.Stat(CLICacheDir(e.home)); err == nil {
		t.Fatal("role credentials must not be persisted")
	}
	// Logout clears the memory cache
	e.m.Logout(context.Background(), e.sess, nil)
	e.writeToken(t, goodToken(time.Hour))
	e.por.expireMillis = time.Now().Add(time.Hour).UnixMilli()
	e.m.RoleCredentials(context.Background(), e.sess, "111122223333", "AdministratorAccess")
	if e.por.credCalls != 5 {
		t.Fatalf("%d calls after logout", e.por.credCalls)
	}
}

func TestToAppErrorMapping(t *testing.T) {
	cases := []struct {
		err  error
		code api.ErrorCode
	}{
		{ErrInProgress, api.ErrLoginInProgress}, {ErrDenied, api.ErrLoginDenied}, {ErrTimeout, api.ErrLoginTimeout},
		{ErrDeviceExpired, api.ErrLoginTimeout}, {ErrPort, api.ErrLoginPort}, {ErrState, api.ErrLoginState},
		{&AuthError{Value: "x"}, api.ErrLoginFailed}, {context.Canceled, api.ErrCancelled}, {ErrNotSignedIn, api.ErrNotSignedIn},
		{ErrSessionExpired, api.ErrSessionExpired}, {ErrInvalidGrant, api.ErrSessionExpired}, {ErrRoleNotAssigned, api.ErrRoleNotAssigned},
		{errors.New("boom"), api.ErrInternal},
	}
	for _, c := range cases {
		if got := ToAppError(c.err, "p"); got.Code != c.code {
			t.Errorf("%v -> %s want %s", c.err, got.Code, c.code)
		}
	}
	if ToAppError(nil, "p") != nil {
		t.Fatal("nil")
	}
}
