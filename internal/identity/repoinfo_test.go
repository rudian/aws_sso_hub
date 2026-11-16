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
