package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
)

type fakePrivate struct {
	private bool
	err     error
	urls    []string

	isolated bool // OpenIsolated starts a fake isolated browser
	isoErr   error
	mu       sync.Mutex
	windows  []*fakeWindow

	openIso func(ctx context.Context) // runs inside OpenIsolated before it returns (tests that block)

	staleRoot chan string // RemoveStale records its root and keep function here
	staleKeep chan func(string) bool
}

func (f *fakePrivate) OpenPrivate(_ context.Context, u string) (bool, error) {
	f.urls = append(f.urls, u)
	return f.private, f.err
}

func (f *fakePrivate) OpenIsolated(ctx context.Context, u, dir string) (signInWindow, error) {
	if f.openIso != nil {
		f.openIso(ctx)
	}
	if f.isoErr != nil || !f.isolated {
		return nil, f.isoErr
	}
	w := &fakeWindow{url: u, dir: dir}
	f.mu.Lock()
	f.windows = append(f.windows, w)
	f.mu.Unlock()
	return w, nil
}

func (f *fakePrivate) RemoveStale(_ context.Context, root string, keep func(string) bool) {
	if f.staleRoot != nil {
		f.staleRoot <- root
		f.staleKeep <- keep
	}
}

// fakeWindow is an isolated browser that records being closed.
type fakeWindow struct {
	url, dir string
	mu       sync.Mutex
	closed   int
}

func (w *fakeWindow) Close(context.Context) error {
	err := os.RemoveAll(w.dir)
	w.mu.Lock()
	w.closed++
	w.mu.Unlock()
	return err
}

func (w *fakeWindow) isClosed() bool { w.mu.Lock(); defer w.mu.Unlock(); return w.closed > 0 }

// waitCond polls cond for up to 2 s (windows are closed in the background).
func waitCond(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

const signInURL = "https://device.sso.us-east-1.amazonaws.com/?user_code=ABCD-EFGH"

func TestOpenSignInPage(t *testing.T) {
	off := false
	cases := []struct {
		name        string
		setting     *bool // nil = default (on)
		fake        *fakePrivate
		wantPrivate bool
		wantPrivCal int
		wantNormal  int
	}{
		{"isolated browser first", nil, &fakePrivate{private: true, isolated: true}, true, 0, 0},
		{"isolation fails, private window", nil, &fakePrivate{private: true, isolated: true, isoErr: errors.New("boom")}, true, 1, 0},
		{"private window", nil, &fakePrivate{private: true}, true, 1, 0},
		{"default browser has no private switch", nil, &fakePrivate{private: false}, false, 1, 1},
		{"detection error falls back", nil, &fakePrivate{err: errors.New("boom")}, false, 1, 1},
		{"setting off", &off, &fakePrivate{private: true}, false, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld(t)
			w.c.Paths.Sandbox = false // pretend to be a real run; the fake runtime never launches anything
			w.c.private = tc.fake
			if tc.setting != nil {
				if _, err := w.c.Settings.Update(api.SettingsPatch{PrivateSignIn: tc.setting}); err != nil {
					t.Fatal(err)
				}
			}
			rt := w.c.RT.(*fakeRuntime)
			got, err := NewAccountService(w.c).OpenSignInPage(signInURL)
			if err != nil || got.Private != tc.wantPrivate {
				t.Fatalf("got (%v,%v)", got, err)
			}
			if len(tc.fake.urls) != tc.wantPrivCal || len(rt.urls) != tc.wantNormal {
				t.Fatalf("private calls %v, normal opens %v", tc.fake.urls, rt.urls)
			}
		})
	}
}

func TestOpenSignInPageRejectsOtherURLs(t *testing.T) {
	w := newWorld(t)
	w.c.Paths.Sandbox = false
	fake := &fakePrivate{private: true}
	w.c.private = fake
	svc := NewAccountService(w.c)
	for _, u := range []string{
		"", "http://device.sso.us-east-1.amazonaws.com/", "https://evil.example.com/",
		"https://amazonaws.com.evil.com/", "https://notamazonaws.com/", "https://my-bucket.s3.amazonaws.com/x", "https://oidc.s3.amazonaws.com/", "https://oidc.s3-accelerate.amazonaws.com/", "https://amazonaws.com/", "https://app.aws.evil.com/", "ftp://x.awsapps.com/",
		"https://user@d-123.awsapps.com/start", "http://127.0.0.1:9/x", "javascript:alert(1)",
	} {
		if _, err := svc.OpenSignInPage(u); err == nil || api.AsAppError(err).Code != api.ErrInvalidInput {
			t.Fatalf("%q: %v", u, err)
		}
	}
	for _, u := range []string{
		"https://device.sso.us-east-1.amazonaws.com/?user_code=X", "https://d-123.awsapps.com/start/#/device",
		"https://oidc.cn-north-1.amazonaws.com.cn/x", "https://corp.awsapps.cn/start", "https://DEVICE.sso.eu-west-1.amazonaws.com/",
		"https://d-123.portal.us-east-1.app.aws/", "https://corp.app.aws/start", "https://x.app.amazonwebservices.com.cn/start",
	} {
		if err := checkSignInURL(u, false); err != nil {
			t.Fatalf("%q: %v", u, err)
		}
	}
	if len(fake.urls) != 0 {
		t.Fatalf("rejected URLs must not be opened: %v", fake.urls)
	}
}

func TestOpenSignInPageSandbox(t *testing.T) {
	w := newWorld(t) // sandbox: loopback http allowed, never a private launch
	fake := &fakePrivate{private: true}
	w.c.private = fake
	got, err := NewAccountService(w.c).OpenSignInPage("http://127.0.0.1:5555/device")
	if err != nil || got.Private || got.Isolated || len(fake.urls) != 0 {
		t.Fatalf("(%v,%v) %v", got, err, fake.urls)
	}
	if rt := w.c.RT.(*fakeRuntime); len(rt.urls) != 1 {
		t.Fatalf("%v", rt.urls)
	}
}

func TestIsolatedSignInWindows(t *testing.T) {
	w := newWorld(t)
	w.c.Paths.Sandbox = false
	fake := &fakePrivate{private: true, isolated: true}
	w.c.private = fake
	w.c.signInDelay = 10 * time.Millisecond
	svc := NewAccountService(w.c)

	got, err := svc.OpenSignInPage(signInURL)
	if err != nil || !got.Private || !got.Isolated || len(fake.urls) != 0 || len(w.c.RT.(*fakeRuntime).urls) != 0 {
		t.Fatalf("%+v %v", got, err)
	}
	first := fake.windows[0]
	if !strings.HasPrefix(first.dir, filepath.Join(w.c.Paths.AppDataDir, "signin-browser")+string(filepath.Separator)) {
		t.Fatalf("profile outside the app folder: %s", first.dir)
	}
	if st, err := os.Stat(first.dir); err != nil || !st.IsDir() || st.Mode().Perm() != 0o700 {
		t.Fatalf("fresh private profile folder: %v %v", st, err)
	}
	// "Open again" replaces the window
	if _, err := svc.OpenSignInPage(signInURL); err != nil {
		t.Fatal(err)
	}
	waitCond(t, first.isClosed)
	second := fake.windows[1]
	if second.isClosed() || second.dir == first.dir {
		t.Fatal("the new window stays open")
	}
	// a sign-in that ended for another page leaves it alone; its own end closes it
	w.c.signInEnded([]string{"https://device.sso.us-east-1.amazonaws.com/?user_code=OTHER"}, true)
	time.Sleep(50 * time.Millisecond)
	if second.isClosed() {
		t.Fatal("closed by another sign-in")
	}
	w.c.signInEnded([]string{"https://device.sso.us-east-1.amazonaws.com/", signInURL}, true)
	waitCond(t, second.isClosed)
	if _, err := os.Stat(second.dir); !os.IsNotExist(err) {
		t.Fatal("profile kept")
	}
	// a window that opens after its sign-in ended is closed at once
	w.c.signInEnded([]string{signInURL}, false)
	if _, err := svc.OpenSignInPage(signInURL); err != nil {
		t.Fatal(err)
	}
	late := fake.windows[len(fake.windows)-1]
	waitCond(t, late.isClosed)
	// quitting the app closes what is still open
	const next = "https://device.sso.us-east-1.amazonaws.com/?user_code=NEXT"
	if _, err := svc.OpenSignInPage(next); err != nil {
		t.Fatal(err)
	}
	third := fake.windows[len(fake.windows)-1]
	w.c.closeSignInWindows()
	if !third.isClosed() {
		t.Fatal("shutdown must close sign-in windows")
	}
}
