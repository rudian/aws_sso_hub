package accounts

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/sso"
	"github.com/rudian/codepass/internal/sso/ssotest"
)

// signedInEnding sets up a sign-in by this app that ends `in` from now (the end is exact, as after AWS showed it)
// and a token that lasts as long. Renewal is off, so only the clock moves the state.
func (h *harness) signedInEnding(in time.Duration) {
	h.store.autoRenew = false
	h.writeToken(in, "CID-a")
	h.setRecord(8*time.Hour-in, "CID-a", 480)
	h.setEnd(in)
}

func (h *harness) ended() []api.SessionStatus {
	var out []api.SessionStatus
	for _, d := range h.emitted(api.EventSessionEnded) {
		out = append(out, d.(api.SessionStatus))
	}
	return out
}

// One "ended" notice per sign-in, when the state becomes expired.
func TestEndedNoticeOncePerSignIn(t *testing.T) {
	h := newHarness(t, true)
	h.signedInEnding(10 * time.Minute)
	h.eng.Tick(bg)
	if len(h.ended()) != 0 {
		t.Fatal("alive")
	}
	h.advance(11 * time.Minute)
	h.eng.Tick(bg)
	got := h.ended()
	if len(got) != 1 || got[0].State != api.SessionExpired || got[0].Portal != "initech" || got[0].LoginAt == "" {
		t.Fatalf("%+v", got)
	}
	rec, _ := h.store.SessionRecord("initech")
	if got[0].LoginAt != rec.LoginAt {
		t.Fatalf("the notice names the sign-in it is about: %s vs %s", got[0].LoginAt, rec.LoginAt)
	}
	h.eng.Tick(bg)
	h.advance(time.Minute)
	h.eng.Tick(bg)
	if len(h.ended()) != 1 {
		t.Fatal("once per sign-in")
	}
	// a new sign-in is a new life: its end is told again
	h.login()
	h.writeTokenKeepClient(t, 10*time.Minute)
	h.setEnd(10 * time.Minute)
	h.eng.Tick(bg)
	h.advance(11 * time.Minute)
	h.eng.Tick(bg)
	if len(h.ended()) != 2 {
		t.Fatalf("second sign-in: %d", len(h.ended()))
	}
}

// A sign-in that ended while the app was closed is told to nobody: at start it is marked as told.
func TestNoEndedNoticeForASignInThatEndedWhileTheAppWasClosed(t *testing.T) {
	h := newHarness(t, true)
	h.signedInEnding(10 * time.Minute)
	h.advance(5 * time.Hour) // the app was closed all that time
	h.eng.Tick(bg)
	h.eng.Tick(bg)
	if len(h.ended()) != 0 {
		t.Fatalf("told for a session that ended while the app was closed: %+v", h.ended())
	}
	rec, _ := h.store.SessionRecord("initech")
	if h.log.Last("initech", kindEnded) != rec.LoginAt {
		t.Fatalf("it is marked as told: %q", h.log.Last("initech", kindEnded))
	}
	// and it stays quiet after a restart
	h.restart()
	h.eng.Tick(bg)
	if len(h.ended()) != 0 {
		t.Fatal("restart")
	}
}

// An expired token that was never seen alive (expired at start, cannot refresh) is told to nobody either.
func TestNoEndedNoticeForAnExpiredTokenAtStart(t *testing.T) {
	h := newHarness(t, true)
	h.writeToken(-10*time.Minute, "CLI")
	cur, _ := sso.ReadToken(sso.TokenCacheDir(h.p.Home), sso.TokenCacheKey("initech", ""))
	cur.RefreshToken = ""
	sso.WriteToken(sso.TokenCacheDir(h.p.Home), sso.TokenCacheKey("initech", ""), cur)
	h.eng.Tick(bg)
	h.eng.Tick(bg)
	if st := h.status("initech"); st.State != api.SessionExpired {
		t.Fatalf("%+v", st)
	}
	if len(h.ended()) != 0 {
		t.Fatal("told at start")
	}
}

// The sign-in is seen alive, the computer sleeps past its end: the first tick after waking sends one notice.
func TestEndedNoticeAfterSleep(t *testing.T) {
	h := newHarness(t, true)
	h.signedInEnding(30 * time.Minute)
	h.eng.Tick(bg)
	h.advance(6 * time.Hour) // asleep: no ticks
	h.eng.Tick(bg)
	h.eng.Tick(bg)
	if len(h.ended()) != 1 {
		t.Fatalf("one notice after waking: %d", len(h.ended()))
	}
}

// A renewal that fails for a network reason is not "expired": nothing is told, the state stays as it was.
func TestNoEndedNoticeForANetworkError(t *testing.T) {
	h := newHarness(t, true)
	h.writeToken(2*time.Minute, "CID-a")
	h.setRecord(time.Hour, "CID-a", 480)
	h.eng.Tick(bg)             // seen alive
	h.advance(3 * time.Minute) // the access token ran out; a refresh token exists
	for i := 0; i < 3; i++ {
		h.oidc.QueueToken(ssotest.TokenResp{Status: 500, ErrType: "InternalServerException"})
		h.eng.Tick(bg)
		h.advance(2 * time.Minute)
	}
	if len(h.oidc.TokenCalls()) == 0 {
		t.Fatal("the app should have tried to renew")
	}
	if st := h.status("initech"); st.State == api.SessionExpired {
		t.Fatalf("a network error is not an end: %+v", st)
	}
	if len(h.ended()) != 0 {
		t.Fatalf("told for a network error: %+v", h.ended())
	}
}

// AWS refusing the renewal after the access token ran out is an end, and is told once.
func TestEndedNoticeWhenAWSRefusesTheRenewal(t *testing.T) {
	h := newHarness(t, true)
	h.store.autoRenew = false
	h.writeToken(2*time.Minute, "CID-a")
	h.setRecord(time.Hour, "CID-a", 480)
	h.eng.Tick(bg)
	h.advance(3 * time.Minute)
	h.store.autoRenew = true
	h.oidc.QueueToken(ssotest.TokenResp{Status: 400, ErrType: "InvalidGrantException"})
	h.eng.Tick(bg)
	if st := h.status("initech"); st.State != api.SessionExpired {
		t.Fatalf("%+v", st)
	}
	if len(h.ended()) != 1 {
		t.Fatalf("%d", len(h.ended()))
	}
}

// Signing out is not "expired".
func TestNoEndedNoticeForASignOut(t *testing.T) {
	h := newHarness(t, true)
	h.login()
	h.eng.Tick(bg)
	if _, err := h.eng.Logout(bg, "initech"); err != nil {
		t.Fatal(err)
	}
	h.advance(10 * time.Hour)
	h.eng.Tick(bg)
	h.eng.Tick(bg)
	if len(h.ended()) != 0 {
		t.Fatal("a sign-out is not an expiry")
	}
	if len(h.log.outs) != 1 || h.log.outs[0] != "initech" {
		t.Fatalf("the log is told of the sign-out: %v", h.log.outs)
	}
}

// A sign-in replaced before it ended never gets an "ended" notice; the log is told of the successful sign-in.
func TestNoEndedNoticeForASignInReplacedBeforeItEnded(t *testing.T) {
	h := newHarness(t, true)
	h.signedInEnding(10 * time.Minute)
	h.eng.Tick(bg)
	h.advance(5 * time.Minute)
	h.login() // signed in again before the end
	h.eng.Tick(bg)
	h.advance(8 * time.Minute) // the old end has passed
	h.eng.Tick(bg)
	if len(h.ended()) != 0 {
		t.Fatalf("%+v", h.ended())
	}
	if len(h.log.ins) != 1 || h.log.ins[0] != "initech" {
		t.Fatalf("the log is told of the sign-in: %v", h.log.ins)
	}
}

func TestEndedNoticeSwitchOff(t *testing.T) {
	h := newHarness(t, true)
	h.store.endedOff = true
	h.signedInEnding(10 * time.Minute)
	h.eng.Tick(bg)
	h.advance(11 * time.Minute)
	h.eng.Tick(bg)
	if len(h.ended()) != 0 {
		t.Fatal("switched off")
	}
	// ... and turning it on later does not tell about the end that already passed
	h.store.endedOff = false
	h.eng.Tick(bg)
	if len(h.ended()) != 0 {
		t.Fatal("already marked as told")
	}
}

// The heads-up key is kept in the notice log: a restart inside the lead time does not send it twice.
func TestHeadsUpKeyAcrossARestart(t *testing.T) {
	h := newHarness(t, true)
	h.signedInEnding(10 * time.Minute) // inside the 15 minute lead time
	h.eng.Tick(bg)
	h.eng.Tick(bg)
	if n := len(h.emitted(api.EventSessionReminder)); n != 1 {
		t.Fatalf("%d", n)
	}
	h.restart()
	h.eng.Tick(bg)
	h.eng.Tick(bg)
	if n := len(h.emitted(api.EventSessionReminder)); n != 1 {
		t.Fatalf("after a restart inside the lead time: %d", n)
	}
	// the next sign-in has its own key
	h.login()
	h.writeTokenKeepClient(t, 30*time.Minute)
	h.setEnd(30 * time.Minute)
	h.eng.Tick(bg)
	h.advance(20 * time.Minute)
	h.eng.Tick(bg)
	if n := len(h.emitted(api.EventSessionReminder)); n != 2 {
		t.Fatalf("a new sign-in: %d", n)
	}
}

// A sign-in that itself started inside the lead time gets no heads-up (a session shorter than the lead time).
func TestNoHeadsUpWhenTheSignInStartedInsideTheLeadTime(t *testing.T) {
	h := newHarness(t, true)
	h.store.autoRenew = false
	h.writeToken(10*time.Minute, "CID-a")
	h.setRecord(3*time.Minute, "CID-a", 13) // signed in 3 minutes ago, the session is 13 minutes long
	h.setEnd(10 * time.Minute)
	h.eng.Tick(bg)
	h.eng.Tick(bg)
	if n := len(h.emitted(api.EventSessionReminder)); n != 0 {
		t.Fatalf("heads-up right after signing in: %d", n)
	}
	if h.log.Last("initech", kindHeadsUp) == "" {
		t.Fatal("it is marked, so the question is not asked again")
	}
}

func TestHeadsUpAndEndedAreIndependentSwitches(t *testing.T) {
	h := newHarness(t, true)
	h.store.notice = false // no heads-up
	h.signedInEnding(10 * time.Minute)
	h.eng.Tick(bg)
	h.advance(11 * time.Minute)
	h.eng.Tick(bg)
	if len(h.emitted(api.EventSessionReminder)) != 0 || len(h.ended()) != 1 {
		t.Fatal("heads-up off, ended on")
	}
}

// With the lead time at one hour and renewal on, the first tick after a sign-in sees only the access token's hour.
// The heads-up is decided after the check renewal, so a long session still gets its heads-up, once.
func TestHeadsUpWithAnHourLeadSurvivesTheCheckRenewal(t *testing.T) {
	for _, isolated := range []bool{true, false} {
		h := newHarness(t, true)
		h.isolated = isolated
		h.store.lead = time.Hour
		h.login()
		loginAt := h.clock()
		h.eng.Tick(bg) // provisional end: the token's hour, no heads-up and nothing marked
		if h.log.Last("initech", kindHeadsUp) != "" {
			t.Fatalf("isolated=%v: marked before the check renewal", isolated)
		}
		h.advance(probeDelay)
		h.eng.Tick(bg)
		for h.clock().Before(loginAt.Add(8*time.Hour - 55*time.Minute)) {
			h.advance(20 * time.Minute)
			h.eng.Tick(bg)
		}
		h.eng.Tick(bg)
		if n := len(h.emitted(api.EventSessionReminder)); n != 1 {
			t.Fatalf("isolated=%v: %d heads-ups", isolated, n)
		}
	}
}

// A legacy pseudo-portal (a profile with sso_start_url and no sso-session) is not a sign-in: no notices, and its
// name (which holds the start URL) never reaches an event.
func TestNoNoticesForALegacyPortal(t *testing.T) {
	h := newHarness(t, true)
	const url = "https://d-9999999999.awsapps.com/start"
	f, err := os.OpenFile(h.p.AWSConfigFile, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("\n[profile umbrella-old]\nsso_start_url = " + url + "\nsso_region = us-west-2\nsso_account_id = 111122223333\nsso_role_name = ReadOnlyAccess\n")
	f.Close()
	tok := &sso.CachedToken{StartURL: url, Region: "us-west-2", AccessToken: "AT-old", ExpiresAt: h.clock().Add(10 * time.Minute)}
	if err := sso.WriteToken(sso.TokenCacheDir(h.p.Home), sso.TokenCacheKey("", url), tok); err != nil {
		t.Fatal(err)
	}
	h.eng.Tick(bg)
	snap, _ := h.eng.Snapshot()
	found := false
	for _, pd := range h.eng.portals(snap) {
		if pd.legacy {
			found = true
			if _, _, ok := h.eng.NoticeInfo(pd.name); ok {
				t.Fatal("a legacy portal has no notice info")
			}
		}
	}
	if !found {
		t.Fatal("the legacy portal is not in the list")
	}
	h.advance(11 * time.Minute)
	h.eng.Tick(bg)
	for _, name := range []string{api.EventSessionReminder, api.EventSessionEnded} {
		for _, d := range h.emitted(name) {
			if st := d.(api.SessionStatus); strings.HasPrefix(st.Portal, "legacy:") {
				t.Fatalf("%s for %s", name, st.Portal)
			}
		}
	}
}
