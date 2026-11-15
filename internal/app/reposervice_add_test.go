package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/api"
)

// outsideRepo creates a clone that is not below the default scan root.
func (w *world) outsideRepo(name, remote string) string {
	w.t.Helper()
	old := w.p.DefaultScanRoot
	defer func() { w.p.DefaultScanRoot = old }()
	w.p.DefaultScanRoot = filepath.Join(w.p.Home, "elsewhere")
	return w.gitRepo(name, remote)
}

func TestAddExistingService(t *testing.T) {
	w := newWorld(t)
	ok(w.acc.Login(bg, "initech", api.LoginModeBrowser))
	repo := w.outsideRepo("initech_infra", ccURL("initech_infra"))

	before := w.rt.count(api.EventAccountsChanged)
	res := ok(w.repo.AddExisting(bg, repo))
	if !res.Added || !res.AutoBound || res.Outcome == nil || !res.Outcome.OK || res.RepoID == "" {
		t.Fatalf("%+v", res)
	}
	r := repoNamed(res.State, "initech_infra")
	if !r.Extra || r.Binding.Source != api.BindingLocal || r.Binding.Profile != "initech-admin" {
		t.Fatalf("repo: %+v", r)
	}
	if w.rt.count(api.EventAccountsChanged) <= before || w.rt.count(api.EventReposChanged) == 0 {
		t.Error("binding must emit accounts:changed and repos:changed")
	}
	if cfg := w.read(filepath.Join(repo, ".git", "config")); !strings.Contains(cfg, "profile = initech-admin") {
		t.Errorf("config:\n%s", cfg)
	}
	if !strings.Contains(w.read(w.p.SettingsFile), repo) {
		t.Error("the path must be stored in extraRepos")
	}

	// listed again: not added a second time
	again := ok(w.repo.AddExisting(bg, repo))
	if again.Added || again.RepoID != res.RepoID {
		t.Fatalf("again: %+v", again)
	}

	// removing it from the list leaves the folder and its configuration alone
	st := ok(w.repo.RemoveExtra(bg, res.RepoID))
	for _, x := range st.Repos {
		if x.ID == res.RepoID {
			t.Error("still listed")
		}
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "config")); err != nil {
		t.Error(err)
	}
	if strings.Contains(w.read(w.p.SettingsFile), repo) {
		t.Error("path still stored")
	}
}

func TestAddExistingServiceSettingOffAndErrors(t *testing.T) {
	w := newWorld(t)
	ok(w.acc.Login(bg, "initech", api.LoginModeBrowser))
	off := false
	if s := ok(w.sys.UpdateSettings(api.SettingsPatch{AutoBindOnAdd: &off})); s.AutoBindOnAdd {
		t.Fatal("setting not stored")
	}
	repo := w.outsideRepo("initech_infra", ccURL("initech_infra"))
	res := ok(w.repo.AddExisting(bg, repo))
	if !res.Added || res.AutoBound {
		t.Fatalf("%+v", res)
	}
	if ae := fails(w.repo.AddExisting(bg, filepath.Join(w.p.Home, "nowhere"))); ae.Code != api.ErrRepoNotFound {
		t.Errorf("%v", ae)
	}
	gh := w.outsideRepo("gh", "https://user:pw@github.com/o/r.git")
	ae := fails(w.repo.AddExisting(bg, gh))
	if ae.Code != api.ErrNotCodeCommit || strings.Contains(ae.Detail, "pw") || strings.Contains(ae.Detail, "user") {
		t.Errorf("%v / %q", ae.Code, ae.Detail)
	}
	if ae := fails(w.repo.RemoveExtra(bg, "no-such")); ae.Code != api.ErrRepoNotFound {
		t.Errorf("%v", ae)
	}
	// a repository below the scan root cannot be "removed from the list"
	in := w.gitRepo("orphan_tool", ccURL("orphan_tool"))
	listed := ok(w.repo.AddExisting(bg, in))
	if listed.Added {
		t.Fatalf("%+v", listed)
	}
	if ae := fails(w.repo.RemoveExtra(bg, listed.RepoID)); ae.Code != api.ErrInvalidInput {
		t.Errorf("%v", ae)
	}
}
