package accounts

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/sso"
)

var recMu sync.Mutex // serialises read-modify-write of session records

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func normMinutes(m int) int {
	if m < minSessionMinutes || m > maxSessionMinutes {
		return defaultSessionMinutes
	}
	return m
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

	var left time.Duration
	if rec.LoginAt != "" && rec.ClientIDHash != "" && rec.ClientIDHash == raw.ClientIDHash {
		if loginAt, err := time.Parse(time.RFC3339, rec.LoginAt); err == nil {
			ends := loginAt.Add(time.Duration(minutes) * time.Minute)
			out.SessionKnown = true
			out.LoginAt, out.SessionEndsAt = fmtTime(loginAt), fmtTime(ends)
			left = ends.Sub(now)
		}
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
		if !out.SessionOverrun {
			pct := int(math.Round(float64(left) / float64(time.Duration(minutes)*time.Minute) * 100))
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
	return fmt.Sprintf("%s|%v|%v|%d|%v|%s|%v|%v", s.State, s.Refreshing, s.SessionKnown, s.SessionMinutes, s.SessionOverrun, code, low, s.CanRefresh)
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
	progress := func(p api.LoginProgress) {
		p.Portal = portal
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
	e.updateRecord(portal, func(r *api.SessionRecord) {
		r.LoginAt = fmtTime(now)
		r.ClientIDHash = sso.ClientIDHash(tok.ClientID)
		r.TokenIssuedAt = fmtTime(now)
	})
	e.mu.Lock()
	delete(e.reminded, portal)
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
	if _, rerr := e.d.Sessions.Refresh(ctx, pd.sess); rerr != nil {
		e.publishSession(pd)
		return api.SessionStatus{}, sso.ToAppError(rerr, portal)
	}
	e.updateRecord(portal, func(r *api.SessionRecord) { r.TokenIssuedAt = fmtTime(e.d.Now()) })
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

// SetSessionLength stores the per-portal session length (minutes, 15..129600) used by the countdown.
func (e *Engine) SetSessionLength(ctx context.Context, portal string, minutes int) (api.SessionStatus, error) {
	if minutes < minSessionMinutes || minutes > maxSessionMinutes {
		return api.SessionStatus{}, api.NewError(api.ErrInvalidInput, "session length out of range").WithParam("field", "minutes")
	}
	snap, err := e.Snapshot()
	if err != nil {
		return api.SessionStatus{}, err
	}
	pd, ok := e.portalDef(snap, portal)
	if !ok {
		return api.SessionStatus{}, api.NewError(api.ErrInvalidInput, "unknown portal").WithParam("field", "portal")
	}
	e.updateRecord(portal, func(r *api.SessionRecord) { r.SessionMinutes = minutes })
	st := e.publishSession(pd)
	e.EmitState(ctx)
	return st, nil
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
		if !pd.legacy && e.d.Store.AutoRenew() && raw.CanRefresh && raw.ExpiresAt.Sub(now) < expiringWindow &&
			now.Sub(last) > 60*time.Second && ctx.Err() == nil {
			e.mu.Lock()
			e.lastRenewal[pd.name] = now
			e.mu.Unlock()
			busy := st
			busy.Refreshing = true
			e.emit(api.EventSessionChanged, busy)
			if _, rerr := e.d.Sessions.Refresh(ctx, pd.sess); rerr == nil {
				e.updateRecord(pd.name, func(r *api.SessionRecord) { r.TokenIssuedAt = fmtTime(e.d.Now()) })
				after := e.sessionStatusFor(pd, e.d.Now())
				e.emit(api.EventSessionRenewed, after)
				e.emit(api.EventSessionChanged, after)
				e.setSig(pd.name, after)
			} else {
				after := e.sessionStatusFor(pd, e.d.Now())
				e.emit(api.EventSessionChanged, after)
				e.setSig(pd.name, after)
			}
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
