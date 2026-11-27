package sso

import (
	"context"
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/rudian/aws_sso_hub/internal/api"
)

var (
	introURLRE = regexp.MustCompile(`^http://127\.0\.0\.1:[0-9]+/start/[0-9a-f]{32}$`)
	buttonRE   = regexp.MustCompile(`class="btn" href="([^"]+)"`)
)

// introVisit is what the simulated browser saw: the intro page, then the done page.
type introVisit struct {
	url    string
	status int
	header http.Header
	body   string
	done   string
	err    error
}

func get(u string) (*http.Response, string, error) {
	resp, err := http.Get(u)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp, string(b), err
}

// introBrowser simulates the browser of a sign-in with an intro page: it loads the intro page, then follows its
// button to AWS, which sends it back with a code. follow=false only loads the intro page.
func (e *env) introBrowser(seen chan<- introVisit, follow bool) func(string) (api.SignInWindow, error) {
	return func(u string) (api.SignInWindow, error) {
		e.mu.Lock()
		e.opened = append(e.opened, u)
		e.mu.Unlock()
		go func() {
			v := introVisit{url: u}
			defer func() { seen <- v }()
			resp, body, err := get(u)
			if err != nil {
				v.err = err
				return
			}
			v.status, v.header, v.body = resp.StatusCode, resp.Header, body
			m := buttonRE.FindStringSubmatch(body)
			if !follow || m == nil {
				return
			}
			pu, err := url.Parse(html.UnescapeString(m[1]))
			if err != nil {
				v.err = err
				return
			}
			q := pu.Query()
			_, v.done, v.err = get(q.Get("redirect_uri") + "?code=THECODE&state=" + url.QueryEscape(q.Get("state")))
		}()
		return api.SignInWindow{Private: true, Isolated: true}, nil
	}
}

// waitFor polls cond for up to 5 s.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatal("condition not met in time")
}

func testIntro() *SignInIntro {
	return &SignInIntro{Portal: `ac<me>`, EndsAt: time.Now().Add(9 * time.Minute),
		Profiles: []IntroProfile{{Name: `dev<x>`, Monogram: "DX", Slot: 2}, {Name: "prod", Monogram: "PR", Slot: 9}}}
}

func TestLoginWithIntroOpensTheIntroPageFirst(t *testing.T) {
	e := newEnv(t)
	seen := make(chan introVisit, 1)
	e.m.cfg.OpenIntro = e.introBrowser(seen, true)
	e.m.cfg.OpenSignIn = func(string) (api.SignInWindow, error) {
		t.Error("a sign-in with an intro page must not open AWS's page itself")
		return api.SignInWindow{}, nil
	}
	var ended []string
	e.m.cfg.SignInEnded = func(pages []string, ok bool) {
		if ok {
			ended = pages
		}
	}
	tok, err := e.m.LoginWith(context.Background(), e.sess, api.LoginModeBrowser, LoginOptions{Portal: "acme", Intro: testIntro()}, e.progress)
	if err != nil || tok.AccessToken != "AT-1" {
		t.Fatalf("%v %+v", err, tok)
	}
	v := <-seen
	if v.err != nil || v.status != http.StatusOK || !introURLRE.MatchString(v.url) {
		t.Fatalf("intro %q: %d %v", v.url, v.status, v.err)
	}
	for k, want := range map[string]string{
		"Cache-Control": "no-store", "Referrer-Policy": "no-referrer", "X-Frame-Options": "DENY", "X-Content-Type-Options": "nosniff",
		"Content-Type": "text/html; charset=utf-8",
	} {
		if got := v.header.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if csp := v.header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP %q", csp)
	}
	for _, want := range []string{"Sign in again: ac&lt;me&gt;", "dev&lt;x&gt;", `class="tile s2"`, `<span class="tile">PR</span>prod`, "Continue to AWS sign-in"} {
		if !strings.Contains(v.body, want) {
			t.Errorf("intro page lacks %q", want)
		}
	}
	if strings.Contains(v.body, "<script") || strings.Contains(v.body, "ac<me>") {
		t.Error("intro page must escape names and carry no script")
	}
	var authURL string
	for _, p := range e.prog {
		if p.AuthorizeURL != "" {
			authURL = p.AuthorizeURL
		}
	}
	if m := buttonRE.FindStringSubmatch(v.body); m == nil || html.UnescapeString(m[1]) != authURL {
		t.Fatalf("the button must continue to the authorize URL %q", authURL)
	}
	if !strings.Contains(v.done, "Signed in to acme") {
		t.Fatalf("done page: %s", v.done)
	}
	if len(ended) != 2 || ended[0] != v.url || ended[1] != authURL {
		t.Fatalf("SignInEnded pages %v (intro %s)", ended, v.url)
	}
}

func TestIntroPageAnswersOnlyItsOwnAddress(t *testing.T) {
	e := newEnv(t)
	seen := make(chan introVisit, 1)
	e.m.cfg.OpenIntro = e.introBrowser(seen, false)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := e.m.LoginWith(ctx, e.sess, api.LoginModeBrowser, LoginOptions{Intro: testIntro()}, e.progress)
		done <- err
	}()
	v := <-seen
	if v.status != http.StatusOK {
		t.Fatalf("intro %d %v", v.status, v.err)
	}
	u, _ := url.Parse(v.url)

	req, _ := http.NewRequest(http.MethodGet, v.url, nil)
	req.Host = "attacker.example:" + u.Port()
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("another Host: %v %v", resp, err)
	} else {
		resp.Body.Close()
	}
	if resp, err := http.Post(v.url, "text/plain", strings.NewReader("x")); err != nil || resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %v %v", resp, err)
	} else {
		resp.Body.Close()
	}
	other := "http://" + u.Host + IntroPathPrefix + strings.Repeat("0", 32)
	if resp, _, err := get(other); err != nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("another intro path: %v %v", resp, err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestIntroNeedsOpenIntro(t *testing.T) {
	e := newEnv(t)
	e.m.cfg.OpenBrowser = e.browser(okCallback)
	if _, err := e.m.LoginWith(context.Background(), e.sess, api.LoginModeBrowser, LoginOptions{Intro: testIntro()}, e.progress); err != nil {
		t.Fatal(err)
	}
	if len(e.opened) != 1 || !strings.HasPrefix(e.opened[0], e.oidc.srv.URL+"/authorize?") {
		t.Fatalf("without OpenIntro the sign-in opens AWS's page: %v", e.opened)
	}
}

func TestBackgroundSignInKeepsTheCurrentState(t *testing.T) {
	e := newEnv(t)
	e.writeToken(t, goodToken(30*time.Minute))
	e.m.cfg.OpenBrowser = func(string) error { return nil } // the user never comes back
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := e.m.LoginWith(ctx, e.sess, api.LoginModeBrowser, LoginOptions{Background: true}, e.progress)
		done <- err
	}()
	waitFor(t, func() bool { return e.m.Status(e.sess).Waiting })
	if st := e.m.Status(e.sess); st.State != api.SessionActive || !st.Waiting {
		t.Fatalf("while a background sign-in waits: %+v", st)
	}
	if _, err := e.m.Login(context.Background(), e.sess, api.LoginModeBrowser, nil); !errors.Is(err, ErrInProgress) {
		t.Fatalf("a second sign-in still waits its turn: %v", err)
	}
	cancel()
	<-done
	if st := e.m.Status(e.sess); st.Waiting || st.State != api.SessionActive {
		t.Fatalf("after it ended: %+v", st)
	}
}

func TestLoginWaitOverridesTheCallbackTimeout(t *testing.T) {
	e := newEnv(t)
	e.m.cfg.OpenBrowser = func(string) error { return nil }
	start := time.Now()
	_, err := e.m.LoginWith(context.Background(), e.sess, api.LoginModeBrowser, LoginOptions{Wait: 300 * time.Millisecond}, e.progress)
	if !errors.Is(err, ErrTimeout) || time.Since(start) > 5*time.Second {
		t.Fatalf("%v after %v", err, time.Since(start))
	}
}

func TestCallbackPageNamesThePortal(t *testing.T) {
	for _, c := range []struct {
		lang string
		ok   bool
		want string
	}{{"zh-CN", true, "已登录 a&lt;b"}, {"zh-CN", false, "a&lt;b 登录没有完成"}, {"en", true, "Signed in to a&lt;b"}, {"en", false, "Sign-in to a&lt;b didn't finish"}} {
		page := string(CallbackPage(c.lang, "a<b", c.ok, "access_denied"))
		if !strings.Contains(page, c.want) || strings.Contains(page, "a<b") {
			t.Errorf("%s ok=%v: want %q in %s", c.lang, c.ok, c.want, page)
		}
	}
}

func TestIntroPageText(t *testing.T) {
	now := time.Date(2026, 11, 27, 16, 50, 0, 0, time.Local)
	in := SignInIntro{Portal: "acme", EndsAt: now.Add(10 * time.Minute), Profiles: []IntroProfile{{Name: "acme-dev", Monogram: "AD", Slot: 1}}}
	zh := string(IntroPage("zh-CN", in, "https://oidc.example/authorize?a=1&b=2", now))
	for _, want := range []string{`<html lang="zh-CN">`, "重新登录：acme", "这次登录 17:00 结束。重新登录会为下面这些配置文件开始新的会话：", "继续到 AWS 登录",
		"不想登录的话，关掉这个窗口就行。", `href="https://oidc.example/authorize?a=1&amp;b=2"`} {
		if !strings.Contains(zh, want) {
			t.Errorf("zh page lacks %q", want)
		}
	}
	in.Profiles, in.EndsAt = nil, now.Add(8*time.Hour) // after midnight: the date is shown
	en := string(IntroPage("en", in, "https://oidc.example/authorize", now))
	want := "This sign-in ends at " + now.Add(8*time.Hour).Format("2006-01-02 15:04") + ". Signing in again starts a new session."
	if !strings.Contains(en, want) || strings.Contains(en, "<ul>") {
		t.Errorf("en page lacks %q or lists profiles: %s", want, en)
	}
}
