package accounts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/sso"
	"github.com/rudian/codepass/internal/sso/ssotest"
)

func (h *harness) login() api.LoginResult {
	h.t.Helper()
	res, err := h.eng.Login(bg, "initech", api.LoginModeBrowser)
	if err != nil {
		h.t.Fatal(err)
	}
	return res
}

func TestLoginEmitsProgress(t *testing.T) {
	h := newHarness(t, true)
	res := h.login()
	var phases []api.LoginPhase
	for _, d := range h.emitted(api.EventLoginProgress) {
		p := d.(api.LoginProgress)
		if p.Portal != "initech" {
			t.Fatalf("portal on progress: %+v", p)
		}
		phases = append(phases, p.Phase)
	}
	if len(phases) != 3 || phases[0] != api.LoginPhaseStarting || phases[1] != api.LoginPhaseWaiting || phases[2] != api.LoginPhaseExchanging {
		t.Fatalf("%v", phases)
	}
	if res.Portal != "initech" || len(res.Profiles) != 2 || res.ExpiresAt != h.clock().Add(time.Hour).Format(time.RFC3339) {
		t.Fatalf("%+v", res)
	}
	changed := h.emitted(api.EventSessionChanged)
	if len(changed) < 2 || changed[0].(api.SessionStatus).State != api.SessionSigningIn || changed[len(changed)-1].(api.SessionStatus).State != api.SessionActive {
		t.Fatalf("session:changed sequence: %v", jsonOf(changed))
	}
	if len(h.emitted(api.EventAccountsChanged)) == 0 {
		t.Fatal("accounts:changed")
	}
	// errors map to contract errors and restore the state
	h2 := newHarness(t, true)
	_, err := h2.eng.Login(bg, "nope", api.LoginModeBrowser)
	if api.AsAppError(err).Code != api.ErrPortalNotFound {
		t.Fatal(err)
	}
}

func TestSessionKnownAfterAppLogin(t *testing.T) {
	h := newHarness(t, true)
	h.login()
	st := h.status("initech")
	if st.State != api.SessionActive || !st.SessionKnown || st.SessionMinutes != 480 || st.SessionOverrun || st.CanRefresh != true {
		t.Fatalf("%+v", st)
	}
	now := h.clock().Format(time.RFC3339)
	if st.LoginAt != now || st.IssuedAt != now || st.SessionEndsAt != h.clock().Add(8*time.Hour).Format(time.RFC3339) {
		t.Fatalf("%+v", st)
	}
	if st.ProgressPct != 100 {
		t.Fatalf("pct %d", st.ProgressPct)
	}
	if st.ExpiresAt != h.clock().Add(time.Hour).Format(time.RFC3339) {
		t.Fatalf("token expiry %s", st.ExpiresAt)
	}
	h.advance(2 * time.Hour)
	h.writeTokenKeepClient(t, 30*time.Minute)
	st = h.status("initech")
	if st.ProgressPct != 75 || st.State != api.SessionActive || !st.SessionKnown {
		t.Fatalf("after 2h: %+v", st)
	}
	// the first GetState after a restart already shows the countdown (no network, no tick needed)
	state, _ := h.eng.State(bg)
	if p := state.Portals[0].Session; !p.SessionKnown || p.ProgressPct != 75 {
		t.Fatalf("%+v", p)
	}
}

// writeTokenKeepClient rewrites the token file keeping the client id of the current file (as a refresh would).
func (h *harness) writeTokenKeepClient(t *testing.T, left time.Duration) {
	t.Helper()
	cur, err := sso.ReadToken(sso.TokenCacheDir(h.p.Home), sso.TokenCacheKey("initech", ""))
	if err != nil {
		t.Fatal(err)
	}
	cur.ExpiresAt = h.clock().Add(left)
	if err := sso.WriteToken(sso.TokenCacheDir(h.p.Home), sso.TokenCacheKey("initech", ""), cur); err != nil {
		t.Fatal(err)
	}
}

func TestSessionUnknownAfterExternalLogin(t *testing.T) {
	h := newHarness(t, true)
	h.login()
	// `aws sso login` in a terminal: new client registration, new token
	h.writeToken(time.Hour, "CLI-OWN-CLIENT")
	st := h.status("initech")
	if st.State != api.SessionActive || st.SessionKnown || st.LoginAt != "" || st.SessionEndsAt != "" || st.ProgressPct != 0 || st.SessionOverrun {
		t.Fatalf("%+v", st)
	}
	if st.SessionMinutes != 480 {
		t.Fatal("minutes always present")
	}
	// token refreshable and expired, unknown lineage -> refreshable; non-renewable and low -> expiring
	h.writeToken(-time.Minute, "CLI-OWN-CLIENT")
	if st := h.status("initech"); st.State != api.SessionRefreshable {
		t.Fatalf("%+v", st)
	}
	h.writeToken(10*time.Minute, "CLI-OWN-CLIENT")
	if st := h.status("initech"); st.State != api.SessionActive || !st.CanRefresh {
		t.Fatalf("renewable token must read active: %+v", st)
	}
	cur, _ := sso.ReadToken(sso.TokenCacheDir(h.p.Home), sso.TokenCacheKey("initech", ""))
	cur.RefreshToken = ""
	sso.WriteToken(sso.TokenCacheDir(h.p.Home), sso.TokenCacheKey("initech", ""), cur)
	if st := h.status("initech"); st.State != api.SessionExpiring || st.CanRefresh {
		t.Fatalf("%+v", st)
	}
}

func TestRefreshKeepsLoginAt(t *testing.T) {
	h := newHarness(t, true)
	h.login()
	loginAt := h.status("initech").LoginAt
	h.advance(50 * time.Minute)
	st, err := h.eng.Refresh(bg, "initech")
	if err != nil {
		t.Fatal(err)
	}
	if st.LoginAt != loginAt || !st.SessionKnown {
		t.Fatalf("LoginAt must survive a refresh: %+v", st)
	}
	if st.IssuedAt != h.clock().Format(time.RFC3339) || st.ExpiresAt != h.clock().Add(time.Hour).Format(time.RFC3339) {
		t.Fatalf("%+v", st)
	}
	rec, _ := h.store.SessionRecord("initech")
	if rec.LoginAt != loginAt || rec.TokenIssuedAt != st.IssuedAt {
		t.Fatalf("%+v", rec)
	}
	if len(h.emitted(api.EventSessionRenewed)) != 1 {
		t.Fatal("session:renewed expected")
	}
	// refresh without any file
	h2 := newHarness(t, true)
	if _, err := h2.eng.Refresh(bg, "initech"); api.AsAppError(err).Code != api.ErrNotSignedIn {
		t.Fatalf("%v", err)
	}
}

func TestStateExpiringBySessionNotToken(t *testing.T) {
	h := newHarness(t, true)
	// token 50 min, session 10 min -> expiring
	h.writeToken(50*time.Minute, "CID-a")
	h.setRecord(470*time.Minute, "CID-a", 480)
	if st := h.status("initech"); st.State != api.SessionExpiring || !st.SessionKnown {
		t.Fatalf("%+v", st)
	}
	// token 5 min + CanRefresh + session 5 h -> active
	h.writeToken(5*time.Minute, "CID-a")
	h.setRecord(3*time.Hour, "CID-a", 480)
	if st := h.status("initech"); st.State != api.SessionActive || !st.CanRefresh {
		t.Fatalf("%+v", st)
	}
	// expired token that can be refreshed while the session is long -> refreshable
	h.writeToken(-time.Minute, "CID-a")
	if st := h.status("initech"); st.State != api.SessionRefreshable {
		t.Fatalf("%+v", st)
	}
	// session exactly at the 15 minute boundary
	h.writeToken(time.Hour, "CID-a")
	h.setRecord(465*time.Minute, "CID-a", 480)
	if st := h.status("initech"); st.State != api.SessionActive {
		t.Fatalf("15 min left is still active: %+v", st)
	}
	h.setRecord(465*time.Minute+time.Second, "CID-a", 480)
	if st := h.status("initech"); st.State != api.SessionExpiring {
		t.Fatalf("%+v", st)
	}
}

func TestSessionOverrun(t *testing.T) {
	h := newHarness(t, true)
	h.writeToken(30*time.Minute, "CID-a")
	h.setRecord(9*time.Hour, "CID-a", 480)
	st := h.status("initech")
	if !st.SessionOverrun || st.State != api.SessionActive || st.ProgressPct != 0 || !st.SessionKnown {
		t.Fatalf("%+v", st)
	}
	h.writeToken(-time.Minute, "CID-a")
	if st := h.status("initech"); !st.SessionOverrun || st.State != api.SessionRefreshable {
		t.Fatalf("%+v", st)
	}
}

func TestEndedEarlyAfterInvalidGrant(t *testing.T) {
	h := newHarness(t, true)
	h.writeToken(-time.Minute, "CID-a")
	h.setRecord(time.Hour, "CID-a", 480)
	h.oidc.QueueToken(ssotest.TokenResp{Status: 400, ErrType: "InvalidGrantException"})
	_, err := h.eng.Refresh(bg, "initech")
	if api.AsAppError(err).Code != api.ErrSessionExpired {
		t.Fatalf("%v", err)
	}
	st := h.status("initech")
	endsAt, _ := time.Parse(time.RFC3339, st.SessionEndsAt)
	if st.State != api.SessionExpired || !st.SessionKnown || endsAt.Sub(h.clock()) <= 30*time.Minute || st.CanRefresh {
		t.Fatalf("the UI derives 'ended early' from expired + sessionEndsAt > now+30m: %+v", st)
	}
	if st.SessionOverrun || st.ProgressPct != 0 {
		t.Fatalf("%+v", st)
	}
}

func TestLogoutClearsLoginKeepsLength(t *testing.T) {
	h := newHarness(t, true)
	h.login()
	if _, err := h.eng.SetSessionLength(bg, "initech", 120); err != nil {
		t.Fatal(err)
	}
	// a role cache file of a profile of the portal
	cache := sso.CLICacheDir(h.p.Home)
	os.MkdirAll(cache, 0o700)
	role := filepath.Join(cache, sso.RoleCacheKey("777788889999", "AdministratorAccess", "initech", "")+".json")
	os.WriteFile(role, []byte(`{"ProviderType":"sso","Credentials":{}}`), 0o600)
	res, err := h.eng.Logout(bg, "initech")
	if err != nil || res.DeletedRoleCaches != 1 || !res.ServerLogout || res.Portal != "initech" {
		t.Fatalf("%+v %v", res, err)
	}
	rec, _ := h.store.SessionRecord("initech")
	if rec.LoginAt != "" || rec.ClientIDHash != "" || rec.TokenIssuedAt != "" || rec.SessionMinutes != 120 {
		t.Fatalf("%+v", rec)
	}
	st := h.status("initech")
	if st.State != api.SessionSignedOut || st.SessionKnown || st.SessionMinutes != 120 {
		t.Fatalf("%+v", st)
	}
	if h.por.LogoutCalls() != 1 {
		t.Fatal("server logout")
	}
	if _, err := os.Stat(role); err == nil {
		t.Fatal("role cache must be deleted")
	}
	last := h.emitted(api.EventSessionChanged)
	if last[len(last)-1].(api.SessionStatus).State != api.SessionSignedOut {
		t.Fatal("session:changed signed-out expected")
	}
}

func TestSetSessionLengthValidates(t *testing.T) {
	h := newHarness(t, true)
	for _, bad := range []int{0, 14, 129601, -5} {
		if _, err := h.eng.SetSessionLength(bg, "initech", bad); api.AsAppError(err).Code != api.ErrInvalidInput {
			t.Errorf("%d accepted", bad)
		}
	}
	if _, err := h.eng.SetSessionLength(bg, "nope", 60); api.AsAppError(err).Code != api.ErrInvalidInput {
		t.Fatalf("%v", err)
	}
	for _, ok := range []int{15, 129600, 480} {
		st, err := h.eng.SetSessionLength(bg, "initech", ok)
		if err != nil || st.SessionMinutes != ok {
			t.Fatalf("%d: %v %+v", ok, err, st)
		}
	}
	if len(h.emitted(api.EventSessionChanged)) != 3 {
		t.Fatalf("session:changed per change, got %d", len(h.emitted(api.EventSessionChanged)))
	}
	// the countdown follows the new length
	h.writeToken(time.Hour, "CID-a")
	h.setRecord(time.Hour, "CID-a", 120)
	st, _ := h.eng.SetSessionLength(bg, "initech", 60)
	if st.State != api.SessionActive || !st.SessionOverrun {
		t.Fatalf("1 h login with a 60 min length is overrun: %+v", st)
	}
	st, _ = h.eng.SetSessionLength(bg, "initech", 90)
	if st.SessionOverrun || st.State != api.SessionActive || st.ProgressPct != 33 {
		t.Fatalf("%+v", st)
	}
}

func TestRecordNeverContainsClientID(t *testing.T) {
	h := newHarness(t, true)
	h.login()
	rec, ok := h.store.SessionRecord("initech")
	if !ok {
		t.Fatal("no record")
	}
	b := jsonOf(rec)
	if strings.Contains(b, "CID-") || strings.Contains(b, "CS-") || strings.Contains(b, "AT-") || strings.Contains(b, "RT-") {
		t.Fatalf("record leaks: %s", b)
	}
	if len(rec.ClientIDHash) != 16 || rec.ClientIDHash != sso.ClientIDHash("CID-1") {
		t.Fatalf("%+v", rec)
	}
	all := jsonOf(h.events)
	for _, s := range []string{"CID-1", "CS-1", "AT-1", "RT-1", "roleSecret"} {
		if strings.Contains(all, s) {
			t.Fatalf("event payload leaks %s", s)
		}
	}
}

func TestTickAutoRenew(t *testing.T) {
	h := newHarness(t, true)
	h.writeToken(-time.Minute, "CID-a") // refreshable
	h.setRecord(time.Hour, "CID-a", 480)
	h.eng.State(bg) // seed
	h.mu.Lock()
	h.events = nil
	h.mu.Unlock()
	h.eng.Tick(bg)
	var seq []string
	for _, e := range h.events {
		switch d := e.data.(type) {
		case api.SessionStatus:
			if e.name == api.EventSessionChanged {
				seq = append(seq, "changed:"+string(d.State)+map[bool]string{true: "+refreshing", false: ""}[d.Refreshing])
			} else {
				seq = append(seq, e.name+":"+string(d.State))
			}
		}
	}
	want := []string{"changed:refreshable+refreshing", "session:renewed:active", "changed:active"}
	if strings.Join(seq, ",") != strings.Join(want, ",") {
		t.Fatalf("events %v want %v", seq, want)
	}
	if len(h.oidc.TokenCalls()) != 1 || h.oidc.TokenCalls()[0]["grantType"] != "refresh_token" {
		t.Fatal("refresh request")
	}
	rec, _ := h.store.SessionRecord("initech")
	if rec.TokenIssuedAt != h.clock().Format(time.RFC3339) || rec.LoginAt == "" {
		t.Fatalf("%+v", rec)
	}
	// no second attempt, no events when nothing changed
	n := len(h.events)
	h.eng.Tick(bg)
	if len(h.events) != n {
		t.Fatalf("tick must be quiet: %v", h.names()[n:])
	}
	// AutoRenew off -> no refresh
	h2 := newHarness(t, true)
	h2.store.autoRenew = false
	h2.writeToken(5*time.Minute, "CID-a")
	h2.eng.Tick(bg)
	if len(h2.oidc.TokenCalls()) != 0 {
		t.Fatal("auto renew disabled")
	}
	// a rejected refresh is attempted once per expiresAt
	h3 := newHarness(t, true)
	h3.writeToken(-time.Minute, "CID-a")
	h3.oidc.QueueToken(ssotest.TokenResp{Status: 400, ErrType: "InvalidGrantException"})
	h3.eng.Tick(bg)
	h3.advance(2 * time.Minute)
	h3.eng.Tick(bg)
	if len(h3.oidc.TokenCalls()) != 1 || h3.status("initech").State != api.SessionExpired {
		t.Fatalf("calls=%d state=%s", len(h3.oidc.TokenCalls()), h3.status("initech").State)
	}
}

func TestTickEmitsOnSessionCrossing15Min(t *testing.T) {
	h := newHarness(t, true)
	h.writeToken(time.Hour, "CID-a")
	h.setRecord(8*time.Hour-20*time.Minute, "CID-a", 480)
	h.eng.State(bg)
	h.eng.Tick(bg)
	n := len(h.emitted(api.EventSessionChanged))
	h.advance(6 * time.Minute) // 14 min left
	h.writeTokenKeepClient(t, time.Hour)
	h.eng.Tick(bg)
	ch := h.emitted(api.EventSessionChanged)
	if len(ch) != n+1 || ch[len(ch)-1].(api.SessionStatus).State != api.SessionExpiring {
		t.Fatalf("%v", jsonOf(ch))
	}
}

func TestTickReminder(t *testing.T) {
	h := newHarness(t, true)
	h.writeToken(time.Hour, "CID-a")
	h.setRecord(8*time.Hour-10*time.Minute, "CID-a", 480)
	h.eng.Tick(bg)
	h.eng.Tick(bg)
	if n := len(h.emitted(api.EventSessionReminder)); n != 1 {
		t.Fatalf("reminder once per login: %d", n)
	}
	// unknown session: token cannot be renewed and ends within the lead time
	h2 := newHarness(t, true)
	h2.writeToken(10*time.Minute, "CLI")
	cur, _ := sso.ReadToken(sso.TokenCacheDir(h2.p.Home), sso.TokenCacheKey("initech", ""))
	cur.RefreshToken = ""
	sso.WriteToken(sso.TokenCacheDir(h2.p.Home), sso.TokenCacheKey("initech", ""), cur)
	h2.eng.Tick(bg)
	h2.eng.Tick(bg)
	if n := len(h2.emitted(api.EventSessionReminder)); n != 1 {
		t.Fatalf("%d", n)
	}
	// notice disabled
	h3 := newHarness(t, true)
	h3.store.notice = false
	h3.writeToken(time.Hour, "CID-a")
	h3.setRecord(8*time.Hour-10*time.Minute, "CID-a", 480)
	h3.eng.Tick(bg)
	if len(h3.emitted(api.EventSessionReminder)) != 0 {
		t.Fatal("expiry notice disabled")
	}
}

func TestRenamePortalMovesToken(t *testing.T) {
	h := newHarness(t, true)
	h.login()
	h.eng.SetSessionLength(bg, "initech", 240)
	cache := sso.CLICacheDir(h.p.Home)
	os.MkdirAll(cache, 0o700)
	oldRole := filepath.Join(cache, sso.RoleCacheKey("777788889999", "AdministratorAccess", "initech", "")+".json")
	os.WriteFile(oldRole, []byte(`{"ProviderType":"sso"}`), 0o600)

	if _, err := h.eng.PreviewRenamePortal("initech", "bad name"); api.AsAppError(err).Code != api.ErrInvalidName {
		t.Fatal(err)
	}
	if _, err := h.eng.PreviewRenamePortal("zzz", "x"); api.AsAppError(err).Code != api.ErrPortalNotFound {
		t.Fatal(err)
	}
	pv, err := h.eng.PreviewRenamePortal("initech", "wl")
	if err != nil || pv.Added != 3 {
		t.Fatalf("%+v %v", pv, err)
	}
	res, err := h.eng.RenamePortal("initech", "wl", pv.BaseHash)
	if err != nil || res.BackupID == "" {
		t.Fatal(err)
	}
	dir := sso.TokenCacheDir(h.p.Home)
	if _, err := os.Stat(filepath.Join(dir, sso.TokenCacheKey("initech", "")+".json")); err == nil {
		t.Fatal("old token file must move")
	}
	tok, err := sso.ReadToken(dir, sso.TokenCacheKey("wl", ""))
	if err != nil || tok.StartURL != portalInitech.StartURL {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldRole); err == nil {
		t.Fatal("role cache of the old session name must go")
	}
	if _, ok := h.store.SessionRecord("initech"); ok {
		t.Fatal("old record remains")
	}
	rec, ok := h.store.SessionRecord("wl")
	if !ok || rec.SessionMinutes != 240 || rec.LoginAt == "" {
		t.Fatalf("%+v", rec)
	}
	cfg, _ := os.ReadFile(h.p.AWSConfigFile)
	if !strings.Contains(string(cfg), "[sso-session wl]") || strings.Count(string(cfg), "sso_session    = wl") != 2 {
		t.Fatalf("%s", cfg)
	}
	// the renamed portal keeps its countdown (lineage survives)
	if st, err := h.eng.SessionStatus("wl"); err != nil || !st.SessionKnown || st.State != api.SessionActive {
		t.Fatalf("%+v %v", st, err)
	}
}

func TestRemovePortalInUse(t *testing.T) {
	h := newHarness(t, true)
	_, err := h.eng.PreviewRemovePortal("initech")
	ae := api.AsAppError(err)
	if ae.Code != api.ErrPortalInUse || ae.Params["n"] != "2" || ae.Params["portal"] != "initech" {
		t.Fatalf("%+v", ae)
	}
	// after removing its profiles the portal can go
	for _, p := range []string{"initech-admin", "initech-readonly"} {
		pv, _ := h.eng.PreviewRemove(p)
		if _, err := h.eng.Remove(p, pv.BaseHash); err != nil {
			t.Fatal(err)
		}
	}
	h.writeToken(time.Hour, "CID-a")
	h.setRecord(time.Hour, "CID-a", 480)
	pv, err := h.eng.PreviewRemovePortal("initech")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.eng.RemovePortal("initech", pv.BaseHash); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(sso.TokenCacheDir(h.p.Home), sso.TokenCacheKey("initech", "")+".json")); err == nil {
		t.Fatal("token file must be deleted")
	}
	if _, ok := h.store.SessionRecord("initech"); ok {
		t.Fatal("record must be deleted")
	}
	if h.por.LogoutCalls() != 0 {
		t.Fatal("no server logout on portal removal")
	}
	cfg, _ := os.ReadFile(h.p.AWSConfigFile)
	if strings.Contains(string(cfg), "sso-session") {
		t.Fatalf("%s", cfg)
	}
}

func TestConsoleURL(t *testing.T) {
	h := newHarness(t, true)
	u, err := h.eng.ConsoleURL("initech-admin", "")
	if err != nil || u != "https://d-1234567890.awsapps.com/start/#/console?account_id=777788889999&role_name=AdministratorAccess" {
		t.Fatal(u, err)
	}
	if _, err := h.eng.ConsoleURL("acme", ""); api.AsAppError(err).Code != api.ErrInvalidInput {
		t.Fatal(err)
	}
	if _, err := h.eng.ConsoleURL("zzz", ""); api.AsAppError(err).Code != api.ErrProfileNotFound {
		t.Fatal(err)
	}
}

func TestShutdownIsNotAThing(t *testing.T) {
	// The engine has no sign-out-on-quit; Tick/State never delete token files.
	h := newHarness(t, true)
	h.login()
	tokPath := filepath.Join(sso.TokenCacheDir(h.p.Home), sso.TokenCacheKey("initech", "")+".json")
	before, _ := os.ReadFile(tokPath)
	h.eng.State(bg)
	h.eng.Tick(bg)
	after, _ := os.ReadFile(tokPath)
	if string(before) != string(after) {
		t.Fatal("State/Tick must not rewrite a healthy token")
	}
}
