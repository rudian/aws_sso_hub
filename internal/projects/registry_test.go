package projects

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/settings"
)

func TestRegistryIDStaysAcrossRelocate(t *testing.T) {
	s, p := newStore(t)
	pr, _ := s.AddProject("Acme")
	id, err := s.IDForPath("/work/acme/api")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := s.IDForPath("/work/acme/api")
	if again != id {
		t.Fatalf("second call %s != %s", again, id)
	}
	_ = s.SetProject(id, pr.ID)
	_ = s.SetHidden(id, true)
	_ = s.Mutate(func(d *Data) error {
		d.FindRepo(id).IdentityOverride = &Identity{Name: "Ann", Email: "ann@example.com"}
		return nil
	})
	if err := s.Relocate(id, "/work/acme/api-main"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.IDForPath("/work/acme/api-main"); got != id {
		t.Fatalf("ID changed on relocate: %s", got)
	}
	if path, ok := s.PathOf(id); !ok || path != filepath.Clean("/work/acme/api-main") {
		t.Fatalf("PathOf %q %v", path, ok)
	}
	// A reopened file agrees; project, hidden flag and identity override survived.
	s2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	d := s2.Snapshot()
	r := d.FindRepo(id)
	if r == nil || r.ProjectID != pr.ID || !r.Hidden || r.IdentityOverride == nil || r.IdentityOverride.Email != "ann@example.com" {
		t.Fatalf("%+v", r)
	}
	if len(d.Repos) != 1 {
		t.Fatalf("records: %d", len(d.Repos))
	}
	// The legacy ID is still the hash of the first path.
	if r.LegacyID != LegacyID("/work/acme/api") {
		t.Fatalf("legacy changed: %s", r.LegacyID)
	}
}

func TestRelocateRefusals(t *testing.T) {
	s, _ := newStore(t)
	a, _ := s.IDForPath("/w/a")
	_, _ = s.IDForPath("/w/b")
	if err := s.Relocate(a, "/w/b"); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("%v", err)
	}
	if err := s.Relocate("nope", "/w/c"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
	if err := s.Relocate(a, " "); err == nil {
		t.Fatal("empty path accepted")
	}
}

func TestRegistryLegacyLookup(t *testing.T) {
	s, _ := newStore(t)
	path := filepath.Join(string(filepath.Separator), "work", "acme", "api")
	id, _ := s.IDForPath(path)
	// The 0.0.1 formula, written out: first 16 hex digits of SHA-1 over the cleaned path.
	sum := sha1.Sum([]byte(filepath.Clean(path)))
	legacy := hex.EncodeToString(sum[:])[:16]
	rec, ok := s.ByLegacyID(legacy)
	if !ok || rec.ID != id || rec.LegacyID != legacy {
		t.Fatalf("%+v %v", rec, ok)
	}
	if _, ok := s.ByLegacyID("0000000000000000"); ok {
		t.Fatal("unknown legacy ID matched")
	}
	if _, ok := s.ByLegacyID(""); ok {
		t.Fatal("empty legacy ID matched")
	}
	if id == legacy || len(id) <= len(legacy) {
		t.Fatalf("ID looks derived from the path: %s", id)
	}
}

func TestSameFolderNameInTwoParents(t *testing.T) {
	s, _ := newStore(t)
	ids, err := s.IDsForPaths([]string{"/work/acme/api", "/work/globex/api"}, ProviderCodeCommit, time.Time{})
	if err != nil || ids[0] == ids[1] || ids[0] == "" {
		t.Fatalf("%v %v", ids, err)
	}
	if n := len(s.Snapshot().Repos); n != 2 {
		t.Fatalf("records: %d", n)
	}
}

func TestConcurrentIDForPath(t *testing.T) {
	_, p := newStore(t)
	// Two Store values on one file (as two services would hold) and many goroutines on one path.
	a, _ := Open(p)
	b, _ := Open(p)
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		got = map[string]bool{}
	)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			st := a
			if i%2 == 1 {
				st = b
			}
			id, err := st.IDForPath("/work/acme/api")
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			got[id] = true
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	if len(got) != 1 {
		t.Fatalf("IDs: %v", got)
	}
	c, _ := Open(p)
	if n := len(c.Snapshot().Repos); n != 1 {
		t.Fatalf("records: %d", n)
	}
}

func TestPathCaseFold(t *testing.T) {
	old := pathFold
	defer func() { pathFold = old }()

	pathFold = true // macOS and Windows
	s, _ := newStore(t)
	a, _ := s.IDForPath("/Work/Acme/API")
	b, _ := s.IDForPath("/work/acme/api")
	if a != b || len(s.Snapshot().Repos) != 1 {
		t.Fatalf("case-insensitive: %s %s", a, b)
	}

	pathFold = false // Linux
	s2, _ := newStore(t)
	a, _ = s2.IDForPath("/Work/Acme/API")
	b, _ = s2.IDForPath("/work/acme/api")
	if a == b || len(s2.Snapshot().Repos) != 2 {
		t.Fatalf("case-sensitive: %s %s", a, b)
	}
}

func TestPathCaseFoldMatchesHost(t *testing.T) {
	// The default follows the platform: Linux compares exactly, macOS and Windows fold case.
	s, _ := newStore(t)
	a, _ := s.IDForPath("/Work/API")
	b, _ := s.IDForPath("/work/api")
	want := runtime.GOOS != "linux"
	if pathFold != want {
		t.Fatalf("pathFold=%v on %s", pathFold, runtime.GOOS)
	}
	if (a == b) != want {
		t.Fatalf("fold=%v same=%v", want, a == b)
	}
}

// settingsWithTests writes a settings.json with the current settings store, holding connection results under
// the 0.0.1 path hash.
func settingsWithTests(t *testing.T, dir string, legacyIDs ...string) (path string, raw []byte) {
	t.Helper()
	path = filepath.Join(dir, "settings.json")
	st, _, err := settings.Open(path, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range legacyIDs {
		if err := st.SetRepoTest(api.TestResult{RepoID: l, Status: api.TestOK, At: fmt.Sprintf("2026-11-0%dT10:00:00Z", i+1), Profile: "acme", Detail: "secret detail"}); err != nil {
			t.Fatal(err)
		}
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, raw
}

func TestImportLegacyTestsAndSettingsUntouched(t *testing.T) {
	dir := t.TempDir()
	sp, raw := settingsWithTests(t, dir, LegacyID("/work/acme/api"), LegacyID("/work/gone/none"))

	s, err := Open(filepath.Join(dir, "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	api1, _ := s.IDForPath("/work/acme/api")
	other, _ := s.IDForPath("/work/acme/other")

	legacy := readLegacyTests(t, sp)
	n, err := s.ImportLegacyTests(legacy)
	if err != nil || n != 1 {
		t.Fatalf("imported %d %v", n, err)
	}
	lt := s.LastTests()
	got, ok := lt[api1]
	if !ok || got.Status != api.TestOK || got.RepoID != api1 || got.Profile != "acme" || got.Detail != "" {
		t.Fatalf("%+v", got)
	}
	if _, ok := lt[other]; ok {
		t.Fatal("untested repository got a result")
	}
	// Running it again changes nothing; a newer result is not overwritten by an old one.
	_ = s.SetLastTest(api1, api.TestResult{Status: api.TestNetwork, At: "2026-11-10T00:00:00Z", Detail: "x"})
	if n, err := s.ImportLegacyTests(legacy); err != nil || n != 0 {
		t.Fatalf("second import %d %v", n, err)
	}
	if lt := s.LastTests()[api1]; lt.Status != api.TestNetwork || lt.RepoID != api1 || lt.Detail != "" {
		t.Fatalf("%+v", lt)
	}

	// More writes of every kind; settings.json stays byte for byte what the old app wrote.
	_ = s.SetHidden(api1, true)
	_ = s.Relocate(api1, "/work/acme/api2")
	_, _, _ = s.reconcileB(func(string) bool { return false })
	_ = s.SetIdentities(map[string]RepoIdentity{api1: {FirstCommit: "abc", OriginURL: "u"}})
	if after, _ := os.ReadFile(sp); string(after) != string(raw) {
		t.Fatal("settings.json changed")
	}
	assertOldAppDecodes(t, sp)
}

func readLegacyTests(t *testing.T, settingsPath string) map[string]api.TestResult {
	t.Helper()
	st, _, err := settings.Open(settingsPath, filepath.Dir(settingsPath), nil)
	if err != nil {
		t.Fatal(err)
	}
	return st.RepoTests()
}

// v001Settings is the shape of settings.json as v0.0.1 decodes it (git show v0.0.1:internal/settings/settings.go,
// type data). Fields it did not know are ignored by encoding/json, as they were in 0.0.1.
type v001Settings struct {
	SchemaVersion        int                           `json:"schemaVersion"`
	Language             api.Language                  `json:"language"`
	Theme                api.Theme                     `json:"theme"`
	Motion               api.MotionPref                `json:"motion"`
	ScanRoots            []string                      `json:"scanRoots"`
	ExtraRepos           []string                      `json:"extraRepos"`
	Reminders            api.ReminderSettings          `json:"reminders"`
	Tray                 api.TraySettings              `json:"tray"`
	TerminalApp          api.TerminalApp               `json:"terminalApp"`
	BackupRetention      int                           `json:"backupRetention"`
	TutorialDone         []string                      `json:"tutorialDone"`
	TutorialLang         string                        `json:"tutorialLang"`
	AWSCLIPath           string                        `json:"awsCliPath"`
	GitPath              string                        `json:"gitPath"`
	AutoBindOnAdd        bool                          `json:"autoBindOnAdd"`
	FirstRunDone         bool                          `json:"firstRunDone"`
	UILanguage           string                        `json:"uiLanguage"`
	AccountColors        map[string]int                `json:"accountColors"`
	Monograms            map[string]string             `json:"monograms"`
	AccountNames         map[string]string             `json:"accountNames"`
	StaticIdentities     map[string]api.StaticIdentity `json:"staticIdentities"`
	Sessions             map[string]api.SessionRecord  `json:"sessions"`
	RepoTests            map[string]api.TestResult     `json:"repoTests"`
	PendingDeactivations []json.RawMessage             `json:"pendingDeactivations"`
}

func assertOldAppDecodes(t *testing.T, settingsPath string) {
	t.Helper()
	b, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var d v001Settings
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("0.0.1 cannot decode settings.json: %v", err)
	}
	if d.SchemaVersion != 1 || len(d.RepoTests) == 0 {
		t.Fatalf("%+v", d)
	}
}

func TestReconcileMissingAndPurge(t *testing.T) {
	p := filepath.Join(t.TempDir(), "projects.json")
	clock := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	s, err := open(p, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.IDForPath("/w/a")
	b, _ := s.IDForPath("/w/b")
	present := map[string]bool{"/w/a": true, "/w/b": true}
	exists := func(path string) bool { return present[filepath.ToSlash(path)] }

	if m, pg, err := s.reconcileB(exists); err != nil || len(m)+len(pg) != 0 {
		t.Fatalf("%v %v %v", m, pg, err)
	}
	present["/w/b"] = false
	m, _, err := s.reconcileB(exists)
	if err != nil || len(m) != 1 || m[0] != b {
		t.Fatalf("%v %v", m, err)
	}
	first := *repoOf(s, b).MissingSince
	if !first.Equal(clock) {
		t.Fatalf("missingSince %v", first)
	}
	// 29 days later: still kept, mark unchanged.
	clock = clock.Add(29 * 24 * time.Hour)
	if m, pg, _ := s.reconcileB(exists); len(m) != 1 || len(pg) != 0 || !repoOf(s, b).MissingSince.Equal(first) {
		t.Fatalf("%v %v", m, pg)
	}
	// The folder comes back: the mark is cleared.
	present["/w/b"] = true
	if m, _, _ := s.reconcileB(exists); len(m) != 0 || repoOf(s, b).MissingSince != nil {
		t.Fatalf("%v", m)
	}
	// Gone again, then 31 days: the record is deleted, the other one stays.
	present["/w/b"] = false
	_, _, _ = s.reconcileB(exists)
	clock = clock.Add(31 * 24 * time.Hour)
	_, pg, err := s.reconcileB(exists)
	if err != nil || len(pg) != 1 || pg[0] != b {
		t.Fatalf("%v %v", pg, err)
	}
	d := s.Snapshot()
	if d.FindRepo(b) != nil || d.FindRepo(a) == nil {
		t.Fatalf("%+v", d.Repos)
	}
}

func movedFixture(t *testing.T) (s *Store, oldID, newID string) {
	t.Helper()
	s, _ = newStore(t)
	pr, _ := s.AddProject("Acme")
	oldID, _ = s.IDForPath("/w/acme_api")
	_ = s.SetProject(oldID, pr.ID)
	_ = s.SetHidden(oldID, true)
	_ = s.Mutate(func(d *Data) error {
		d.FindRepo(oldID).IdentityOverride = &Identity{Name: "Ann", Email: "ann@example.com"}
		return nil
	})
	_ = s.SetIdentities(map[string]RepoIdentity{oldID: {FirstCommit: "c0ffee", OriginURL: "https://git.example.com/acme_api.git"}})
	// The folder vanishes (moved by hand outside the app).
	_, _, _ = s.reconcileB(func(string) bool { return false })
	// A scan finds the repository at a new place.
	newID, _ = s.IDForPath("/elsewhere/acme_api_new")
	_ = s.SetIdentities(map[string]RepoIdentity{newID: {FirstCommit: "c0ffee", OriginURL: "https://git.example.com/acme_api.git"}})
	return s, oldID, newID
}

func TestMovedFromAndReattach(t *testing.T) {
	s, oldID, newID := movedFixture(t)
	if _, ok := s.MovedFrom(oldID); ok {
		t.Fatal("the missing record is not a new find")
	}
	m, ok := s.MovedFrom(newID)
	if !ok || m.ID != oldID {
		t.Fatalf("%+v %v", m, ok)
	}
	if err := s.Reattach("r_other", newID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong old ID accepted: %v", err)
	}
	if err := s.Reattach(oldID, newID); err != nil {
		t.Fatal(err)
	}
	d := s.Snapshot()
	if len(d.Repos) != 1 {
		t.Fatalf("records: %d", len(d.Repos))
	}
	r := d.FindRepo(oldID)
	if r == nil || r.Path != filepath.Clean("/elsewhere/acme_api_new") || r.MissingSince != nil || !r.Hidden || r.ProjectID == "" ||
		r.IdentityOverride == nil || r.IdentityOverride.Email != "ann@example.com" {
		t.Fatalf("%+v", r)
	}
	if r.LegacyID != LegacyID("/w/acme_api") {
		t.Fatalf("legacy ID changed: %s", r.LegacyID)
	}
	if id, _ := s.IDForPath("/elsewhere/acme_api_new"); id != oldID {
		t.Fatalf("the new folder resolves to %s", id)
	}
}

func TestMovedFromNeedsExactlyOne(t *testing.T) {
	// Two records with the same identity vanish in one scan and the same folder is found in it: ambiguous.
	s, _ := newStore(t)
	t0 := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	o1, _ := s.IDForPath("/w/acme_api")
	o2, _ := s.IDForPath("/w/acme_api_copy")
	ident := RepoIdentity{FirstCommit: "c0ffee", OriginURL: "https://git.example.com/acme_api.git"}
	_ = s.SetIdentities(map[string]RepoIdentity{o1: ident, o2: ident})
	scan := t0.Add(time.Hour)
	newID, _ := s.IDsForPaths([]string{"/elsewhere/acme_api_new"}, ProviderCodeCommit, scan)
	_ = s.SetIdentities(map[string]RepoIdentity{newID[0]: ident})
	_, _, _ = s.Reconcile(func(p string) Presence {
		if filepath.ToSlash(p) == "/elsewhere/acme_api_new" {
			return PresencePresent
		}
		return PresenceGone
	}, scan)
	if _, ok := s.MovedFrom(newID[0]); ok {
		t.Fatal("offered with two candidates")
	}

	// A different origin URL or first commit is no match.
	s2, _, n2 := movedFixture(t)
	_ = s2.SetIdentities(map[string]RepoIdentity{n2: {FirstCommit: "c0ffee", OriginURL: "https://git.example.com/other.git"}})
	if _, ok := s2.MovedFrom(n2); ok {
		t.Fatal("different origin matched")
	}
	s3, _, n3 := movedFixture(t)
	_ = s3.Mutate(func(d *Data) error { d.FindRepo(n3).FirstCommit = "decaf"; return nil })
	if _, ok := s3.MovedFrom(n3); ok {
		t.Fatal("different first commit matched")
	}
	// A record without a first commit never matches anything.
	s4, _ := newStore(t)
	x, _ := s4.IDForPath("/a")
	y, _ := s4.IDForPath("/b")
	_, _, _ = s4.reconcileB(func(p string) bool { return filepath.ToSlash(p) == "/b" })
	if _, ok := s4.MovedFrom(y); ok {
		t.Fatalf("matched %s without identity", x)
	}
}

func TestSetIdentitiesKeepsKnownFirstCommit(t *testing.T) {
	s, _ := newStore(t)
	id, _ := s.IDForPath("/a")
	_ = s.SetIdentities(map[string]RepoIdentity{id: {FirstCommit: "aaa", OriginURL: "u1"}})
	_ = s.SetIdentities(map[string]RepoIdentity{id: {FirstCommit: "", OriginURL: ""}}) // unreadable repository
	r := repoOf(s, id)
	if r.FirstCommit != "aaa" || r.OriginURL != "u1" {
		t.Fatalf("%+v", r)
	}
	_ = s.SetIdentities(map[string]RepoIdentity{id: {FirstCommit: "aaa", OriginURL: "u2"}}) // remote changed
	if r := repoOf(s, id); r.OriginURL != "u2" {
		t.Fatalf("%+v", r)
	}
}

func TestSnapshotDeepCopiesMissingSince(t *testing.T) { // FU-02a
	s, _ := newStore(t)
	id, _ := s.IDForPath("/a")
	_, _, _ = s.reconcileB(func(string) bool { return false })
	d := s.Snapshot()
	want := *d.FindRepo(id).MissingSince
	*d.FindRepo(id).MissingSince = want.Add(48 * time.Hour)
	if got := repoOf(s, id).MissingSince; got == nil || !got.Equal(want) {
		t.Fatalf("snapshot shares MissingSince with the store: %v", got)
	}
}

func TestOpenMemoryNeverWrites(t *testing.T) {
	s := OpenMemory()
	id, err := s.IDForPath("/a")
	if err != nil || id == "" {
		t.Fatal(id, err)
	}
	if err := s.SetLastTest(id, api.TestResult{Status: api.TestOK}); err != nil {
		t.Fatal(err)
	}
	if len(s.LastTests()) != 1 {
		t.Fatal("not kept in memory")
	}
	if err := s.Mutate(func(d *Data) error { return errors.New("boom") }); err == nil || len(s.Snapshot().Repos) != 1 {
		t.Fatal("rollback")
	}
}

// repoOf returns a copy of the record, or nil.
func repoOf(s *Store, id string) *RepoRecord {
	d := s.Snapshot()
	return d.FindRepo(id)
}

// reconcileB is Reconcile for tests that only care about present / gone.
func (s *Store) reconcileB(exists func(string) bool) ([]string, []string, error) {
	return s.Reconcile(func(p string) Presence {
		if exists(p) {
			return PresencePresent
		}
		return PresenceGone
	}, time.Time{})
}
