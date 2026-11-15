package gitx_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/gitx/gitxtest"
)

func mkRepoDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestFindRepoDirsDepthAndExcludes(t *testing.T) {
	root := t.TempDir()
	mkRepoDir(t, filepath.Join(root, "a"))                         // depth 1
	mkRepoDir(t, filepath.Join(root, "b", "c", "d", "e"))          // depth 4: found
	mkRepoDir(t, filepath.Join(root, "b", "c", "d", "e2", "deep")) // depth 5: ignored
	mkRepoDir(t, filepath.Join(root, "node_modules", "pkg"))       // excluded
	mkRepoDir(t, filepath.Join(root, ".hidden", "r"))              // hidden dirs skipped
	mkRepoDir(t, filepath.Join(root, "p"))                         // repo
	mkRepoDir(t, filepath.Join(root, "p", "sub"))                  // nested: not descended
	if err := os.WriteFile(filepath.Join(root, "wt-file-repo.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "wtrepo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "wtrepo", ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "a"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	var doneRoots []string
	dirs, err := gitx.FindRepoDirs(context.Background(), []string{root, filepath.Join(root, "does-not-exist"), root}, gitx.DefaultScanOptions,
		func(r string, found int) {
			if found == 5 || found == 0 {
				doneRoots = append(doneRoots, r)
			}
		})
	if err != nil {
		t.Fatal(err)
	}
	var rel []string
	for _, d := range dirs {
		r, _ := filepath.Rel(root, d)
		rel = append(rel, r)
	}
	want := "a,b/c/d/e,p,wtrepo"
	if strings.Join(rel, ",") != want {
		t.Errorf("repos = %v, want %s", rel, want)
	}
	if len(doneRoots) < 2 {
		t.Errorf("progress callbacks: %v", doneRoots)
	}
}

func TestInspectSkipsNonCodeCommit(t *testing.T) {
	sb := gitxtest.New(t)
	repo := sb.InitRepo(t, "gh", map[string]string{"origin": "https://github.com/o/r.git"})
	_, ok, err := sb.Git.Inspect(context.Background(), repo)
	if err != nil || ok {
		t.Errorf("ok=%v err=%v", ok, err)
	}
	empty := sb.InitRepo(t, "empty", nil)
	if _, ok, err := sb.Git.Inspect(context.Background(), empty); err != nil || ok {
		t.Errorf("no remotes: ok=%v err=%v", ok, err)
	}
}

func TestInspectPrimaryAndEntries(t *testing.T) {
	ctx := context.Background()
	sb := gitxtest.New(t)
	repo := sb.InitRepo(t, "r", map[string]string{
		"origin": "https://github.com/o/r.git",
		"cc":     "https://u:pw@" + host + "/v1/repos/web-app",
		"cc2":    "ssh://" + host + "/v1/repos/other",
	})
	appendConfig(t, repo, "[remote \"cc2\"]\n\tpushurl = https://"+host+"/v1/repos/other\n")
	sr, ok, err := sb.Git.Inspect(ctx, repo)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if len(sr.Remotes) != 3 || sr.Remotes[sr.Primary].Name != "cc" {
		t.Fatalf("remotes %+v primary %d", sr.Remotes, sr.Primary)
	}
	if r := sr.Remotes[sr.Primary]; r.Info.Repo != "web-app" || r.Info.User != "u" || r.Info.Kind != api.RemoteHTTPS {
		t.Errorf("primary %+v", r)
	}
	for _, r := range sr.Remotes {
		if r.Name == "origin" && r.Info.Kind != api.RemoteOther {
			t.Errorf("github remote kind %s", r.Info.Kind)
		}
		if r.Name == "cc2" && (r.PushInfo == nil || r.PushInfo.Kind != api.RemoteHTTPS || r.Info.Kind != api.RemoteSSH) {
			t.Errorf("cc2 %+v", r)
		}
	}
	// origin wins when it is CodeCommit
	repo2 := sb.InitRepo(t, "r2", map[string]string{"origin": ccURL, "aaa": "https://" + host + "/v1/repos/zzz"})
	sr, ok, err = sb.Git.Inspect(ctx, repo2)
	if err != nil || !ok || sr.Remotes[sr.Primary].Name != "origin" {
		t.Errorf("origin primary: %+v %v %v", sr, ok, err)
	}
}

func TestInspectInsteadOf(t *testing.T) {
	sb := gitxtest.New(t)
	sb.WriteGlobal(t, "[url \"https://"+host+"/v1/repos/\"]\n\tinsteadOf = cc:\n")
	repo := sb.InitRepo(t, "r", map[string]string{"origin": "cc:web-app"})
	sr, ok, err := sb.Git.Inspect(context.Background(), repo)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	r := sr.Remotes[sr.Primary]
	if r.URL != ccURL || r.Info.Repo != "web-app" {
		t.Errorf("insteadOf not applied: %+v", r)
	}
}

func TestInspectWorktreeGitFile(t *testing.T) {
	ctx := context.Background()
	sb := gitxtest.New(t)
	repo := sb.InitRepo(t, "main", map[string]string{"origin": ccURL})
	sb.CommitFile(t, repo, "a", "x")
	wt := filepath.Join(sb.Root, "wt")
	if res, err := sb.Git.Run(ctx, repo, nil, "worktree", "add", "-q", wt, "-b", "feature"); err != nil || res.ExitCode != 0 {
		t.Fatalf("worktree add: %v %s", err, res.Stderr)
	}
	if st, err := os.Stat(filepath.Join(wt, ".git")); err != nil || st.IsDir() {
		t.Fatalf("expected a .git file in the worktree: %v", err)
	}
	dirs, err := gitx.FindRepoDirs(ctx, []string{sb.Root}, gitx.DefaultScanOptions, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range dirs {
		if d == wt {
			found = true
		}
	}
	if !found {
		t.Errorf("worktree not found by the walker: %v", dirs)
	}
	sr, ok, err := sb.Git.Inspect(ctx, wt)
	if err != nil || !ok || sr.Remotes[sr.Primary].Info.Repo != "web-app" {
		t.Errorf("inspect worktree: %+v %v %v", sr, ok, err)
	}
}

func TestInspectManyOrder(t *testing.T) {
	sb := gitxtest.New(t)
	var dirs []string
	for _, n := range []string{"a", "b", "c"} {
		dirs = append(dirs, sb.InitRepo(t, n, map[string]string{"origin": "https://" + host + "/v1/repos/" + n}))
	}
	dirs = append(dirs, sb.InitRepo(t, "gh", map[string]string{"origin": "https://github.com/o/r"}))
	repos, errs := sb.Git.InspectMany(context.Background(), dirs, 3)
	if len(errs) != 0 || len(repos) != 3 || repos[0].Path != dirs[0] || repos[2].Path != dirs[2] {
		t.Errorf("repos %d errs %v", len(repos), errs)
	}
}
