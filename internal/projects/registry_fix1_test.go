package projects

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
)

var identAcme = RepoIdentity{FirstCommit: "c0ffee", OriginURL: "https://git.example.com/acme_api.git"}

func goneExcept(keep ...string) func(string) Presence {
	return func(p string) Presence {
		for _, k := range keep {
			if filepath.ToSlash(p) == k {
				return PresencePresent
			}
		}
		return PresenceGone
	}
}

// Two checkouts of one remote: one is deleted, the survivor must not be offered as "moved from" it, while a
// real rename still is.
func TestDeletedSecondCheckoutIsNotAMove(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	ids, _ := s.IDsForPaths([]string{"/w/acme_api", "/w/acme_api_review"}, ProviderCodeCommit, t0)
	api1, review := ids[0], ids[1]
	_ = s.SetIdentities(map[string]RepoIdentity{api1: identAcme, review: identAcme})
	pr, _ := s.AddProject("Acme")
	_ = s.SetProject(api1, pr.ID)
	_ = s.Mutate(func(d *Data) error {
		d.FindRepo(api1).IdentityOverride = &Identity{Name: "Ann", Email: "ann@example.com"}
		return nil
	})
	// The review checkout is deleted (not moved); the next scan sees only /w/acme_api.
	t1 := t0.Add(48 * time.Hour)
	if _, _, err := s.Reconcile(goneExcept("/w/acme_api"), t1); err != nil {
		t.Fatal(err)
	}
	if m, ok := s.MovedFrom(api1); ok {
		t.Fatalf("survivor offered as moved from %+v", m)
	}
	if err := s.Reattach(review, api1); err == nil {
		t.Fatal("re-attach onto the survivor accepted")
	}
	d := s.Snapshot()
	if r := d.FindRepo(api1); r == nil || r.ProjectID != pr.ID || r.IdentityOverride == nil {
		t.Fatalf("survivor lost its data: %+v", r)
	}

	// The rename case: old vanishes and the new folder is first seen in the same scan.
	t2 := t1.Add(time.Hour)
	nid, _ := s.IDsForPaths([]string{"/w/acme_api_main"}, ProviderCodeCommit, t2)
	_ = s.SetIdentities(map[string]RepoIdentity{nid[0]: identAcme})
	_, _, _ = s.Reconcile(goneExcept("/w/acme_api_main"), t2)
	// api1 and review are both missing now with the same identity: two candidates, no offer.
	if _, ok := s.MovedFrom(nid[0]); ok {
		t.Fatal("two candidates must not offer")
	}
}

func TestRenameStillOffered(t *testing.T) {
	s, _ := newStore(t)
	t0 := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	old, _ := s.IDsForPaths([]string{"/w/acme_api"}, ProviderCodeCommit, t0)
	_ = s.SetIdentities(map[string]RepoIdentity{old[0]: identAcme})
	t1 := t0.Add(time.Hour)
	nid, _ := s.IDsForPaths([]string{"/w/acme_api_main"}, ProviderCodeCommit, t1)
	_ = s.SetIdentities(map[string]RepoIdentity{nid[0]: identAcme})
	_, _, _ = s.Reconcile(goneExcept("/w/acme_api_main"), t1)
	m, ok := s.MovedFrom(nid[0])
	if !ok || m.ID != old[0] {
		t.Fatalf("%+v %v", m, ok)
	}
	// A zero FirstSeen (a record from before the field) never matches.
	_ = s.Mutate(func(d *Data) error { d.FindRepo(nid[0]).FirstSeen = nil; return nil })
	if _, ok := s.MovedFrom(nid[0]); ok {
		t.Fatal("zero FirstSeen matched")
	}
}

func TestReattachRefusesRecordWithData(t *testing.T) {
	s, oldID, newID := movedFixture(t)
	if _, ok := s.MovedFrom(newID); !ok {
		t.Fatal("fixture has no offer")
	}
	for name, mut := range map[string]func(r *RepoRecord){
		"project":  func(r *RepoRecord) { r.ProjectID = "p_x" },
		"hidden":   func(r *RepoRecord) { r.Hidden = true },
		"identity": func(r *RepoRecord) { r.IdentityOverride = &Identity{Name: "A", Email: "a@example.com"} },
		"binding":  func(r *RepoRecord) { r.Binding = &Binding{Kind: BindingAWSProfile, Profile: "acme"} },
		"ai":       func(r *RepoRecord) { r.AI = &RepoAIConfig{DefaultTool: "claude"} },
		"integr":   func(r *RepoRecord) { r.Integrated = map[string]string{"origin/main": "abc"} },
	} {
		s2, o2, n2 := movedFixture(t)
		_ = s2.Mutate(func(d *Data) error { mut(d.FindRepo(n2)); return nil })
		if err := s2.Reattach(o2, n2); !errors.Is(err, ErrHasData) {
			t.Fatalf("%s: %v", name, err)
		}
		if repoOf(s2, n2) == nil {
			t.Fatalf("%s: record dropped", name)
		}
	}
	if err := s.Reattach(oldID, newID); err != nil {
		t.Fatal(err)
	}
}

func TestReattachChecksInsideTheWrite(t *testing.T) {
	s, oldID, newID := movedFixture(t)
	// The old folder comes back between the offer and the answer: it is no longer missing.
	_, _, _ = s.Reconcile(func(string) Presence { return PresencePresent }, time.Time{})
	if err := s.Reattach(oldID, newID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
	if repoOf(s, newID) == nil {
		t.Fatal("new record dropped")
	}
}

func TestDeclineMoveSticks(t *testing.T) {
	s, p := newStore(t)
	t0 := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	old, _ := s.IDsForPaths([]string{"/w/acme_api"}, ProviderCodeCommit, t0)
	_ = s.SetIdentities(map[string]RepoIdentity{old[0]: identAcme})
	nid, _ := s.IDsForPaths([]string{"/w/other"}, ProviderCodeCommit, t0.Add(time.Hour))
	_ = s.SetIdentities(map[string]RepoIdentity{nid[0]: identAcme})
	_, _, _ = s.Reconcile(goneExcept("/w/other"), t0.Add(time.Hour))
	if _, ok := s.MovedFrom(nid[0]); !ok {
		t.Fatal("no offer")
	}
	if err := s.DeclineMove(nid[0], old[0]); err != nil {
		t.Fatal(err)
	}
	if err := s.DeclineMove(nid[0], old[0]); err != nil { // idempotent
		t.Fatal(err)
	}
	if err := s.DeclineMove("r_nope", old[0]); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
	_, _, _ = s.Reconcile(goneExcept("/w/other"), t0.Add(2*time.Hour))
	if _, ok := s.MovedFrom(nid[0]); ok {
		t.Fatal("declined offer came back after a rescan")
	}
	s2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s2.MovedFrom(nid[0]); ok {
		t.Fatal("declined offer came back after reopening")
	}
	if n := len(repoOf(s2, nid[0]).DeclinedMoves); n != 1 {
		t.Fatalf("declined list: %d", n)
	}
}

func TestStripURLUserinfo(t *testing.T) {
	for in, want := range map[string]string{
		"https://ghp_PROBEtoken1234@github.com/acme/api.git":      "https://github.com/acme/api.git",
		"https://ann:secret@github.com/acme/api.git":              "https://github.com/acme/api.git",
		"https://github.com/acme/api.git":                         "https://github.com/acme/api.git",
		"https://github.com/acme/api@v2":                          "https://github.com/acme/api@v2",
		"ssh://git@git.example.com:2222/acme/api.git":             "ssh://git.example.com:2222/acme/api.git",
		"git@github.com:acme/api.git":                             "github.com:acme/api.git",
		"github.com:acme/api.git":                                 "github.com:acme/api.git",
		"/srv/git/acme@2/api.git":                                 "/srv/git/acme@2/api.git",
		"https://git-codecommit.us-east-1.amazonaws.com/v1/repos": "https://git-codecommit.us-east-1.amazonaws.com/v1/repos",
		"": "",
	} {
		if got := StripURLUserinfo(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestSetIdentitiesNeverStoresCredentials(t *testing.T) {
	s, p := newStore(t)
	a, _ := s.IDForPath("/w/a")
	b, _ := s.IDForPath("/w/b")
	_ = s.SetIdentities(map[string]RepoIdentity{
		a: {FirstCommit: "c0ffee", OriginURL: "https://ghp_PROBEtoken1234@github.com/acme/api.git"},
		b: {FirstCommit: "c0ffee", OriginURL: "https://ann@github.com/acme/api.git"},
	})
	raw, _ := os.ReadFile(p)
	if strings.Contains(string(raw), "ghp_PROBE") || strings.Contains(string(raw), "ann@") {
		t.Fatalf("credentials in projects.json: %s", raw)
	}
	d := s.Snapshot()
	if d.FindRepo(a).OriginURL != d.FindRepo(b).OriginURL {
		t.Fatal("checkouts differing only in credentials compare unequal")
	}
}

// 0.0.1 hash reuse: a relocated record keeps the hash of its old path; a different repository created at that
// path must not get the old result.
func TestImportSkipsAmbiguousLegacyHash(t *testing.T) {
	s, _ := newStore(t)
	a, _ := s.IDForPath("/code/acme_api")
	legacy := map[string]api.TestResult{LegacyID("/code/acme_api"): {Status: api.TestOK, At: "2026-10-30T08:00:00Z", Profile: "acme"}}
	if n, err := s.ImportLegacyTests(legacy); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	if err := s.Relocate(a, "/code/api-main"); err != nil {
		t.Fatal(err)
	}
	b, _ := s.IDForPath("/code/acme_api") // a different repository at the old path: same hash
	if n, _ := s.ImportLegacyTests(legacy); n != 0 {
		t.Fatalf("imported %d onto an ambiguous hash", n)
	}
	if s.LastTests()[b].Status != "" {
		t.Fatalf("new repository got the old result: %+v", s.LastTests()[b])
	}
	if _, ok := s.ByLegacyID(LegacyID("/code/acme_api")); ok {
		t.Fatal("ByLegacyID answered for an ambiguous hash")
	}
	if _, ok := s.ByLegacyID(LegacyID("/code/other")); ok {
		t.Fatal("unknown hash found")
	}
}

func TestUnknownPresenceLeavesRecordAlone(t *testing.T) {
	s, _ := newStore(t)
	id, _ := s.IDForPath("/w/a")
	m, pg, err := s.Reconcile(func(string) Presence { return PresenceUnknown }, time.Time{})
	if err != nil || len(m) != 0 || len(pg) != 0 || repoOf(s, id).MissingSince != nil {
		t.Fatalf("%v %v %v", m, pg, err)
	}
	// An already-missing record that cannot be probed is not purged either.
	old := time.Now().Add(-40 * 24 * time.Hour)
	_, _, _ = s.Reconcile(func(string) Presence { return PresenceGone }, old)
	_, pg, _ = s.Reconcile(func(string) Presence { return PresenceUnknown }, time.Time{})
	if len(pg) != 0 || repoOf(s, id) == nil {
		t.Fatal("purged on an unknown probe")
	}
}

func TestProbePathPermissionDeniedIsUnknown(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits do not apply")
	}
	parent := t.TempDir()
	repo := filepath.Join(parent, "acme_api")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if ProbePath(repo) != PresencePresent {
		t.Fatal("present folder not found")
	}
	if err := os.Chmod(parent, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(parent, 0o755)
	if got := ProbePath(repo); got != PresenceUnknown {
		t.Fatalf("permission error read as %v", got)
	}
	s, _ := newStore(t)
	id, _ := s.IDForPath(repo)
	_, _, _ = s.Reconcile(ProbePath, time.Time{})
	if repoOf(s, id).MissingSince != nil {
		t.Fatal("record marked missing though the folder exists")
	}
	_ = os.Chmod(parent, 0o755)
	if ProbePath(filepath.Join(parent, "nope")) != PresenceGone {
		t.Fatal("absent folder not gone")
	}
}

func TestSetRepoPathClearsMissing(t *testing.T) {
	s, _ := newStore(t)
	id, _ := s.IDForPath("/w/a")
	_, _, _ = s.reconcileB(func(string) bool { return false })
	if repoOf(s, id).MissingSince == nil {
		t.Fatal("not marked")
	}
	if err := s.SetRepoPath(id, "/w/b"); err != nil {
		t.Fatal(err)
	}
	if repoOf(s, id).MissingSince != nil {
		t.Fatal("SetRepoPath kept the missing mark")
	}
}

func TestFirstSeenAndProviderFilledIn(t *testing.T) {
	s, _ := newStore(t)
	at := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	id, _ := s.IDForPath("/w/a")
	_, _ = s.IDsForPaths([]string{"/w/a", "/w/b"}, ProviderCodeCommit, at)
	d := s.Snapshot()
	if d.FindRepo(id).Provider != ProviderCodeCommit {
		t.Fatal("provider not filled in on an existing record")
	}
	bID, _ := s.IDsForPaths([]string{"/w/b"}, "", time.Time{})
	if r := d.FindByPath("/w/b"); r == nil || r.FirstSeen == nil || !r.FirstSeen.Equal(at) || r.ID != bID[0] {
		t.Fatalf("%+v", r)
	}
}

func TestFoldMatchRespellsStoredPath(t *testing.T) {
	root := t.TempDir()
	upper := filepath.Join(root, "Acme_API")
	if err := os.MkdirAll(upper, 0o755); err != nil {
		t.Fatal(err)
	}
	lower := filepath.Join(root, "acme_api")
	if _, err := os.Stat(lower); err != nil {
		t.Skip("case-sensitive volume")
	}
	old := pathFold
	defer func() { pathFold = old }()
	pathFold = true
	s, _ := newStore(t)
	id, _ := s.IDForPath(lower)
	// The folder was renamed in case only; the scan now reports the new spelling: same record, new spelling.
	got, _ := s.IDForPath(upper)
	if got != id {
		t.Fatalf("case rename made a new record: %s %s", got, id)
	}
	if p, _ := s.PathOf(id); p != upper {
		t.Fatalf("stored spelling %q, want %q", p, upper)
	}
}

func TestFoldMatchKeepsTwoDifferentFolders(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "Acme"), filepath.Join(root, "acme")
	if err := os.MkdirAll(a, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(b, 0o755); err != nil {
		t.Skip("case-insensitive volume")
	}
	if sa, _ := os.Stat(a); sa != nil {
		if sb, _ := os.Stat(b); sb != nil && os.SameFile(sa, sb) {
			t.Skip("case-insensitive volume")
		}
	}
	old := pathFold
	defer func() { pathFold = old }()
	pathFold = true
	s, _ := newStore(t)
	ia, _ := s.IDForPath(a)
	ib, _ := s.IDForPath(b)
	if ia == ib || len(s.Snapshot().Repos) != 2 {
		t.Fatalf("two different folders became one record: %s %s", ia, ib)
	}
}
