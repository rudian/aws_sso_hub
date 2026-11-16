package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/notify"
)

// notifyWorld is a world whose notifications go to a fake backend (never the system's), started the way Wails
// starts the service.
func notifyWorld(t *testing.T, mut ...func(*ContainerOptions)) (*world, *notify.Fake) {
	t.Helper()
	fake := notify.NewFake()
	opts := []func(*ContainerOptions){func(o *ContainerOptions) { o.NotifyBackend = fake }}
	opts = append(opts, mut...)
	w := newWorld(t, opts...)
	if err := NewNotifyService(w.c).ServiceStartup(context.Background(), applicationServiceOptions()); err != nil {
		t.Fatal(err)
	}
	return w, fake
}

func navigations(w *world) []api.NavigateRequest {
	var out []api.NavigateRequest
	for _, e := range w.rt.snapshot() {
		if e.Name == api.EventNavigate {
			out = append(out, e.Data.(api.NavigateRequest))
		}
	}
	return out
}

func toasts(w *world) []api.NotifyToast {
	var out []api.NotifyToast
	for _, e := range w.rt.snapshot() {
		if e.Name == api.EventNotifyToast {
			out = append(out, e.Data.(api.NotifyToast))
		}
	}
	return out
}

func endedStatus(portal string) api.SessionStatus {
	return api.SessionStatus{Portal: portal, State: api.SessionExpired, LoginAt: "2026-11-16T01:00:00Z"}
}

// An "ended" event of the engine becomes a notification with the kind's button, the portal's account names and
// identifiers only; the account ID, the role and the start URL never appear.
func TestEndedEventSendsANotice(t *testing.T) {
	w, fake := notifyWorld(t)
	if err := w.c.Settings.MergeAccountNames(map[string]string{acctInitech: "initech-prod"}); err != nil {
		t.Fatal(err)
	}
	w.c.emit(api.EventSessionEnded, endedStatus("initech"))
	waitCond(t, func() bool { return len(fake.Sent()) == 1 })
	n := fake.Sent()[0]
	if n.ID != "signin:initech" || n.Kind != notify.KindSignInEnded || !n.HasAction || n.Button == "" {
		t.Fatalf("%+v", n)
	}
	if !strings.Contains(n.Title, "initech") || !strings.Contains(n.Body, "initech-prod") {
		t.Fatalf("%q / %q", n.Title, n.Body)
	}
	for _, s := range []string{acctInitech, "AdministratorAccess", portalURL, "awsapps", "https://"} {
		if strings.Contains(n.Title+n.Body, s) {
			t.Fatalf("%q in %q / %q", s, n.Title, n.Body)
		}
		for _, v := range n.Data {
			if strings.Contains(v, s) {
				t.Fatalf("%q in the data map %v", s, n.Data)
			}
		}
	}
	if len(n.Data) != 3 || n.Data[notify.DataPortal] != "initech" || n.Data[notify.DataTime] != "2026-11-16T01:00:00Z" {
		t.Fatalf("%v", n.Data)
	}
}

func TestHeadsUpEventSendsASilentNotice(t *testing.T) {
	w, fake := notifyWorld(t)
	w.c.emit(api.EventSessionReminder, api.SessionStatus{Portal: "initech", State: api.SessionExpiring, LoginAt: "2026-11-16T01:00:00Z"})
	waitCond(t, func() bool { return len(fake.Sent()) == 1 })
	n := fake.Sent()[0]
	if n.Kind != notify.KindSignInHeadsUp || !n.Silent || !strings.Contains(n.Body, "15") {
		t.Fatalf("%+v", n)
	}
}

// Sandbox mode (and any build that is not production without the opt-in) uses the fake, which also shows each
// notice as a toast in the window.
func TestSandboxUsesTheFakeAndShowsToasts(t *testing.T) {
	w := newWorld(t) // no backend given: chosen by the build and the sandbox flag
	if !w.c.Paths.Sandbox {
		t.Fatal("the test world is a sandbox")
	}
	b, fake := w.c.notifyBackend(ContainerOptions{})
	if _, isFake := b.(*notify.Fake); !isFake || !fake {
		t.Fatalf("sandbox mode must use the fake, got %T", b)
	}
	if err := NewNotifyService(w.c).ServiceStartup(context.Background(), applicationServiceOptions()); err != nil {
		t.Fatal(err)
	}
	w.c.emit(api.EventSessionEnded, endedStatus("initech"))
	waitCond(t, func() bool { return len(toasts(w)) == 1 })
	tt := toasts(w)[0]
	if tt.Button == "" || !tt.Sticky || tt.Tone != api.NotifyToneDanger || tt.ID == "" || !strings.Contains(tt.Title, "initech") {
		t.Fatalf("%+v", tt)
	}
}

// The service started by Wails never fails the app, whatever the backend does.
func TestAStartFailureDoesNotStopTheApp(t *testing.T) {
	fake := notify.NewFake()
	fake.SetStartError(os.ErrPermission)
	w := newWorld(t, func(o *ContainerOptions) { o.NotifyBackend = fake })
	if err := NewNotifyService(w.c).ServiceStartup(context.Background(), applicationServiceOptions()); err != nil {
		t.Fatalf("a start failure must not stop the app: %v", err)
	}
	if st, _ := NewNotifyService(w.c).GetState(); st.Available {
		t.Fatalf("%+v", st)
	}
	w.c.emit(api.EventSessionEnded, endedStatus("initech"))
	waitCond(t, func() bool { return len(toasts(w)) == 1 }) // the window still gets the notice
	if len(fake.Sent()) != 0 {
		t.Fatal("nothing goes to a backend that did not start")
	}
	if err := NewNotifyService(w.c).ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
}

// A click on the button asks the window to start that portal's sign-in (the login modal, as the pass's button
// does): the navigation carries the login action, and the window is not brought forward.
func TestButtonOpensThePortalLoginInTheWindow(t *testing.T) {
	w, fake := notifyWorld(t)
	NewNotifyService(w.c).SetReady(true)
	w.c.emit(api.EventSessionEnded, endedStatus("initech"))
	waitCond(t, func() bool { return len(fake.Sent()) == 1 })
	if err := fake.Respond("signin:initech", notify.ButtonID); err != nil {
		t.Fatal(err)
	}
	nav := navigations(w)
	if len(nav) != 1 || nav[0].Page != "accounts" || nav[0].Action != "login" || nav[0].Arg != "initech" {
		t.Fatalf("%+v", nav)
	}
	w.rt.mu.Lock()
	shown, urls := w.rt.shown, len(w.rt.urls)
	w.rt.mu.Unlock()
	if shown != 0 {
		t.Fatal("the button does not bring the window forward")
	}
	if urls != 0 {
		t.Fatal("Go starts no sign-in itself: the window's login modal does")
	}
}

// The press of a toast goes the same way (sandbox mode shows toasts instead of system notifications).
func TestToastButtonOpensTheLogin(t *testing.T) {
	w, _ := notifyWorld(t, func(o *ContainerOptions) { o.NotifyAlsoToast = true })
	svc := NewNotifyService(w.c)
	svc.SetReady(true)
	w.c.emit(api.EventSessionEnded, endedStatus("initech"))
	waitCond(t, func() bool { return len(toasts(w)) == 1 })
	if err := svc.RespondToast(toasts(w)[0].ID, api.NotifyActionButton); err != nil {
		t.Fatal(err)
	}
	if nav := navigations(w); len(nav) != 1 || nav[0].Action != "login" || nav[0].Arg != "initech" {
		t.Fatalf("%+v", nav)
	}
}

// A click on the notification itself shows the window and opens Accounts at that portal, highlighted once.
func TestBodyClickShowsThePortal(t *testing.T) {
	w, fake := notifyWorld(t)
	NewNotifyService(w.c).SetReady(true)
	w.c.emit(api.EventSessionEnded, endedStatus("initech"))
	waitCond(t, func() bool { return len(fake.Sent()) == 1 })
	if err := fake.Respond("signin:initech", notify.DefaultActionID); err != nil {
		t.Fatal(err)
	}
	nav := navigations(w)
	if len(nav) != 1 || nav[0] != (api.NavigateRequest{Page: "accounts", Action: "show", Arg: "initech"}) {
		t.Fatalf("%+v", nav)
	}
	if w.rt.shown != 1 {
		t.Fatal("the window comes forward")
	}
	if st, _ := w.c.Accounts.SessionStatus("initech"); st.State == api.SessionActive {
		t.Fatal("a body click starts no sign-in")
	}
}

// A stale button: the portal is signed in again already. No sign-in starts; Accounts opens with "Already signed in".
func TestStaleButtonOnlyShowsAlreadySignedIn(t *testing.T) {
	w, fake := notifyWorld(t)
	NewNotifyService(w.c).SetReady(true)
	// "other" holds a valid token (a sign-in that is alive); an "ended" notice for it is out of date
	w.c.emit(api.EventSessionEnded, endedStatus("other"))
	waitCond(t, func() bool { return len(fake.Sent()) == 1 })
	if err := fake.Respond("signin:other", notify.ButtonID); err != nil {
		t.Fatal(err)
	}
	nav := navigations(w)
	if len(nav) != 1 || nav[0] != (api.NavigateRequest{Page: "accounts", Action: "signed-in", Arg: "other"}) {
		t.Fatalf("%+v", nav)
	}
	time.Sleep(50 * time.Millisecond)
	if w.oidc.RegCount() != 0 || len(w.oidc.TokenCalls()) != 0 || len(w.rt.urls) != 0 {
		t.Fatal("a stale button must not start a sign-in")
	}
}

func TestStaleRules(t *testing.T) {
	t0 := "2026-11-16T01:00:00Z"
	later, earlier := "2026-11-16T05:00:00Z", "2026-11-15T20:00:00Z"
	heads := notify.Click{Kind: notify.KindSignInHeadsUp, Data: map[string]string{notify.DataTime: t0}}
	ended := notify.Click{Kind: notify.KindSignInEnded, Data: map[string]string{notify.DataTime: t0}}
	alive := func(loginAt string) api.SessionStatus {
		return api.SessionStatus{State: api.SessionActive, SessionKnown: true, LoginAt: loginAt}
	}
	cases := []struct {
		name string
		k    notify.Click
		st   api.SessionStatus
		want bool
	}{
		{"heads-up, the same sign-in still runs: signing in again is the point", heads, alive(t0), false},
		{"heads-up, a newer sign-in", heads, alive(later), true},
		{"heads-up, an older record", heads, alive(earlier), false},
		{"heads-up, the session ended meanwhile", heads, api.SessionStatus{State: api.SessionExpired, LoginAt: t0}, false},
		{"heads-up, signed out", heads, api.SessionStatus{State: api.SessionSignedOut}, false},
		{"heads-up without a time", notify.Click{Kind: notify.KindSignInHeadsUp, Data: map[string]string{}}, alive(later), false},
		{"ended, signed in again", ended, alive(later), true},
		{"ended, signed in elsewhere (no record)", ended, api.SessionStatus{State: api.SessionActive}, true},
		{"ended, still expired", ended, api.SessionStatus{State: api.SessionExpired, LoginAt: t0}, false},
		{"ended, signed out since", ended, api.SessionStatus{State: api.SessionSignedOut}, false},
	}
	for _, c := range cases {
		if got := staleSignInClick(c.k, c.st); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

// A portal that no longer exists: Accounts opens with nothing selected. A replayed or forged click for another
// portal name can only ever reach that name.
func TestClickForAMissingPortal(t *testing.T) {
	w, fake := notifyWorld(t)
	NewNotifyService(w.c).SetReady(true)
	_ = fake.RespondRaw(notify.Response{ID: "signin:gone", Category: string(notify.KindSignInEnded), Action: notify.ButtonID,
		Data: map[string]string{notify.DataKind: "signin.ended", notify.DataPortal: "gone"}})
	nav := navigations(w)
	if len(nav) != 1 || nav[0] != (api.NavigateRequest{Page: "accounts"}) {
		t.Fatalf("%+v", nav)
	}
	time.Sleep(50 * time.Millisecond)
	if len(w.rt.urls) != 0 {
		t.Fatal("no sign-in")
	}
}

// A click that launched the app arrives before the window has loaded: it waits for the accounts and the startup
// gates, runs once they pass, and is dropped when a gate blocks.
func TestClickThatLaunchedTheAppWaitsForTheGates(t *testing.T) {
	w, fake := notifyWorld(t)
	click := notify.Response{ID: "signin:initech", Category: string(notify.KindSignInEnded), Action: notify.DefaultActionID,
		Data: map[string]string{notify.DataKind: "signin.ended", notify.DataPortal: "initech", notify.DataTime: "2026-11-16T01:00:00Z"}}
	_ = fake.RespondRaw(click)
	if len(navigations(w)) != 0 || w.rt.shown != 0 {
		t.Fatal("nothing happens before the window reports ready")
	}
	svc := NewNotifyService(w.c)
	_ = svc.SetReady(true)
	if nav := navigations(w); len(nav) != 1 || nav[0].Arg != "initech" {
		t.Fatalf("handled once the gates pass: %+v", nav)
	}
	_ = svc.SetReady(true)
	if len(navigations(w)) != 1 {
		t.Fatal("handled once")
	}
	// later clicks run at once
	_ = fake.RespondRaw(click)
	if len(navigations(w)) != 2 {
		t.Fatal("an open gate handles clicks directly")
	}

	// a gate that blocks drops the click, and the click is not kept for later
	w2, fake2 := notifyWorld(t)
	_ = fake2.RespondRaw(click)
	svc2 := NewNotifyService(w2.c)
	_ = svc2.SetReady(false)
	_ = svc2.SetReady(true)
	if len(navigations(w2)) != 0 || w2.rt.shown != 0 {
		t.Fatal("a blocked gate drops the click")
	}
	_ = fake2.RespondRaw(click) // blocked, then open: now it works
	if len(navigations(w2)) != 1 {
		t.Fatal()
	}
	w3, fake3 := notifyWorld(t)
	_ = NewNotifyService(w3.c).SetReady(false)
	_ = fake3.RespondRaw(click)
	if len(navigations(w3)) != 0 {
		t.Fatal("a click during a blocked gate is dropped")
	}
}

// While the app quits a click does nothing: no sign-in page opens.
func TestClickDuringQuitDoesNothing(t *testing.T) {
	w, fake := notifyWorld(t)
	NewNotifyService(w.c).SetReady(true)
	w.c.emit(api.EventSessionEnded, endedStatus("initech"))
	waitCond(t, func() bool { return len(fake.Sent()) == 1 })
	w.c.Shutdown()
	_ = fake.Respond("signin:initech", notify.ButtonID)
	_ = fake.Respond("signin:initech", notify.DefaultActionID)
	time.Sleep(50 * time.Millisecond)
	if len(w.rt.urls) != 0 || w.rt.shown != 0 || len(navigations(w)) != 0 {
		t.Fatal("a click that arrives while the app quits must do nothing")
	}
}

// startRunningSignIn starts initech's sign-in in the background with nobody finishing it, and returns the page.
func startRunningSignIn(t *testing.T, w *world) string {
	t.Helper()
	w.rt.browser = nil // the page opens, nobody finishes the sign-in
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _, _ = w.c.Accounts.Login(ctx, "initech", api.LoginModeBrowser) }()
	waitCond(t, func() bool { st, _ := w.c.Accounts.SessionStatus("initech"); return st.State == api.SessionSigningIn })
	var url string
	waitCond(t, func() bool {
		w.c.notifyc.mu.Lock()
		defer w.c.notifyc.mu.Unlock()
		url = w.c.notifyc.authURL["initech"]
		return url != ""
	})
	return url
}

// A sign-in already running for the portal: only its window is brought forward. No second sign-in starts.
func TestButtonWhileSigningInOnlyBringsTheWindowForward(t *testing.T) {
	w, fake := notifyWorld(t)
	NewNotifyService(w.c).SetReady(true)
	url := startRunningSignIn(t, w)
	win := &fakeWindow{url: url}
	w.c.signInMu.Lock()
	w.c.signInWin[url] = []signInWindow{win}
	w.c.signInMu.Unlock()
	w.c.emit(api.EventSessionEnded, endedStatus("initech"))
	waitCond(t, func() bool { return len(fake.Sent()) == 1 })
	_ = fake.Respond("signin:initech", notify.ButtonID)
	waitCond(t, func() bool { return win.focusCount() == 1 })
	time.Sleep(50 * time.Millisecond)
	if w.oidc.RegCount() != 1 || win.focusCount() != 1 || len(navigations(w)) != 0 {
		t.Fatalf("a second sign-in started: %d registrations, %d focus calls, %+v", w.oidc.RegCount(), win.focusCount(), navigations(w))
	}
}

type noFocusWindow struct{}

func (noFocusWindow) Close(context.Context) error { return nil }

// A running sign-in whose window cannot be focused (a normal or private browser window, or an isolated browser
// without a focus call) has its page opened again in front, like "Open again"; no new sign-in starts.
func TestButtonWhileSigningInOpensThePageAgainWhenNothingCanBeFocused(t *testing.T) {
	w, fake := notifyWorld(t)
	NewNotifyService(w.c).SetReady(true)
	url := startRunningSignIn(t, w)
	w.c.signInMu.Lock()
	w.c.signInWin[url] = []signInWindow{noFocusWindow{}}
	w.c.signInMu.Unlock()
	w.rt.mu.Lock()
	before := len(w.rt.urls)
	w.rt.mu.Unlock()
	w.c.emit(api.EventSessionEnded, endedStatus("initech"))
	waitCond(t, func() bool { return len(fake.Sent()) == 1 })
	_ = fake.Respond("signin:initech", notify.ButtonID)
	waitCond(t, func() bool { w.rt.mu.Lock(); defer w.rt.mu.Unlock(); return len(w.rt.urls) == before+1 })
	time.Sleep(50 * time.Millisecond)
	w.rt.mu.Lock()
	got, last := len(w.rt.urls), w.rt.urls[len(w.rt.urls)-1]
	w.rt.mu.Unlock()
	if got != before+1 || last != url || w.oidc.RegCount() != 1 {
		t.Fatalf("%d pages, last %q, %d registrations", got, last, w.oidc.RegCount())
	}
}

// The button texts follow the language; a language change registers them again.
func TestLanguageChangeRegistersTheButtonTextsAgain(t *testing.T) {
	w, fake := notifyWorld(t)
	if regs := fake.Registrations(); len(regs) != 2 {
		t.Fatalf("%+v", regs)
	}
	if err := w.sys.SetUILanguage("zh-CN"); err != nil {
		t.Fatal(err)
	}
	regs := fake.Registrations()
	if len(regs) != 4 || regs[3].Button != "重新登录" {
		t.Fatalf("%+v", regs)
	}
	w.c.emit(api.EventSessionEnded, endedStatus("initech"))
	waitCond(t, func() bool { return len(fake.Sent()) == 1 })
	if !strings.HasPrefix(fake.Sent()[0].Title, "登录已过期") {
		t.Fatal(fake.Sent()[0].Title)
	}
}

// The first successful sign-in that started with the window in front asks the system for permission, in the
// background, once.
func TestFirstSignInAsksForPermissionOnce(t *testing.T) {
	w, fake := notifyWorld(t)
	fake.SetPermission(notify.Permission{Allowed: false, CanTellOff: true})
	if st, _ := NewNotifyService(w.c).GetState(); st.Permission != api.NotifyNotAsked {
		t.Fatalf("%+v", st)
	}
	login := func() {
		if _, err := w.c.Accounts.Login(context.Background(), "initech", api.LoginModeBrowser); err != nil {
			t.Fatal(err)
		}
	}
	login()
	waitCond(t, func() bool { n, _ := fake.Counts(); return n == 1 })
	login()
	time.Sleep(50 * time.Millisecond)
	if n, _ := fake.Counts(); n != 1 {
		t.Fatalf("asked %d times", n)
	}
	waitCond(t, func() bool { return w.rt.count(api.EventNotifyState) == 1 })
	st, _ := NewNotifyService(w.c).GetState()
	if st.Permission != api.NotifyAllowed {
		t.Fatalf("%+v", st)
	}
}

// switchingRuntime tells whether the window is in front; the test flips it between the sign-in's start and end.
type switchingRuntime struct {
	*fakeRuntime
	front atomic.Bool
}

func (r *switchingRuntime) WindowFocused() bool { return r.front.Load() }
func (r *switchingRuntime) WindowVisible() bool { return true }

// At the end of a sign-in the browser is in front, not the window: the question goes by the start.
func TestPermissionQuestionGoesByTheWindowAtTheStartOfTheSignIn(t *testing.T) {
	for _, tc := range []struct {
		name       string
		frontStart bool
		asked      int
	}{{"window in front at the start", true, 1}, {"window not in front at the start", false, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			w, fake := notifyWorld(t)
			rt := &switchingRuntime{fakeRuntime: w.rt}
			w.c.RT = rt
			fake.SetPermission(notify.Permission{Allowed: false, CanTellOff: true})
			log := noticeLog{w.c}
			rt.front.Store(tc.frontStart)
			log.SignInStarting("initech")
			rt.front.Store(!tc.frontStart) // the browser took the front while the person approved
			log.SignedIn("initech")
			time.Sleep(100 * time.Millisecond)
			if n, _ := fake.Counts(); n != tc.asked {
				t.Fatalf("asked %d times, want %d", n, tc.asked)
			}
		})
	}
}

// The permission row: the state, the question and the system settings.
func TestSettingsRowCalls(t *testing.T) {
	w, fake := notifyWorld(t)
	svc := NewNotifyService(w.c)
	fake.SetPermission(notify.Permission{Allowed: false, CanTellOff: true})
	if st, _ := svc.GetState(); st.Permission != api.NotifyNotAsked || !st.Available {
		t.Fatalf("%+v", st)
	}
	if _, err := svc.RequestPermission(); err != nil {
		t.Fatal(err)
	}
	waitCond(t, func() bool { n, _ := fake.Counts(); return n == 1 })
	fake.SetPermission(notify.Permission{Allowed: false, CanTellOff: true}) // the person said no
	waitCond(t, func() bool { return w.rt.count(api.EventNotifyState) >= 1 })
	if st, _ := svc.GetState(); st.Permission != api.NotifyOff {
		t.Fatalf("asked and refused is off: %+v", st)
	}
	if err := svc.OpenSystemSettings(); err != nil {
		t.Fatal(err)
	}
	if _, n := fake.Counts(); n != 1 {
		t.Fatal("system settings")
	}
	// off while the window is visible: the notice appears as a toast
	w.c.emit(api.EventSessionEnded, endedStatus("initech"))
	waitCond(t, func() bool { return len(toasts(w)) == 1 })
	if len(fake.Sent()) != 0 {
		t.Fatal("not sent to a system that refuses")
	}
}

// A damaged or newer notifications.json never stops the app or the sign-in loop.
func TestDamagedNotificationsFileDoesNotStopTheApp(t *testing.T) {
	for name, body := range map[string]string{"damaged": "{{{{ not json", "newer": `{"schemaVersion": 99}`} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			file := notificationsFile(w.p)
			if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			c, err := NewContainer(w.p, w.rt, ContainerOptions{
				NotifyBackend: notify.NewFake(), TickInterval: time.Hour, ScanDelay: time.Hour, DisableLoginShell: true,
				Endpoints: Endpoints{OIDC: w.oidc.URL, Portal: w.por.URL, STS: w.aws.URL, IAM: w.aws.URL, SignIn: w.aws.URL},
			})
			if err != nil {
				t.Fatalf("a bad notifications.json must not stop the app: %v", err)
			}
			defer c.Shutdown()
			c.Accounts.Tick(context.Background()) // the sign-in loop runs
			if _, err := c.Accounts.Login(context.Background(), "initech", api.LoginModeBrowser); err != nil {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(file)
			matches, _ := filepath.Glob(file + ".corrupt-*")
			switch name {
			case "damaged":
				if len(matches) != 1 {
					t.Fatalf("moved aside: %v", matches)
				}
			case "newer":
				if string(b) != body {
					t.Fatalf("a newer file is never rewritten: %s", b)
				}
			}
		})
	}
}
