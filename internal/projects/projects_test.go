package projects

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/backup"
	"github.com/rudian/codepass/internal/settings"
	"github.com/rudian/codepass/internal/store"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "projects.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	return s, p
}

func TestOpenAbsentCreatesNothing(t *testing.T) {
	_, p := newStore(t)
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("file created on open")
	}
}

func TestRoundTrip(t *testing.T) {
	s, p := newStore(t)
	pr, err := s.AddProject("  Acme web ")
	if err != nil || pr.Name != "Acme web" || pr.Order != 0 {
		t.Fatalf("%+v %v", pr, err)
	}
	rec, created, err := s.EnsureRepo("/work/acme/api", ProviderGitHub)
	if err != nil || !created {
		t.Fatal(err, created)
	}
	if err := s.SetProject(rec.ID, pr.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetHidden(rec.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetIntegrated(rec.ID, "origin/main", "abc123"); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	d := s2.Snapshot()
	r := d.FindRepo(rec.ID)
	if r == nil || r.ProjectID != pr.ID || !r.Hidden || r.Integrated["origin/main"] != "abc123" || r.Provider != ProviderGitHub {
		t.Fatalf("%+v", r)
	}
	if d.SchemaVersion != 1 || len(d.Projects) != 1 {
		t.Fatalf("%+v", d)
	}
}

func TestRepoIDIsRandomAndSurvivesMove(t *testing.T) {
	s, _ := newStore(t)
	a, _, _ := s.EnsureRepo("/work/acme/api", ProviderOther)
	if strings.Contains(a.ID, a.LegacyID) || a.ID == a.LegacyID {
		t.Fatal("ID derived from path")
	}
	if a.LegacyID != LegacyID("/work/acme/api") || len(a.LegacyID) != 16 {
		t.Fatalf("legacy %q", a.LegacyID)
	}
	s2, _ := newStore(t)
	other, _, _ := s2.EnsureRepo("/work/acme/api", ProviderOther)
	idRe := regexp.MustCompile(`^r_[0-9a-f]{32}$`)
	if other.ID == a.ID || !idRe.MatchString(a.ID) || !idRe.MatchString(other.ID) {
		t.Fatalf("ids %q %q", a.ID, other.ID)
	}
	again, created, _ := s.EnsureRepo("/work/acme/api/", ProviderOther)
	if created || again.ID != a.ID {
		t.Fatal("same path got a second record")
	}
	if err := s.SetRepoPath(a.ID, "/work/acme/api-renamed"); err != nil {
		t.Fatal(err)
	}
	d := s.Snapshot()
	r := d.FindRepo(a.ID)
	if r.Path != filepath.Clean("/work/acme/api-renamed") || r.LegacyID != a.LegacyID {
		t.Fatalf("%+v", r)
	}
	if d.FindByLegacyID(a.LegacyID) == nil || d.FindByLegacyID("") != nil {
		t.Fatal("legacy lookup")
	}
	b, _, _ := s.EnsureRepo("/work/globex/web", ProviderOther)
	if err := s.SetRepoPath(b.ID, "/work/acme/api-renamed"); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("%v", err)
	}
	if b.ID == a.ID {
		t.Fatal("equal IDs")
	}
}

func TestLegacyIDMatches001Hash(t *testing.T) {
	// sha1 of the cleaned path, first 16 hex digits: the 0.0.1 repository ID.
	path, want := "/work/acme/api", "bcdfc0ba3ce2b296"
	if runtime.GOOS == "windows" {
		path, want = "/work/acme/api", "dc90093d5f86de27" // Clean gives \work\acme\api
	}
	if got := LegacyID(path); got != want {
		t.Fatal(got)
	}
}

func TestNotFound(t *testing.T) {
	s, _ := newStore(t)
	if err := s.SetHidden("nope", true); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	rec, _, _ := s.EnsureRepo("/a", ProviderOther)
	if err := s.SetProject(rec.ID, "p_missing"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.AddProject("  "); err == nil {
		t.Fatal("empty name")
	}
}

func TestNewerVersionReportedNotOverwritten(t *testing.T) {
	p := filepath.Join(t.TempDir(), "projects.json")
	newer := `{"schemaVersion":9,"projects":[],"repos":[],"extra":"keep me"}`
	_ = os.WriteFile(p, []byte(newer), 0o600)
	if _, err := Open(p); !errors.Is(err, store.ErrNewerVersion) {
		t.Fatalf("%v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != newer {
		t.Fatal("overwritten")
	}
}

func TestSettingsFileUntouched(t *testing.T) {
	dir := t.TempDir()
	sp := filepath.Join(dir, "settings.json")
	orig := `{"schemaVersion":1,"language":"en"}`
	_ = os.WriteFile(sp, []byte(orig), 0o600)
	s, err := Open(filepath.Join(dir, "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _ = s.EnsureRepo("/a", ProviderOther)
	if b, _ := os.ReadFile(sp); string(b) != orig {
		t.Fatal("settings.json changed")
	}
	// The 0.0.1 app only rewrites settings.json and snapshots into backups/; projects.json stays as it is.
	pp := filepath.Join(dir, "projects.json")
	beforeP, _ := os.ReadFile(pp)
	bak := pp + ".v0.bak"
	_ = os.WriteFile(bak, []byte("x"), 0o600)
	st, _, err := settings.Open(sp, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetUILanguage("en"); err != nil {
		t.Fatal(err)
	}
	if err := st.CompleteFirstRun(); err != nil {
		t.Fatal(err)
	}
	bs, err := backup.Open(filepath.Join(dir, "backups"), func() int { return 1 }, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := bs.Snapshot(sp, "settings", api.BackupGitGlobal, api.ReasonAddProfiles, nil); err != nil {
			t.Fatal(err)
		}
	}
	afterP, _ := os.ReadFile(pp)
	if string(beforeP) != string(afterP) || !strings.Contains(string(afterP), `"schemaVersion": 1`) {
		t.Fatal("projects.json changed")
	}
	if b, err := os.ReadFile(bak); err != nil || string(b) != "x" {
		t.Fatal("sibling file changed")
	}
}

func TestSnapshotIsACopy(t *testing.T) {
	s, _ := newStore(t)
	rec, _, _ := s.EnsureRepo("/a", ProviderOther)
	_ = s.SetIntegrated(rec.ID, "origin/main", "1")
	d := s.Snapshot()
	d.Repos[0].Integrated["origin/main"] = "tampered"
	d.Repos[0].Path = "/x"
	d2 := s.Snapshot()
	if d2.Repos[0].Integrated["origin/main"] != "1" || d2.Repos[0].Path == "/x" {
		t.Fatal("snapshot aliased")
	}
}

func TestConcurrentWriters(t *testing.T) {
	s, p := newStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, _, err := s.EnsureRepo(filepath.Join("/work", string(rune('a'+i%26)), string(rune('a'+i/26))), ProviderOther); err != nil {
				t.Error(err)
			}
			if _, err := s.AddProject("p"); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	s2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	d := s2.Snapshot()
	if len(d.Repos) != 30 || len(d.Projects) != 30 {
		t.Fatalf("lost writes: %d repos %d projects", len(d.Repos), len(d.Projects))
	}
	orders := map[int]bool{}
	for _, pr := range d.Projects {
		orders[pr.Order] = true
	}
	if len(orders) != 30 {
		t.Fatal("duplicate project order")
	}
}

func TestUndecodableBodyRecovery(t *testing.T) {
	p := filepath.Join(t.TempDir(), "projects.json")
	bad := `{"schemaVersion":1,"projects":[{"id":"p_1","name":"acme","order":"1"}],"repos":[]}`
	_ = os.WriteFile(p, []byte(bad), 0o600)
	if _, err := Open(p); !errors.Is(err, store.ErrCorrupt) {
		t.Fatalf("%v", err)
	}
	f, _ := store.Open(p, store.Spec{Current: SchemaVersion})
	var d Data
	if _, err := f.Load(&d); !errors.Is(err, store.ErrCorrupt) {
		t.Fatal(err)
	}
	if err := f.Save(&Data{}); !errors.Is(err, store.ErrCorrupt) {
		t.Fatalf("save should be refused: %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != bad {
		t.Fatal("overwritten")
	}
	if _, err := Quarantine(p); err != nil {
		t.Fatal(err)
	}
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddProject("fresh"); err != nil {
		t.Fatal(err)
	}
}

func TestTwoStoresSamePathKeepAllWrites(t *testing.T) {
	p := filepath.Join(t.TempDir(), "projects.json")
	a, _ := Open(p)
	b, _ := Open(p)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		for _, s := range []*Store{a, b} {
			wg.Add(1)
			go func(s *Store) {
				defer wg.Done()
				if _, err := s.AddProject("p"); err != nil {
					t.Error(err)
				}
			}(s)
		}
	}
	wg.Wait()
	c, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(c.Snapshot().Projects); n != 40 {
		t.Fatalf("kept %d of 40", n)
	}
}
