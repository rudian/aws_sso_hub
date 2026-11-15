package repos_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/repos"
)

func (f *fixture) add(path string) api.AddRepoResult {
	f.t.Helper()
	res, err := f.eng.AddExisting(context.Background(), path)
	if err != nil {
		f.t.Fatalf("AddExisting(%s): %v", path, err)
	}
	return res
}

func (f *fixture) addErr(path string) *api.AppError {
	f.t.Helper()
	_, err := f.eng.AddExisting(context.Background(), path)
	if err == nil {
		f.t.Fatalf("AddExisting(%s): expected an error", path)
	}
	return api.AsAppError(err)
}

// outside creates a CodeCommit clone outside the scan root.
func (f *fixture) outside(dir, remote string) string {
	return f.sb.InitRepo(f.t, filepath.Join("elsewhere", dir), map[string]string{"origin": remote})
}

func (f *fixture) localConfig(repo string) string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(repo, ".git", "config"))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

func TestAddExistingNotARepo(t *testing.T) {
	f := newFixture(t)
	plain := filepath.Join(f.sb.Root, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(plain, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{plain, file, filepath.Join(f.sb.Root, "missing"), "relative/dir", ""} {
		if e := f.addErr(p); e.Code != api.ErrRepoNotFound {
			t.Errorf("%q: %v", p, e)
		}
	}
	if len(f.store.ExtraRepos()) != 0 {
		t.Error("nothing may be recorded")
	}
}

func TestAddExistingResolvesTopLevel(t *testing.T) {
	f := newFixture(t)
	repo := f.outside("web-app", ccURL("web-app"))
	sub := filepath.Join(repo, "src", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	res := f.add(sub)
	if !res.Added || res.RepoID == "" {
		t.Fatalf("%+v", res)
	}
	r := f.byName(res.State, "web-app")
	if r.ID != res.RepoID || r.Path != repo {
		t.Errorf("top level not resolved: %s vs %s", r.Path, repo)
	}
	if got := f.store.ExtraRepos(); len(got) != 1 || got[0] != repo {
		t.Errorf("extra repos: %v", got)
	}
}

func TestAddExistingNoCodeCommit(t *testing.T) {
	f := newFixture(t)
	gh := f.sb.InitRepo(t, "elsewhere/gh", map[string]string{"origin": "https://bob:hunter2@github.com/o/r.git", "up": "git@gitlab.example.com:o/r.git"})
	e := f.addErr(gh)
	if e.Code != api.ErrNotCodeCommit {
		t.Fatalf("%v", e)
	}
	if e.Detail != "github.com, gitlab.example.com" || strings.Contains(e.Error()+e.Detail, "hunter2") || strings.Contains(e.Detail, "bob") {
		t.Errorf("detail: %q", e.Detail)
	}
	if len(f.store.ExtraRepos()) != 0 {
		t.Error("nothing may be recorded")
	}
	noRemote := f.sb.InitRepo(t, "elsewhere/none", nil)
	if e := f.addErr(noRemote); e.Code != api.ErrNotCodeCommit {
		t.Errorf("no remote: %v", e)
	}
}

func TestAddExistingAlreadyListed(t *testing.T) {
	f := newFixture(t)
	inRoot := f.newRepo("inroot", ccURL("inroot_repo"))
	f.state()
	res := f.add(inRoot)
	if res.Added || len(f.store.ExtraRepos()) != 0 {
		t.Errorf("scan-root repo must not be added: %+v", res)
	}
	// even before the first scan
	f2 := newFixture(t)
	repo := f2.newRepo("inroot", ccURL("inroot_repo"))
	if res := f2.add(repo); res.Added || res.RepoID == "" {
		t.Errorf("unscanned: %+v", res)
	}
	// a hand-added repo is listed once
	out := f.outside("x", ccURL("x_repo"))
	if r := f.add(out); !r.Added {
		t.Fatal("first add")
	}
	if r := f.add(out); r.Added || len(f.store.ExtraRepos()) != 1 {
		t.Errorf("second add: %+v %v", r, f.store.ExtraRepos())
	}
}

func TestAddExistingAddsExtraOutsideRoots(t *testing.T) {
	f := newFixture(t)
	repo := f.outside("x", ccURL("x_repo"))
	before := f.events.count(api.EventReposChanged)
	res := f.add(repo)
	if !res.Added || res.AutoBound || res.Outcome != nil {
		t.Fatalf("%+v", res)
	}
	if got := f.byName(res.State, "x"); !got.Extra || got.ID != res.RepoID {
		t.Errorf("repo: %+v", got)
	}
	if f.events.count(api.EventReposChanged) <= before {
		t.Error("repos:changed expected")
	}
	// a repo from a scan root is not Extra
	in := f.newRepo("in", ccURL("in_repo"))
	_ = in
	st, _ := f.eng.Scan(context.Background())
	if f.byName(st, "in").Extra || !f.byName(st, "x").Extra {
		t.Error("Extra flag wrong")
	}
	// the repo is still listed after a rescan, and deep or hidden paths below a root count as outside
	deep := f.sb.InitRepo(t, "code/a/b/c/d/e/too-deep", map[string]string{"origin": ccURL("deep_repo")})
	if r := f.add(deep); !r.Added {
		t.Errorf("too deep for the scan: %+v", r)
	}
	hidden := f.sb.InitRepo(t, "code/.hidden/h", map[string]string{"origin": ccURL("hidden_repo")})
	if r := f.add(hidden); !r.Added {
		t.Errorf("hidden folder: %+v", r)
	}
}

func TestAddExistingAutoBindsSingleSSO(t *testing.T) {
	f := newFixture(t)
	repo := f.outside("web-app", ccURL("web-app"))
	f.ccAPIs["INITECH"].repos["web-app"] = "rid-initech"
	res := f.add(repo)
	if !res.Added || !res.AutoBound || res.Outcome == nil || !res.Outcome.OK || res.Outcome.Repo == nil {
		t.Fatalf("%+v", res)
	}
	r := f.byName(res.State, "web-app")
	if r.Binding.Source != api.BindingLocal || r.Binding.Profile != "initech-admin" || r.ToFix || r.Owner.Status != api.OwnerSingle {
		t.Errorf("binding: %+v owner %+v", r.Binding, r.Owner)
	}
	cfg := f.localConfig(repo)
	if !strings.Contains(cfg, `[credential "https://`+hostAP+`"]`) || !strings.Contains(cfg, "profile = initech-admin") || !strings.Contains(cfg, "accountId = 777788889999") {
		t.Errorf("local config:\n%s", cfg)
	}
	if len(f.backup.snaps) != 1 || f.backup.snaps[0].Reason != api.ReasonBind || f.backup.snaps[0].Kind != api.BackupGitLocal {
		t.Errorf("snapshots: %+v", f.backup.snaps)
	}
	// Unbind is an exact undo
	if _, err := f.eng.Unbind(context.Background(), res.RepoID); err != nil {
		t.Fatal(err)
	}
	if cfg := f.localConfig(repo); strings.Contains(cfg, "credential") || strings.Contains(cfg, "awsssohub") {
		t.Errorf("unbind left config behind:\n%s", cfg)
	}
}

func TestAddExistingNoAutoBindWhenLocallyBound(t *testing.T) {
	f := newFixture(t)
	repo := f.newRepo("web-app", ccURL("web-app"))
	f.ccAPIs["INITECH"].repos["web-app"] = "rid-initech"
	if err := f.sb.Git.Bind(context.Background(), gitx.BindSpec{RepoDir: repo, Hosts: []string{hostAP}, Profile: "initech-readonly", AccountID: acctInitech, AWSPath: f.aws, Lang: "en", GOOS: "darwin"}); err != nil {
		t.Fatal(err)
	}
	res := f.add(repo)
	if res.AutoBound || res.Outcome != nil {
		t.Fatalf("a locally bound repo must not be re-bound: %+v", res)
	}
	r := f.byName(res.State, "web-app")
	if r.Binding.Profile != "initech-readonly" || r.Owner.Candidates[0].Profile != "initech-readonly" {
		t.Errorf("%+v / %+v", r.Binding, r.Owner.Candidates)
	}
	if len(f.backup.snaps) != 0 {
		t.Errorf("no snapshot expected: %+v", f.backup.snaps)
	}
}

func TestAddExistingNoAutoBindForStaticSuggestion(t *testing.T) {
	f := newFixture(t)
	repo := f.outside("web-app", ccURL("web-app"))
	f.ccAPIs["ACME"].repos["web-app"] = "rid-acme"
	res := f.add(repo)
	if !res.Added || res.AutoBound {
		t.Fatalf("%+v", res)
	}
	o := f.byName(res.State, "web-app").Owner
	if o.Status != api.OwnerSingle || o.Candidates[0].Profile != "acme" {
		t.Errorf("the suggestion is still shown: %+v", o)
	}
	if strings.Contains(f.localConfig(repo), "credential") {
		t.Error("config must stay untouched")
	}
}

func TestAddExistingNoAutoBindWhenSettingOff(t *testing.T) {
	f := newFixture(t)
	f.store.noAutoBind = true
	repo := f.outside("web-app", ccURL("web-app"))
	f.ccAPIs["INITECH"].repos["web-app"] = "rid-initech"
	res := f.add(repo)
	if res.AutoBound || !res.Added {
		t.Fatalf("%+v", res)
	}
	if o := f.byName(res.State, "web-app").Owner; o.Status != api.OwnerSingle || o.Candidates[0].Profile != "initech-admin" {
		t.Errorf("owner: %+v", o)
	}
	// AWS CLI missing: same
	f.store.noAutoBind = false
	*f.awsOK = false
	repo2 := f.outside("other", ccURL("other"))
	f.ccAPIs["INITECH"].repos["other"] = "rid-initech2"
	if res := f.add(repo2); res.AutoBound {
		t.Errorf("AWS CLI not ready: %+v", res)
	}
}

func TestAddExistingNoAutoBindForSSHAndUnknownOwner(t *testing.T) {
	f := newFixture(t)
	ssh := f.outside("sshrepo", "ssh://"+hostAP+"/v1/repos/sshrepo")
	f.ccAPIs["INITECH"].repos["sshrepo"] = "rid"
	if res := f.add(ssh); res.AutoBound || !res.Added {
		t.Errorf("ssh: %+v", res)
	}
	nobody := f.outside("nobody", ccURL("nobody"))
	if res := f.add(nobody); res.AutoBound {
		t.Errorf("no owner: %+v", res)
	}
}

func (f *fixture) ambiguousSetup() (repo string, matchSecond func(bool)) {
	bareA := f.sb.InitBare(f.t, "acct_a.git")
	bareB := f.sb.InitBare(f.t, "acct_b.git")
	work := f.sb.InitRepo(f.t, "bwork", nil)
	f.sb.CommitFile(f.t, work, "other.txt", "different\n")
	if res, err := f.sb.Git.Run(context.Background(), work, nil, "push", "-q", "--force", bareB, "HEAD:refs/heads/main"); err != nil || res.ExitCode != 0 {
		f.t.Fatalf("push: %v %s", err, res.Stderr)
	}
	repo = f.outside("web-app", ccURL("web-app"))
	if res, err := f.sb.Git.Run(context.Background(), repo, nil, "fetch", "-q", bareA, "main:refs/remotes/origin/main"); err != nil || res.ExitCode != 0 {
		f.t.Fatalf("fetch: %v %s", err, res.Stderr)
	}
	f.ccAPIs["ACME"].repos["web-app"] = "rid-acme"       // 111122223333 is compared first
	f.ccAPIs["INITECH"].repos["web-app"] = "rid-initech" // 777788889999 second
	calls := 0
	matchSecond = func(second bool) {
		calls = 0
		*f.remote = func(region, name string) string {
			calls++
			if !second { // both candidates point at the same history: a tie
				return bareA
			}
			return []string{bareB, bareA}[calls-1] // only the second candidate has the local tip
		}
	}
	return repo, matchSecond
}

func TestAddExistingAmbiguousResolvedByHeadMatch(t *testing.T) {
	f := newFixture(t)
	repo, match := f.ambiguousSetup()
	match(true)
	res := f.add(repo)
	o := f.byName(res.State, "web-app").Owner
	if o.Status != api.OwnerAmbiguous || len(o.Candidates) != 2 || o.Candidates[0].HeadMatch || !o.Candidates[1].HeadMatch {
		t.Fatalf("owner: %+v", o)
	}
	if !res.AutoBound || res.Outcome == nil {
		t.Fatalf("a unique head match must bind: %+v", res)
	}
	if b := f.byName(res.State, "web-app").Binding; b.Profile != "initech-admin" || b.Source != api.BindingLocal {
		t.Errorf("binding: %+v", b)
	}
}

func TestAddExistingAmbiguousTieStaysUnbound(t *testing.T) {
	f := newFixture(t)
	repo, match := f.ambiguousSetup()
	match(false)
	res := f.add(repo)
	o := f.byName(res.State, "web-app").Owner
	if o.Status != api.OwnerAmbiguous || res.AutoBound {
		t.Fatalf("a tie needs the user's choice: %+v / %+v", res, o)
	}
}

func TestSuggestedProfileOrdering(t *testing.T) {
	f := newFixture(t)
	// one account, four profiles; every provider answers for the same account
	const acct = "999900001111"
	*f.profs = []repos.ProfileInfo{
		{Name: "zz-sso", AccountID: acct, Kind: api.ProfileKindSSO, ColorSlot: 2, Monogram: "ZZ", Usable: true, Provider: prov("ACME")},
		{Name: "bb-sso", AccountID: acct, Kind: api.ProfileKindSSO, ColorSlot: 2, Monogram: "BB", Usable: false, NotSignedInPortal: "p", Provider: prov("ACME")},
		{Name: "aa-static", AccountID: acct, Kind: api.ProfileKindStatic, ColorSlot: 2, Monogram: "AA", Usable: true, Provider: prov("ACME")},
		{Name: "cc-static", AccountID: acct, Kind: api.ProfileKindStatic, ColorSlot: 2, Monogram: "CC", Usable: true, Provider: prov("ACME")},
	}
	f.ccAPIs["ACME"].account = acct
	suggest := func(name string) string {
		t.Helper()
		id := f.byName(f.state(), name).ID
		st, err := f.eng.DetectOwners(context.Background(), []string{id})
		if err != nil {
			t.Fatal(err)
		}
		o := f.byName(st, name).Owner
		if len(o.Candidates) != 1 {
			t.Fatalf("%s: %+v", name, o)
		}
		return o.Candidates[0].Profile
	}
	for _, n := range []string{"r1", "r2", "r3", "r4"} {
		f.newRepo(n, ccURL(n+"_repo"))
		f.ccAPIs["ACME"].repos[n+"_repo"] = "rid-" + n
	}
	// (d) SSO beats static, then the name decides; a signed-out SSO profile is still suggested
	if got := suggest("r1"); got != "bb-sso" {
		t.Errorf("name tie-break among SSO: %s", got)
	}
	// (c) the profile with the most bound repositories wins among equals
	id2 := f.byName(f.state(), "r2").ID
	id3 := f.byName(f.state(), "r3").ID
	for _, id := range []string{id2, id3} {
		if _, err := f.eng.Bind(context.Background(), id, "zz-sso"); err != nil {
			t.Fatal(err)
		}
	}
	if got := suggest("r1"); got != "zz-sso" {
		t.Errorf("most bound repos: %s", got)
	}
	// (a) a repository bound to a static profile keeps it
	id4 := f.byName(f.state(), "r4").ID
	if _, err := f.eng.Bind(context.Background(), id4, "cc-static"); err != nil {
		t.Fatal(err)
	}
	if got := suggest("r4"); got != "cc-static" {
		t.Errorf("already bound profile: %s", got)
	}
	// following the global default does not count as being bound
	f.sb.WriteGlobal(t, "[credential]\n\thelper = \n\thelper = !aws codecommit credential-helper $@ --profile aa-static\n\tusehttppath = true\n")
	f.newRepo("r5", ccURL("r5_repo"))
	f.ccAPIs["ACME"].repos["r5_repo"] = "rid-r5"
	if _, err := f.eng.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := suggest("r5"); got != "zz-sso" {
		t.Errorf("global default must not pin the suggestion: %s", got)
	}
}

func TestRemoveExtraOnlyExtras(t *testing.T) {
	f := newFixture(t)
	in := f.newRepo("inroot", ccURL("inroot_repo"))
	out := f.outside("x", ccURL("x_repo"))
	res := f.add(out)
	inID := f.byName(res.State, "inroot").ID
	_, err := f.eng.RemoveExtra(context.Background(), inID)
	if e := api.AsAppError(err); err == nil || e.Code != api.ErrInvalidInput || e.Params["field"] != "repoId" {
		t.Fatalf("scan-root repo: %v", err)
	}
	if _, err := f.eng.RemoveExtra(context.Background(), "0000000000000000"); err == nil || api.AsAppError(err).Code != api.ErrRepoNotFound {
		t.Fatalf("unknown id: %v", err)
	}
	st, err := f.eng.RemoveExtra(context.Background(), res.RepoID)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Repos) != 1 || len(f.store.ExtraRepos()) != 0 {
		t.Errorf("state: %d repos, extras %v", len(st.Repos), f.store.ExtraRepos())
	}
	// the folder and its config are untouched
	if _, err := os.Stat(filepath.Join(out, ".git", "config")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(in); err != nil {
		t.Error(err)
	}
	// a repository that sits under a scan root but also in extraRepos is not removable
	_ = f.store.AddExtraRepo(in)
	st, _ = f.eng.Scan(context.Background())
	if f.byName(st, "inroot").Extra {
		t.Error("a scan-root repo is never Extra")
	}
}
