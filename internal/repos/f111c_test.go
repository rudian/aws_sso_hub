package repos_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/paths"
	"github.com/rudian/codepass/internal/repos"
)

// DetectOwners(nil) is what the app runs after every first scan: a hidden repository is out of it.
func TestDetectOwnersSkipsHiddenAndPending(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.newRepo("shown", ccURL("svc_shown"))
	f.newRepo("hid", ccURL("svc_hid"))
	f.ccAPIs["ACME"].repos["svc_shown"] = "rid-1"
	f.ccAPIs["ACME"].repos["svc_hid"] = "rid-2"
	st := f.state()
	if _, err := f.eng.SetHidden(ctx, f.byName(st, "hid").ID, true); err != nil {
		t.Fatal(err)
	}
	st, err := f.eng.DetectOwners(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if o := f.byName(st, "shown").Owner; o.Status != api.OwnerSingle {
		t.Errorf("shown: %+v", o)
	}
	if len(st.HiddenRepos) != 1 {
		t.Fatalf("hidden: %+v", st.HiddenRepos)
	}
	if o := st.HiddenRepos[0].Owner; o.Status != api.OwnerUnknown || o.CheckedWith != 0 {
		t.Errorf("a hidden repository was checked against AWS: %+v", o)
	}
}

func TestHidingAFoundRowTakesItOutOfTheReview(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.sb.InitRepo(t, "code/gh", map[string]string{"origin": ghURL})
	st := f.state()
	if len(st.Found) != 1 {
		t.Fatalf("found: %+v", st.Found)
	}
	id := st.Found[0].ID
	st, err := f.eng.SetHidden(ctx, id, true)
	if err != nil || len(st.Found) != 0 || st.Counts.Found != 0 || len(st.HiddenRepos) != 1 || st.Counts.Hidden != 1 {
		t.Fatalf("hide: %v found %d hidden %d", err, len(st.Found), len(st.HiddenRepos))
	}
	st, err = f.eng.SetHidden(ctx, id, false)
	if err != nil || len(st.Found) != 0 || len(st.HiddenRepos) != 0 || len(st.Repos) != 1 {
		t.Fatalf("unhide: %v found %d hidden %d repos %d", err, len(st.Found), len(st.HiddenRepos), len(st.Repos))
	}
}

// A shown GitHub repository whose folder is renamed stays in the list with the question.
func TestMovedShownGitHubRepositoryStaysListed(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	dir := f.sb.InitRepo(t, "code/gh", map[string]string{"origin": ghURL})
	f.sb.CommitFile(t, dir, "README.md", "hi\n")
	f.state()
	st := f.reviewAll()
	oldID := st.Repos[0].ID
	moved := filepath.Join(f.code, "gh-main")
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	st = f.scan()
	if len(st.Repos) != 1 || len(st.Found) != 0 || st.Counts.Found != 0 {
		t.Fatalf("repos %d found %d", len(st.Repos), len(st.Found))
	}
	row := st.Repos[0]
	if row.MovedFrom == nil || row.MovedFrom.OldID != oldID {
		t.Fatalf("no question: %+v", row.MovedFrom)
	}
	st, err := f.eng.ReattachMoved(ctx, row.ID)
	if err != nil || len(st.Repos) != 1 || st.Repos[0].ID != oldID || st.Repos[0].MovedFrom != nil {
		t.Fatalf("yes: %v %+v", err, st.Repos)
	}
}

func TestRemoveExtraOfANonCodeCommitFolder(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	outside := f.sb.InitRepo(t, "elsewhere/tool", map[string]string{"origin": ghURL})
	f.state()
	res, err := f.eng.AddFolder(ctx, outside)
	if err != nil || !res.Added || len(f.store.ExtraRepos()) != 1 {
		t.Fatalf("%v %+v", err, res)
	}
	st, err := f.eng.RemoveExtra(ctx, res.RepoID)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Repos) != 0 || len(f.store.ExtraRepos()) != 0 {
		t.Errorf("repos %d extras %v", len(st.Repos), f.store.ExtraRepos())
	}
	if _, err := f.eng.RemoveExtra(ctx, "nope"); api.AsAppError(err).Code != api.ErrRepoNotFound {
		t.Errorf("unknown: %v", err)
	}
}

func TestAddFolderBringsBackAHiddenCodeCommitRepository(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	dir := f.newRepo("acme_api", ccURL("acme_api"))
	st := f.state()
	id := f.byName(st, "acme_api").ID
	if _, err := f.eng.SetHidden(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	res, err := f.eng.AddFolder(ctx, dir)
	if err != nil || res.RepoID != id || len(res.State.Repos) != 1 || len(res.State.HiddenRepos) != 0 {
		t.Fatalf("%v repos %d hidden %d", err, len(res.State.Repos), len(res.State.HiddenRepos))
	}
}

func TestOtherRemoteTokenIsMasked(t *testing.T) {
	f := newFixture(t)
	const token = "ghp_EXAMPLEabcdefghijklmnopqrstuvwxyz0"
	f.sb.InitRepo(t, "code/tok", map[string]string{"origin": "https://" + token + "@github.com/acme/web.git"})
	f.state()
	st := f.reviewAll()
	r := f.byName(st, "tok")
	if strings.Contains(r.RemoteURL, token) || !r.HasUserinfo {
		t.Errorf("remote url %q userinfo %v", r.RemoteURL, r.HasUserinfo)
	}
	for name, c := range map[string]struct {
		url  string
		want bool
	}{
		"bbuser": {"https://alex_acme@bitbucket.org/globex/api-main.git", false},
		"bbtok":  {"https://x-token-auth@bitbucket.org/globex/api-main.git", false},
		"userpw": {"https://user:pw@bitbucket.org/globex/api-main.git", true},
		// FU-11e: a long Bitbucket account name is not a token; a name that starts like one is
		"bblong":  {"https://alexander.very-long-account-name@bitbucket.org/globex/api-main.git", false},
		"bbatbb":  {"https://ATBBxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx@bitbucket.org/globex/api-main.git", true},
		"bbatatt": {"https://ATATT3xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx@bitbucket.org/globex/api-main.git", true},
	} {
		f.sb.InitRepo(t, "code/"+name, map[string]string{"origin": c.url})
		f.scan()
		if r := f.byName(f.reviewAll(), name); r.HasUserinfo != c.want {
			t.Errorf("%s: userinfo %v want %v", name, r.HasUserinfo, c.want)
		}
	}
	f.sb.InitRepo(t, "code/ssh", map[string]string{"origin": "ssh://git@github.com/acme/web.git"})
	f.scan()
	if r := f.byName(f.reviewAll(), "ssh"); r.HasUserinfo {
		t.Errorf("ssh user is not a credential: %+v", r)
	}
}

// A subscription that reported a removal or a stop delivers nothing more; the engine subscribes again.
func TestWatchRemovedAndStoppedResubscribe(t *testing.T) {
	f := newFixture(t)
	dir := f.newRepo("a", ccURL("a_repo"))
	f.sb.CommitFile(t, dir, "README.md", "x\n")
	w := newFakeWatch()
	eng := f.engineOn(f.reg, w)
	defer eng.Close()
	ctx := context.Background()
	if _, err := eng.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if got := w.logged(); got != "+a" {
		t.Fatalf("log %q", got)
	}
	w.fireEvent(dir, repos.WatchRemoved) // the old subscription is dead
	if _, err := eng.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if got := w.logged(); got != "+a,-a,+a" {
		t.Fatalf("after removal: %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w.fireEvent(dir, repos.WatchStopped) // resubscribes at once and reads the status once
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st, _ := eng.State(ctx)
		if s := f.byName(st, "a").Status; s != nil && s.Untracked == 1 && len(w.watched()) == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	st, _ := eng.State(ctx)
	if s := f.byName(st, "a").Status; s == nil || s.Untracked != 1 {
		t.Fatalf("no status after a stop: %+v", s)
	}
	if got := w.logged(); got != "+a,-a,+a,-a,+a" {
		t.Errorf("after stop: %q", got)
	}
}

// Background fetch never prompts and never runs the repository's own hooks.
// (The repository's own hooks are covered by TestBackgroundFetchSkipsRepositoryHooks: a fetch that fails with a
// 401 never reaches a ref transaction, so a hook assertion here could not fail.)
func TestBackgroundFetchDoesNotPrompt(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("WWW-Authenticate", `Basic realm="x"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	f := newFixture(t)
	dir := f.sb.InitRepo(t, "code/web", map[string]string{"origin": srv.URL + "/acme/web.git"})
	_ = dir
	ctx := context.Background()
	f.state()
	st := f.reviewAll()
	id := f.byName(st, "web").ID
	if err := f.eng.MarkOpened(id); err != nil {
		t.Fatal(err)
	}
	f.sb.ResetLogs(t)
	if err := f.eng.Fetch(ctx, id); err == nil {
		t.Fatal("the fetch of a server that wants credentials succeeded")
	}
	if hits.Load() == 0 {
		t.Fatal("the fetch never reached the server")
	}
	if log := f.sb.AskpassLog(t); log != "" {
		t.Errorf("askpass was called: %q", log)
	}
}

func TestBackgroundFetchSkipsRepositoryHooks(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	bare := f.sb.InitBare(t, "up.git")
	dir := filepath.Join(f.code, "plain")
	if res, err := f.sb.Git.Run(ctx, "", nil, "clone", "-q", bare, dir); err != nil || res.ExitCode != 0 {
		t.Fatalf("clone: %v %s", err, res.Stderr)
	}
	work := filepath.Join(f.sb.Root, "up.git-work")
	f.sb.CommitFile(t, work, "more.txt", "more\n")
	if res, err := f.sb.Git.Run(ctx, work, nil, "push", "-q", bare, "main"); err != nil || res.ExitCode != 0 {
		t.Fatalf("push: %v %s", err, res.Stderr)
	}
	marker := filepath.Join(f.sb.Root, "hook-ran")
	hook := filepath.Join(dir, ".git", "hooks", "reference-transaction")
	if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.state()
	id := f.byName(f.reviewAll(), "plain").ID
	if err := f.eng.MarkOpened(id); err != nil {
		t.Fatal(err)
	}
	if err := f.eng.Fetch(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the repository's hook ran during the background fetch")
	}
}

func TestProjectNameLimitCountsCharacters(t *testing.T) {
	f := newFixture(t)
	f.newRepo("a", ccURL("a_repo"))
	f.state()
	ctx := context.Background()
	if _, err := f.eng.CreateProject(ctx, strings.Repeat("仓", 100)); err != nil {
		t.Errorf("100 characters: %v", err)
	}
	if _, err := f.eng.CreateProject(ctx, strings.Repeat("仓", 101)); api.AsAppError(err).Code != api.ErrInvalidInput {
		t.Errorf("101 characters: %v", err)
	}
}

// A copy of a repository that was never listed stays held back in the review set.
func TestMovedNeverReviewedRepositoryStaysFound(t *testing.T) {
	f := newFixture(t)
	dir := f.sb.InitRepo(t, "code/gh", map[string]string{"origin": ghURL})
	f.sb.CommitFile(t, dir, "README.md", "hi\n")
	f.state()
	if err := os.Rename(dir, filepath.Join(f.code, "gh-main")); err != nil {
		t.Fatal(err)
	}
	st := f.scan()
	if len(st.Repos) != 0 || len(st.Found) != 1 {
		t.Fatalf("repos %d found %d", len(st.Repos), len(st.Found))
	}
}

// FU-11a: adding a CodeCommit repository that is shown already writes nothing, so a projects.json that cannot be
// written does not turn a successful add into E_FILE_IO.
func TestAddFolderOfAShownCodeCommitRepositoryWritesNothing(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not apply")
	}
	ctx := context.Background()
	f := newFixture(t)
	dir := f.newRepo("acme_api", ccURL("acme_api"))
	data := t.TempDir()
	eng, _, _ := engineWith(t, f, data)
	if st, err := eng.Scan(ctx); err != nil || len(st.Repos) != 1 {
		t.Fatalf("%v %+v", err, st)
	}
	if err := os.Chmod(data, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(data, 0o755)
	res, err := eng.AddFolder(ctx, dir)
	if err != nil || len(res.State.Repos) != 1 {
		t.Fatalf("%v %+v", err, res.State.Repos)
	}
}

// FU-11f: a watch that keeps dying is not subscribed again at once every time, and costs no status read each time.
func TestWatchThatKeepsStoppingBacksOff(t *testing.T) {
	f := newFixture(t)
	dir := f.newRepo("a", ccURL("a_repo"))
	f.sb.CommitFile(t, dir, "README.md", "x\n")
	w := newFakeWatch()
	eng := repos.New(repos.Deps{
		Paths: paths.Paths{Home: f.sb.Home}, Git: func(ctx context.Context) (*gitx.Git, error) { return f.sb.Git, nil },
		Store: f.store, Registry: f.reg, Watch: w, GOOS: "darwin", Now: func() time.Time { return f.clock },
		WatchRetryBase: 400 * time.Millisecond,
	})
	defer eng.Close()
	ctx := context.Background()
	if _, err := eng.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	subs := func() int { return strings.Count(w.logged(), "+a") }
	if subs() != 1 {
		t.Fatalf("log %q", w.logged())
	}
	w.fireEvent(dir, repos.WatchStopped) // the first stop: subscribed again at once
	waitFor(t, func() bool { return subs() == 2 })
	w.fireEvent(dir, repos.WatchStopped) // a second one right away: it waits
	time.Sleep(150 * time.Millisecond)
	if subs() != 2 {
		t.Fatalf("subscribed again at once after a quick second stop: %q", w.logged())
	}
	waitFor(t, func() bool { return subs() == 3 }) // after the wait it is back
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("timed out")
}
