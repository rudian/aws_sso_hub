package notify

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/rudian/codepass/internal/api"
)

type clickLog struct {
	mu sync.Mutex
	c  []Click
}

func (l *clickLog) add(c Click) { l.mu.Lock(); l.c = append(l.c, c); l.mu.Unlock() }
func (l *clickLog) all() []Click {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Click(nil), l.c...)
}

type rig struct {
	t      *testing.T
	fake   *Fake
	svc    *Service
	lang   string
	toasts []api.NotifyToast
	states []api.NotifyState
	hidden bool
	headsC clickLog
	endedC clickLog
	mu     sync.Mutex
}

func newRig(t *testing.T, mut func(*Options)) *rig {
	t.Helper()
	r := &rig{t: t, fake: NewFake(), lang: "en"}
	o := Options{
		Backend: r.fake, Lang: func() string { r.mu.Lock(); defer r.mu.Unlock(); return r.lang },
		Toast:   func(x api.NotifyToast) { r.mu.Lock(); r.toasts = append(r.toasts, x); r.mu.Unlock() },
		State:   func(s api.NotifyState) { r.mu.Lock(); r.states = append(r.states, s); r.mu.Unlock() },
		Visible: func() bool { return !r.hidden },
		Go:      func(f func()) { f() }, // the permission question runs inline in tests
	}
	if mut != nil {
		mut(&o)
	}
	r.svc = New(o)
	r.svc.RegisterKind(KindSignInHeadsUp, KindSpec{Button: SignInButton, OnClick: r.headsC.add})
	r.svc.RegisterKind(KindSignInEnded, KindSpec{Button: SignInButton, OnClick: r.endedC.add})
	return r
}

func (r *rig) setLang(l string) { r.mu.Lock(); r.lang = l; r.mu.Unlock() }
func (r *rig) toastList() []api.NotifyToast {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]api.NotifyToast(nil), r.toasts...)
}

func endedNotice() Notice {
	return SignInEndedNotice("en", SignInInfo{Portal: "acme", Accounts: []Account{{Name: "acme-prod"}}, SignInTime: "2026-11-16T01:00:00Z"})
}

func TestRegistersButtonTextsAtStartAndOnALanguageChange(t *testing.T) {
	r := newRig(t, nil)
	r.svc.Start(context.Background())
	regs := r.fake.Registrations()
	if len(regs) != 2 {
		t.Fatalf("both kinds are registered at start: %+v", regs)
	}
	for _, g := range regs {
		if g.Button != "Sign in again" {
			t.Fatalf("%+v", g)
		}
	}
	r.setLang("zh-CN")
	r.svc.LanguageChanged()
	regs = r.fake.Registrations()
	if len(regs) != 4 || regs[2].Button != "重新登录" || regs[3].Button != "重新登录" {
		t.Fatalf("a language change registers the texts again: %+v", regs)
	}
	// a kind registered after start is registered at once
	r.svc.RegisterKind("later.kind", KindSpec{Button: func(string) string { return "Go" }})
	if regs = r.fake.Registrations(); regs[len(regs)-1].Kind != "later.kind" {
		t.Fatalf("%+v", regs)
	}
}

func TestSendGoesToTheSystemWithItsButtonAndIdentifiersOnly(t *testing.T) {
	r := newRig(t, nil)
	r.svc.Start(context.Background())
	if d := r.svc.Send(endedNotice()); d != DeliveredSystem {
		t.Fatalf("delivery %s", d)
	}
	sent := r.fake.Sent()
	if len(sent) != 1 || !sent[0].HasAction || sent[0].Button != "Sign in again" || sent[0].ID != "signin:acme" || sent[0].Silent {
		t.Fatalf("%+v", sent)
	}
	if len(sent[0].Data) != 3 || sent[0].Data[DataKind] != "signin.ended" || sent[0].Data[DataPortal] != "acme" || sent[0].Data[DataTime] != "2026-11-16T01:00:00Z" {
		t.Fatalf("the data map holds the kind, the portal and the time: %+v", sent[0].Data)
	}
	if len(r.toastList()) != 0 {
		t.Fatal("no toast when the system shows it")
	}
}

func TestClickIsRoutedByKind(t *testing.T) {
	r := newRig(t, nil)
	r.svc.Start(context.Background())
	r.svc.Send(SignInHeadsUpNotice("en", SignInInfo{Portal: "acme", LeadMinutes: 15, SignInTime: "t1"}))
	if err := r.fake.Respond("signin:acme", ButtonID); err != nil {
		t.Fatal(err)
	}
	if got := r.headsC.all(); len(got) != 1 || got[0].Action != api.NotifyActionButton || got[0].Data[DataPortal] != "acme" || len(r.endedC.all()) != 0 {
		t.Fatalf("heads-up button: %+v / %+v", got, r.endedC.all())
	}
	// on macOS "ended" replaces the heads-up under the same id; the click then belongs to the ended kind
	r.svc.Send(endedNotice())
	if err := r.fake.Respond("signin:acme", DefaultActionID); err != nil {
		t.Fatal(err)
	}
	if got := r.endedC.all(); len(got) != 1 || got[0].Action != api.NotifyActionBody || got[0].Kind != KindSignInEnded {
		t.Fatalf("ended body: %+v", got)
	}
	// a response without a category is routed by the kind in its data map (Windows body click)
	if err := r.fake.RespondRaw(Response{ID: "signin:acme", Action: DefaultActionID, Data: map[string]string{DataKind: "signin.headsup", DataPortal: "globex"}}); err != nil {
		t.Fatal(err)
	}
	if got := r.headsC.all(); len(got) != 2 || got[1].Data[DataPortal] != "globex" {
		t.Fatalf("%+v", got)
	}
	// unknown kinds and unknown actions are ignored
	_ = r.fake.RespondRaw(Response{ID: "x", Category: "nobody.kind", Action: ButtonID})
	_ = r.fake.RespondRaw(Response{ID: "x", Category: string(KindSignInEnded), Action: "something.else"})
	if len(r.endedC.all()) != 1 || len(r.headsC.all()) != 2 {
		t.Fatal("ignored clicks reached a handler")
	}
}

func TestAPanickingHandlerDoesNotBreakTheService(t *testing.T) {
	r := newRig(t, nil)
	r.svc.RegisterKind("boom", KindSpec{Button: SignInButton, OnClick: func(Click) { panic("no") }})
	r.svc.Start(context.Background())
	r.svc.Send(Notice{Kind: "boom", ID: "b", Title: "t"})
	if err := r.fake.Respond("b", ButtonID); err != nil {
		t.Fatal(err)
	}
	if r.svc.Send(endedNotice()) != DeliveredSystem {
		t.Fatal("still works")
	}
}

// The framework's service refuses to start without a bundle identifier. The app must go on with toasts only.
func TestAStartFailureLeavesToastsOnly(t *testing.T) {
	r := newRig(t, nil)
	r.fake.SetStartError(errors.New("notifications require a valid bundle identifier"))
	r.svc.Start(context.Background()) // must not panic or return an error
	if r.svc.Available() {
		t.Fatal("not available")
	}
	st := r.svc.State()
	if st.Available || st.Permission != api.NotifyAllowed {
		t.Fatalf("%+v", st)
	}
	if d := r.svc.Send(endedNotice()); d != DeliveredToast {
		t.Fatalf("a toast in the window: %s", d)
	}
	if len(r.fake.Sent()) != 0 || len(r.fake.Registrations()) != 0 {
		t.Fatal("nothing reaches a backend that did not start")
	}
	tt := r.toastList()
	if len(tt) != 1 || tt[0].Button != "Sign in again" || !tt[0].Sticky || tt[0].Tone != api.NotifyToneDanger {
		t.Fatalf("%+v", tt)
	}
	r.hidden = true
	if d := r.svc.Send(endedNotice()); d != DeliveredNone {
		t.Fatalf("a hidden window gets nothing extra: %s", d)
	}
	r.svc.Shutdown() // a service that never started shuts down quietly
}

type panicBackend struct{ *Fake }

func (panicBackend) Start(context.Context) error { panic("cgo blew up") }

func TestAPanicWhileStartingIsCaught(t *testing.T) {
	r := newRig(t, func(o *Options) { o.Backend = panicBackend{NewFake()} })
	r.svc.Start(context.Background())
	if r.svc.Available() {
		t.Fatal()
	}
}

func TestNoBackendMeansToastsOnly(t *testing.T) {
	r := newRig(t, func(o *Options) { o.Backend = nil })
	r.svc.Start(context.Background())
	if d := r.svc.Send(endedNotice()); d != DeliveredToast {
		t.Fatal(d)
	}
	if err := r.svc.OpenSettings(); err != nil {
		t.Fatal(err)
	}
}

func TestPermissionStates(t *testing.T) {
	r := newRig(t, nil)
	r.svc.Start(context.Background())
	if st := r.svc.State(); st.Permission != api.NotifyAllowed || !st.Available {
		t.Fatalf("%+v", st)
	}
	// macOS: not allowed and never asked
	r.fake.SetPermission(Permission{Allowed: false, CanTellOff: true})
	if st := r.svc.State(); st.Permission != api.NotifyNotAsked {
		t.Fatalf("%+v", st)
	}
	// nothing goes to the system, the window shows it
	if d := r.svc.Send(endedNotice()); d != DeliveredToast || len(r.fake.Sent()) != 0 {
		t.Fatalf("%s %d", d, len(r.fake.Sent()))
	}
	// asked and still not allowed: off
	r.svc.Store().SetAsked()
	if st := r.svc.State(); st.Permission != api.NotifyOff {
		t.Fatalf("%+v", st)
	}
	// Windows cannot tell: whatever the backend says, it is "allowed"
	r.fake.SetPermission(Permission{Allowed: false, CanTellOff: false})
	if st := r.svc.State(); st.Permission != api.NotifyAllowed {
		t.Fatalf("%+v", st)
	}
}

func TestSendWhenTheSystemRefusesFallsBackToAToast(t *testing.T) {
	r := newRig(t, nil)
	r.svc.Start(context.Background())
	r.fake.SetSendError(ErrRefused)
	if d := r.svc.Send(endedNotice()); d != DeliveredToast {
		t.Fatal(d)
	}
	r.hidden = true
	if d := r.svc.Send(endedNotice()); d != DeliveredNone {
		t.Fatal(d)
	}
}

func TestAskOnceAsksOnceInTheBackground(t *testing.T) {
	r := newRig(t, nil)
	r.fake.SetPermission(Permission{Allowed: false, CanTellOff: true})
	r.fake.SetRequestAnswer(true)
	r.svc.Start(context.Background())
	r.svc.AskOnce()
	r.svc.AskOnce()
	if n, _ := r.fake.Counts(); n != 1 {
		t.Fatalf("asked %d times", n)
	}
	if !r.svc.Store().Asked() {
		t.Fatal("asked is remembered")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.states) != 1 || r.states[0].Permission != api.NotifyAllowed {
		t.Fatalf("the answer is reported: %+v", r.states)
	}
}

func TestAskOnceNeverAsksWhenAllowedOrAlreadyAsked(t *testing.T) {
	r := newRig(t, nil)
	r.svc.Start(context.Background())
	r.svc.AskOnce() // allowed
	r.fake.SetPermission(Permission{Allowed: false, CanTellOff: true})
	r.svc.Store().SetAsked()
	r.svc.AskOnce() // asked before, turned off: never asks again by itself
	if n, _ := r.fake.Counts(); n != 0 {
		t.Fatalf("asked %d times", n)
	}
}

func TestAskOnceDoesNotBlockTheCaller(t *testing.T) {
	release := make(chan struct{})
	var wg sync.WaitGroup
	r := newRig(t, func(o *Options) {
		o.Go = func(f func()) { wg.Add(1); go func() { defer wg.Done(); <-release; f() }() }
	})
	r.fake.SetPermission(Permission{Allowed: false, CanTellOff: true})
	r.svc.Start(context.Background())
	r.svc.AskOnce() // returns although the question has not been answered
	if n, _ := r.fake.Counts(); n != 0 {
		t.Fatal("the question runs in the background")
	}
	close(release)
	wg.Wait()
	if n, _ := r.fake.Counts(); n != 1 {
		t.Fatal(n)
	}
}

func TestRequestAndOpenSettings(t *testing.T) {
	r := newRig(t, nil)
	r.fake.SetPermission(Permission{Allowed: false, CanTellOff: true})
	r.svc.Start(context.Background())
	r.svc.Request()
	if err := r.svc.OpenSettings(); err != nil {
		t.Fatal(err)
	}
	if req, set := r.fake.Counts(); req != 1 || set != 1 {
		t.Fatal(req, set)
	}
}

func TestSandboxModeAlsoShowsEveryNoticeAsAToast(t *testing.T) {
	r := newRig(t, func(o *Options) { o.AlsoToast = true })
	r.svc.Start(context.Background())
	r.hidden = true // even with a hidden window
	if d := r.svc.Send(endedNotice()); d != DeliveredSystem {
		t.Fatal(d)
	}
	if len(r.toastList()) != 1 || len(r.fake.Sent()) != 1 {
		t.Fatal("both")
	}
}

func TestToastPressRunsTheSameRouting(t *testing.T) {
	r := newRig(t, func(o *Options) { o.AlsoToast = true })
	r.svc.Start(context.Background())
	r.svc.Send(endedNotice())
	tt := r.toastList()
	r.svc.RespondToast(tt[0].ID, api.NotifyActionButton)
	got := r.endedC.all()
	if len(got) != 1 || got[0].Action != api.NotifyActionButton || got[0].Data[DataPortal] != "acme" {
		t.Fatalf("%+v", got)
	}
	r.svc.RespondToast(tt[0].ID, api.NotifyActionButton) // a replayed press does nothing
	r.svc.RespondToast("nothing#1", api.NotifyActionBody)
	r.svc.RespondToast(tt[0].ID, "weird")
	if len(r.endedC.all()) != 1 {
		t.Fatal("replayed or unknown presses are ignored")
	}
}

func TestSendOfAnUnregisteredKindIsDropped(t *testing.T) {
	r := newRig(t, nil)
	r.svc.Start(context.Background())
	if d := r.svc.Send(Notice{Kind: "nobody", ID: "x", Title: "t"}); d != DeliveredNone || len(r.fake.Sent()) != 0 {
		t.Fatal(d)
	}
}

func TestRemoveDeliveredNeedsARunningService(t *testing.T) {
	r := newRig(t, nil)
	r.svc.RemoveDelivered("signin:acme") // before start: nothing
	r.svc.Start(context.Background())
	r.svc.RemoveDelivered("signin:acme")
	if got := r.fake.Removed(); len(got) != 1 || got[0] != "signin:acme" {
		t.Fatalf("%v", got)
	}
}
