package app

import (
	"context"
	"testing"
	"time"
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

// A sign-in that finishes opening while the app is quitting must not leave a browser behind.
func TestSignInWindowOpenedDuringShutdownIsClosed(t *testing.T) {
	w := newWorld(t)
	w.c.Paths.Sandbox = false
	fake := &fakePrivate{private: true, isolated: true}
	w.c.private = fake
	w.c.Shutdown()
	if _, err := NewAccountService(w.c).OpenSignInPage(signInURL); err != nil {
		t.Fatal(err)
	}
	if len(fake.windows) == 1 {
		waitCond(t, fake.windows[0].isClosed)
	}
}

// Only sign-in pages use the isolated window: the other callers (update page, install pages, a link in the
// app, the portal, the console) open a normal window even when the private sign-in setting is on.
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
	tools := NewToolService(w.c, nil)
	_ = tools.OpenInstallPage("git") // a tool id the install pages know; the open goes through the runtime
	if len(rt.urls) < 2 {
		t.Fatalf("normal opens: %v", rt.urls)
	}
	if len(fake.urls) != 0 || len(fake.windows) != 0 {
		t.Fatalf("a non-sign-in page used the private or isolated window: %v %d", fake.urls, len(fake.windows))
	}
}
