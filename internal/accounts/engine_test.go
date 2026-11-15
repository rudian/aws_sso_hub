package accounts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/ini"
	"github.com/rudian/codepass/internal/sso/ssotest"
)

func profileByName(st api.AccountsState, n string) api.Profile {
	for _, p := range st.Profiles {
		if p.Name == n {
			return p
		}
	}
	return api.Profile{}
}

func TestStateFromFixture(t *testing.T) {
	h := newHarness(t, true)
	st, err := h.eng.State(bg)
	if err != nil {
		t.Fatal(err)
	}
	if st.ReadOnly || len(st.Issues) != 0 || st.StaticCount != 2 || len(st.Profiles) != 4 {
		t.Fatalf("%+v", st)
	}
	if st.ConfigFile != "~/.aws/config" && !strings.HasSuffix(st.ConfigFile, "config") {
		t.Fatal(st.ConfigFile)
	}
	if len(st.Portals) != 1 || st.Portals[0].Name != "initech" || len(st.Portals[0].Profiles) != 2 || st.Portals[0].Session.State != api.SessionSignedOut {
		t.Fatalf("%+v", st.Portals)
	}
	if st.Portals[0].StartURLHost != "d-1234567890.awsapps.com/start" || st.Portals[0].Draft || st.Portals[0].LegacyFormat {
		t.Fatalf("%+v", st.Portals[0])
	}
	sk, ph := profileByName(st, "acme"), profileByName(st, "globex")
	wm, wt := profileByName(st, "initech-admin"), profileByName(st, "initech-readonly")
	if sk.Kind != api.ProfileKindStatic || sk.StaticState != api.StaticUnverified || sk.MaskedKeyID != "AKIA••••••••••••MPLE" || !sk.HasStaticKeys {
		t.Fatalf("%+v", sk)
	}
	if wm.Portal != "initech" || wm.AccountID != "777788889999" || wm.Role != "AdministratorAccess" || wm.Kind != api.ProfileKindSSO {
		t.Fatalf("%+v", wm)
	}
	if sk.ColorSlot != 1 || ph.ColorSlot != 3 || wm.ColorSlot != 2 || wt.ColorSlot != 2 {
		t.Fatalf("slots %d %d %d %d", sk.ColorSlot, ph.ColorSlot, wm.ColorSlot, wt.ColorSlot)
	}
	if sk.Monogram != "AC" || ph.Monogram != "GL" || wm.Monogram != "IA" || wt.Monogram != "IR" {
		t.Fatalf("monograms %s %s %s %s", sk.Monogram, ph.Monogram, wm.Monogram, wt.Monogram)
	}
	if st.Profiles[0].Name > st.Profiles[1].Name {
		t.Fatal("profiles must be sorted by name")
	}
	if h.store.colors["777788889999"] != 2 || h.store.colors["profile:acme"] != 1 {
		t.Fatalf("colours not persisted: %v", h.store.colors)
	}
	for _, s := range []string{"fakeSecret", "AKIAIOSFODNN7EXAMPLE"} {
		if strings.Contains(jsonOf(st), s) {
			t.Fatalf("secret %s in state", s)
		}
	}
}

func TestStateReadOnlyOnParseError(t *testing.T) {
	h := newHarness(t, true)
	os.WriteFile(h.p.AWSConfigFile, []byte("[profile a]\nsso_session = nope\ngarbage line\n"), 0o600)
	st, _ := h.eng.State(bg)
	if !st.ReadOnly || len(st.Issues) != 1 || st.Issues[0].Line != 3 {
		t.Fatalf("%+v", st.Issues)
	}
	_, err := h.eng.Create(api.CreateProfilesRequest{Portal: "x", Profiles: []api.NewProfileSpec{{Name: "n"}}}, "")
	if err == nil {
		t.Fatal("must fail")
	}
	// read-only: even a valid create plan is refused
	h2 := newHarness(t, true)
	snap, _ := h2.eng.Snapshot()
	good := snap.ConfigHash()
	os.WriteFile(h2.p.AWSConfigFile, []byte("[profile a]\nbroken\n"), 0o600)
	snap2, _ := h2.eng.Snapshot()
	if _, werr := h2.eng.writeConfig(snap2, snap2.ConfigHash(), []byte("x"), api.ReasonAddProfiles, nil); api.AsAppError(werr).Code != api.ErrReadOnly {
		t.Fatalf("got %v (hash %s)", werr, good)
	}
}

func TestDraftLifecycle(t *testing.T) {
	h := newHarness(t, true)
	p, err := h.eng.AddDraft(api.PortalInput{StartURL: "https://d-1111111111.awsapps.com/start#/", Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Draft || p.Name != "d-1111111111" || p.StartURL != "https://d-1111111111.awsapps.com/start" || p.Session.State != api.SessionSignedOut || p.Profiles == nil {
		t.Fatalf("%+v", p)
	}
	if !h.eng.IsDraft("d-1111111111") {
		t.Fatal("IsDraft")
	}
	if s, ok := h.eng.DraftSession("d-1111111111"); !ok || !s.ScopesSet || s.Scopes[0] != "sso:account:access" {
		t.Fatalf("%+v", s)
	}
	if _, ok := h.eng.SessionFor("d-1111111111"); !ok {
		t.Fatal("SessionFor draft")
	}
	st, _ := h.eng.State(bg)
	if len(st.Portals) != 2 || st.Portals[1].Name != "d-1111111111" || !st.Portals[1].Draft {
		t.Fatalf("%+v", st.Portals)
	}
	if len(h.emitted(api.EventAccountsChanged)) == 0 {
		t.Fatal("accounts:changed expected")
	}
	if _, err := h.eng.AddDraft(api.PortalInput{Name: "d-1111111111", StartURL: "https://d-2222222222.awsapps.com/start", Region: "us-east-1"}); api.AsAppError(err).Code != api.ErrPortalExists {
		t.Fatalf("%v", err)
	}
	h.eng.DiscardDraft("d-1111111111")
	if h.eng.IsDraft("d-1111111111") {
		t.Fatal("not discarded")
	}
	h.eng.AddDraft(api.PortalInput{StartURL: "https://d-3333333333.awsapps.com/start", Region: "us-east-1"})
	h.eng.ConsumeDraft("d-3333333333")
	if h.eng.IsDraft("d-3333333333") {
		t.Fatal("not consumed")
	}
	if url, err := h.eng.PortalURL("initech"); err != nil || url != portalInitech.StartURL {
		t.Fatal(url, err)
	}
	if _, err := h.eng.PortalURL("nope"); api.AsAppError(err).Code != api.ErrPortalNotFound {
		t.Fatal(err)
	}
}

func TestCheckPortalInput(t *testing.T) {
	h := newHarness(t, true)
	cases := []struct {
		name string
		in   api.PortalInput
		want map[string]api.ErrorCode
		out  api.PortalInputCheck
	}{
		{"ok", api.PortalInput{StartURL: "https://d-9999999999.awsapps.com", Region: "eu-west-1"}, nil,
			api.PortalInputCheck{Name: "d-9999999999", StartURL: "https://d-9999999999.awsapps.com/start", Region: "eu-west-1"}},
		{"guessed region", api.PortalInput{StartURL: "https://ssoins-7223a1b2c3d4e5f6.portal.us-east-1.app.aws"}, nil,
			api.PortalInputCheck{Name: "ssoins-7223a1b2c3d4e5f6", Region: "us-east-1", StartURL: "https://ssoins-7223a1b2c3d4e5f6.portal.us-east-1.app.aws"}},
		{"bad url", api.PortalInput{StartURL: "https://sso.example.com", Region: "us-east-1", Name: "x"}, map[string]api.ErrorCode{"startUrl": api.ErrInvalidStartURL}, api.PortalInputCheck{}},
		{"bad region", api.PortalInput{StartURL: "https://d-1.awsapps.com/start", Region: "mars", Name: "x"}, map[string]api.ErrorCode{"region": api.ErrInvalidRegion}, api.PortalInputCheck{}},
		{"bad name", api.PortalInput{StartURL: "https://d-1.awsapps.com/start", Region: "us-east-1", Name: "a b"}, map[string]api.ErrorCode{"name": api.ErrInvalidName}, api.PortalInputCheck{}},
		{"duplicate name", api.PortalInput{StartURL: "https://d-1.awsapps.com/start", Region: "us-east-1", Name: "initech"}, map[string]api.ErrorCode{"name": api.ErrPortalExists}, api.PortalInputCheck{}},
	}
	for _, c := range cases {
		got := h.eng.CheckPortalInput(c.in)
		if got.Errors == nil {
			t.Fatalf("%s: Errors must be non-nil", c.name)
		}
		if len(got.Errors) != len(c.want) {
			t.Errorf("%s: errors %v want %v", c.name, got.Errors, c.want)
		}
		for k, v := range c.want {
			if got.Errors[k] != v {
				t.Errorf("%s: %s = %s want %s", c.name, k, got.Errors[k], v)
			}
		}
		if c.want == nil && (got.Name != c.out.Name || got.StartURL != c.out.StartURL || got.Region != c.out.Region) {
			t.Errorf("%s: got %+v want %+v", c.name, got, c.out)
		}
	}
}

func TestDiscoverPartialRoles(t *testing.T) {
	h := newHarness(t, true)
	h.writeToken(2*60*60*1e9, "CID-x")
	h.por.SetAccounts(
		[]ssotest.Account{{ID: "777788889999", Name: "initech", Email: "a@x@example.com"}, {ID: "111122223333", Name: "acme", Email: "b@x@example.com"}},
		[]ssotest.Account{{ID: "444455556666", Name: "globex", Email: "c@x@example.com"}},
	)
	h.por.SetRoles("777788889999", "ReadOnlyAccess", "AdministratorAccess")
	h.por.SetRoles("111122223333", "AdministratorAccess", "PowerUserAccess")
	h.por.FailRoles("444455556666")
	res, err := h.eng.Discover(bg, "initech")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 || res[0].AccountName != "acme" || res[1].AccountName != "globex" || res[2].AccountName != "initech" {
		t.Fatalf("order: %v", jsonOf(res))
	}
	if res[1].RolesError == nil || len(res[1].Roles) != 0 || res[1].Roles == nil {
		t.Fatalf("partial failure: %+v", res[1])
	}
	if res[0].RolesError != nil || len(res[0].Roles) != 2 || res[0].Roles[0].Name != "AdministratorAccess" {
		t.Fatalf("%+v", res[0])
	}
	if res[0].Roles[0].SuggestedProfile != "acme-admin" || res[0].Roles[1].SuggestedProfile != "acme-poweruser" {
		t.Fatalf("%+v", res[0].Roles)
	}
	// existing profile: initech/AdministratorAccess == initech-admin
	if res[2].Roles[0].ExistingProfile != "initech-admin" || res[2].Roles[1].ExistingProfile != "initech-readonly" {
		t.Fatalf("%+v", res[2].Roles)
	}
	if res[2].ColorSlot != 2 {
		t.Fatalf("known account colour: %d", res[2].ColorSlot)
	}
	if res[0].ColorSlot == 0 || res[0].ColorSlot == res[2].ColorSlot {
		t.Fatalf("new accounts get unused slots: %d", res[0].ColorSlot)
	}
	if h.store.names["111122223333"] != "acme" {
		t.Fatal("account names must be merged")
	}
	// not signed in
	h2 := newHarness(t, true)
	if _, err := h2.eng.Discover(bg, "initech"); api.AsAppError(err).Code != api.ErrNotSignedIn {
		t.Fatalf("%v", err)
	}
	if _, err := h.eng.Discover(bg, "zzz"); api.AsAppError(err).Code != api.ErrPortalNotFound {
		t.Fatal(err)
	}
	h.por.SetUnauthorized(true)
	if _, err := h.eng.Discover(bg, "initech"); api.AsAppError(err).Code != api.ErrSessionExpired {
		t.Fatalf("%v", err)
	}
	// LoadRoles
	h.por.SetUnauthorized(false)
	da, err := h.eng.LoadRoles(bg, "initech", "777788889999")
	if err != nil || len(da.Roles) != 2 || da.Roles[0].ExistingProfile != "initech-admin" || da.AccountName != "initech" {
		t.Fatalf("%+v %v", da, err)
	}
}

func TestCreateProfilesWritesAndBacksUp(t *testing.T) {
	h := newHarness(t, true)
	before, _ := os.ReadFile(h.p.AWSConfigFile)
	req := api.CreateProfilesRequest{Portal: "initech", Profiles: []api.NewProfileSpec{
		{Name: "initech-prod", AccountID: "777788889999", AccountName: "initech", Role: "ReadOnlyAccess"},
		{Name: "initech-dev", AccountID: "777788889999", AccountName: "initech", Role: "PowerUserAccess", Region: "us-west-2"},
	}}
	pv, err := h.eng.PreviewCreate(req)
	if err != nil {
		t.Fatal(err)
	}
	if pv.Added != 14 || pv.Removed != 0 || pv.BaseHash == "" || pv.Path != h.p.AWSConfigFile {
		t.Fatalf("%+v", pv)
	}
	res, err := h.eng.Create(req, pv.BaseHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Profiles) != 2 || res.BackupID == "" {
		t.Fatalf("%+v", res)
	}
	after, _ := os.ReadFile(h.p.AWSConfigFile)
	add := "\n[profile initech-prod]\nsso_session    = initech\nsso_account_id = 777788889999\nsso_role_name  = ReadOnlyAccess\nregion         = us-west-2\noutput         = json\n" +
		"\n[profile initech-dev]\nsso_session    = initech\nsso_account_id = 777788889999\nsso_role_name  = PowerUserAccess\nregion         = us-west-2\noutput         = json\n"
	if string(after) != string(before)+add {
		t.Fatalf("config:\n%s", after)
	}
	ent, bak, err := h.bk.Get(res.BackupID)
	if err != nil || string(bak) != string(before) || ent.Reason != api.ReasonAddProfiles || ent.ReasonParams["n"] != "2" || ent.Kind != api.BackupAWSConfig {
		t.Fatalf("%+v %v", ent, err)
	}
	if len(h.writes) != 1 || h.writes[0] != h.p.AWSConfigFile {
		t.Fatalf("OnWrite: %v", h.writes)
	}
	if len(h.emitted(api.EventAccountsChanged)) == 0 {
		t.Fatal("accounts:changed")
	}
	fi, _ := os.Stat(h.p.AWSConfigFile)
	if fi.Mode().Perm() != 0o600 {
		t.Fatal(fi.Mode())
	}
	// validation
	for name, bad := range map[string]api.CreateProfilesRequest{
		"dup":      {Portal: "initech", Profiles: []api.NewProfileSpec{{Name: "initech-admin", AccountID: "777788889999", Role: "R"}}},
		"badname":  {Portal: "initech", Profiles: []api.NewProfileSpec{{Name: "a b", AccountID: "777788889999", Role: "R"}}},
		"inreq":    {Portal: "initech", Profiles: []api.NewProfileSpec{{Name: "n1", AccountID: "777788889999", Role: "R"}, {Name: "n1", AccountID: "777788889999", Role: "R"}}},
		"noportal": {Portal: "zzz", Profiles: []api.NewProfileSpec{{Name: "n1", AccountID: "777788889999", Role: "R"}}},
		"region":   {Portal: "initech", Profiles: []api.NewProfileSpec{{Name: "n1", AccountID: "777788889999", Role: "R", Region: "mars"}}},
	} {
		if _, err := h.eng.PreviewCreate(bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestCreateProfilesWithDraftPortal(t *testing.T) {
	h := newHarness(t, true)
	h.eng.AddDraft(api.PortalInput{Name: "acme", StartURL: "https://d-1111111111.awsapps.com/start", Region: "us-east-1"})
	req := api.CreateProfilesRequest{Portal: "acme", Profiles: []api.NewProfileSpec{{Name: "acme-admin", AccountID: "111122223333", Role: "AdministratorAccess"}}}
	pv, err := h.eng.PreviewCreate(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.eng.Create(req, pv.BaseHash); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(h.p.AWSConfigFile)
	d, err := ini.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := d.Get("sso-session acme", "sso_start_url"); v != "https://d-1111111111.awsapps.com/start" {
		t.Fatalf("session section missing:\n%s", b)
	}
	if v, _ := d.Get("sso-session acme", "sso_registration_scopes"); v != "sso:account:access" {
		t.Fatal(v)
	}
	if h.eng.IsDraft("acme") {
		t.Fatal("draft must be consumed")
	}
	st, _ := h.eng.State(bg)
	if len(st.Portals) != 2 {
		t.Fatalf("%+v", st.Portals)
	}
}

func TestCreateProfilesRejectsStaleBaseHash(t *testing.T) {
	h := newHarness(t, true)
	req := api.CreateProfilesRequest{Portal: "initech", Profiles: []api.NewProfileSpec{{Name: "initech-prod", AccountID: "777788889999", Role: "ReadOnlyAccess"}}}
	pv, _ := h.eng.PreviewCreate(req)
	// the user edits the file in an editor
	f, _ := os.OpenFile(h.p.AWSConfigFile, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("# edited\n")
	f.Close()
	edited, _ := os.ReadFile(h.p.AWSConfigFile)
	_, err := h.eng.Create(req, pv.BaseHash)
	ae := api.AsAppError(err)
	if ae.Code != api.ErrConfigChanged || ae.Params["file"] == "" {
		t.Fatalf("%v", err)
	}
	now, _ := os.ReadFile(h.p.AWSConfigFile)
	if string(now) != string(edited) {
		t.Fatal("file must be untouched")
	}
	if l, _ := h.bk.List(); len(l) != 0 {
		t.Fatal("no backup before a refused write")
	}
}

func TestRemoveProfile(t *testing.T) {
	h := newHarness(t, true)
	pv, err := h.eng.PreviewRemove("globex")
	if err != nil || pv.Removed != 3 {
		t.Fatalf("%+v %v", pv, err)
	}
	creds0, _ := os.ReadFile(h.p.AWSCredentialsFile)
	res, err := h.eng.Remove("globex", pv.BaseHash)
	if err != nil || res.BackupID == "" {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(h.p.AWSConfigFile)
	if strings.Contains(string(b), "globex") || !strings.Contains(string(b), "# ===== 平台基础设施") {
		t.Fatalf("%s", b)
	}
	creds1, _ := os.ReadFile(h.p.AWSCredentialsFile)
	if string(creds0) != string(creds1) {
		t.Fatal("credentials must stay untouched")
	}
	if _, err := h.eng.Remove("globex", pv.BaseHash); err == nil {
		t.Fatal("stale hash / missing profile must fail")
	}
	if _, err := h.eng.PreviewRemove("nope"); api.AsAppError(err).Code != api.ErrProfileNotFound {
		t.Fatal(err)
	}
}

func TestIdentifyStaticInvalidKey(t *testing.T) {
	h := newHarness(t, true)
	// globex's key is unknown to STS -> rejected
	h.aws.Identities["AKIAIOSFODNN7EXAMPLE"] = awsapiIdentity("111122223333", "arn:aws:iam::111122223333:user/acme-dev")
	id, err := h.eng.IdentifyStatic(bg, "globex")
	if err != nil || id.ErrorCode != "InvalidClientTokenId" || id.AccountID != "" || id.MaskedKeyID != "AKIA••••••••••••MPLE" {
		t.Fatalf("%+v %v", id, err)
	}
	st, _ := h.eng.State(bg)
	if p := profileByName(st, "globex"); p.StaticState != api.StaticInvalid || p.StaticError != "InvalidClientTokenId" {
		t.Fatalf("%+v", p)
	}
	// the valid key: identity stored, colour re-keyed to the account id
	id, err = h.eng.IdentifyStatic(bg, "acme")
	if err != nil || id.AccountID != "111122223333" || id.IAMUser != "acme-dev" || id.IsRoot || id.ErrorCode != "" {
		t.Fatalf("%+v %v", id, err)
	}
	st, _ = h.eng.State(bg)
	sk := profileByName(st, "acme")
	if sk.StaticState != api.StaticOK || sk.AccountID != "111122223333" || sk.IAMUser != "acme-dev" || sk.ColorSlot != 1 {
		t.Fatalf("%+v", sk)
	}
	if h.store.colors["111122223333"] != 1 {
		t.Fatalf("colour must be re-keyed: %v", h.store.colors)
	}
	if _, ok := h.store.colors["profile:acme"]; ok {
		t.Fatal("old profile key must be gone")
	}
	if _, err := h.eng.IdentifyStatic(bg, "initech-admin"); api.AsAppError(err).Code != api.ErrNoStaticKey {
		t.Fatalf("%v", err)
	}
	if strings.Contains(jsonOf(id), "fakeSecret") || strings.Contains(jsonOf(h.emitted(api.EventAccountsChanged)), "AKIAIOSF") {
		t.Fatal("secret leaked")
	}
	// network failure is an error, not an identity
	h.eng.d.Endpoints.STS = "http://127.0.0.1:1"
	if _, err := h.eng.IdentifyStatic(bg, "acme"); api.AsAppError(err).Code != api.ErrNetwork {
		t.Fatalf("%v", err)
	}
}

func TestSetIdentity(t *testing.T) {
	h := newHarness(t, true)
	st, err := h.eng.SetIdentity(api.IdentityUpdate{Profile: "initech-admin", ColorSlot: 8, Monogram: "xq"})
	if err != nil {
		t.Fatal(err)
	}
	if p := profileByName(st, "initech-admin"); p.ColorSlot != 8 || p.Monogram != "XQ" {
		t.Fatalf("%+v", p)
	}
	if p := profileByName(st, "initech-readonly"); p.ColorSlot != 8 {
		t.Fatal("same account shares the slot")
	}
	if st, _ = h.eng.SetIdentity(api.IdentityUpdate{Profile: "initech-admin", ColorSlot: 8}); profileByName(st, "initech-admin").Monogram != "IA" {
		t.Fatal("empty monogram removes the override")
	}
	for _, bad := range []api.IdentityUpdate{{Profile: "initech-admin", ColorSlot: 0}, {Profile: "initech-admin", ColorSlot: 9}, {Profile: "initech-admin", ColorSlot: 1, Monogram: "ABC"}} {
		if _, err := h.eng.SetIdentity(bad); api.AsAppError(err).Code != api.ErrInvalidInput {
			t.Errorf("%+v accepted", bad)
		}
	}
	if _, err := h.eng.SetIdentity(api.IdentityUpdate{Profile: "zzz", ColorSlot: 1}); api.AsAppError(err).Code != api.ErrProfileNotFound {
		t.Fatal(err)
	}
}

func TestProfileCredsUsable(t *testing.T) {
	h := newHarness(t, true)
	find := func(cs []ProfileCreds, n string) ProfileCreds {
		for _, c := range cs {
			if c.Name == n {
				return c
			}
		}
		t.Fatalf("%s missing", n)
		return ProfileCreds{}
	}
	cs, err := h.eng.ProfileCreds(bg)
	if err != nil || len(cs) != 4 {
		t.Fatal(err, len(cs))
	}
	sk := find(cs, "acme")
	if !sk.Usable || sk.Provider == nil || sk.Kind != api.ProfileKindStatic || sk.Monogram != "AC" {
		t.Fatalf("%+v", sk)
	}
	wm := find(cs, "initech-admin")
	if wm.Usable || wm.Provider != nil || wm.NotSignedInPortal != "initech" || wm.AccountID != "777788889999" || wm.Portal != "initech" {
		t.Fatalf("%+v", wm)
	}
	h.writeToken(time_hours(2), "CID-x")
	cs, _ = h.eng.ProfileCreds(bg)
	wm = find(cs, "initech-admin")
	if !wm.Usable || wm.Provider == nil || wm.NotSignedInPortal != "" {
		t.Fatalf("%+v", wm)
	}
	creds, err := wm.Provider.Retrieve(bg)
	if err != nil || creds.AccessKeyID != "ASIAIOSFODNN7EXAMPLE" {
		t.Fatalf("SSO provider must use portal credentials: %v", err)
	}
}

func TestFilesNotTouchedByReads(t *testing.T) {
	h := newHarness(t, true)
	a, _ := os.ReadFile(h.p.AWSConfigFile)
	h.eng.State(bg)
	h.eng.Reload(bg)
	h.eng.CheckPortalInput(api.PortalInput{})
	b, _ := os.ReadFile(h.p.AWSConfigFile)
	if string(a) != string(b) {
		t.Fatal("reads must not write")
	}
	if _, err := os.Stat(filepath.Join(h.p.Home, ".aws", "sso")); err == nil {
		t.Fatal("no token dir expected")
	}
}
