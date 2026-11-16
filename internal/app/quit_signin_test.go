package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
)

// terminate stands in for Wails' terminate step: ShouldQuit decides, and when it says yes Wails runs its
// shutdown (OnShutdown -> Container.Shutdown) before the process ends. Every quit path of the app (Cmd+Q, the
// menu, the Dock, the tray, the update restart, a logout) ends in this step.
func terminate(c *Container, life *Lifecycle) bool {
	if !life.ShouldQuit() {
		return false
	}
	c.Shutdown()
	return true
}

// openFakeSignIn starts one fake isolated sign-in browser through the account service.
func openFakeSignIn(t *testing.T) (*world, *fakeWindow) {
	t.Helper()
	w := newWorld(t)
	w.c.Paths.Sandbox = false
	fake := &fakePrivate{private: true, isolated: true}
	w.c.private = fake
	if _, err := NewAccountService(w.c).OpenSignInPage(signInURL); err != nil {
		t.Fatal(err)
	}
	if len(fake.windows) != 1 || fake.windows[0].isClosed() {
		t.Fatal("the fake isolated browser should be open")
	}
	return w, fake.windows[0]
}

func TestEveryQuitPathClosesSignInWindows(t *testing.T) {
	idle := func() *Lifecycle {
		l := NewLifecycle()
		l.SetLoggingOut(func() bool { return false })
		return l
	}
	cases := []struct {
		name string
		run  func(t *testing.T, w *world)
	}{
		{"Cmd+Q, menu, Dock, tray with nothing running", func(t *testing.T, w *world) {
			if !terminate(w.c, idle()) {
				t.Fatal("must quit")
			}
		}},
		{"quit while an AI task runs, answer 'quit now'", func(t *testing.T, w *world) {
			l := liveLifecycle()
			l.SetQuitDecision(func(context.Context) QuitAnswer { return QuitNow })
			done := make(chan bool, 1)
			l.SetQuit(func() { done <- terminate(w.c, l) }) // Wails' Quit asks ShouldQuit again
			if terminate(w.c, l) {
				t.Fatal("the first terminate is cancelled while the dialog decides")
			}
			select {
			case ok := <-done:
				if !ok {
					t.Fatal("the approved quit must go through")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("no quit")
			}
		}},
		{"update restart through QuitApproved", func(t *testing.T, w *world) {
			l := liveLifecycle()
			if err := l.QuitApproved(func() error {
				if !terminate(w.c, l) {
					t.Fatal("approved quit must go through")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		}},
		{"logout, restart or shutdown of the computer", func(t *testing.T, w *world) {
			l := liveLifecycle()
			l.SetLoggingOut(func() bool { return true })
			if !terminate(w.c, l) || !l.LoggingOut() {
				t.Fatal("a session end quits at once")
			}
		}},
		{"Wails stops the services", func(t *testing.T, w *world) {
			if err := NewAccountService(w.c).ServiceShutdown(); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, win := openFakeSignIn(t)
			tc.run(t, w)
			if !win.isClosed() {
				t.Fatal("the isolated sign-in browser stayed open")
			}
		})
	}
}

// While the app is quitting a sign-in page opens nothing: not an isolated window, not a private window, and
// not the normal browser either.
func TestSignInPageAfterShutdownOpensNothing(t *testing.T) {
	for _, private := range []bool{true, false} { // the "private sign-in" setting on and off (FU-21c)
		name := "private sign-in on"
		if !private {
			name = "private sign-in off"
		}
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			w.c.Paths.Sandbox = false
			if _, err := w.c.Settings.Update(api.SettingsPatch{PrivateSignIn: &private}); err != nil {
				t.Fatal(err)
			}
			fake := &fakePrivate{private: true, isolated: true}
			w.c.private = fake
			rt := w.c.RT.(*fakeRuntime)
			w.c.Shutdown()
			_, err := NewAccountService(w.c).OpenSignInPage(signInURL)
			var ae *api.AppError
			if !errors.As(err, &ae) || ae.Code != api.ErrCancelled {
				t.Fatalf("want E_CANCELLED, got %v", err)
			}
			if len(fake.windows) != 0 || len(fake.urls) != 0 || len(rt.urls) != 0 {
				t.Fatalf("something opened: %d windows, %v private, %v normal", len(fake.windows), fake.urls, rt.urls)
			}
		})
	}
}

// A sign-in whose isolated browser is still starting when the app quits: Shutdown waits for it and the
// window is closed by the time Shutdown returns.
func TestSignInWindowOpenedDuringShutdownIsClosed(t *testing.T) {
	w := newWorld(t)
	w.c.Paths.Sandbox = false
	started := make(chan struct{})
	fake := &fakePrivate{private: true, isolated: true}
	fake.openIso = func(context.Context) {
		close(started)
		for i := 0; i < 500 && w.c.ctx.Err() == nil; i++ { // blocks until Shutdown has begun
			time.Sleep(2 * time.Millisecond)
		}
	}
	w.c.private = fake
	opened := make(chan error, 1)
	go func() {
		_, err := NewAccountService(w.c).OpenSignInPage(signInURL)
		opened <- err
	}()
	<-started
	w.c.Shutdown()
	rt := w.c.RT.(*fakeRuntime)
	fake.mu.Lock()
	n := len(fake.windows)
	fake.mu.Unlock()
	if n != 1 {
		t.Fatalf("the window should have opened: %d", n)
	}
	if !fake.windows[0].isClosed() {
		t.Fatal("the window opened during the quit was still open when Shutdown returned")
	}
	// the call ends with E_CANCELLED and nothing else opens: without the check after the isolated attempt it
	// would fall through to a private window or the normal browser (FU-21c)
	var ae *api.AppError
	if err := <-opened; !errors.As(err, &ae) || ae.Code != api.ErrCancelled {
		t.Fatalf("want E_CANCELLED, got %v", err)
	}
	if len(fake.urls) != 0 || len(rt.urls) != 0 {
		t.Fatalf("a page opened during the quit: %v private, %v normal", fake.urls, rt.urls)
	}
}

// FU-21a: an isolated launch that is under way when the quit begins is not cancelled half way: it returns its
// handle and the window is closed, instead of a started browser nobody closes.
func TestIsolatedLaunchUnderWayAtQuitIsClosed(t *testing.T) {
	w := newWorld(t)
	w.c.Paths.Sandbox = false
	started := make(chan struct{})
	fake := &fakePrivate{private: true, isolated: true, startThenFail: true}
	fake.openIso = func(context.Context) { // the browser takes its time to start; the quit begins meanwhile
		close(started)
		for i := 0; i < 500 && w.c.ctx.Err() == nil; i++ {
			time.Sleep(2 * time.Millisecond)
		}
	}
	w.c.private = fake
	opened := make(chan error, 1)
	go func() {
		_, err := NewAccountService(w.c).OpenSignInPage(signInURL)
		opened <- err
	}()
	<-started
	w.c.Shutdown()
	<-opened
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.windows) != 1 {
		t.Fatalf("%d windows", len(fake.windows))
	}
	if !fake.windows[0].isClosed() {
		t.Fatal("the browser that was starting when the app quit is still open")
	}
}

// FU-21b: the old window after "Open again" is closed in the background; a quit right after waits for that
// close (the profile folder must be gone, and on Windows a profile in use is kept by RemoveStale).
func TestShutdownWaitsForACloseInFlight(t *testing.T) {
	w := newWorld(t)
	w.c.Paths.Sandbox = false
	fake := &fakePrivate{private: true, isolated: true, slowFirst: 700 * time.Millisecond}
	w.c.private = fake
	for i := 0; i < 2; i++ { // the second open is "Open again": it closes the first window in the background
		if _, err := NewAccountService(w.c).OpenSignInPage(signInURL); err != nil {
			t.Fatal(err)
		}
	}
	w.c.Shutdown()
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.windows) != 2 {
		t.Fatalf("%d windows", len(fake.windows))
	}
	for i, win := range fake.windows {
		if !win.isClosed() {
			t.Fatalf("window %d was not closed when Shutdown returned", i)
		}
	}
}

// Crash leftovers are removed at start, and only profiles made before this run started.
func TestStartLoopsRemovesStaleSignInProfiles(t *testing.T) {
	w := newWorld(t)
	w.c.Paths.Sandbox = false
	fake := &fakePrivate{staleRoot: make(chan string, 1), staleKeep: make(chan func(string) bool, 1)}
	w.c.private = fake
	w.c.StartLoops()
	select {
	case root := <-fake.staleRoot:
		keep := <-fake.staleKeep
		if root != filepath.Join(w.c.Paths.AppDataDir, "signin-browser") {
			t.Fatalf("root %q", root)
		}
		fresh := filepath.Join(t.TempDir(), "fresh")
		old := filepath.Join(t.TempDir(), "old")
		for _, d := range []string{fresh, old} {
			if err := os.MkdirAll(d, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chtimes(old, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
		if !keep(fresh) || keep(old) {
			t.Fatalf("keep(fresh)=%v keep(old)=%v", keep(fresh), keep(old))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RemoveStale was not called at start")
	}
	w.c.Shutdown()
}

// Only sign-in pages use the isolated window: the other callers (update page, install pages, a link in the
// app, the portal) open a normal window even when the private sign-in setting is on. The console is covered
// by console_test.go.
func TestOnlySignInUsesTheIsolatedWindow(t *testing.T) {
	w := newWorld(t)
	w.c.Paths.Sandbox = false
	fake := &fakePrivate{private: true, isolated: true}
	w.c.private = fake
	rt := w.c.RT.(*fakeRuntime)
	if err := NewSystemService(w.c).OpenURL("https://example.com/docs"); err != nil {
		t.Fatal(err)
	}
	if err := NewUpdateService(w.c).OpenDownloadPage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := NewToolService(w.c, nil).OpenInstallPage(api.SetupGit); err != nil {
		t.Fatal(err)
	}
	if len(rt.urls) != 3 || rt.urls[2] != installPages[api.SetupGit][runtime.GOOS] {
		t.Fatalf("normal opens: %v", rt.urls)
	}
	if err := NewAccountService(w.c).OpenPortal("initech"); err != nil {
		t.Fatal(err)
	}
	if len(rt.urls) != 4 {
		t.Fatalf("the portal did not open normally: %v", rt.urls)
	}
	if len(fake.urls) != 0 || len(fake.windows) != 0 {
		t.Fatalf("a non-sign-in page used the private or isolated window: %v %d", fake.urls, len(fake.windows))
	}
}
