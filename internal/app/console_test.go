package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/browser"
)

func openedURLs(rt *fakeRuntime) []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]string(nil), rt.urls...)
}

// The Console button signs in with the role credentials of the app's own sign-in (AWS federation), not with
// the browser's portal session. Neither the link nor the credentials cross the bridge or reach the log.
func TestOpenConsoleSignsInWithTheAppSession(t *testing.T) {
	w := newWorld(t)
	call := func(profile string) *api.AppError {
		err := w.acc.OpenConsole(bg, profile)
		w.rec(nil, err)
		return api.AsAppError(err)
	}
	if ae := call("initech-admin"); ae == nil || ae.Code != api.ErrNotSignedIn {
		t.Fatalf("before sign-in: %+v", ae)
	}

	ok(w.acc.Login(bg, "initech", api.LoginModeBrowser))
	n := len(openedURLs(w.rt))
	if ae := call("initech-admin"); ae != nil {
		t.Fatal(ae)
	}
	urls := openedURLs(w.rt)
	if len(urls) != n+1 {
		t.Fatalf("opened %d URLs, want 1", len(urls)-n)
	}
	u, err := url.Parse(urls[n])
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Scheme+"://"+u.Host != w.aws.URL || u.Path != "/federation" || q.Get("Action") != "login" || q.Get("SigninToken") != "SIGNIN-ASIAROLEKEY00EXAMPLE" ||
		q.Get("Issuer") != portalURL || q.Get("Destination") != "https://console.aws.amazon.com/console/home?region=us-west-2" {
		t.Fatalf("opened %q", urls[n])
	}
	calls := w.aws.SignIns()
	if len(calls) != 1 || calls[0].SessionID != "ASIAROLEKEY00EXAMPLE" || calls[0].SessionKey != "roleSecretNotReal" || calls[0].SessionToken != "roleSessionTokNotReal" {
		t.Fatalf("federation calls: %+v", calls)
	}

	w.aws.FailSignIn(400)
	if ae := call("initech-admin"); ae == nil || ae.Code != api.ErrAWSAPI {
		t.Fatalf("federation failure: %+v", ae)
	}
	if ae := call("acme"); ae == nil || ae.Code != api.ErrInvalidInput {
		t.Fatalf("static profile: %+v", ae)
	}
	if got := len(openedURLs(w.rt)); got != n+1 {
		t.Fatalf("a failed console sign-in opened a URL (%d)", got-n-1)
	}

	assertNoConsoleSecrets(t, w)
}

// assertNoConsoleSecrets checks that neither the console sign-in link nor the role credentials reached a
// recorded result or error, an event or the log.
func assertNoConsoleSecrets(t *testing.T, w *world) {
	t.Helper()
	w.mu.Lock()
	corpus := append([]string(nil), w.recorded...)
	w.mu.Unlock()
	for _, ev := range w.rt.snapshot() {
		b, err := json.Marshal(ev.Data)
		if err != nil {
			t.Fatalf("event %s is not JSON: %v", ev.Name, err)
		}
		corpus = append(corpus, ev.Name+" "+string(b))
	}
	logText, err := os.ReadFile(filepath.Join(w.p.LogDir, "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	corpus = append(corpus, string(logText))
	secrets := append([]string{"roleSecretNotReal", "roleSessionTokNotReal", "SIGNIN-", "/federation?"}, secretStrings...)
	for i, text := range corpus {
		for _, s := range secrets {
			if strings.Contains(text, s) {
				t.Fatalf("%q found in item %d: %.200s", s, i, text)
			}
		}
	}
}

// fakeConsole stands in for Google Chrome and records what the console opened through a proxy.
type fakeConsole struct {
	mu        sync.Mutex
	chromeErr error // returned by ChromeApp
	failOpen  bool  // OpenChromeWithProxy fails with an error that quotes the link
	opens     []consoleOpen
}

type consoleOpen struct{ url, dir, proxy string }

func (f *fakeConsole) ChromeApp() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return "/Applications/Google Chrome.app", f.chromeErr
}

func (f *fakeConsole) OpenChromeWithProxy(_ context.Context, u, dir, proxy string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opens = append(f.opens, consoleOpen{u, dir, proxy})
	if f.failOpen {
		return errors.New("cannot start Google Chrome for " + u)
	}
	return nil
}

func (f *fakeConsole) set(chromeErr error, failOpen bool) {
	f.mu.Lock()
	f.chromeErr, f.failOpen = chromeErr, failOpen
	f.mu.Unlock()
}

func (f *fakeConsole) opened() []consoleOpen {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]consoleOpen(nil), f.opens...)
}

func setConsoleProxy(w *world, portal, proxy string) {
	w.t.Helper()
	ok(w.sys.UpdateSettings(api.SettingsPatch{ConsoleProxies: map[string]string{portal: proxy}}))
}

// A portal with a console proxy opens the console in Chrome through that proxy; any other portal, or the same
// portal once the proxy is off, keeps the default browser.
func TestOpenConsoleThroughProxy(t *testing.T) {
	w := newWorld(t)
	ok(w.acc.Login(bg, "initech", api.LoginModeBrowser))
	w.c.Paths.Sandbox = false // pretend to be a real run; the fakes launch nothing
	fake := &fakeConsole{}
	w.c.console = fake
	call := func() *api.AppError {
		err := w.acc.OpenConsole(bg, "initech-admin")
		w.rec(nil, err)
		return api.AsAppError(err)
	}
	n := len(openedURLs(w.rt))

	setConsoleProxy(w, "other", "198.51.100.7:3128")
	if ae := call(); ae != nil {
		t.Fatal(ae)
	}
	if len(openedURLs(w.rt)) != n+1 || len(fake.opened()) != 0 {
		t.Fatalf("no proxy for the portal: default browser %d, Chrome %v", len(openedURLs(w.rt))-n, fake.opened())
	}

	setConsoleProxy(w, "initech", " 192.0.2.10:03128 ")
	if ae := call(); ae != nil {
		t.Fatal(ae)
	}
	got := fake.opened()
	dir := filepath.Join(w.p.AppDataDir, "console-browser", "192.0.2.10_3128")
	if len(got) != 1 || got[0].dir != dir || got[0].proxy != "192.0.2.10:3128" || len(openedURLs(w.rt)) != n+1 {
		t.Fatalf("Chrome %+v, default browser %d", got, len(openedURLs(w.rt))-n)
	}
	u, err := url.Parse(got[0].url)
	if err != nil || u.Path != "/federation" || u.Query().Get("SigninToken") != "SIGNIN-ASIAROLEKEY00EXAMPLE" {
		t.Fatalf("not the app's own console sign-in: %q", got[0].url)
	}
	for _, d := range []string{dir, filepath.Dir(dir)} {
		if st, err := os.Stat(d); err != nil || !st.IsDir() || st.Mode().Perm() != 0o700 {
			t.Fatalf("profile folder %s: %v %v", d, st, err)
		}
	}

	// Checked before the sign-in link is made: without Chrome, or off macOS, no link and no launch.
	signIns := len(w.aws.SignIns())
	for _, tc := range []struct {
		err  error
		code api.ErrorCode
	}{{browser.ErrNoChrome, api.ErrChromeMissing}, {browser.ErrUnsupported, api.ErrUnsupportedOS}} {
		fake.set(tc.err, false)
		if ae := call(); ae == nil || ae.Code != tc.code {
			t.Fatalf("%v: %+v", tc.err, ae)
		}
	}
	if len(w.aws.SignIns()) != signIns || len(fake.opened()) != 1 || len(openedURLs(w.rt)) != n+1 {
		t.Fatal("a failed check must neither make a sign-in link nor open anything")
	}

	// A failed launch reports E_INTERNAL and never the launcher's text, which can quote the link.
	fake.set(nil, true)
	if ae := call(); ae == nil || ae.Code != api.ErrInternal || strings.Contains(ae.Message+ae.Detail, "federation") || len(fake.opened()) != 2 {
		t.Fatalf("%+v", ae)
	}
	fake.set(nil, false)

	// Sandbox mode never starts a real browser.
	w.c.Paths.Sandbox = true
	if ae := call(); ae == nil || ae.Code != api.ErrSandbox || len(fake.opened()) != 2 {
		t.Fatalf("sandbox: %+v", ae)
	}
	w.c.Paths.Sandbox = false

	// Turned off: the default browser again, exactly as without the setting.
	setConsoleProxy(w, "initech", "")
	if ae := call(); ae != nil || len(openedURLs(w.rt)) != n+2 || len(fake.opened()) != 2 {
		t.Fatalf("%+v", ae)
	}
	assertNoConsoleSecrets(t, w)
}

// The real launcher with a fake open command: the arguments Chrome gets and the proxy auto-config among them.
// An ISO partition makes the console link the access portal shortcut, so no AWS call is needed.
func TestOpenConsoleThroughProxyStartsChrome(t *testing.T) {
	w := newWorld(t)
	cfg := strings.Replace(configText, "sso_region     = us-west-2", "sso_region     = us-iso-east-1", 1)
	if cfg == configText {
		t.Fatal("the config has no sso_region to replace")
	}
	if err := os.WriteFile(w.p.AWSConfigFile, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	apps := filepath.Join(t.TempDir(), "Applications")
	chrome := filepath.Join(apps, "Google Chrome.app")
	if err := os.MkdirAll(chrome, 0o755); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var calls [][]string
	op := &browser.Opener{GOOS: "darwin", Home: w.p.Home, Applications: apps,
		Output: func(_ context.Context, name string, args ...string) ([]byte, error) {
			mu.Lock()
			calls = append(calls, append([]string{name}, args...))
			mu.Unlock()
			return nil, nil
		}}
	w.c.Paths.Sandbox = false
	w.c.console = op
	launched := func() [][]string { mu.Lock(); defer mu.Unlock(); return append([][]string(nil), calls...) }

	setConsoleProxy(w, "initech", "203.0.113.5:3128")
	if err := w.acc.OpenConsole(bg, "initech-admin"); err != nil {
		t.Fatal(err)
	}
	const link = portalURL + "/#/console?account_id=" + acctInitech + "&role_name=AdministratorAccess"
	const pacPrefix = "--proxy-pac-url=data:application/x-ns-proxy-autoconfig;base64,"
	dir := filepath.Join(w.p.AppDataDir, "console-browser", "203.0.113.5_3128")
	got := launched()
	if len(got) != 1 || len(got[0]) != 10 {
		t.Fatalf("launched %q", got)
	}
	pacArg := got[0][6]
	want := []string{"/usr/bin/open", "-n", "-a", chrome, "--args", "--user-data-dir=" + dir, pacArg, "--no-first-run", "--no-default-browser-check", link}
	if !reflect.DeepEqual(got[0], want) || !strings.HasPrefix(pacArg, pacPrefix) {
		t.Fatalf("launched\n %q\nwant %q", got[0], want)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(pacArg, pacPrefix))
	if err != nil {
		t.Fatalf("PAC: %v", err)
	}
	pac := string(raw)
	for _, d := range []string{"amazonaws.com", "aws.amazon.com", "awsapps.com", "app.aws", "api.aws"} {
		if !strings.Contains(pac, `"`+d+`"`) {
			t.Errorf("PAC lacks %s", d)
		}
	}
	if !strings.Contains(pac, `if (url.substring(0, 6) != "https:" && url.substring(0, 4) != "wss:") return "DIRECT";`) ||
		!strings.Contains(pac, `if (host == aws[i] || dnsDomainIs(host, "." + aws[i])) return "PROXY 203.0.113.5:3128";`) ||
		strings.Count(pac, "PROXY") != 1 || strings.Contains(pac, "; DIRECT") {
		t.Fatalf("PAC:\n%s", pac)
	}

	// Off macOS and without Chrome: a clear error and nothing launched.
	op.GOOS = "windows"
	if ae := api.AsAppError(w.acc.OpenConsole(bg, "initech-admin")); ae == nil || ae.Code != api.ErrUnsupportedOS {
		t.Fatalf("windows: %+v", ae)
	}
	op.GOOS = "darwin"
	if err := os.RemoveAll(chrome); err != nil {
		t.Fatal(err)
	}
	if ae := api.AsAppError(w.acc.OpenConsole(bg, "initech-admin")); ae == nil || ae.Code != api.ErrChromeMissing {
		t.Fatalf("no Chrome: %+v", ae)
	}
	if len(launched()) != 1 {
		t.Fatalf("launched %q", launched())
	}
}

// A console proxy stays with its portal: renamed with it, removed with it, and the frontend hears of it.
func TestConsoleProxyFollowsThePortal(t *testing.T) {
	w := newWorld(t)
	setConsoleProxy(w, "other", "198.51.100.7:3128")
	setConsoleProxy(w, "initech", "192.0.2.10:3128")
	changed := w.rt.count(api.EventSettingsChanged)

	p := ok(w.acc.PreviewRenamePortal("other", "other-2"))
	ok(w.acc.RenamePortal("other", "other-2", p.BaseHash))
	want := map[string]string{"initech": "192.0.2.10:3128", "other-2": "198.51.100.7:3128"}
	if got := w.c.Settings.API().ConsoleProxies; !reflect.DeepEqual(got, want) {
		t.Fatalf("after rename: %#v", got)
	}
	if w.rt.count(api.EventSettingsChanged) != changed+1 {
		t.Fatal("settings:changed must follow the rename")
	}

	p = ok(w.acc.PreviewRemovePortal("other-2"))
	ok(w.acc.RemovePortal("other-2", p.BaseHash))
	if got := w.c.Settings.API().ConsoleProxies; !reflect.DeepEqual(got, map[string]string{"initech": "192.0.2.10:3128"}) {
		t.Fatalf("after remove: %#v", got)
	}
	if w.rt.count(api.EventSettingsChanged) != changed+2 {
		t.Fatal("settings:changed must follow the removal")
	}
}
