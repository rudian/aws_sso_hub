package projects

import (
	"errors"
	"testing"
	"time"
)

// FU-18a: a found record that already has data of its own is not offered as "moved here", so the question
// goes away instead of failing on "yes". Reattach still refuses it with ErrHasData if asked anyway.
func TestMovedFromSkipsFoundRecordWithData(t *testing.T) {
	for name, mut := range map[string]func(r *RepoRecord){
		"project":  func(r *RepoRecord) { r.ProjectID = "p_x" },
		"hidden":   func(r *RepoRecord) { r.Hidden = true },
		"identity": func(r *RepoRecord) { r.IdentityOverride = &Identity{Name: "A", Email: "a@example.com"} },
	} {
		s, oldID, newID := movedFixture(t)
		if m, ok := s.MovedFrom(newID); !ok || m.ID != oldID {
			t.Fatalf("%s: fixture has no offer", name)
		}
		_ = s.Mutate(func(d *Data) error { mut(d.FindRepo(newID)); return nil })
		if _, ok := s.MovedFrom(newID); ok {
			t.Errorf("%s: the question stays after the found record got data of its own", name)
		}
		if err := s.Reattach(oldID, newID); !errors.Is(err, ErrHasData) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A pending (not yet reviewed) record has no data of its own: the question stays.
	s, _, newID := movedFixture(t)
	_ = s.Mutate(func(d *Data) error { d.FindRepo(newID).Pending = true; return nil })
	if _, ok := s.MovedFrom(newID); !ok {
		t.Error("a pending record lost its question")
	}
}

func TestIDsForFoundPendingAndProvider(t *testing.T) {
	s, _ := newStore(t)
	at := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	ids, created, err := s.IDsForFound([]string{"/w/a", "/w/b"}, ProviderGitHub, at, true)
	if err != nil || !created[0] || !created[1] {
		t.Fatalf("%v %v", err, created)
	}
	d := s.Snapshot()
	if r := d.FindRepo(ids[0]); r == nil || !r.Pending || r.Provider != ProviderGitHub || r.FirstSeen == nil {
		t.Fatalf("%+v", r)
	}
	// Known folders are not created again, stay pending, and take the provider of the current remote.
	ids2, created2, err := s.IDsForFound([]string{"/w/a"}, ProviderBitbucket, at.Add(time.Hour), true)
	if err != nil || created2[0] || ids2[0] != ids[0] {
		t.Fatalf("%v %v", err, created2)
	}
	d = s.Snapshot()
	if r := d.FindRepo(ids[0]); !r.Pending || r.Provider != ProviderBitbucket {
		t.Fatalf("%+v", r)
	}
	// The review: show one, hide the other. Hide beats nothing else; both leave the pending set.
	if err := s.ResolvePending([]string{ids[0]}, []string{ids[1], "r_unknown"}); err != nil {
		t.Fatal(err)
	}
	d = s.Snapshot()
	if a, b := d.FindRepo(ids[0]), d.FindRepo(ids[1]); a.Pending || a.Hidden || b.Pending || !b.Hidden {
		t.Fatalf("%+v %+v", a, b)
	}
	// A shown repository is not hidden by a later "hide" for a record that is not pending... it is: hide is explicit.
	if err := s.ResolvePending(nil, []string{ids[0]}); err != nil {
		t.Fatal(err)
	}
	if d = s.Snapshot(); !d.FindRepo(ids[0]).Hidden {
		t.Error("explicit hide ignored")
	}
}

func TestProjectEditsAndDismissals(t *testing.T) {
	s, p := newStore(t)
	pr, _ := s.AddProject("Acme")
	ids, _ := s.IDsForPaths([]string{"/w/a"}, ProviderCodeCommit, time.Time{})
	_ = s.SetProject(ids[0], pr.ID)
	if err := s.RenameProject(pr.ID, " Globex "); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameProject("p_x", "n"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
	if err := s.SetProjectCollapsed(pr.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.DismissSuggestion("prefix:initech"); err != nil {
		t.Fatal(err)
	}
	if err := s.DismissSuggestion("prefix:initech"); err != nil { // twice is fine
		t.Fatal(err)
	}
	s2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	d := s2.Snapshot()
	if got := d.FindProject(pr.ID); got.Name != "Globex" || !got.Collapsed || len(d.DismissedSuggestions) != 1 {
		t.Fatalf("%+v %v", got, d.DismissedSuggestions)
	}
	if err := s2.RemoveProject(pr.ID); err != nil {
		t.Fatal(err)
	}
	d = s2.Snapshot()
	if len(d.Projects) != 0 || d.FindRepo(ids[0]).ProjectID != "" {
		t.Fatalf("%+v", d)
	}
	// Old files without the new fields load as before: nothing pending.
	if d.FindRepo(ids[0]).Pending {
		t.Error("pending by default")
	}
}
