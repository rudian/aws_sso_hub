package repos_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/paths"
	"github.com/rudian/codepass/internal/projects"
	"github.com/rudian/codepass/internal/repos"
)

const (
	ghURL = "https://github.com/acme/web.git"
	bbURL = "git@bitbucket.org:globex/api-main.git"
)

// ---- strategies and names ----

func TestStrategyTable(t *testing.T) {
	cases := []struct {
		url  string
		want projects.Provider
		name string
	}{
		{"https://git-codecommit.us-east-1.amazonaws.com/v1/repos/acme_api", projects.ProviderCodeCommit, "acme_api"},
		{"codecommit::us-east-1://globex@grc_repo", projects.ProviderCodeCommit, "grc_repo"},
		{"ssh://git-codecommit.us-east-1.amazonaws.com/v1/repos/ssh_repo", projects.ProviderCodeCommit, "ssh_repo"},
		{"https://github.com/acme/web.git", projects.ProviderGitHub, "web"},
		{"git@github.com:acme/web", projects.ProviderGitHub, "web"},
		{"ssh://ssh.github.com:443/acme/web.git", projects.ProviderGitHub, "web"},
		{"https://user:pw@GitHub.com/acme/web/", projects.ProviderGitHub, "web"},
		{"git@bitbucket.org:globex/api-main.git", projects.ProviderBitbucket, "api-main"},
		{"https://acme@bitbucket.org/globex/api-main.git", projects.ProviderBitbucket, "api-main"},
		{"https://git.example.com/team/tools.git", projects.ProviderOther, "tools"},
		{"git@example.com:team/tools.git", projects.ProviderOther, "tools"},
		{"/srv/git/tools.git", projects.ProviderOther, "tools"},
		{"https://github.com.example.net/acme/web.git", projects.ProviderOther, "web"},
		{"https://notgithub.com/acme/web.git", projects.ProviderOther, "web"},
		{"", projects.ProviderOther, ""},
	}
	for _, c := range cases {
		if got := repos.ProviderFor(c.url); got != c.want {
			t.Errorf("ProviderFor(%q) = %s, want %s", c.url, got, c.want)
		}
	}
	// The remote name is checked through the state (it is not exported).
	f := newFixture(t)
	for i, c := range cases {
		if c.url == "" {
			continue
		}
		f.sb.InitRepo(t, filepath.Join("code", fmt.Sprintf("r%02d", i)), map[string]string{"origin": c.url})
	}
	f.sb.InitRepo(t, "code/noremote", nil)
	f.state()
	st := f.reviewAll()
	for i, c := range cases {
		if c.url == "" {
			continue
		}
		r := f.byName(st, fmt.Sprintf("r%02d", i))
		wantProv := api.RepoProvider(c.want)
		if r.RemoteName != c.name || r.Provider != wantProv || r.LocalName != fmt.Sprintf("r%02d", i) {
			t.Errorf("%s: remoteName %q provider %s localName %q", c.url, r.RemoteName, r.Provider, r.LocalName)
		}
		if c.want != projects.ProviderCodeCommit && r.Binding.Source != api.BindingMachine {
			t.Errorf("%s: binding %s", c.url, r.Binding.Source)
		}
	}
	nr := f.byName(st, "noremote")
	if nr.RemoteName != "" || nr.Provider != api.ProviderOther || nr.Remote != "" || nr.RemoteURL != "" {
		t.Errorf("no remote: %+v", nr)
	}
}

// reviewAll shows every repository that waits for the review and returns the new state.
func (f *fixture) reviewAll() api.ReposState {
	f.t.Helper()
	st := f.state()
	var ids []string
	for _, r := range st.Found {
		ids = append(ids, r.ID)
	}
	st, err := f.eng.ResolveFound(context.Background(), ids, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	return st
}

func TestGitHubAndBitbucketAreListedWithMachineDefaultBinding(t *testing.T) {
	f := newFixture(t)
	f.newRepo("cc", ccURL("cc_repo"))
	f.sb.InitRepo(t, "code/gh", map[string]string{"origin": ghURL})
	f.sb.InitRepo(t, "code/bb", map[string]string{"origin": bbURL})
	st := f.state()
	if len(st.Repos) != 1 || len(st.Found) != 2 || st.Counts.Found != 2 {
		t.Fatalf("before the review: repos %d found %d", len(st.Repos), len(st.Found))
	}
	st = f.reviewAll()
	if len(st.Repos) != 3 || len(st.Found) != 0 || st.Counts.All != 3 {
		t.Fatalf("after the review: %+v", st.Counts)
	}
	for _, n := range []string{"gh", "bb"} {
		r := f.byName(st, n)
		if r.Binding.Source != api.BindingMachine || r.ToFix || r.Binding.Profile != "" || r.Pending || r.Hidden {
			t.Errorf("%s: %+v", n, r)
		}
	}
	if st.Counts.ToFix != 1 || st.Counts.Bound != 0 { // only the CodeCommit repository can need a binding
		t.Errorf("counts: %+v", st.Counts)
	}
	// An account operation on one of them is refused as "not CodeCommit".
	gh := f.byName(st, "gh")
	if _, err := f.eng.PreviewBind(context.Background(), gh.ID, "acme"); err == nil || api.AsAppError(err).Code != api.ErrNotCodeCommit {
		t.Errorf("bind of a GitHub repository: %v", err)
	}
}

// ---- upgrade ----

// The owner's 13 repositories are in projects.json (F1-18 made their records on the first scan after the
// upgrade). New repositories found next to them wait for the review; the 13 stay. A fixture data folder in a
// temp dir stands in for the real one.
func TestUpgradeKeepsThirteenKnownRepositoriesAndNewOnesWait(t *testing.T) {
	f := newFixture(t)
	data := t.TempDir()
	var known []string
	for i := 0; i < 13; i++ {
		known = append(known, f.newRepo(fmt.Sprintf("known%02d", i), ccURL(fmt.Sprintf("known_%02d", i))))
	}
	reg, err := projects.Open(filepath.Join(data, "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.IDsForPaths(known, projects.ProviderCodeCommit, f.clock); err != nil {
		t.Fatal(err)
	}
	// The next version finds forks and experiments as well.
	f.sb.InitRepo(t, "code/fork-a", map[string]string{"origin": "https://github.com/someone/a.git"})
	f.sb.InitRepo(t, "code/fork-b", map[string]string{"origin": "https://github.com/someone/b.git"})
	f.sb.InitRepo(t, "code/scratch", nil)
	f.newRepo("newcc", ccURL("new_cc")) // a CodeCommit clone made outside the app keeps today's behaviour: listed

	eng := f.engineOn(reg, nil)
	st, err := eng.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Repos) != 14 || st.Counts.All != 14 || len(st.Found) != 3 || st.Counts.Found != 3 {
		t.Fatalf("repos %d (counts %+v) found %d", len(st.Repos), st.Counts, len(st.Found))
	}
	have := map[string]bool{}
	for _, r := range st.Repos {
		have[r.Path] = true
	}
	for _, k := range known {
		if !have[k] {
			t.Errorf("known repository %s left the list", k)
		}
	}
	for _, r := range st.Found {
		if !r.Pending || r.Status != nil {
			t.Errorf("found row: %+v", r)
		}
	}
	// The review set survives a restart (a new engine on the same file), and a rescan does not resolve it.
	reg2, err := projects.Open(filepath.Join(data, "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	st2, err := f.engineOn(reg2, nil).Scan(context.Background())
	if err != nil || len(st2.Repos) != 14 || len(st2.Found) != 3 {
		t.Fatalf("after restart: %v repos %d found %d", err, len(st2.Repos), len(st2.Found))
	}
	// "Hide them all": the three leave the review set and become hidden, and they stay out of the next scan.
	var ids []string
	for _, r := range st2.Found {
		ids = append(ids, r.ID)
	}
	eng2 := f.engineOn(reg2, nil)
	if _, err := eng2.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	st3, err := eng2.ResolveFound(context.Background(), nil, ids)
	if err != nil || len(st3.Found) != 0 || len(st3.HiddenRepos) != 3 || len(st3.Repos) != 14 || st3.Counts.Hidden != 3 {
		t.Fatalf("hide them all: %v found %d hidden %d repos %d", err, len(st3.Found), len(st3.HiddenRepos), len(st3.Repos))
	}
	if st4, _ := eng2.Scan(context.Background()); len(st4.Found) != 0 || len(st4.Repos) != 14 {
		t.Errorf("rescan: found %d repos %d", len(st4.Found), len(st4.Repos))
	}
	// An unknown ID is refused.
	if _, err := eng2.ResolveFound(context.Background(), []string{"nope"}, nil); err == nil || api.AsAppError(err).Code != api.ErrRepoNotFound {
		t.Errorf("unknown ID: %v", err)
	}
}

// engineOn builds a second engine on the same repositories, sharing the fixture's settings and git.
func (f *fixture) engineOn(reg *projects.Store, w repos.WatchPort) *repos.Engine {
	return repos.New(repos.Deps{
		Paths: paths.Paths{Home: f.sb.Home}, Git: func(ctx context.Context) (*gitx.Git, error) { return f.sb.Git, nil },
		Store: f.store, Registry: reg, Watch: w, GOOS: "darwin", Now: func() time.Time { return f.clock },
	})
}

// ---- hidden repositories are excluded from background work ----

type fakeWatch struct {
	mu   sync.Mutex
	subs map[string]func(repos.WatchEvent)
	log  []string
}

func newFakeWatch() *fakeWatch { return &fakeWatch{subs: map[string]func(repos.WatchEvent){}} }

func (w *fakeWatch) Subscribe(dir string, fn func(repos.WatchEvent)) (func(), error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.subs[dir] = fn
	w.log = append(w.log, "+"+filepath.Base(dir))
	return func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		delete(w.subs, dir)
		w.log = append(w.log, "-"+filepath.Base(dir))
	}, nil
}

func (w *fakeWatch) logged() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Join(w.log, ",")
}

func (w *fakeWatch) watched() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []string
	for d := range w.subs {
		out = append(out, filepath.Base(d))
	}
	sort.Strings(out)
	return out
}

func (w *fakeWatch) fire(dir string) { w.fireEvent(dir, repos.WatchChanged) }

func (w *fakeWatch) fireEvent(dir string, ev repos.WatchEvent) {
	w.mu.Lock()
	fn := w.subs[dir]
	w.mu.Unlock()
	if fn != nil {
		fn(ev)
	}
}

func TestHiddenRepositoriesAreExcludedFromBackgroundWork(t *testing.T) {
	f := newFixture(t)
	bound := f.newRepo("bound", ccURL("bound_repo"))
	hid := f.newRepo("hid", ccURL("hid_repo"))
	f.sb.InitRepo(t, "code/gh", map[string]string{"origin": ghURL})
	for _, d := range []string{bound, hid} {
		f.sb.CommitFile(t, d, "README.md", "hi\n")
	}
	for _, d := range []string{bound, hid} { // both are bound, one will be hidden
		if err := f.sb.Git.Bind(context.Background(), gitx.BindSpec{RepoDir: d, Hosts: []string{hostAP}, Profile: "acme", AccountID: acctAcme, AWSPath: f.aws, Lang: "en", GOOS: "darwin"}); err != nil {
			t.Fatal(err)
		}
	}
	w := newFakeWatch()
	eng := f.engineOn(f.reg, w)
	defer eng.Close()
	ctx := context.Background()
	st, err := eng.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Before the review only the two CodeCommit repositories are shown and watched; the GitHub one waits.
	if got := w.watched(); strings.Join(got, ",") != "bound,hid" {
		t.Fatalf("watched before review: %v", got)
	}
	if got := eng.FetchTargets(); len(got) != 2 {
		t.Fatalf("fetch targets: %v", got)
	}
	hidID := f.byName(st, "hid").ID
	if _, err := eng.SetHidden(ctx, hidID, true); err != nil {
		t.Fatal(err)
	}
	if got := w.watched(); strings.Join(got, ",") != "bound" {
		t.Errorf("a hidden repository is still watched: %v", got)
	}
	targets := eng.FetchTargets()
	if len(targets) != 1 || targets[0] == hidID {
		t.Errorf("a hidden repository is a fetch target: %v", targets)
	}
	if err := eng.Fetch(ctx, hidID); err == nil {
		t.Error("fetch of a hidden repository ran")
	}
	if err := eng.MarkOpened(hidID); err != nil { // opening does not bring it back
		t.Fatal(err)
	}
	if err := eng.Fetch(ctx, hidID); err == nil {
		t.Error("fetch of an opened but hidden repository ran")
	}
	st2, err := eng.RefreshStatus(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(st2.Repos) != 1 || st2.Repos[0].Status == nil || len(st2.HiddenRepos) != 1 || st2.HiddenRepos[0].Status != nil {
		t.Fatalf("status: repos %+v hidden %+v", st2.Repos, st2.HiddenRepos)
	}
	if st2.Counts.All != 1 || st2.Counts.Hidden != 1 || st2.HiddenRepos[0].LastCommitAt == "" {
		t.Errorf("counts %+v hidden row %+v", st2.Counts, st2.HiddenRepos[0])
	}
	// Unhide: watched, listed, a target again (it is bound).
	if _, err := eng.SetHidden(ctx, hidID, false); err != nil {
		t.Fatal(err)
	}
	if got := w.watched(); strings.Join(got, ",") != "bound,hid" {
		t.Errorf("watched after unhide: %v", got)
	}
	// The review: showing the GitHub repository starts watching it; hiding it afterwards stops again.
	st, _ = eng.State(ctx)
	ghID := st.Found[0].ID
	if _, err := eng.ResolveFound(ctx, []string{ghID}, nil); err != nil {
		t.Fatal(err)
	}
	if got := w.watched(); strings.Join(got, ",") != "bound,gh,hid" {
		t.Errorf("watched after review: %v", got)
	}
	if got := eng.FetchTargets(); len(got) != 2 { // the GitHub repository is neither bound nor opened
		t.Errorf("an unbound, unopened repository is a fetch target: %v", got)
	}
	_ = eng.MarkOpened(ghID)
	if got := eng.FetchTargets(); len(got) != 3 {
		t.Errorf("an opened repository is not a fetch target: %v", got)
	}
}

func TestWatcherEventRefreshesStatusOfOneRepository(t *testing.T) {
	f := newFixture(t)
	a := f.newRepo("a", ccURL("a_repo"))
	b := f.newRepo("b", ccURL("b_repo"))
	f.sb.CommitFile(t, a, "README.md", "hi\n")
	f.sb.CommitFile(t, b, "README.md", "hi\n")
	w := newFakeWatch()
	eng := f.engineOn(f.reg, w)
	defer eng.Close()
	ctx := context.Background()
	if _, err := eng.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.RefreshStatus(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	w.fire(a) // only a's folder reports a change
	deadline := time.Now().Add(5 * time.Second)
	var sa, sb *api.RepoStatus
	for time.Now().Before(deadline) {
		st, _ := eng.State(ctx)
		sa, sb = f.byName(st, "a").Status, f.byName(st, "b").Status
		if sa != nil && sa.Untracked == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if sa == nil || sa.Untracked != 1 || sa.Changes != 1 || sa.Branch != "main" {
		t.Fatalf("a: %+v", sa)
	}
	if sb == nil || sb.Untracked != 0 {
		t.Fatalf("b was re-read without an event: %+v", sb)
	}
}

func TestFetchRunsOnlyForBoundOrOpenedRepositories(t *testing.T) {
	f := newFixture(t)
	bare := f.sb.InitBare(t, "up.git")
	dir := filepath.Join(f.code, "plain")
	if res, err := f.sb.Git.Run(context.Background(), "", nil, "clone", "-q", bare, dir); err != nil || res.ExitCode != 0 {
		t.Fatalf("clone: %v %s", err, res.Stderr)
	}
	// A new commit on the remote.
	work := filepath.Join(f.sb.Root, "up.git-work")
	f.sb.CommitFile(t, work, "more.txt", "more\n")
	if res, err := f.sb.Git.Run(context.Background(), work, nil, "push", "-q", bare, "main"); err != nil || res.ExitCode != 0 {
		t.Fatalf("push: %v %s", err, res.Stderr)
	}
	want := f.sb.CommitFile(t, work, "more2.txt", "more\n")
	if res, err := f.sb.Git.Run(context.Background(), work, nil, "push", "-q", bare, "main"); err != nil || res.ExitCode != 0 {
		t.Fatalf("push: %v %s", err, res.Stderr)
	}
	ctx := context.Background()
	eng := f.engineOn(f.reg, nil)
	st, err := eng.Scan(ctx)
	if err != nil || len(st.Found) != 1 {
		t.Fatalf("%v %+v", err, st.Found)
	}
	id := st.Found[0].ID
	if st.Found[0].Provider != api.ProviderOther || st.Found[0].RemoteName != "up" {
		t.Errorf("found row: %+v", st.Found[0])
	}
	remoteRef := func() string {
		res, _ := f.sb.Git.Run(ctx, dir, nil, "rev-parse", "-q", "--verify", "refs/remotes/origin/main")
		return strings.TrimSpace(string(res.Stdout))
	}
	before := remoteRef()
	if before == want {
		t.Fatal("fixture: the clone is already up to date")
	}
	// Waiting for the review: not a target, and BackgroundFetch does nothing.
	if err := eng.Fetch(ctx, id); err == nil {
		t.Fatal("fetch ran for a repository that waits for the review")
	}
	if ok, failed := eng.BackgroundFetch(ctx); ok != 0 || failed != 0 {
		t.Fatalf("background fetch: %d %d", ok, failed)
	}
	if _, err := eng.ResolveFound(ctx, []string{id}, nil); err != nil {
		t.Fatal(err)
	}
	// Shown but neither bound nor opened: still no fetch.
	if err := eng.Fetch(ctx, id); err == nil || api.AsAppError(err).Code != api.ErrInvalidInput {
		t.Fatalf("fetch of an unbound, unopened repository: %v", err)
	}
	if ok, failed := eng.BackgroundFetch(ctx); ok+failed != 0 || remoteRef() != before {
		t.Fatalf("background fetch touched an unopened repository: %d %d", ok, failed)
	}
	// Opened: fetched.
	if err := eng.MarkOpened(id); err != nil {
		t.Fatal(err)
	}
	if ok, failed := eng.BackgroundFetch(ctx); ok != 1 || failed != 0 {
		t.Fatalf("background fetch after open: %d %d", ok, failed)
	}
	if got := remoteRef(); got != want {
		t.Errorf("origin/main = %s, want %s", got, want)
	}
	if err := eng.MarkOpened("nope"); err == nil {
		t.Error("unknown ID accepted")
	}
}
