package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudian/aws_sso_hub/internal/api"
)

func portalOf(st api.AccountsState, name string) (api.Portal, bool) {
	for _, p := range st.Portals {
		if p.Name == name {
			return p, true
		}
	}
	return api.Portal{}, false
}

func profileOf(st api.AccountsState, name string) (api.Profile, bool) {
	for _, p := range st.Profiles {
		if p.Name == name {
			return p, true
		}
	}
	return api.Profile{}, false
}

func repoNamed(rs api.ReposState, name string) api.Repo {
	for _, r := range rs.Repos {
		if r.Name == name {
			return r
		}
	}
	active.t.Fatalf("repo %s not in state", name)
	return api.Repo{}
}

// runScenario walks one user through the app: sign in, migrate acme to SSO, scan and bind repositories,
// retire the global default. It asserts as it goes; every result and error is recorded for the secret scan.
func runScenario(w *world) {
	t := w.t
	t.Helper()

	awsPath := w.fake
	set := ok(w.sys.UpdateSettings(api.SettingsPatch{AWSCLIPath: &awsPath}))
	if set.AWSCLIPath != awsPath {
		t.Fatalf("settings: %+v", set)
	}
	if w.rt.count(api.EventSettingsChanged) != 1 {
		t.Fatal("settings:changed must be emitted once")
	}
	info := ok(w.sys.GetAppInfo())
	if !info.Sandbox || !info.FirstRun || info.Version == "" {
		t.Fatalf("app info: %+v", info)
	}

	// --- accounts: first paint comes from disk, no network
	st := ok(w.acc.GetState(bg))
	if p, found := portalOf(st, "initech"); !found || p.Session.State != api.SessionSignedOut {
		t.Fatalf("initech before login: %+v", p)
	}
	if p, found := portalOf(st, "other"); !found || p.Session.State != api.SessionActive || p.Session.SessionKnown {
		t.Fatalf("other (signed in by another tool): %+v", p)
	}
	if st.StaticCount != 2 {
		t.Fatalf("static count %d", st.StaticCount)
	}
	id := ok(w.acc.IdentifyStatic(bg, "acme"))
	if id.AccountID != acctAcme || id.MaskedKeyID != "AKIA••••••••••••MPLE" || id.IAMUser != "acme-dev" {
		t.Fatalf("identity: %+v", id)
	}

	// --- login through the (fake) browser
	lr := ok(w.acc.Login(bg, "initech", api.LoginModeBrowser))
	if lr.Portal != "initech" || lr.ExpiresAt == "" {
		t.Fatalf("login: %+v", lr)
	}
	st = ok(w.acc.GetState(bg))
	p, _ := portalOf(st, "initech")
	if p.Session.State != api.SessionActive || !p.Session.SessionKnown || p.Session.SessionEndsAt == "" || p.Session.SessionMinutes != 480 {
		t.Fatalf("session after login: %+v", p.Session)
	}
	if w.rt.count(api.EventLoginProgress) == 0 || w.rt.count(api.EventSessionChanged) == 0 {
		t.Fatalf("events: %v", w.rt.names())
	}
	disc := ok(w.acc.DiscoverAccounts(bg, "initech"))
	if len(disc) != 1 || disc[0].AccountID != acctAcme || len(disc[0].Roles) != 1 || disc[0].Roles[0].Name != "AdministratorAccess" {
		t.Fatalf("discover: %+v", disc)
	}

	// --- migrate acme into initech
	ms := ok(w.mig.Start(bg, "acme"))
	if ms.Step != api.MigrationIdentify || !ms.KeyInMemory || ms.Identity.AccountID != acctAcme {
		t.Fatalf("start: %+v", ms)
	}
	ok(w.mig.ChoosePortal("acme", "initech"))
	ok(w.mig.ChooseAccount("acme", api.MigrationAccountChoice{AccountID: acctAcme, AccountName: "acme", Role: "AdministratorAccess"}))
	ms = ok(w.mig.Preview("acme"))
	if ms.ConfigPreview == nil {
		t.Fatal("no config preview")
	}
	ms = ok(w.mig.Apply(bg, "acme", ms.ConfigPreview.BaseHash))
	if ms.Step != api.MigrationVerify || ms.Verify == nil || !ms.Verify.OK || ms.Verify.AccountID != acctAcme {
		t.Fatalf("apply: %+v", ms)
	}
	ms = ok(w.mig.PreviewKeyRemoval("acme"))
	ms = ok(w.mig.RemoveKey("acme", ms.CredentialsPreview.BaseHash))
	if !ms.KeyRemoved {
		t.Fatalf("remove key: %+v", ms)
	}
	ms = ok(w.mig.Deactivate(bg, "acme"))
	if ms.Deactivation == nil || ms.Deactivation.Status != api.DeactivationDone {
		t.Fatalf("deactivate: %+v", ms.Deactivation)
	}
	ok(w.mig.Finish("acme"))
	st = ok(w.acc.GetState(bg))
	if pr, found := profileOf(st, "acme"); !found || pr.Kind != api.ProfileKindSSO || pr.Portal != "initech" || pr.HasStaticKeys {
		t.Fatalf("acme after migration: %+v", pr)
	}
	if strings.Contains(w.read(w.p.AWSCredentialsFile), "acme") {
		t.Fatal("acme key must be gone from the credentials file")
	}
	if !strings.Contains(w.read(w.p.AWSConfigFile), "[sso-session other]") {
		t.Fatal("the rest of the config must be untouched")
	}

	// --- repositories
	w.gitRepo("web-app", ccURL("web-app"))
	w.gitRepo("infra-terraform", ccURL("infra-terraform"))
	w.gitRepo("orphan_tool", ccURL("orphan_tool"))
	rs := ok(w.repo.Scan(bg))
	if rs.Counts.All != 3 || !rs.AWSCLIReady {
		t.Fatalf("scan: counts %+v ready %v", rs.Counts, rs.AWSCLIReady)
	}
	rs = ok(w.repo.DetectOwners(bg, nil))
	ba := repoNamed(rs, "web-app")
	if ba.Owner.Status != api.OwnerSingle || len(ba.Owner.Candidates) != 1 || ba.Owner.Candidates[0].AccountID != acctAcme || ba.Owner.Candidates[0].Profile != "acme" {
		t.Fatalf("owner of web-app: %+v", ba.Owner)
	}
	if o := repoNamed(rs, "infra-terraform").Owner; o.Status != api.OwnerSingle || o.Candidates[0].AccountID != acctInitech {
		t.Fatalf("owner of infra-terraform: %+v", o)
	}
	if o := repoNamed(rs, "orphan_tool").Owner; o.Status != api.OwnerNone {
		t.Fatalf("owner of orphan_tool: %+v", o)
	}

	prev := ok(w.repo.PreviewBind(bg, ba.ID, "acme"))
	if len(prev.Commands) == 0 {
		t.Fatalf("bind preview: %+v", prev)
	}
	before := w.rt.count(api.EventAccountsChanged)
	out := ok(w.repo.Bind(bg, ba.ID, "acme"))
	if !out.OK || out.Repo == nil || out.Repo.Binding.Source != api.BindingLocal || out.Repo.Binding.Profile != "acme" {
		t.Fatalf("bind: %+v", out)
	}
	if w.rt.count(api.EventAccountsChanged) <= before || w.rt.count(api.EventReposChanged) == 0 {
		t.Fatal("a binding change must emit repos:changed and accounts:changed")
	}
	gc := w.read(filepath.Join(ba.Path, ".git", "config"))
	if !strings.Contains(gc, `[credential "https://`+hostAP+`"]`) || !strings.Contains(gc, awsPath) {
		t.Fatalf("local git config:\n%s", gc)
	}

	// --- global git: migrate the URL rule, then remove the default
	gs := ok(w.glob.GetState(bg))
	if gs.GlobalDefault == nil || gs.GlobalDefault.Profile != "acme" || len(gs.URLRules) != 1 || gs.Guard.Installed {
		t.Fatalf("global state: %+v", gs)
	}
	remove := api.DefaultChangeRequest{Action: api.DefaultRemove}
	if ae := fails(w.glob.PreviewDefaultChange(bg, remove)); ae.Code != api.ErrURLRulesFirst {
		t.Fatalf("remove before migrating the rule: %v", ae)
	}
	wl := repoNamed(rs, "infra-terraform")
	plan := ok(w.glob.PreviewURLRuleMigration(bg, gs.URLRules[0].Pattern))
	mr := ok(w.glob.MigrateURLRule(bg, api.URLRuleMigration{Pattern: gs.URLRules[0].Pattern, Assignments: []api.BindItem{{RepoID: wl.ID, Profile: "initech-admin"}}}, plan.Preview.BaseHash))
	if !mr.RuleRemoved || len(mr.Outcomes) != 1 || !mr.Outcomes[0].OK {
		t.Fatalf("migrate rule: %+v", mr)
	}
	pv := ok(w.glob.PreviewDefaultChange(bg, remove))
	res := ok(w.glob.ApplyDefaultChange(bg, remove, pv.Preview.BaseHash))
	if !res.State.Guard.Installed || res.State.GlobalDefault != nil || res.BackupID == "" {
		t.Fatalf("after removing the default: %+v", res.State)
	}
	if g := w.read(w.p.GitGlobalFile); !strings.Contains(g, "git-codecommit.*.amazonaws.com") {
		t.Fatalf("guard missing:\n%s", g)
	}

	// --- system
	if bks := ok(w.sys.ListBackups()); len(bks) < 4 {
		t.Fatalf("expected backups for config, credentials, local git, global git: %d", len(bks))
	}
	ok(w.sys.CheckHealth(bg))
	sum := ok(w.sys.FirstRunSummary(bg))
	if sum.Repos != 3 || sum.Portals < 2 {
		t.Fatalf("first-run summary: %+v", sum)
	}
	env := ok(w.sys.CheckEnvironment(bg))
	if env.AWSCLI.Path != awsPath || len(env.Files) != 3 {
		t.Fatalf("environment: %+v", env)
	}
	w.c.Log.Info("scenario finished", "profile", "acme")
}

func TestServicesHappyPath(t *testing.T) {
	w := newWorld(t)
	runScenario(w)
}

func TestNoSecretsCrossBridge(t *testing.T) {
	w := newWorld(t)
	runScenario(w)
	w.c.Shutdown()

	var corpus []string
	corpus = append(corpus, w.recorded...)
	for _, ev := range w.rt.snapshot() {
		b, err := json.Marshal(ev.Data)
		if err != nil {
			t.Fatalf("event %s is not JSON: %v", ev.Name, err)
		}
		corpus = append(corpus, ev.Name+" "+string(b))
	}
	if len(w.recorded) < 30 || len(w.rt.snapshot()) < 10 {
		t.Fatalf("scan corpus too small: %d results, %d events", len(w.recorded), len(w.rt.snapshot()))
	}
	files := []string{w.p.SettingsFile, filepath.Join(w.p.LogDir, "app.log")}
	baks, _ := filepath.Glob(filepath.Join(w.p.BackupDir, "*.bak"))
	if len(baks) == 0 {
		t.Fatal("no backups to scan")
	}
	files = append(files, baks...)
	files = append(files, filepath.Join(w.p.BackupDir, "index.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		corpus = append(corpus, string(b))
	}
	for i, text := range corpus {
		for _, secret := range secretStrings {
			if strings.Contains(text, secret) {
				t.Fatalf("secret %q found in item %d: %.200s", secret, i, text)
			}
		}
	}
	// the fixture is live: the secrets really exist on disk, so the scan above proves something
	tokenFile := filepath.Join(w.p.SSOCacheDir, "*.json")
	toks, _ := filepath.Glob(tokenFile)
	found := false
	for _, f := range toks {
		if b, err := os.ReadFile(f); err == nil && strings.Contains(string(b), "ACCESS-TOKEN-SECRET") {
			found = true
		}
	}
	if !found {
		t.Fatal("the secret fixture token is missing; the scan would prove nothing")
	}
	// slices cross the bridge as [] never null
	for _, text := range w.recorded {
		for _, key := range []string{"issues", "portals", "profiles", "repos", "candidates", "lines", "roles", "findings", "skippedPortals", "urlRules", "otherHelpers", "outcomes"} {
			if strings.Contains(text, `"`+key+`":null`) {
				t.Fatalf("%s is null in %.300s", key, text)
			}
		}
	}
}

func TestEveryErrorIsAppError(t *testing.T) {
	w := newWorld(t)
	bad := api.Theme("neon")
	calls := map[string]func() error{
		"Accounts.IdentifyStatic": func() error { _, e := w.acc.IdentifyStatic(bg, "no-such"); return e },
		"Accounts.Login":          func() error { _, e := w.acc.Login(bg, "no-such", api.LoginModeBrowser); return e },
		"Accounts.LoginMode":      func() error { _, e := w.acc.Login(bg, "initech", api.LoginMode("carrier-pigeon")); return e },
		"Accounts.RemoveProfile":  func() error { _, e := w.acc.RemoveProfile("no-such", "x"); return e },
		"Accounts.AddPortalDraft": func() error {
			_, e := w.acc.AddPortalDraft(api.PortalInput{Name: "x y", StartURL: "http://nope"})
			return e
		},
		"Accounts.OpenPortal": func() error { return w.acc.OpenPortal("no-such") },
		"Migration.Start":     func() error { _, e := w.mig.Start(bg, "no-such"); return e },
		"Migration.Get":       func() error { _, e := w.mig.Get("no-such"); return e },
		"Migration.Apply":     func() error { _, e := w.mig.Apply(bg, "no-such", ""); return e },
		"Repos.Bind":          func() error { _, e := w.repo.Bind(bg, "no-such", "p"); return e },
		"Repos.PreviewBind":   func() error { _, e := w.repo.PreviewBind(bg, "no-such", "p"); return e },
		"Repos.OpenFolder":    func() error { return w.repo.OpenFolder("no-such") },
		"Repos.AddExisting":   func() error { _, e := w.repo.AddExisting(bg, "relative/path"); return e },
		"Repos.RemoveExtra":   func() error { _, e := w.repo.RemoveExtra(bg, "no-such"); return e },
		"GitGlobal.PreviewDefault": func() error {
			_, e := w.glob.PreviewDefaultChange(bg, api.DefaultChangeRequest{Action: "explode"})
			return e
		},
		"GitGlobal.MigrateURLRule": func() error {
			_, e := w.glob.MigrateURLRule(bg, api.URLRuleMigration{Pattern: "https://nope"}, "")
			return e
		},
		"System.UpdateSettings": func() error { _, e := w.sys.UpdateSettings(api.SettingsPatch{Theme: &bad}); return e },
		"System.OpenURL":        func() error { return w.sys.OpenURL("http://example.com") },
		"System.RevealPath":     func() error { return w.sys.RevealPath("/etc") },
		"System.RestoreBackup":  func() error { _, e := w.sys.RestoreBackup("no-such", false); return e },
		"System.PreviewRestore": func() error { _, e := w.sys.PreviewRestore("no-such"); return e },
		"System.SetUILanguage":  func() error { return w.sys.SetUILanguage("fr") },
		"System.OpenTerminal":   func() error { return w.sys.OpenTerminal(bg, "bad profile;rm", "") },
		"System.OpenAWSConfig":  func() error { _ = os.Remove(w.p.AWSConfigFile); return w.sys.OpenAWSConfig() },
	}
	for name, call := range calls {
		err := call()
		if err == nil {
			t.Errorf("%s: expected an error", name)
			continue
		}
		var ae *api.AppError
		if !errors.As(err, &ae) {
			t.Errorf("%s: not an *api.AppError: %T %v", name, err, err)
			continue
		}
		if ae.Code == api.ErrInternal || ae.Code == api.ErrNotImplemented {
			t.Errorf("%s: unexpected code %s (%s)", name, ae.Code, ae.Detail)
		}
	}
}
