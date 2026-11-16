package accounts

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/sso"
)

var recMu sync.Mutex // serialises read-modify-write of session records

// How the app learns when an IAM Identity Center session ends. AWS never tells apps the session length an
// administrator set, but it never issues an access token that outlives the session: a token normally lives
// one hour, and a token issued in the session's last hour is cut short to the session's end. So a token
// that does not get the full hour shows exactly when the session ends. probeDelay after a sign-in by this
// app, the ticker renews once to catch sessions of an hour or less right away.
const (
	fullGrant  = time.Hour
	grantSlack = 30 * time.Second
	probeDelay = 2 * time.Minute
)

// cutShort reports whether AWS gave a newly issued access token less than the full hour.
func cutShort(granted time.Duration) bool { return granted > 0 && granted < fullGrant-grantSlack }

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func normMinutes(m int) int {
	if m < minSessionMinutes || m > maxSessionMinutes {
		return defaultSessionMinutes
	}
	return m
}

// sessionEnd returns when the session of the current sign-in ends and whether that is exact: the end AWS
// showed (record SessionEndsAt), else LoginAt plus the learned (or default) length, but never before the
// access token expires. A token that expires clearly after the recorded end (renewed elsewhere after a
// refusal) proves that end wrong; a second or so is clock rounding (the AWS CLI times tokens itself).
func sessionEnd(rec api.SessionRecord, loginAt time.Time, minutes int, tokenExpires time.Time) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, rec.SessionEndsAt); err == nil && t.After(loginAt) && !tokenExpires.After(t.Add(grantSlack)) {
		if tokenExpires.After(t) {
			t = tokenExpires
		}
		return t, true
	}
	ends := loginAt.Add(time.Duration(minutes) * time.Minute)
	if tokenExpires.After(ends) {
		ends = tokenExpires
	}
	return ends, false
}

// setSessionEnd stores the exact end of the current sign-in's session. When that sign-in started a new
// session, the portal's session length is learned from it for the next sign-ins.
func setSessionEnd(r *api.SessionRecord, end time.Time) {
	r.SessionEndsAt = fmtTime(end)
	if !r.NewSession {
		return
	}
	loginAt, err := time.Parse(time.RFC3339, r.LoginAt)
	if err != nil {
		return
	}
	if m := int(math.Round(end.Sub(loginAt).Minutes())); m >= minSessionMinutes && m <= maxSessionMinutes {
		r.SessionMinutes = m
	}
}

// sessionStatusFor implements the user-facing session state.
func (e *Engine) sessionStatusFor(pd portalDef, now time.Time) api.SessionStatus {
	raw := e.d.Sessions.Status(pd.sess)
	rec, _ := e.d.Store.SessionRecord(pd.name)
	minutes := normMinutes(rec.SessionMinutes)
	out := api.SessionStatus{
		Portal: pd.name, State: raw.State, SessionMinutes: minutes, CanRefresh: raw.CanRefresh,
		Refreshing: raw.Refreshing, Error: raw.Err, IssuedAt: rec.TokenIssuedAt,
	}
	if !raw.ExpiresAt.IsZero() {
		out.ExpiresAt = fmtTime(raw.ExpiresAt)
	}
	switch raw.State {
	case api.SessionSignedOut, api.SessionError, api.SessionSigningIn:
		return out
	}

	var left, total time.Duration
	if rec.LoginAt != "" && rec.ClientIDHash != "" && rec.ClientIDHash == raw.ClientIDHash {
		if loginAt, err := time.Parse(time.RFC3339, rec.LoginAt); err == nil {
			ends, exact := sessionEnd(rec, loginAt, minutes, raw.ExpiresAt)
			total = ends.Sub(loginAt)
			out.SessionKnown, out.SessionEndExact = true, exact
			out.LoginAt, out.SessionEndsAt = fmtTime(loginAt), fmtTime(ends)
			out.SessionMinutes = max(1, int(math.Round(total.Minutes())))
			left = ends.Sub(now)
		}
	}
	if out.SessionEndExact && left <= 0 {
		// AWS showed that the session ended: a renewal can only be refused now
		out.State, out.CanRefresh = api.SessionExpired, false
		return out
	}
	if raw.State == api.SessionExpired {
		out.State = api.SessionExpired
		return out
	}
	refreshableNow := raw.State == api.SessionRefreshable
	activeOrRefreshable := api.SessionActive
	if refreshableNow {
		activeOrRefreshable = api.SessionRefreshable
	}
	if out.SessionKnown {
		switch {
		case left >= expiringWindow:
			out.State = activeOrRefreshable
		case left > 0:
			out.State = api.SessionExpiring
		default:
			out.State = activeOrRefreshable
			out.SessionOverrun = true
		}
		if !out.SessionOverrun && total > 0 {
			pct := int(math.Round(float64(left) / float64(total) * 100))
			if pct < 0 {
				pct = 0
			}
			if pct > 100 {
				pct = 100
			}
			out.ProgressPct = pct
		}
		return out
	}
	switch {
	case refreshableNow:
		out.State = api.SessionRefreshable
	case raw.State == api.SessionExpiring && !raw.CanRefresh:
		out.State = api.SessionExpiring
	default:
		out.State = api.SessionActive
	}
	return out
}

func sigOf(s api.SessionStatus, now time.Time) string {
	code := ""
	if s.Error != nil {
		code = string(s.Error.Code)
	}
	low := false
	if s.SessionKnown && s.SessionEndsAt != "" {
		if t, err := time.Parse(time.RFC3339, s.SessionEndsAt); err == nil {
			low = t.Sub(now) < expiringWindow
		}
	}
	return fmt.Sprintf("%s|%v|%v|%d|%v|%s|%v|%v|%v|%s", s.State, s.Refreshing, s.SessionKnown, s.SessionMinutes, s.SessionOverrun, code, low, s.CanRefresh,
		s.SessionEndExact, s.SessionEndsAt)
}

func (e *Engine) seedSig(portal string, s api.SessionStatus) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.lastSig[portal]; !ok {
		e.lastSig[portal] = sigOf(s, e.d.Now())
	}
}

func (e *Engine) setSig(portal string, s api.SessionStatus) {
	e.mu.Lock()
	e.lastSig[portal] = sigOf(s, e.d.Now())
	e.mu.Unlock()
}

// SessionStatus returns the user-facing session status of one portal.
func (e *Engine) SessionStatus(portal string) (api.SessionStatus, error) {
	snap, err := e.Snapshot()
	if err != nil {
		return api.SessionStatus{}, err
	}
	pd, ok := e.portalDef(snap, portal)
	if !ok {
		return api.SessionStatus{}, api.NewError(api.ErrPortalNotFound, "portal not found").WithParam("portal", portal)
	}
	return e.sessionStatusFor(pd, e.d.Now()), nil
}

func (e *Engine) updateRecord(portal string, fn func(r *api.SessionRecord)) {
	recMu.Lock()
	defer recMu.Unlock()
	r, _ := e.d.Store.SessionRecord(portal)
	fn(&r)
	_ = e.d.Store.SetSessionRecord(portal, r)
}

func (e *Engine) publishSession(pd portalDef) api.SessionStatus {
	st := e.sessionStatusFor(pd, e.d.Now())
	e.setSig(pd.name, st)
	e.emit(api.EventSessionChanged, st)
	return st
}

// Login runs the interactive sign-in and records the session lineage (login time, client id hash).
func (e *Engine) Login(ctx context.Context, portal string, mode api.LoginMode) (api.LoginResult, error) {
	snap, err := e.Snapshot()
	if err != nil {
		return api.LoginResult{}, err
	}
	pd, ok := e.portalDef(snap, portal)
	if !ok {
		return api.LoginResult{}, api.NewError(api.ErrPortalNotFound, "portal not found").WithParam("portal", portal)
	}
	isolated := false // the sign-in page opened in a fresh browser of its own
	progress := func(p api.LoginProgress) {
		p.Portal = portal
		isolated = isolated || p.IsolatedWindow
		e.emit(api.EventLoginProgress, p)
		if p.Phase == api.LoginPhaseStarting {
			e.publishSession(pd)
		}
	}
	tok, lerr := e.d.Sessions.Login(ctx, pd.sess, mode, progress)
	if lerr != nil {
		if !isInProgress(lerr) {
			e.publishSession(pd)
		}
		return api.LoginResult{}, sso.ToAppError(lerr, portal)
	}
	now := e.d.Now()
	// The session ends when the first token does if this app cannot renew it, or if AWS already cut it short.
	renewable := e.d.Sessions.Status(pd.sess).CanRefresh
	endsNow := !renewable || cutShort(tok.Granted)
	e.updateRecord(portal, func(r *api.SessionRecord) {
		r.LoginAt = fmtTime(now)
		r.ClientIDHash = sso.ClientIDHash(tok.ClientID)
		r.TokenIssuedAt = fmtTime(now)
		r.SessionEndsAt = ""
		// Only a sign-in in a fresh browser of its own surely started a new session (a browser that was still
		// signed in hands out its running session, and a full first token does not tell that apart while the
		// session has more than an hour left). Only then is the length learned.
		r.NewSession = isolated && !cutShort(tok.Granted)
		if endsNow {
			setSessionEnd(r, tok.ExpiresAt)
		}
	})
	e.mu.Lock()
	delete(e.reminded, portal)
	if !endsNow {
		e.probes[portal] = fmtTime(now)
	} else {
		delete(e.probes, portal)
	}
	e.mu.Unlock()
	e.publishSession(pd)
	e.EmitState(ctx)
	return api.LoginResult{Portal: portal, ExpiresAt: fmtTime(tok.ExpiresAt), Profiles: nonNil(pd.profiles)}, nil
}

func isInProgress(err error) bool { return sso.ToAppError(err, "").Code == api.ErrLoginInProgress }

// Refresh renews the access token now (the UI's "renew" action). LoginAt is never touched.
func (e *Engine) Refresh(ctx context.Context, portal string) (api.SessionStatus, error) {
	snap, err := e.Snapshot()
	if err != nil {
		return api.SessionStatus{}, err
	}
	pd, ok := e.portalDef(snap, portal)
	if !ok {
		return api.SessionStatus{}, api.NewError(api.ErrPortalNotFound, "portal not found").WithParam("portal", portal)
	}
	tok, rerr := e.d.Sessions.Refresh(ctx, pd.sess)
	if rerr != nil {
		if errors.Is(rerr, sso.ErrInvalidGrant) {
			e.noteRefusal(pd, e.d.Now())
		}
		e.publishSession(pd)
		return api.SessionStatus{}, sso.ToAppError(rerr, portal)
	}
	e.noteRenewal(portal, tok, e.d.Now())
	st := e.publishSession(pd)
	e.emit(api.EventSessionRenewed, st)
	e.EmitState(ctx)
	return st, nil
}

// Logout signs one portal out natively: clears LoginAt/ClientIDHash, keeps the session length.
func (e *Engine) Logout(ctx context.Context, portal string) (api.LogoutResult, error) {
	snap, err := e.Snapshot()
	if err != nil {
		return api.LogoutResult{}, err
	}
	pd, ok := e.portalDef(snap, portal)
	if !ok {
		return api.LogoutResult{}, api.NewError(api.ErrPortalNotFound, "portal not found").WithParam("portal", portal)
	}
	var roles []sso.AccountRole
	for _, name := range pd.profiles {
		if p, ok := snap.Profile(name); ok && p.AccountID != "" && p.Role != "" {
			roles = append(roles, sso.AccountRole{AccountID: p.AccountID, Role: p.Role})
		}
	}
	n, server, lerr := e.d.Sessions.Logout(ctx, pd.sess, roles)
	if lerr != nil {
		return api.LogoutResult{}, sso.ToAppError(lerr, portal)
	}
	e.updateRecord(portal, func(r *api.SessionRecord) {
		*r = api.SessionRecord{SessionMinutes: r.SessionMinutes}
	})
	e.mu.Lock()
	delete(e.reminded, portal)
	e.mu.Unlock()
	e.publishSession(pd)
	e.EmitState(ctx)
	return api.LogoutResult{Portal: portal, DeletedRoleCaches: n, ServerLogout: server}, nil
}

// noteRenewal records a renewal by this app. A token that no longer gets the full hour shows where the
// session ends (see fullGrant).
func (e *Engine) noteRenewal(portal string, tok *sso.CachedToken, now time.Time) {
	hash := sso.ClientIDHash(tok.ClientID)
	e.updateRecord(portal, func(r *api.SessionRecord) {
		r.TokenIssuedAt = fmtTime(now)
		if cutShort(tok.Granted) && r.LoginAt != "" && r.ClientIDHash == hash {
			setSessionEnd(r, tok.ExpiresAt)
		}
	})
}

// noteRefusal records a renewal AWS refused. While the access token is still valid, the session ends when it
// expires. No length is learned from it: a refusal can also mean that the session was signed out or revoked.
func (e *Engine) noteRefusal(pd portalDef, now time.Time) {
	raw := e.d.Sessions.Status(pd.sess)
	if !raw.ExpiresAt.After(now) {
		return
	}
	e.updateRecord(pd.name, func(r *api.SessionRecord) {
		// no more renewals: the session ends with this token, whatever was recorded before
		if r.LoginAt != "" && r.ClientIDHash == raw.ClientIDHash {
			r.SessionEndsAt = fmtTime(raw.ExpiresAt)
		}
	})
}

// probeDue reports whether the check renewal after this app's sign-in (probeDelay later) is due for the
// sign-in that is still current. It shows a session of an hour or less right away.
func (e *Engine) probeDue(portal string, st api.SessionStatus, now time.Time) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	at, ok := e.probes[portal]
	if !ok {
		return false
	}
	if !st.SessionKnown || st.LoginAt != at || st.SessionEndExact {
		delete(e.probes, portal)
		return false
	}
	loginAt, err := time.Parse(time.RFC3339, at)
	return err == nil && !now.Before(loginAt.Add(probeDelay))
}

// Tick is the 30 s ticker body: renew tokens, emit session changes and reminders.
func (e *Engine) Tick(ctx context.Context) {
	snap, err := e.Snapshot()
	if err != nil {
		return
	}
	now := e.d.Now()
	changed := false
	for _, pd := range e.portals(snap) {
		raw := e.d.Sessions.Status(pd.sess)
		st := e.sessionStatusFor(pd, now)

		e.mu.Lock()
		last := e.lastRenewal[pd.name]
		e.mu.Unlock()
		renew := raw.ExpiresAt.Sub(now) < expiringWindow
		if end, err := time.Parse(time.RFC3339, st.SessionEndsAt); err == nil && st.SessionEndExact && !raw.ExpiresAt.Before(end.Add(-grantSlack)) {
			renew = false // the token already lasts until the session ends: a renewal cannot extend it
		}
		probe := !renew && e.probeDue(pd.name, st, now)
		if !pd.legacy && e.d.Store.AutoRenew() && raw.CanRefresh && (renew || probe) &&
			now.Sub(last) > 60*time.Second && ctx.Err() == nil {
			e.mu.Lock()
			e.lastRenewal[pd.name] = now
			e.mu.Unlock()
			if !probe { // the probe is a quiet check, not a renewal the user needs to see
				busy := st
				busy.Refreshing = true
				e.emit(api.EventSessionChanged, busy)
			}
			tok, rerr := e.d.Sessions.Refresh(ctx, pd.sess)
			if rerr == nil || errors.Is(rerr, sso.ErrInvalidGrant) {
				e.mu.Lock()
				delete(e.probes, pd.name) // a network error leaves the probe for the next tick
				e.mu.Unlock()
			}
			if rerr == nil {
				e.noteRenewal(pd.name, tok, e.d.Now())
			} else if errors.Is(rerr, sso.ErrInvalidGrant) {
				e.noteRefusal(pd, e.d.Now())
			}
			after := e.sessionStatusFor(pd, e.d.Now())
			if rerr == nil && !probe {
				e.emit(api.EventSessionRenewed, after)
			}
			e.emit(api.EventSessionChanged, after)
			e.setSig(pd.name, after)
			changed = true
			continue
		}
		e.mu.Lock()
		prev, seen := e.lastSig[pd.name]
		e.mu.Unlock()
		if sig := sigOf(st, now); !seen || sig != prev {
			e.setSig(pd.name, st)
			e.emit(api.EventSessionChanged, st)
			changed = true
		}
		e.maybeRemind(pd.name, st, raw, now)
	}
	if changed {
		e.EmitState(ctx)
	}
}

func (e *Engine) maybeRemind(portal string, st api.SessionStatus, raw sso.Status, now time.Time) {
	if !e.d.Store.ExpiryNotice() {
		return
	}
	lead := e.d.Store.ReminderLead()
	key := ""
	if st.SessionKnown {
		if t, err := time.Parse(time.RFC3339, st.SessionEndsAt); err == nil {
			if left := t.Sub(now); left > 0 && left <= lead {
				key = "L:" + st.LoginAt
			}
		}
	} else if (raw.State == api.SessionActive || raw.State == api.SessionExpiring) && !raw.CanRefresh {
		if left := raw.ExpiresAt.Sub(now); left > 0 && left <= lead {
			key = "E:" + fmtTime(raw.ExpiresAt)
		}
	}
	if key == "" {
		return
	}
	e.mu.Lock()
	dup := e.reminded[portal] == key
	e.reminded[portal] = key
	e.mu.Unlock()
	if !dup {
		e.emit(api.EventSessionReminder, st)
	}
}
