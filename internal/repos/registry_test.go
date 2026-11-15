package repos_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/paths"
	"github.com/rudian/codepass/internal/projects"
	"github.com/rudian/codepass/internal/repos"
	"github.com/rudian/codepass/internal/settings"
)

func TestIDsAreNotPathHashes(t *testing.T) {
	f := newFixture(t)
	a := f.newRepo("acme/api", ccURL("api"))
	b := f.newRepo("globex/api", ccURL("api"))
	_ = a
	_ = b
	st := f.state()
	if len(st.Repos) != 2 || st.Repos[0].ID == st.Repos[1].ID {
		t.Fatalf("two folders with one name must be two repositories: %+v", st.Repos)
	}
	for _, r := range st.Repos {
		if r.ID == projects.LegacyID(r.Path) {
			t.Fatalf("ID %s is the path hash", r.ID)
		}
		if got, ok := f.eng.RepoPath(r.ID); !ok || got != r.Path {
			t.Fatalf("RepoPath %q %v", got, ok)
		}
	}
	// A new scan keeps the IDs.
	ids := map[string]string{}
	for _, r := range st.Repos {
		ids[r.Path] = r.ID
	}
	if _, err := f.eng.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, r := range f.state().Repos {
		if ids[r.Path] != r.ID {
			t.Fatalf("ID of %s changed", r.Path)
		}
	}
}

func TestRenamedFolderIsOfferedAndReattached(t *testing.T) {
	f := newFixture(t)
	dir := f.newRepo("acme_api", ccURL("acme_api"))
	f.sb.CommitFile(t, dir, "README.md", "hi\n")
	st := f.state()
	f.ccAPIs["ACME"].repos["acme_api"] = "rid-acme"
	st, err := f.eng.DetectOwners(context.Background(), nil)
	if err != nil || st.Repos[0].Owner.Status != api.OwnerSingle {
		t.Fatalf("owner not detected: %v %+v", err, st.Repos[0].Owner)
	}
	old := st.Repos[0]
	if err := setOwnData(f.reg, old.ID); err != nil { // was SetHidden: a hidden repository now leaves the list (F1-11)
		t.Fatal(err)
	}
	if err := f.reg.SetLastTest(old.ID, api.TestResult{Status: api.TestOK, At: "2026-11-01T00:00:00Z", Profile: "acme"}); err != nil {
		t.Fatal(err)
	}
	if len(f.eng.MovedOffers()) != 0 {
		t.Fatal("offer without a move")
	}

	// Renamed by hand, outside the app.
	moved := filepath.Join(f.code, "api-main")
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	st, err = f.eng.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Repos) != 1 || st.Repos[0].ID == old.ID {
		t.Fatalf("expected a new row for the moved folder: %+v", st.Repos)
	}
	offers := f.eng.MovedOffers()
	o, ok := offers[st.Repos[0].ID]
	if !ok || o.OldID != old.ID || o.Name != "acme_api" {
		t.Fatalf("offers: %+v", offers)
	}

	st, err = f.eng.ReattachMoved(context.Background(), st.Repos[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Repos) != 1 || st.Repos[0].ID != old.ID || st.Repos[0].Path != moved {
		t.Fatalf("after re-attach: %+v", st.Repos)
	}
	if st.Repos[0].LastTest == nil || st.Repos[0].LastTest.Status != api.TestOK {
		t.Fatalf("test result lost: %+v", st.Repos[0].LastTest)
	}
	if st.Repos[0].Owner.Status != api.OwnerSingle {
		t.Fatalf("owner detection lost on re-attach: %+v", st.Repos[0].Owner)
	}
	d := f.reg.Snapshot()
	if r := d.FindRepo(old.ID); r == nil || r.IdentityOverride == nil || r.MissingSince != nil || len(d.Repos) != 1 {
		t.Fatalf("%+v", d.Repos)
	}
	if len(f.eng.MovedOffers()) != 0 {
		t.Fatal("offer remains after the answer")
	}
	if _, err := f.eng.ReattachMoved(context.Background(), "nope"); err == nil {
		t.Fatal("unknown repository accepted")
	}
}

func TestCopiedFolderIsNotAMove(t *testing.T) {
	f := newFixture(t)
	dir := f.newRepo("acme_api", ccURL("acme_api"))
	f.sb.CommitFile(t, dir, "README.md", "hi\n")
	f.state()
	// A copy next to the original: the original is still on disk, so nothing is missing and nothing offered.
	copyDir := f.newRepo("acme_api_copy", ccURL("acme_api"))
	f.sb.CommitFile(t, copyDir, "README.md", "hi\n")
	if _, err := f.eng.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if o := f.eng.MovedOffers(); len(o) != 0 {
		t.Fatalf("%+v", o)
	}
}

// engineWith builds an engine over a real settings.json and a projects.json in dir.
func engineWith(t *testing.T, f *fixture, dir string) (*repos.Engine, *projects.Store, string) {
	t.Helper()
	sp := filepath.Join(dir, "settings.json")
	ss, _, err := settings.Open(sp, f.code, nil)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := projects.Open(filepath.Join(dir, "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	eng := repos.New(repos.Deps{
		Paths:    paths.Paths{Home: f.sb.Home},
		Git:      func(ctx context.Context) (*gitx.Git, error) { return f.sb.Git, nil },
		Store:    ss,
		Registry: reg,
		GOOS:     "darwin",
	})
	return eng, reg, sp
}

func TestImportOldTestResultsAndSettingsUntouched(t *testing.T) {
	f := newFixture(t)
	dir := f.newRepo("acme_api", ccURL("acme_api"))
	other := f.newRepo("acme_web", ccURL("acme_web"))
	_ = other

	data := t.TempDir()
	sp := filepath.Join(data, "settings.json")
	// What 0.0.1 saved: a result under the path hash of the folder.
	old, _, err := settings.Open(sp, f.code, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.SetRepoTest(api.TestResult{RepoID: projects.LegacyID(dir), Status: api.TestOK, At: "2026-10-30T08:00:00Z", Profile: "acme"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(sp)

	eng, reg, _ := engineWith(t, f, data)
	st, err := eng.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var tested, untested int
	for _, r := range st.Repos {
		if r.LastTest == nil {
			untested++
			continue
		}
		tested++
		if r.Path != dir || r.LastTest.Status != api.TestOK || r.LastTest.RepoID != r.ID {
			t.Fatalf("%+v", r)
		}
	}
	if tested != 1 || untested != 1 {
		t.Fatalf("tested %d untested %d", tested, untested)
	}
	// The result is now in projects.json; a new test goes there too, and settings.json is never written.
	id, _ := reg.IDForPath(dir)
	if reg.LastTests()[id].Profile != "acme" {
		t.Fatalf("%+v", reg.LastTests())
	}
	if err := reg.SetLastTest(id, api.TestResult{Status: api.TestNetwork, At: "2026-11-02T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(sp)
	if string(after) != string(before) {
		t.Fatal("settings.json changed")
	}
	// The 0.0.1 settings struct still decodes it, and its test result is intact.
	var v struct {
		SchemaVersion int                       `json:"schemaVersion"`
		ScanRoots     []string                  `json:"scanRoots"`
		RepoTests     map[string]api.TestResult `json:"repoTests"`
	}
	if err := json.Unmarshal(after, &v); err != nil || v.SchemaVersion != 1 || v.RepoTests[projects.LegacyID(dir)].Status != api.TestOK {
		t.Fatalf("%v %+v", err, v)
	}
	// No new keys: the key set is what the old app wrote.
	var keys map[string]json.RawMessage
	_ = json.Unmarshal(after, &keys)
	for _, k := range []string{"projects", "repos", "repoIds", "legacyId", "missingSince"} {
		if _, ok := keys[k]; ok {
			t.Fatalf("settings.json grew a key %q", k)
		}
	}
}
