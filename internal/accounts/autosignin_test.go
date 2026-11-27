package accounts

import (
	"strings"
	"testing"
	"time"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/sso"
)

// endingIn sets up a sign-in of initech by this app whose exact session end AWS showed, left from now; the access
// token lasts until that end, as a token issued in the session's last hour does.
func (h *harness) endingIn(left time.Duration) {
	h.writeToken(left, "CID-a")
	h.setRecord(8*time.Hour-left, "CID-a", 480)
	h.setEnd(left)
}

// waitFor polls cond for up to 5 s.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatal("condition not met in time")
}

func (h *harness) autoRunning(portal string) bool {
	h.eng.mu.Lock()
	defer h.eng.mu.Unlock()
	return h.eng.autos[portal] != nil
}

func (h *harness) introCount() int { h.mu.Lock(); defer h.mu.Unlock(); return len(h.intros) }

// neverBack is a browser whose user never comes back from the sign-in page.
func neverBack(string) error { return nil }

func TestAutoSignInOpensBeforeTheEnd(t *testing.T) {
	h := newHarness(t, true)
	h.store.autoSignIn = true
	h.endingIn(9 * time.Minute)
	before, _ := h.store.SessionRecord("initech")
	h.eng.Tick(bg)
	waitFor(t, func() bool {
		r, _ := h.store.SessionRecord("initech")
		return r.LoginAt != before.LoginAt && !h.autoRunning("initech")
	})

	h.mu.Lock()
	pages := append([]string(nil), h.intros...)
	h.mu.Unlock()
	if len(pages) != 1 {
		t.Fatalf("intro pages %d", len(pages))
	}
	for _, want := range []string{"Sign in again: initech", "initech-admin", "initech-readonly", "This sign-in ends at"} {
		if !strings.Contains(pages[0], want) {
			t.Errorf("intro page lacks %q", want)
		}
	}
	if n := len(h.emitted(api.EventLoginProgress)); n != 0 {
		t.Fatalf("the sign-in window follows only the sign-ins it started: %d progress events", n)
	}
	waiting := false
	for _, d := range h.emitted(api.EventSessionChanged) {
		if st := d.(api.SessionStatus); st.SignInWaiting {
			waiting = true
			if st.State == api.SessionSigningIn {
				t.Fatalf("a waiting sign-in must not look like signing in: %+v", st)
			}
		}
	}
	if !waiting {
		t.Fatal("no session:changed showed the waiting sign-in")
	}
	if st := h.status("initech"); st.SignInWaiting || st.State != api.SessionActive || st.SessionEndExact {
		t.Fatalf("after the new sign-in: %+v", st)
	}
}

func TestAutoSignInWaitsForItsMoment(t *testing.T) {
	cases := []struct {
		name  string
		setup func(h *harness)
	}{
		{"more than the lead left", func(h *harness) { h.endingIn(11 * time.Minute) }},
		{"estimated end", func(h *harness) {
			h.writeToken(9*time.Minute, "CID-a")
			h.setRecord(8*time.Hour-9*time.Minute, "CID-a", 480)
		}},
		{"switched off", func(h *harness) { h.store.autoSignIn = false; h.endingIn(9 * time.Minute) }},
		{"session no longer than the lead", func(h *harness) {
			h.writeToken(9*time.Minute, "CID-a")
			h.setRecord(time.Minute, "CID-a", 480)
			h.setEnd(9 * time.Minute)
		}},
		{"signed in elsewhere", func(h *harness) {
			h.endingIn(9 * time.Minute)
			h.writeToken(9*time.Minute, "CID-cli") // the AWS CLI signed in since: not this app's sign-in
		}},
		{"ended", func(h *harness) {
			h.writeToken(time.Minute, "CID-a")
			h.setRecord(8*time.Hour, "CID-a", 480)
			h.setEnd(-time.Second)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, true)
			h.store.autoSignIn = true
			c.setup(h)
			h.eng.Tick(bg)
			time.Sleep(100 * time.Millisecond)
			if h.autoRunning("initech") || h.introCount() != 0 {
				t.Fatal("must not open the sign-in page")
			}
		})
	}
}

func TestAutoDueSkipsLegacyPortalsAndDrafts(t *testing.T) {
	h := newHarness(t, true)
	h.store.autoSignIn = true
	now := h.clock()
	st := api.SessionStatus{State: api.SessionExpiring, SessionKnown: true, SessionEndExact: true,
		LoginAt: now.Add(-8 * time.Hour).Format(time.RFC3339), SessionEndsAt: now.Add(9 * time.Minute).Format(time.RFC3339)}
	if _, ok := h.eng.autoDueAt(portalDef{name: "initech", sess: portalInitech}, st, now); !ok {
		t.Fatal("a config portal is due")
	}
	for _, pd := range []portalDef{
		{name: "legacy:d-1234567890.awsapps.com/start", legacy: true, sess: sso.Session{StartURL: portalInitech.StartURL, Region: "us-west-2"}},
		{name: "draft", draft: true, sess: portalInitech},
	} {
		if _, ok := h.eng.autoDueAt(pd, st, now); ok {
			t.Errorf("%s must not be due", pd.name)
		}
	}
}

func TestAutoSignInOpensOncePerSignInAndEnd(t *testing.T) {
	h := newHarness(t, true)
	h.store.autoSignIn = true
	h.openIntro = neverBack
	h.endingIn(9 * time.Minute)
	h.eng.Tick(bg)
	if !h.autoRunning("initech") {
		t.Fatal("not opened")
	}
	h.eng.Tick(bg) // still waiting: nothing new
	h.eng.stopAutoSignIn("initech")
	if h.autoRunning("initech") {
		t.Fatal("still waiting")
	}

	h.eng.Tick(bg) // the user closed the window: not again for this sign-in
	if h.autoRunning("initech") {
		t.Fatal("opened twice for one sign-in")
	}

	// a sign-in that handed back the same session: a new LoginAt, the same end
	h.setRecord(time.Hour, "CID-a", 480)
	h.setEnd(9 * time.Minute)
	h.eng.Tick(bg)
	if h.autoRunning("initech") {
		t.Fatal("opened twice for one session end")
	}

	// a real new session ends later: it opens again near its own end
	h.advance(3 * time.Hour)
	h.endingIn(8 * time.Minute)
	h.eng.Tick(bg)
	if !h.autoRunning("initech") {
		t.Fatal("a new session must open again")
	}
	h.eng.stopAutoSignIn("initech")
}

func TestPortalStaysUsableWhileTheSignInWaits(t *testing.T) {
	h := newHarness(t, true)
	h.store.autoSignIn = true
	h.openIntro = neverBack
	h.endingIn(9 * time.Minute)
	h.eng.Tick(bg)
	defer h.eng.stopAutoSignIn("initech")
	waitFor(t, func() bool { return h.status("initech").SignInWaiting })
	st := h.status("initech")
	if st.State != api.SessionExpiring || !st.SessionEndExact || st.SessionEndsAt == "" || !st.CanRefresh {
		t.Fatalf("the current sign-in must keep working: %+v", st)
	}
	creds, err := h.eng.ProfileCreds(bg)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range creds {
		if c.Portal == "initech" && !c.Usable {
			t.Fatalf("profile %s unusable while the sign-in page waits", c.Name)
		}
	}
}

func TestSignInButtonTakesOver(t *testing.T) {
	h := newHarness(t, true)
	h.store.autoSignIn = true
	h.openIntro = neverBack
	h.endingIn(9 * time.Minute)
	h.eng.Tick(bg)
	waitFor(t, func() bool { return h.status("initech").SignInWaiting })
	if _, err := h.eng.Login(bg, "initech", api.LoginModeBrowser); err != nil {
		t.Fatalf("the button must take over: %v", err)
	}
	if h.autoRunning("initech") || h.status("initech").SignInWaiting {
		t.Fatal("the automatic sign-in must have stopped")
	}
}

func TestSignOutStopsTheWaitingSignIn(t *testing.T) {
	h := newHarness(t, true)
	h.store.autoSignIn = true
	h.openIntro = neverBack
	h.endingIn(9 * time.Minute)
	h.eng.Tick(bg)
	waitFor(t, func() bool { return h.status("initech").SignInWaiting })
	if _, err := h.eng.Logout(bg, "initech"); err != nil {
		t.Fatal(err)
	}
	if st := h.status("initech"); h.autoRunning("initech") || st.SignInWaiting || st.State != api.SessionSignedOut {
		t.Fatalf("%+v", st)
	}
}
