package accounts

import (
	"context"
	"time"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/awsconfig"
	"github.com/rudian/aws_sso_hub/internal/sso"
)

// Opening the sign-in page by itself before a session ends (docs/superpowers/specs/2026-11-27-auto-sign-in-design.md).
// At the end of an IAM Identity Center session only a new interactive sign-in helps. So, SignInLead before an
// exact session end, the app starts the browser sign-in of the SSO sign-in button by itself, in the background.
// The browser first shows the app's own page, which names the portal and its profiles. The current sign-in
// keeps working while the page waits, and the SSO sign-in button or Sign out ends the wait.

// minAutoWait is the shortest time a sign-in the app opened by itself waits for the user: the usual sign-in
// timeout. Otherwise it waits until the current session ends.
const minAutoWait = 10 * time.Minute

// autoRun is a sign-in the app opened by itself, while it waits.
type autoRun struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// autoMark is what the app last opened the sign-in page for: the sign-in (LoginAt) and its session end.
type autoMark struct {
	loginAt string
	end     time.Time
}

// autoLogin is the part of a sign-in that only one the app opened by itself has.
type autoLogin struct {
	intro *sso.SignInIntro
	wait  time.Duration
}

// sameEnd reports whether two session ends are the same end (a token can outlive the recorded end by a few
// seconds of clock rounding).
func sameEnd(a, b time.Time) bool { return a.Sub(b).Abs() <= time.Minute }

// autoDueAt returns the session end the app opens the sign-in page of pd for now, if it does: automatic sign-in
// on, a portal with an sso-session, a current sign-in by this app that is still valid, an exact session end
// (an estimate moves with each renewal) at most SignInLead away, and a session longer than SignInLead (it would
// open right after every sign-in).
func (e *Engine) autoDueAt(pd portalDef, st api.SessionStatus, now time.Time) (time.Time, bool) {
	if pd.legacy || pd.draft || pd.sess.Name == "" || !e.d.Store.AutoSignIn() || !st.SessionKnown || !st.SessionEndExact || st.SignInWaiting {
		return time.Time{}, false
	}
	switch st.State {
	case api.SessionActive, api.SessionExpiring, api.SessionRefreshable:
	default:
		return time.Time{}, false
	}
	end, err1 := time.Parse(time.RFC3339, st.SessionEndsAt)
	loginAt, err2 := time.Parse(time.RFC3339, st.LoginAt)
	if err1 != nil || err2 != nil {
		return time.Time{}, false
	}
	lead := e.d.Store.SignInLead()
	if left := end.Sub(now); left <= 0 || left > lead || end.Sub(loginAt) <= lead {
		return time.Time{}, false
	}
	return end, true
}

// claimAuto registers the sign-in the app opens by itself for pd now, if it is due and the app has not opened
// one yet for this sign-in or for this session end (a sign-in that handed back the same session has the same
// end). At most one waits per portal.
func (e *Engine) claimAuto(ctx context.Context, pd portalDef, st api.SessionStatus, now time.Time) (*autoRun, time.Time, bool) {
	end, ok := e.autoDueAt(pd, st, now)
	if !ok {
		return nil, time.Time{}, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.autos[pd.name] != nil {
		return nil, time.Time{}, false
	}
	if m, seen := e.autoDone[pd.name]; seen && (m.loginAt == st.LoginAt || sameEnd(m.end, end)) {
		return nil, time.Time{}, false
	}
	actx, cancel := context.WithCancel(ctx)
	run := &autoRun{ctx: actx, cancel: cancel, done: make(chan struct{})}
	e.autos[pd.name] = run
	e.autoDone[pd.name] = autoMark{loginAt: st.LoginAt, end: end}
	return run, end, true
}

// maybeAutoSignIn opens the sign-in page of pd by itself when it is due (see claimAuto). The sign-in runs in the
// background: Tick goes on.
func (e *Engine) maybeAutoSignIn(ctx context.Context, snap *awsconfig.Snapshot, pd portalDef, st api.SessionStatus, now time.Time) {
	run, end, ok := e.claimAuto(ctx, pd, st, now)
	if !ok {
		return
	}
	auto := &autoLogin{intro: e.introFor(snap, pd, end), wait: max(minAutoWait, end.Sub(now))}
	go func() {
		defer close(run.done)
		defer run.cancel()
		defer func() {
			e.mu.Lock()
			if e.autos[pd.name] == run {
				delete(e.autos, pd.name)
			}
			e.mu.Unlock()
		}()
		_, _ = e.login(run.ctx, pd.name, api.LoginModeBrowser, auto)
	}()
}

// introFor is the intro page of pd: the portal, its profiles with their account colours and monograms, and the
// session end.
func (e *Engine) introFor(snap *awsconfig.Snapshot, pd portalDef, end time.Time) *sso.SignInIntro {
	ids := e.assignIdentities(snap)
	in := &sso.SignInIntro{Portal: pd.name, EndsAt: end}
	for _, n := range pd.profiles {
		p := sso.IntroProfile{Name: n}
		if r, ok := ids.res[n]; ok {
			p.Slot, p.Monogram = r.Slot, r.Monogram
		}
		in.Profiles = append(in.Profiles, p)
	}
	return in
}

// stopAutoSignIn ends the sign-in the app opened by itself for portal, if one waits, and waits (briefly) until
// it has ended, so that a sign-in started next does not find it still running.
func (e *Engine) stopAutoSignIn(portal string) {
	e.mu.Lock()
	run := e.autos[portal]
	e.mu.Unlock()
	if run == nil {
		return
	}
	run.cancel()
	select {
	case <-run.done:
	case <-time.After(5 * time.Second):
	}
}
