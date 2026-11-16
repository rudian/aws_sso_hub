package repos_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/paths"
	"github.com/rudian/codepass/internal/projects"
	"github.com/rudian/codepass/internal/repos"
)

func (f *fixture) git(dir string, args ...string) {
	f.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "HOME="+f.sb.Home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+f.sb.GlobalConfig)
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (f *fixture) scan() api.ReposState {
	f.t.Helper()
	st, err := f.eng.Scan(context.Background())
	if err != nil {
		f.t.Fatal(err)
	}
	return st
}

// Keeping a second checkout and deleting it later is normal: the survivor must not be offered as "moved from"
// the deleted copy, and answering must not be possible.
func TestDeletedSecondCheckoutIsNotOfferedOnTheSurvivor(t *testing.T) {
	f := newFixture(t)
	dir := f.newRepo("acme_api", ccURL("acme_api"))
	f.sb.CommitFile(t, dir, "README.md", "hi\n")
	review := filepath.Join(f.code, "acme_api_review")
	if out, err := exec.Command("cp", "-R", dir, review).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	st := f.scan()
	if len(st.Repos) != 2 {
		t.Fatalf("expected two checkouts: %+v", st.Repos)
	}
	survivor := f.byName(st, "acme_api")
	if err := setOwnData(f.reg, survivor.ID); err != nil { // was SetHidden: a hidden repository now leaves the list (F1-11)
		t.Fatal(err)
	}
	f.clock = f.clock.Add(48 * time.Hour)
	if err := os.RemoveAll(review); err != nil {
		t.Fatal(err)
	}
	st = f.scan()
	if len(st.Repos) != 1 {
		t.Fatalf("%+v", st.Repos)
	}
	if o := f.eng.MovedOffers(); len(o) != 0 {
		t.Fatalf("offer on the survivor: %+v", o)
	}
	if _, err := f.eng.ReattachMoved(context.Background(), survivor.ID); err == nil {
		t.Fatal("re-attach accepted")
	}
	f.clock = f.clock.Add(time.Hour)
	f.scan()
	snap := f.reg.Snapshot()
	if r := snap.FindRepo(survivor.ID); r == nil || r.IdentityOverride == nil {
		t.Fatalf("survivor lost: %+v", r)
	}
}

func TestOriginCredentialsNeverReachProjectsJSON(t *testing.T) {
	f := newFixture(t)
	dir := f.newRepo("acme_api", ccURL("acme_api"))
	f.sb.CommitFile(t, dir, "README.md", "hi\n")
	f.git(dir, "remote", "set-url", "origin", "https://ghp_PROBEtoken1234@"+hostAP+"/v1/repos/acme_api")

	data := t.TempDir()
	eng, reg, _ := engineWith(t, f, data)
	if _, err := eng.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(data, "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "ghp_PROBE") || strings.Contains(string(raw), "PROBEtoken") {
		t.Fatalf("token in projects.json: %s", raw)
	}
	for _, f := range []string{".bak", ".v1.bak"} {
		if b, err := os.ReadFile(filepath.Join(data, "projects.json"+f)); err == nil && strings.Contains(string(b), "PROBE") {
			t.Fatalf("token in %s", f)
		}
	}

	// The same repository moved and its remote now carries another user: still recognised as a move.
	old := reg.Snapshot().Repos[0].ID
	moved := filepath.Join(f.code, "api-main")
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	f.git(moved, "remote", "set-url", "origin", "https://bob@"+hostAP+"/v1/repos/acme_api")
	if _, err := eng.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	offers := eng.MovedOffers()
	if len(offers) != 1 {
		t.Fatalf("no offer for credentials-only difference: %+v", offers)
	}
	for _, o := range offers {
		if o.OldID != old {
			t.Fatalf("%+v", o)
		}
	}
}

func TestDeclineMovedSticksAcrossScansAndRestart(t *testing.T) {
	f := newFixture(t)
	dir := f.newRepo("acme_api", ccURL("acme_api"))
	f.sb.CommitFile(t, dir, "README.md", "hi\n")
	data := t.TempDir()
	eng, _, sp := engineWith(t, f, data)
	ctx := context.Background()
	if _, err := eng.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	settingsBefore, _ := os.ReadFile(sp)
	defer func() { // the 0.0.1 app's file is untouched by every registry path above
		if after, _ := os.ReadFile(sp); string(after) != string(settingsBefore) {
			t.Error("settings.json changed")
		}
	}()
	moved := filepath.Join(f.code, "api-main")
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	st, err := eng.Scan(ctx)
	if err != nil || len(eng.MovedOffers()) != 1 {
		t.Fatalf("%v %+v", err, eng.MovedOffers())
	}
	newID := st.Repos[0].ID
	if _, err := eng.DeclineMoved(ctx, "nope"); err == nil {
		t.Fatal("unknown repository accepted")
	}
	st, err = eng.DeclineMoved(ctx, newID)
	if err != nil || len(st.Repos) != 1 || st.Repos[0].ID != newID {
		t.Fatalf("%v %+v", err, st.Repos)
	}
	if len(eng.MovedOffers()) != 0 {
		t.Fatal("offer remains after No")
	}
	if _, err := eng.Scan(ctx); err != nil || len(eng.MovedOffers()) != 0 {
		t.Fatalf("offer came back on rescan: %v %+v", err, eng.MovedOffers())
	}
	eng2, _, _ := engineWith(t, f, data) // restart: projects.json is read again
	if _, err := eng2.Scan(ctx); err != nil || len(eng2.MovedOffers()) != 0 {
		t.Fatalf("offer came back after restart: %v %+v", err, eng2.MovedOffers())
	}
}

// A data folder that cannot be written must not hide repositories that already have a record.
func TestScanListsKnownRepositoriesWhenRegistryIsReadOnly(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not apply")
	}
	f := newFixture(t)
	f.newRepo("acme_api", ccURL("acme_api"))
	data := t.TempDir()
	eng, _, _ := engineWith(t, f, data)
	ctx := context.Background()
	if st, err := eng.Scan(ctx); err != nil || len(st.Repos) != 1 {
		t.Fatalf("%v %+v", err, st)
	}
	f.newRepo("acme_web", ccURL("acme_web"))
	if err := os.Chmod(data, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(data, 0o755)
	st, err := eng.Scan(ctx)
	if err != nil {
		t.Fatalf("scan failed on a read-only data folder: %v", err)
	}
	if len(st.Repos) != 1 || st.Repos[0].Name != "acme_api" {
		t.Fatalf("known repository not listed: %+v", st.Repos)
	}
	// Writable again: the new folder is picked up by the next scan.
	_ = os.Chmod(data, 0o755)
	if st, err = eng.Scan(ctx); err != nil || len(st.Repos) != 2 {
		t.Fatalf("%v %+v", err, st.Repos)
	}

	// Nothing known and nothing writable: a file error, not an internal error.
	data2 := t.TempDir()
	eng2, _, _ := engineWith(t, f, data2)
	if err := os.Chmod(data2, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(data2, 0o755)
	_, err = eng2.Scan(ctx)
	if ae := api.AsAppError(err); err == nil || ae.Code != api.ErrFileIO || ae.Params["file"] == "" {
		t.Fatalf("want E_FILE_IO with a file name, got %v", err)
	}
}

func TestEngineWithoutRegistryFailsLoudly(t *testing.T) {
	f := newFixture(t)
	f.newRepo("acme_api", ccURL("acme_api"))
	eng := repos.New(repos.Deps{
		Paths: paths.Paths{Home: f.sb.Home},
		Git:   func(ctx context.Context) (*gitx.Git, error) { return f.sb.Git, nil },
		Store: f.store, GOOS: "darwin",
	})
	if st, err := eng.Scan(context.Background()); err == nil {
		t.Fatalf("scan worked without a registry: %+v", st.Repos)
	}
}

func TestAddExistingThroughSymlinkedParentMakesOneRecord(t *testing.T) {
	f := newFixture(t)
	dir := f.newRepo("acme_api", ccURL("acme_api"))
	f.state()
	link := filepath.Join(f.sb.Root, "code_link")
	if err := os.Symlink(f.code, link); err != nil {
		t.Skip("no symlinks")
	}
	f.add(filepath.Join(link, "acme_api"))
	if n := len(f.reg.Snapshot().Repos); n != 1 {
		t.Fatalf("records: %d (%v)", n, f.reg.Snapshot().Repos)
	}
	_ = dir
}

func TestAddedAndClonedRecordsAreCodeCommit(t *testing.T) {
	f := newFixture(t)
	out := f.outside("acme_web", ccURL("acme_web"))
	f.add(out)
	d := f.reg.Snapshot()
	if len(d.Repos) != 1 || d.Repos[0].Provider != projects.ProviderCodeCommit {
		t.Fatalf("%+v", d.Repos)
	}
}

// A folder moved or renamed by hand and then listed with "Add existing" (no rescan in between) is still offered
// as the old repository, and answering yes keeps the hidden flag.
func TestAddExistingAfterManualMoveIsOffered(t *testing.T) {
	for name, target := range map[string]func(f *fixture) string{
		"outside the scan root": func(f *fixture) string { return filepath.Join(f.sb.Root, "elsewhere", "acme_api") },
		"inside the scan root":  func(f *fixture) string { return filepath.Join(f.code, "api-main") },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			dir := f.newRepo("acme_api", ccURL("acme_api"))
			f.sb.CommitFile(t, dir, "README.md", "hi\n")
			eng, reg, _ := engineWith(t, f, t.TempDir())
			ctx := context.Background()
			if _, err := eng.Scan(ctx); err != nil {
				t.Fatal(err)
			}
			old := reg.Snapshot().Repos[0].ID
			if err := reg.SetHidden(old, true); err != nil {
				t.Fatal(err)
			}
			pr, err := reg.AddProject("Acme")
			if err != nil {
				t.Fatal(err)
			}
			if err := reg.SetProject(old, pr.ID); err != nil {
				t.Fatal(err)
			}
			if err := reg.Mutate(func(d *projects.Data) error {
				d.FindRepo(old).IdentityOverride = &projects.Identity{Name: "Ann", Email: "ann@example.com"}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			dst := target(f)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(dir, dst); err != nil {
				t.Fatal(err)
			}
			res, err := eng.AddExisting(ctx, dst)
			if err != nil {
				t.Fatal(err)
			}
			offers := eng.MovedOffers()
			if o, ok := offers[res.RepoID]; !ok || o.OldID != old {
				t.Fatalf("no offer for %s: %+v", res.RepoID, offers)
			}
			if _, err := eng.ReattachMoved(ctx, res.RepoID); err != nil {
				t.Fatal(err)
			}
			d := reg.Snapshot()
			r := d.FindRepo(old)
			if r == nil || !r.Hidden || r.MissingSince != nil {
				t.Fatalf("record not kept: %+v", r)
			}
			// the project and the identity of the old record survive the move too (FU-18h)
			if r.ProjectID != pr.ID || r.IdentityOverride == nil || *r.IdentityOverride != (projects.Identity{Name: "Ann", Email: "ann@example.com"}) {
				t.Fatalf("project or identity lost: %+v", r)
			}
		})
	}
}

// FU-14h: a caller outside the engine that gave the new row data of its own drops the question.
func TestForgetMovedOffersDropsTheQuestion(t *testing.T) {
	f := newFixture(t)
	dir := f.newRepo("acme_api", ccURL("acme_api"))
	f.sb.CommitFile(t, dir, "README.md", "hi\n")
	eng, reg, _ := engineWith(t, f, t.TempDir())
	ctx := context.Background()
	if _, err := eng.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(f.code, "api-main")
	if err := os.Rename(dir, dst); err != nil {
		t.Fatal(err)
	}
	res, err := eng.AddExisting(ctx, dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := eng.MovedOffers()[res.RepoID]; !ok {
		t.Fatal("no offer to start from")
	}
	_ = reg
	eng.ForgetMovedOffers(res.RepoID)
	if _, ok := eng.MovedOffers()[res.RepoID]; ok {
		t.Fatal("the question stays")
	}
}
