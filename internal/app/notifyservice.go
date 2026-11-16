package app

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/codepass/internal/accounts"
	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/notify"
	"github.com/rudian/codepass/internal/paths"
)

// System notifications (plan 4.15, F1-22). internal/notify owns the system side; this file connects it to the
// accounts engine (the heads-up and "ended" events, the sign-in and sign-out hooks) and routes a click on a
// sign-in notice to the sign-in or to the Accounts page.

// NotifyService is the bound Wails service the window uses for the permission row of Settings, the toasts that
// stand in for notifications, and the startup gate of clicks that launched the app.
type NotifyService struct{ c *Container }

// NewNotifyService wires the service to the container. It must be the first service in the application's list,
// so the system service has started (and its delegate is set) before the accounts ticker can send anything.
func NewNotifyService(c *Container) *NotifyService { return &NotifyService{c: c} }

// ServiceStartup starts the system notification service inside internal/notify. A start failure is logged there;
// the app goes on with toasts in the window only, so this never returns an error.
func (s *NotifyService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.c.Notify.Start(ctx)
	return nil
}

// ServiceShutdown stops the system service. Delivered notifications stay in the notification centre.
func (s *NotifyService) ServiceShutdown() error {
	s.c.Notify.Shutdown()
	return nil
}

// GetState is the permission state for Settings.
func (s *NotifyService) GetState() (api.NotifyState, error) { return s.c.Notify.State(), nil }

// RequestPermission asks the system for permission in the background ("Allow notifications") and returns the
// state at that moment; the answer arrives as the notify:state event.
func (s *NotifyService) RequestPermission() (api.NotifyState, error) {
	s.c.Notify.Request()
	return s.c.Notify.State(), nil
}

// OpenSystemSettings opens the system's notification settings ("Open System Settings").
func (s *NotifyService) OpenSystemSettings() error { return fail(s.c.Notify.OpenSettings()) }

// RespondToast routes a press on a notification toast (the body or its button) like a press on the system's
// notification.
func (s *NotifyService) RespondToast(id string, action api.NotifyAction) error {
	s.c.Notify.RespondToast(id, action)
	return nil
}

// SetReady tells the app that the window has loaded the accounts and passed the startup gates (open = true), or
// that a gate blocks (open = false). Clicks that arrived before that, such as the one that launched the app,
// are handled when it opens and dropped when it blocks.
func (s *NotifyService) SetReady(open bool) error {
	s.c.setNotifyGate(open)
	return nil
}

// ---- the container's side ------------------------------------------------------------------------------------

type gateState int

const (
	gateUnknown gateState = iota // the window has not reported yet: clicks wait
	gateOpen
	gateBlocked
)

// maxPendingClicks bounds the clicks kept for the window (one per portal and action is all that matters).
const maxPendingClicks = 16

// notifyCtl is the container's notification state.
type notifyCtl struct {
	mu      sync.Mutex
	gate    gateState
	pending []notify.Click
	authURL map[string]string // portal -> the authorize URL of the sign-in under way (to find its window)
	front   map[string]bool   // portal -> the window was in front when its sign-in started
}

// notifyBackend picks the system's service or the fake (see notify.UseReal).
func (c *Container) notifyBackend(o ContainerOptions) (b notify.Backend, fake bool) {
	if o.NotifyBackend != nil {
		return o.NotifyBackend, false
	}
	if notify.UseReal(c.Paths.Sandbox) {
		return notify.NewSystem(), false
	}
	return notify.NewFake(), true
}

// windowVisible reports whether the window can be seen (a runtime that cannot tell counts as visible).
func (c *Container) windowVisible() bool {
	if v, ok := c.RT.(interface{ WindowVisible() bool }); ok {
		return v.WindowVisible()
	}
	return true
}

// windowFocused reports whether the window is in front.
func (c *Container) windowFocused() bool {
	if v, ok := c.RT.(interface{ WindowFocused() bool }); ok {
		return v.WindowFocused()
	}
	return true
}

// initNotify builds the notification service and its notice log. It runs before the accounts engine is built,
// because the engine is given the log.
func (c *Container) initNotify(o ContainerOptions) {
	b, fake := c.notifyBackend(o)
	if fake {
		c.Log.Info("system notifications are off in this build; notices appear in the window", "sandbox", c.Paths.Sandbox)
	}
	c.notifyc = &notifyCtl{authURL: map[string]string{}, front: map[string]bool{}}
	c.Notify = notify.New(notify.Options{
		Backend:   b,
		Store:     notify.OpenStore(notificationsFile(c.Paths), o.Now, c.Log),
		Log:       c.Log,
		Lang:      c.Settings.UILanguage,
		Toast:     func(t api.NotifyToast) { c.emit(api.EventNotifyToast, t) },
		Visible:   c.windowVisible,
		AlsoToast: fake || o.NotifyAlsoToast,
		State:     func(s api.NotifyState) { c.emit(api.EventNotifyState, s) },
		Go:        c.spawn,
	})
	c.Notify.RegisterKind(notify.KindSignInHeadsUp, notify.KindSpec{Button: notify.SignInButton, OnClick: c.onSignInClick})
	c.Notify.RegisterKind(notify.KindSignInEnded, notify.KindSpec{Button: notify.SignInButton, OnClick: c.onSignInClick})
	c.OnUILanguage(func(string) { c.Notify.LanguageChanged() })
}

// notificationsFile is the path of notifications.json, next to settings.json.
func notificationsFile(p paths.Paths) string {
	return filepath.Join(filepath.Dir(p.SettingsFile), "notifications.json")
}

// wireNotify subscribes to the engine's events. It runs after the engines exist.
func (c *Container) wireNotify() {
	c.Subscribe(func(name string, data any) {
		switch name {
		case api.EventSessionReminder:
			if st, ok := data.(api.SessionStatus); ok {
				c.sendAsync(func() { c.sendSignIn(notify.KindSignInHeadsUp, st) })
			}
		case api.EventSessionEnded:
			if st, ok := data.(api.SessionStatus); ok {
				c.sendAsync(func() { c.sendSignIn(notify.KindSignInEnded, st) })
			}
		case api.EventLoginProgress:
			if p, ok := data.(api.LoginProgress); ok {
				u := p.AuthorizeURL // the page the sign-in opened: the browser flow's, or the device flow's
				if u == "" {
					u = p.VerificationURIComplete
				}
				if u != "" {
					c.notifyc.mu.Lock()
					c.notifyc.authURL[p.Portal] = u
					c.notifyc.mu.Unlock()
				}
			}
		}
	})
}

// sendAsync runs a send away from the engine's ticker: the system can take a moment to answer.
func (c *Container) sendAsync(f func()) {
	c.lifeMu.Lock()
	closed := c.closed
	c.lifeMu.Unlock()
	if closed {
		return
	}
	c.spawn(f)
}

// signInInfo gathers what a sign-in notice names for a portal.
func (c *Container) signInInfo(st api.SessionStatus) (notify.SignInInfo, bool) {
	in := notify.SignInInfo{Portal: st.Portal, LeadMinutes: int(c.Settings.ReminderLead() / time.Minute), SignInTime: st.LoginAt}
	// The Identity Center user is not known to the app yet: User stays empty and the text leaves it out.
	startURL, accts, ok := c.Accounts.NoticeInfo(st.Portal)
	if !ok {
		return in, false
	}
	in.StartURL = startURL
	for _, a := range accts {
		in.Accounts = append(in.Accounts, notify.Account{Name: a.Name, AccountID: a.AccountID, Role: a.Role})
	}
	return in, true
}

func (c *Container) sendSignIn(kind notify.Kind, st api.SessionStatus) {
	lang := c.Settings.UILanguage()
	in, ok := c.signInInfo(st)
	if !ok {
		return // a portal that is gone, or a legacy pseudo-portal (named after its start URL): nothing to say
	}
	var n notify.Notice
	if kind == notify.KindSignInEnded {
		n = notify.SignInEndedNotice(lang, in)
	} else {
		n = notify.SignInHeadsUpNotice(lang, in)
	}
	c.Notify.Send(n)
}

// noticeLog is the accounts engine's NoticeLog: notifications.json for the keys, the notification service for
// the delivered notice and the permission question.
type noticeLog struct{ c *Container }

func (l noticeLog) Last(portal, kind string) string { return l.c.Notify.Store().Last(portal, kind) }
func (l noticeLog) Mark(portal, kind, key string)   { l.c.Notify.Store().Mark(portal, kind, key) }

// SignInStarting notes whether the window is in front as the person starts the sign-in. At the end of the
// sign-in the browser is the front app (the person has just approved there), so the question of permission
// goes by the start.
func (l noticeLog) SignInStarting(portal string) {
	front := l.c.windowFocused()
	n := l.c.notifyc
	n.mu.Lock()
	n.front[portal] = front
	n.mu.Unlock()
}

// SignedIn: the portal's record starts over, its delivered notice goes, and the first time ever, for a sign-in
// that started with the window in front, the system is asked for permission. Both the removal and the question
// run in the background: the sign-in never waits for the notification centre.
func (l noticeLog) SignedIn(portal string) {
	l.c.Notify.Store().Reset(portal)
	n := l.c.notifyc
	n.mu.Lock()
	front := n.front[portal]
	delete(n.front, portal)
	n.mu.Unlock()
	l.c.sendAsync(func() {
		l.c.Notify.RemoveDelivered(notify.SignInID(portal))
		if front {
			l.c.Notify.AskOnce()
		}
	})
}

// SignedOut: nothing is sent for a sign-out, and a delivered heads-up is taken back.
func (l noticeLog) SignedOut(portal string) {
	l.c.Notify.Store().Reset(portal)
	l.c.sendAsync(func() { l.c.Notify.RemoveDelivered(notify.SignInID(portal)) })
}

var _ accounts.NoticeLog = noticeLog{}

// ---- clicks ----------------------------------------------------------------------------------------------------

// setNotifyGate is NotifyService.SetReady.
func (c *Container) setNotifyGate(open bool) {
	n := c.notifyc
	n.mu.Lock()
	var run []notify.Click
	if open {
		n.gate = gateOpen
		run, n.pending = n.pending, nil
	} else {
		n.gate = gateBlocked
		n.pending = nil // a gate blocks: the click is dropped
	}
	n.mu.Unlock()
	for _, k := range run {
		c.handleSignInClick(k)
	}
}

// onSignInClick receives every press on a sign-in notice or its button.
func (c *Container) onSignInClick(k notify.Click) {
	n := c.notifyc
	n.mu.Lock()
	switch n.gate {
	case gateUnknown:
		// the click that launched the app arrives before the window has loaded; keep the newest few
		if len(n.pending) >= maxPendingClicks {
			n.pending = n.pending[1:]
		}
		n.pending = append(n.pending, k)
		n.mu.Unlock()
		return
	case gateBlocked:
		n.mu.Unlock()
		return
	}
	n.mu.Unlock()
	c.handleSignInClick(k)
}

// showPortal brings the window forward and opens Accounts at the portal.
func (c *Container) showPortal(portal, action string) {
	c.RT.ShowWindow()
	req := api.NavigateRequest{Page: "accounts"}
	if portal != "" {
		req.Action, req.Arg = action, portal
	}
	c.emit(api.EventNavigate, req)
}

func (c *Container) handleSignInClick(k notify.Click) {
	if c.ctx.Err() != nil {
		return // the app is quitting: a click does nothing
	}
	portal := k.Data[notify.DataPortal]
	st, err := c.Accounts.SessionStatus(portal)
	if portal == "" || err != nil {
		// the portal no longer exists: the Accounts page opens with nothing selected
		c.showPortal("", "")
		return
	}
	if k.Action != api.NotifyActionButton {
		c.showPortal(portal, "show") // a click on the notification itself
		return
	}
	if staleSignInClick(k, st) {
		c.showPortal(portal, "signed-in") // "Already signed in"
		return
	}
	if st.State == api.SessionSigningIn {
		c.spawn(func() { c.focusSignIn(portal) }) // it can wait for a browser: never inside the system's callback
		return
	}
	// The window starts the sign-in, as the pass's button does: the login modal opens the isolated window with
	// its progress, Cancel and Open again, the device fallback, and the toasts of the result. Clicks are handled
	// only once the window has reported ready, so it is there to take this.
	c.emit(api.EventNavigate, api.NavigateRequest{Page: "accounts", Action: "login", Arg: portal})
}

// staleSignInClick: the button of a notice that is out of date. "Signed in again" means the portal's current
// sign-in is newer than the one the notice was sent for. The "ended" notice is out of date once the portal is
// signed in at all (it was sent when the sign-in was over); the heads-up button, pressed while the same
// sign-in still runs, is not: signing in again before the end is its point.
func staleSignInClick(k notify.Click, st api.SessionStatus) bool {
	live := st.State == api.SessionActive || st.State == api.SessionRefreshable || st.State == api.SessionExpiring
	if k.Kind == notify.KindSignInEnded {
		return live
	}
	at, err1 := time.Parse(time.RFC3339, k.Data[notify.DataTime])
	cur, err2 := time.Parse(time.RFC3339, st.LoginAt)
	return err1 == nil && err2 == nil && cur.After(at)
}

// signInFocuser is implemented by an isolated browser window that can come to the front.
type signInFocuser interface {
	Focus(ctx context.Context) error
}

// focusSignIn brings the browser window of the portal's running sign-in forward; it never starts a second
// sign-in. A page that has no window to focus (a private or normal browser window, or an isolated browser that
// cannot come forward) is opened again the way "Open again" does it: the same authorize URL, in front, with no
// new sign-in (it does nothing while the app quits).
func (c *Container) focusSignIn(portal string) {
	c.notifyc.mu.Lock()
	url := c.notifyc.authURL[portal]
	c.notifyc.mu.Unlock()
	if url == "" {
		return
	}
	c.signInMu.Lock()
	ws := append([]signInWindow(nil), c.signInWin[url]...)
	c.signInMu.Unlock()
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()
	for _, w := range ws {
		if f, ok := w.(signInFocuser); ok {
			if err := f.Focus(ctx); err == nil {
				return
			} else {
				c.Log.Warn("cannot bring the sign-in window forward", "error", err.Error())
			}
		}
	}
	if _, err := c.openSignIn(url); err != nil && c.ctx.Err() == nil {
		c.Log.Warn("cannot open the sign-in page again", "error", err.Error())
	}
}
