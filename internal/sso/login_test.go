package sso

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
)

// browser simulates the user's browser: it parses the authorize URL and calls the callback.
func (e *env) browser(params func(redirect, state string) string) func(string) error {
	return func(u string) error {
		e.mu.Lock()
		e.opened = append(e.opened, u)
		e.mu.Unlock()
		pu, err := url.Parse(u)
		if err != nil {
			return err
		}
		q := pu.Query()
		go func() {
			time.Sleep(30 * time.Millisecond)
			resp, err := http.Get(q.Get("redirect_uri") + params(q.Get("redirect_uri"), q.Get("state")))
			if err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}()
		return nil
	}
}

func okCallback(_, state string) string { return "?code=THECODE&state=" + url.QueryEscape(state) }

func TestLoginPKCEWritesCLIToken(t *testing.T) {
	e := newEnv(t)
	e.m.cfg.OpenBrowser = e.browser(okCallback)
	tok, err := e.m.Login(context.Background(), e.sess, api.LoginModeBrowser, e.progress)
	if err != nil {
		t.Fatal(err)
	}
	got := e.readToken(t)
	if got.AccessToken != tok.AccessToken || got.AccessToken != "AT-1" || got.RefreshToken != "RT-1" || got.StartURL != e.sess.StartURL || got.Region != "us-west-2" {
		t.Fatalf("token file %+v", got)
	}
	if got.ClientID != "CID-1" || got.RegistrationExpiresAt.Before(time.Now().Add(24*time.Hour)) {
		t.Fatalf("registration data missing: %+v", got)
	}
	fi, _ := os.Stat(filepath.Join(TokenCacheDir(e.home), TokenCacheKey("initech", "")+".json"))
	if fi.Mode().Perm() != 0o600 {
		t.Fatal(fi.Mode())
	}
	reg := e.oidc.regBodies[0]
	uris, _ := reg["redirectUris"].([]any)
	grants, _ := reg["grantTypes"].([]any)
	if len(uris) != 1 || uris[0] != "http://127.0.0.1/oauth/callback" || len(grants) != 2 || reg["issuerUrl"] != e.sess.StartURL ||
		reg["clientName"] != "codepass-initech" || reg["clientType"] != "public" {
		t.Fatalf("register body %v", reg)
	}
	ct := e.oidc.tokBodies[0]
	if ct["grantType"] != "authorization_code" || ct["code"] != "THECODE" || ct["codeVerifier"] == "" || !strings.HasPrefix(ct["redirectUri"].(string), "http://127.0.0.1:") ||
		!strings.HasSuffix(ct["redirectUri"].(string), "/oauth/callback") || ct["redirectUri"] == "http://127.0.0.1/oauth/callback" {
		t.Fatalf("token body %v", ct)
	}
	if !strings.HasPrefix(e.opened[0], e.oidc.srv.URL+"/authorize?response_type=code&client_id=CID-1&redirect_uri=") {
		t.Fatalf("authorize url %s", e.opened[0])
	}
	ph := e.phases()
	if len(ph) != 3 || ph[0] != api.LoginPhaseStarting || ph[1] != api.LoginPhaseWaiting || ph[2] != api.LoginPhaseExchanging {
		t.Fatalf("phases %v", ph)
	}
	if !e.prog[1].BrowserOpened || e.prog[1].AuthorizeURL != e.opened[0] || e.prog[1].ExpiresAt == "" {
		t.Fatalf("waiting progress %+v", e.prog[1])
	}
	if e.m.LoggingIn(e.sess) {
		t.Fatal("login flag must be cleared")
	}
	// the CLI's own registration location was never used
	if _, err := os.Stat(TokenCacheDir(e.home) + "/" + RegistrationCacheKey(e.sess.StartURL, e.sess.Region, e.sess.Scopes, "initech") + ".json"); err == nil {
		t.Fatal("must not write the CLI registration file")
	}
}

func TestLoginPKCEDenied(t *testing.T) {
	e := newEnv(t)
	e.m.cfg.OpenBrowser = e.browser(func(_, state string) string { return "?error=access_denied&state=" + url.QueryEscape(state) })
	_, err := e.m.Login(context.Background(), e.sess, api.LoginModeBrowser, nil)
	if !errors.Is(err, ErrDenied) || ToAppError(err, "p").Code != api.ErrLoginDenied {
		t.Fatal(err)
	}
	e2 := newEnv(t)
	e2.m.cfg.OpenBrowser = e2.browser(func(_, state string) string { return "?error=server_error&state=" + url.QueryEscape(state) })
	_, err = e2.m.Login(context.Background(), e2.sess, api.LoginModeBrowser, nil)
	var ae *AuthError
	if !errors.As(err, &ae) || ae.Value != "server_error" {
		t.Fatal(err)
	}
	if a := ToAppError(err, "p"); a.Code != api.ErrLoginFailed || a.Detail != "server_error" {
		t.Fatalf("%+v", a)
	}
}

// Requests that do not carry this login's state, or that are addressed to another host name, are ignored: they
// cannot complete the sign-in, abort it, or force a new client registration.
func TestForgedCallbacksAreIgnored(t *testing.T) {
	e := newEnv(t)
	var statuses []int
	var mu sync.Mutex
	e.m.cfg.OpenBrowser = func(u string) error {
		e.mu.Lock()
		e.opened = append(e.opened, u)
		e.mu.Unlock()
		pu, _ := url.Parse(u)
		q := pu.Query()
		ru, _ := url.Parse(q.Get("redirect_uri"))
		go func() {
			time.Sleep(30 * time.Millisecond)
			base := "http://" + ru.Host + ru.Path
			get := func(rawURL, host string) int {
				req, _ := http.NewRequest(http.MethodGet, rawURL, nil)
				if host != "" {
					req.Host = host
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return -1
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				return resp.StatusCode
			}
			forged := []int{
				get(base+"?error=access_denied", ""),
				get(base+"?error=invalid_client&state=wrong", ""),
				get(base+"?code=x&state=guess", ""),
				get(base+"?code=x", ""),
				get(base+"?code=EVIL&state="+url.QueryEscape(q.Get("state")), "evil.example"),
				get(base+"?error=access_denied&state="+url.QueryEscape(q.Get("state")), "localhost:"+ru.Port()),
			}
			mu.Lock()
			statuses = forged
			mu.Unlock()
			get(base+"?code=THECODE&state="+url.QueryEscape(q.Get("state")), "")
		}()
		return nil
	}
	tok, err := e.m.Login(context.Background(), e.sess, api.LoginModeBrowser, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "AT-1" {
		t.Errorf("token %+v", tok)
	}
	mu.Lock()
	defer mu.Unlock()
	for i, s := range statuses {
		if s != http.StatusBadRequest {
			t.Errorf("forged request %d answered %d, want 400", i, s)
		}
	}
	if len(statuses) != 6 {
		t.Errorf("forged requests sent: %v", statuses)
	}
	if e.oidc.regCount != 1 || len(e.oidc.tokBodies) != 1 || e.oidc.tokBodies[0]["code"] != "THECODE" {
		t.Errorf("registrations=%d token requests=%v", e.oidc.regCount, e.oidc.tokBodies)
	}
	if len(e.opened) != 1 {
		t.Errorf("the browser was opened %d times", len(e.opened))
	}
}

func TestLoginRejectsInvalidRegionBeforeAnyRequest(t *testing.T) {
	for _, mode := range []api.LoginMode{api.LoginModeBrowser, api.LoginModeDevice} {
		for _, region := range []string{"evil.example#", "", "us-east-1.evil.example", "US-EAST-1"} {
			e := newEnv(t)
			opened := 0
			e.m.cfg.OpenBrowser = func(string) error { opened++; return nil }
			e.m.cfg.Endpoints.OIDC = "" // the default endpoint resolver would otherwise be bypassed
			s := e.sess
			s.Region = region
			_, err := e.m.Login(context.Background(), s, mode, nil)
			if ae, ok := err.(*api.AppError); !ok || ae.Code != api.ErrInvalidRegion {
				t.Errorf("%s %q: %v", mode, region, err)
			}
			if opened != 0 || e.oidc.regCount != 0 {
				t.Errorf("%s %q: browser opens=%d registrations=%d", mode, region, opened, e.oidc.regCount)
			}
		}
	}
	e := newEnv(t)
	s := e.sess
	s.Region = "a.b"
	if _, err := e.m.authorizeBase(context.Background(), s); err == nil {
		t.Error("authorizeBase must validate the region")
	}
}

func TestLoginPKCETimeout(t *testing.T) {
	e := newEnv(t)
	e.m.cfg.OpenBrowser = func(string) error { return nil }
	e.m.cfg.CallbackTimeout = 200 * time.Millisecond
	start := time.Now()
	_, err := e.m.Login(context.Background(), e.sess, api.LoginModeBrowser, nil)
	if !errors.Is(err, ErrTimeout) || time.Since(start) > 3*time.Second {
		t.Fatal(err, time.Since(start))
	}
	if ToAppError(err, "p").Code != api.ErrLoginTimeout {
		t.Fatal("mapping")
	}
}

func TestLoginPKCECancel(t *testing.T) {
	e := newEnv(t)
	var addr string
	var mu sync.Mutex
	e.m.cfg.OpenBrowser = func(u string) error {
		pu, _ := url.Parse(u)
		ru, _ := url.Parse(pu.Query().Get("redirect_uri"))
		mu.Lock()
		addr = ru.Host
		mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	_, err := e.m.Login(ctx, e.sess, api.LoginModeBrowser, nil)
	if !errors.Is(err, context.Canceled) || ToAppError(err, "p").Code != api.ErrCancelled {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if addr == "" {
		t.Fatal("browser never opened")
	}
	if c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond); err == nil {
		c.Close()
		t.Fatal("callback port still accepting connections")
	}
	if e.m.LoggingIn(e.sess) {
		t.Fatal("flag")
	}
}

func TestCallbackOnlyPath(t *testing.T) {
	e := newEnv(t)
	got := make(chan [3]int, 1)
	e.m.cfg.OpenBrowser = func(u string) error {
		pu, _ := url.Parse(u)
		ru, _ := url.Parse(pu.Query().Get("redirect_uri"))
		go func() {
			base := "http://" + ru.Host
			r1, _ := http.Get(base + "/x")
			r2, _ := http.Post(base+"/oauth/callback", "text/plain", strings.NewReader("a"))
			r3, _ := http.Get(base + "/oauth/callback?code=C&state=" + url.QueryEscape(pu.Query().Get("state")))
			b, _ := io.ReadAll(r3.Body)
			if !strings.Contains(string(b), "signed in") || r3.Header.Get("Content-Type") != "text/html; charset=utf-8" {
				got <- [3]int{-1, -1, -1}
				return
			}
			got <- [3]int{r1.StatusCode, r2.StatusCode, r3.StatusCode}
		}()
		return nil
	}
	if _, err := e.m.Login(context.Background(), e.sess, api.LoginModeBrowser, nil); err != nil {
		t.Fatal(err)
	}
	if r := <-got; r != [3]int{404, 405, 200} {
		t.Fatalf("%v", r)
	}
}

func TestRegistrationReused(t *testing.T) {
	e := newEnv(t)
	e.m.cfg.OpenBrowser = e.browser(okCallback)
	for i := 0; i < 2; i++ {
		if _, err := e.m.Login(context.Background(), e.sess, api.LoginModeBrowser, nil); err != nil {
			t.Fatal(err)
		}
	}
	if e.oidc.regCount != 1 {
		t.Fatalf("registered %d times", e.oidc.regCount)
	}
	if got := e.readToken(t); got.ClientID != "CID-1" || got.AccessToken != "AT-2" {
		t.Fatalf("%+v", got)
	}
	files, _ := filepath.Glob(filepath.Join(e.m.cfg.RegistrationsDir, "*.json"))
	if len(files) != 1 {
		t.Fatal(files)
	}
	if fi, _ := os.Stat(files[0]); fi.Mode().Perm() != 0o600 {
		t.Fatal(fi.Mode())
	}
}

func TestInvalidClientReRegistersOnce(t *testing.T) {
	e := newEnv(t)
	e.m.cfg.OpenBrowser = e.browser(okCallback)
	e.oidc.queueToken(fakeResp{400, "InvalidClientException", ""})
	if _, err := e.m.Login(context.Background(), e.sess, api.LoginModeBrowser, nil); err != nil {
		t.Fatal(err)
	}
	if e.oidc.regCount != 2 || len(e.opened) != 2 {
		t.Fatalf("regs=%d browser opens=%d", e.oidc.regCount, len(e.opened))
	}
	if got := e.readToken(t); got.ClientID != "CID-2" {
		t.Fatalf("%+v", got)
	}
	// a second consecutive failure is not retried again
	e2 := newEnv(t)
	e2.m.cfg.OpenBrowser = e2.browser(okCallback)
	e2.oidc.queueToken(fakeResp{400, "InvalidClientException", ""}, fakeResp{400, "InvalidClientException", ""})
	_, err := e2.m.Login(context.Background(), e2.sess, api.LoginModeBrowser, nil)
	if err == nil || e2.oidc.regCount != 2 {
		t.Fatalf("err=%v regs=%d", err, e2.oidc.regCount)
	}
}

func TestLoginDevicePollsAndSlowsDown(t *testing.T) {
	e := newEnv(t)
	var sleeps []time.Duration
	e.m.cfg.Sleep = func(_ context.Context, d time.Duration) error { sleeps = append(sleeps, d); return nil }
	e.oidc.queueToken(pending(), pending(), slowDown(), okResp())
	tok, err := e.m.Login(context.Background(), e.sess, api.LoginModeDevice, e.progress)
	if err != nil {
		t.Fatal(err)
	}
	if len(sleeps) != 3 || sleeps[0] != time.Second || sleeps[1] != time.Second || sleeps[2] != 6*time.Second {
		t.Fatalf("sleeps %v", sleeps)
	}
	if tok.AccessToken != "AT-4" || e.readToken(t).AccessToken != "AT-4" {
		t.Fatal("token")
	}
	reg := e.oidc.regBodies[0]
	if _, has := reg["grantTypes"]; has || reg["scopes"] == nil {
		t.Fatalf("device registration body %v", reg)
	}
	ph := e.phases()
	if len(ph) != 2 || ph[1] != api.LoginPhaseDevice {
		t.Fatalf("%v", ph)
	}
	p := e.prog[1]
	if p.UserCode != "ABCD-EFGH" || p.VerificationURI != "https://device.example/" || !strings.Contains(p.VerificationURIComplete, "user_code") || p.ExpiresAt == "" {
		t.Fatalf("%+v", p)
	}
	if len(e.opened) != 0 {
		t.Fatal("browser must not be opened automatically")
	}
	if e.oidc.tokBodies[0]["grantType"] != "urn:ietf:params:oauth:grant-type:device_code" || e.oidc.tokBodies[0]["deviceCode"] != "DC" {
		t.Fatal(e.oidc.tokBodies[0])
	}
}

func TestLoginDeviceExpired(t *testing.T) {
	e := newEnv(t)
	e.m.cfg.Sleep = func(context.Context, time.Duration) error { return nil }
	e.oidc.queueToken(pending(), fakeResp{400, "ExpiredTokenException", ""})
	_, err := e.m.Login(context.Background(), e.sess, api.LoginModeDevice, nil)
	if !errors.Is(err, ErrDeviceExpired) || ToAppError(err, "p").Code != api.ErrLoginTimeout {
		t.Fatal(err)
	}
	// local deadline
	e2 := newEnv(t)
	base := time.Now()
	calls := 0
	e2.m.cfg.Now = func() time.Time { calls++; return base.Add(time.Duration(calls) * 5 * time.Minute) }
	e2.m.cfg.Sleep = func(context.Context, time.Duration) error { return nil }
	e2.oidc.queueToken(pending(), pending(), pending(), pending(), pending())
	if _, err := e2.m.Login(context.Background(), e2.sess, api.LoginModeDevice, nil); !errors.Is(err, ErrDeviceExpired) {
		t.Fatal(err)
	}
}

func TestLoginInProgress(t *testing.T) {
	e := newEnv(t)
	release := make(chan struct{})
	started := make(chan struct{})
	e.m.cfg.OpenBrowser = func(string) error { close(started); return nil }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := e.m.Login(ctx, e.sess, api.LoginModeBrowser, nil)
		close(release)
		done <- err
	}()
	<-started
	if !e.m.LoggingIn(e.sess) || e.m.Status(e.sess).State != api.SessionSigningIn {
		t.Fatal("state must be signing-in")
	}
	_, err := e.m.Login(context.Background(), e.sess, api.LoginModeBrowser, nil)
	if !errors.Is(err, ErrInProgress) || ToAppError(err, "initech").Code != api.ErrLoginInProgress || ToAppError(err, "initech").Params["portal"] != "initech" {
		t.Fatal(err)
	}
	cancel()
	<-release
	<-done
}

func TestCallbackPageEscapes(t *testing.T) {
	p := string(CallbackPage("en", false, `<script>alert("x")</script>`))
	if strings.Contains(p, "<script>") || !strings.Contains(p, "&lt;script&gt;") {
		t.Fatal(p)
	}
	for _, c := range []struct {
		lang string
		ok   bool
		want string
	}{{"zh-CN", true, "登录成功"}, {"zh-CN", false, "登录没有完成"}, {"en", true, "You're signed in"}, {"en", false, "Sign-in didn't finish"}} {
		page := string(CallbackPage(c.lang, c.ok, "access_denied"))
		if !strings.Contains(page, c.want) || strings.Contains(page, "http://") || strings.Contains(page, "https://") || !strings.Contains(page, `<meta charset="utf-8">`) {
			t.Errorf("%s ok=%v: missing %q or external URL", c.lang, c.ok, c.want)
		}
	}
	if !strings.Contains(string(CallbackPage("zh-CN", false, "access_denied")), "浏览器返回了：access_denied。回到 Codepass 重试即可。") {
		t.Fatal("zh error body")
	}
}

func TestLoginPKCEReportsPrivateWindow(t *testing.T) {
	for _, tc := range []struct {
		name        string
		private     bool
		err         error
		wantOpened  bool
		wantPrivate bool
	}{
		{"private", true, nil, true, true},
		{"normal", false, nil, true, false},
		{"failed", true, errors.New("no browser"), false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			plain := false
			e.m.cfg.OpenBrowser = func(string) error { plain = true; return nil }
			approve := e.browser(okCallback)
			e.m.cfg.OpenSignIn = func(u string) (bool, error) {
				if tc.err != nil {
					// the user opens the printed URL by hand
					approve(u)
					return false, tc.err
				}
				return tc.private, approve(u)
			}
			if _, err := e.m.Login(context.Background(), e.sess, api.LoginModeBrowser, e.progress); err != nil {
				t.Fatal(err)
			}
			if plain {
				t.Fatal("OpenBrowser must not be used when OpenSignIn is set")
			}
			w := e.prog[1]
			if w.Phase != api.LoginPhaseWaiting || w.BrowserOpened != tc.wantOpened || w.PrivateWindow != tc.wantPrivate {
				t.Fatalf("waiting progress %+v", w)
			}
		})
	}
}
