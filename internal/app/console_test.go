package app

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/api"
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
