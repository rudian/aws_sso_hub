package identity

import (
	"path/filepath"
	"testing"

	"github.com/rudian/codepass/internal/api"
)

// FU-13b: only "not a git repository" is E_REPO_NOT_FOUND; a config git cannot parse is E_GIT_FAILED (or
// E_FILE_IO) with the config's display path, so the UI can say "cannot write this repository's config".
func TestPreviewRepoAndMoveWithCorruptConfig(t *testing.T) {
	r := newRig(t)
	r.setGlobal("Alex Global", "global@example.com")
	work := filepath.Join(r.root, "work")
	bad := r.repo(work, "acme_bad")
	r.project("acme", bad)
	r.write(r.localCfg(bad), "[user\n\tname = broken\n")

	check := func(t *testing.T, err error) {
		t.Helper()
		ae := api.AsAppError(err)
		if err == nil || ae.Code == api.ErrRepoNotFound || (ae.Code != api.ErrGitFailed && ae.Code != api.ErrFileIO) {
			t.Fatalf("want E_GIT_FAILED or E_FILE_IO, got %v", err)
		}
		if f := ae.Params["file"]; f != r.p.Display(r.localCfg(bad)) {
			t.Fatalf("file param %q want %q: %+v", f, r.p.Display(r.localCfg(bad)), ae)
		}
	}
	t.Run("PreviewRepo", func(t *testing.T) {
		_, err := r.eng.PreviewRepo(bg, api.RepoIdentityRequest{RepoID: bad.ID, Mode: api.RepoModeOwn, Identity: ptr(acme())})
		check(t, err)
	})
	t.Run("PreviewMove", func(t *testing.T) {
		_, err := r.eng.PreviewMove(bg, bad.ID, "")
		check(t, err)
	})
}

// A folder that is not a repository any more keeps E_REPO_NOT_FOUND.
func TestPreviewRepoNotARepository(t *testing.T) {
	r := newRig(t)
	work := filepath.Join(r.root, "work")
	gone := r.repo(work, "acme_gone")
	r.project("acme", gone)
	if err := removeAll(filepath.Join(gone.Path, ".git")); err != nil {
		t.Fatal(err)
	}
	_, err := r.eng.PreviewRepo(bg, api.RepoIdentityRequest{RepoID: gone.ID, Mode: api.RepoModeOwn, Identity: ptr(acme())})
	if ae := api.AsAppError(err); err == nil || ae.Code != api.ErrRepoNotFound {
		t.Fatalf("want E_REPO_NOT_FOUND, got %v", err)
	}
}

// A corrupt ~/.gitconfig fails every repository's read, but the path names that file, not the repository's config.
func TestPreviewRepoCorruptGlobalNamesTheGlobalFile(t *testing.T) {
	r := newRig(t)
	work := filepath.Join(r.root, "work")
	ok := r.repo(work, "acme_ok")
	r.project("acme", ok)
	r.write(r.p.GitGlobalFile, "[user\n\tname = broken\n")
	_, err := r.eng.PreviewRepo(bg, api.RepoIdentityRequest{RepoID: ok.ID, Mode: api.RepoModeOwn, Identity: ptr(acme())})
	ae := api.AsAppError(err)
	if err == nil || ae.Code != api.ErrGitFailed {
		t.Fatalf("want E_GIT_FAILED, got %v", err)
	}
	if f := ae.Params["file"]; filepath.Base(f) != ".gitconfig" {
		t.Fatalf("file param %q should name the global config: %+v", f, ae)
	}
}

// FU-16c: the reason git adds after the file name is not part of the path.
func TestGitFileInMessageStopsAtGitsReason(t *testing.T) {
	for msg, want := range map[string]string{
		"fatal: bad config line 1 in file /home/alex/.gitconfig":                                          "/home/alex/.gitconfig",
		"error: bad numeric config value 'x' for 'core.a' in file .git/config: invalid unit":              ".git/config",
		"fatal: bad config variable 'a.b' in file '/x/config' at line 3":                                  "/x/config",
		"fatal: bad config line 2 in file C:\\Users\\alex\\.gitconfig":                                    "C:\\Users\\alex\\.gitconfig",
		"error: bad boolean config value 'maybe' for 'core.b' in file /x y/config: invalid boolean value": "/x y/config",
	} {
		m := gitFileInMessage.FindStringSubmatch(msg)
		if m == nil || m[1] != want {
			t.Errorf("%q: got %v, want %q", msg, m, want)
		}
	}
}

// FU-16d: in a project preview a corrupt ~/.gitconfig is reported with the global file's path for every
// member, not as each repository's own config.
func TestProjectPreviewCorruptGlobalNamesTheGlobalFile(t *testing.T) {
	r := newRig(t)
	work := filepath.Join(r.root, "work")
	a := r.repo(work, "acme_api")
	b := r.repo(work, "acme_web")
	p := r.project("acme", a, b)
	r.write(r.p.GitGlobalFile, "[user\n\tname = broken\n")
	pv, err := r.eng.PreviewProject(bg, api.ProjectIdentityRequest{ProjectID: p.ID, Identity: ptr(acme())})
	if err != nil {
		t.Fatal(err)
	}
	if len(pv.Repos) != 2 {
		t.Fatalf("%+v", pv.Repos)
	}
	for _, it := range pv.Repos {
		if it.Action != api.RepoActionError || it.Error == nil || it.Error.Code != api.ErrGitFailed ||
			filepath.Base(it.Error.Params["file"]) != ".gitconfig" {
			t.Errorf("%s: %+v", it.Name, it)
		}
	}
}
