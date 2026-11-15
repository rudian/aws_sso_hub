package projects

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rudian/codepass/internal/api"
)

// MissingKeep is how long a record of a repository that vanished from disk is kept, so a folder moved by
// hand can be re-attached (plan 4.4a).
const MissingKeep = 30 * 24 * time.Hour

// This file is the repository registry (F1-18): the one place that maps a folder to a repository ID and back.
// An ID is random, made the first time a path is seen, and never computed from a path again; the 0.0.1 hash
// lives only in RepoRecord.LegacyID. Every write goes through Store.Mutate, so it is atomic, serialised, and
// follows the store's backup-before-first-write-of-a-version rule. Nothing here reads or writes settings.json.

// OpenMemory returns a registry that lives in memory only and never touches the disk. The repository engine
// uses it when it is given no registry (tests, tools); the app passes the real projects.json store.
func OpenMemory() *Store {
	return &Store{d: Data{SchemaVersion: SchemaVersion}, now: time.Now}
}

// IDForPath returns the ID of the repository at path, creating the record (random ID, LegacyID from the path)
// the first time the path is seen. Paths that differ only in case are one record on macOS and Windows.
func (s *Store) IDForPath(path string) (string, error) {
	ids, err := s.IDsForPaths([]string{path}, "", time.Time{})
	if err != nil {
		return "", err
	}
	return ids[0], nil
}

// IDsForPaths is IDForPath for many paths with one write; the result is parallel to paths. provider is
// stored on records that are created here (and filled in on an existing record that has none). at is the start
// time of the scan, stored as FirstSeen on new records; pass the same time to Reconcile. The zero time means now.
// When two paths differ only in case and both exist as different files (a case-sensitive volume), they are two
// records; when the match is the same file under another spelling, the stored spelling becomes the one found.
func (s *Store) IDsForPaths(paths []string, provider Provider, at time.Time) ([]string, error) {
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			return nil, errors.New("projects: empty path")
		}
	}
	if at.IsZero() {
		at = s.now()
	}
	at = at.UTC()
	ids := make([]string, len(paths))
	err := s.Mutate(func(d *Data) error {
		changed := false
		for i, p := range paths {
			if r, respell := d.findPath(p); r != nil {
				ids[i] = r.ID
				if respell {
					r.Path, changed = filepath.Clean(p), true
				}
				if r.Provider == "" && provider != "" {
					r.Provider, changed = provider, true
				}
				continue
			}
			seen := at
			r := RepoRecord{ID: NewID("r_"), LegacyID: LegacyID(p), Path: filepath.Clean(p), Provider: provider, FirstSeen: &seen}
			d.Repos = append(d.Repos, r)
			ids[i] = r.ID
			changed = true
		}
		if !changed {
			return errNoChange
		}
		return nil
	})
	if err != nil && !errors.Is(err, errNoChange) {
		return nil, err
	}
	return ids, nil
}

// IDsForExisting is IDsForPaths that never creates or writes: the result is parallel to paths and holds "" for
// a folder that has no record yet. The engine uses it when projects.json cannot be written.
func (s *Store) IDsForExisting(paths []string) []string {
	d := s.Snapshot()
	ids := make([]string, len(paths))
	for i, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if r, _ := d.findPath(p); r != nil {
			ids[i] = r.ID
		}
	}
	return ids
}

// PathOf returns the current folder of a repository.
func (s *Store) PathOf(id string) (string, bool) {
	d := s.Snapshot()
	if r := d.FindRepo(id); r != nil {
		return r.Path, true
	}
	return "", false
}

// ByLegacyID returns the record whose 0.0.1 path hash is legacy. A hash carried by more than one record is
// ambiguous and reports not found.
func (s *Store) ByLegacyID(legacy string) (RepoRecord, bool) {
	d := s.Snapshot()
	if r := d.FindByLegacyID(legacy); r != nil {
		return *r, true
	}
	return RepoRecord{}, false
}

// Relocate gives the repository a new folder. The ID, project, hidden flag, identity override and everything
// else stay; a "missing" mark is cleared. A folder that another record already uses is refused.
func (s *Store) Relocate(id, newPath string) error {
	if strings.TrimSpace(newPath) == "" {
		return errors.New("projects: empty path")
	}
	return s.Mutate(func(d *Data) error {
		r := d.FindRepo(id)
		if r == nil {
			return fmt.Errorf("%w: repository %s", ErrNotFound, id)
		}
		if o := d.FindByPath(newPath); o != nil && o.ID != id {
			return ErrDuplicate
		}
		r.Path = filepath.Clean(newPath)
		r.MissingSince = nil
		return nil
	})
}

// RepoIdentity is what identifies a repository's history apart from its folder.
type RepoIdentity struct {
	FirstCommit string // smallest root commit hash, "" when there is no commit
	OriginURL   string // URL of the "origin" remote with the user and password removed, "" when there is none
}

// SetIdentities stores the identity of repositories (by ID) in one write. An empty FirstCommit never replaces
// a known one (an unreadable repository must not erase it).
func (s *Store) SetIdentities(m map[string]RepoIdentity) error {
	if len(m) == 0 {
		return nil
	}
	err := s.Mutate(func(d *Data) error {
		changed := false
		for id, ident := range m {
			ident.OriginURL = StripURLUserinfo(ident.OriginURL)
			r := d.FindRepo(id)
			if r == nil {
				continue
			}
			if ident.FirstCommit != "" && r.FirstCommit != ident.FirstCommit {
				r.FirstCommit, changed = ident.FirstCommit, true
			}
			if r.OriginURL != ident.OriginURL && (ident.OriginURL != "" || ident.FirstCommit != "") {
				r.OriginURL, changed = ident.OriginURL, true
			}
		}
		if !changed {
			return errNoChange
		}
		return nil
	})
	if errors.Is(err, errNoChange) {
		return nil
	}
	return err
}

// Presence is what a folder probe found.
type Presence int

const (
	// PresenceUnknown: the probe could not tell (permission denied, I/O error). The record is left as it is.
	PresenceUnknown Presence = iota
	PresencePresent
	PresenceGone
)

// ProbePath reports whether path is a repository folder: only "does not exist" counts as gone; any other
// error (a permission error when macOS privacy access is denied, EIO) is unknown.
func ProbePath(path string) Presence {
	_, err := os.Stat(filepath.Join(path, ".git"))
	switch {
	case err == nil:
		return PresencePresent
	case errors.Is(err, fs.ErrNotExist):
		return PresenceGone
	}
	return PresenceUnknown
}

// Reconcile compares the records with the disk. probe is called, outside any lock, for every record's folder.
// A record whose folder is gone gets MissingSince = at (kept if already set); one whose folder is back loses
// the mark; one missing for more than MissingKeep is deleted; one whose state is unknown is left unchanged.
// at is the scan's start time (zero means now), the same value given to IDsForPaths. It returns the IDs that
// are missing now and the IDs that were deleted.
func (s *Store) Reconcile(probe func(path string) Presence, at time.Time) (missing, purged []string, err error) {
	if at.IsZero() {
		at = s.now()
	}
	now := at.UTC()
	seen := map[string]Presence{}
	for _, r := range s.Snapshot().Repos {
		if _, ok := seen[r.Path]; !ok {
			seen[r.Path] = probe(r.Path)
		}
	}
	err = s.Mutate(func(d *Data) error {
		changed := false
		kept := d.Repos[:0:0]
		missing, purged = nil, nil
		for _, r := range d.Repos {
			switch seen[r.Path] { // a path added since the snapshot is unknown: untouched
			case PresencePresent:
				if r.MissingSince != nil {
					r.MissingSince, changed = nil, true
				}
			case PresenceGone:
				switch {
				case r.MissingSince == nil:
					t := now
					r.MissingSince, changed = &t, true
				case now.Sub(*r.MissingSince) > MissingKeep:
					purged = append(purged, r.ID)
					changed = true
					continue
				}
			}
			if r.MissingSince != nil {
				missing = append(missing, r.ID)
			}
			kept = append(kept, r)
		}
		if len(purged) > 0 {
			gone := map[string]bool{}
			for _, id := range purged {
				gone[id] = true
			}
			for i := range kept {
				var left []string
				for _, id := range kept[i].DeclinedMoves {
					if !gone[id] {
						left = append(left, id)
					}
				}
				kept[i].DeclinedMoves = left
			}
		}
		if !changed {
			return errNoChange
		}
		d.Repos = kept
		return nil
	})
	if errors.Is(err, errNoChange) {
		// Nothing was written; recompute the missing list from the unchanged data.
		d := s.Snapshot()
		missing, purged = nil, nil
		for _, r := range d.Repos {
			if r.MissingSince != nil {
				missing = append(missing, r.ID)
			}
		}
		return missing, nil, nil
	}
	return missing, purged, err
}

// ErrHasData: re-attaching would delete a record that has data of its own.
var ErrHasData = errors.New("projects: the found record already has data of its own")

// hasOwnData reports whether a record carries anything the person set.
func (r *RepoRecord) hasOwnData() bool {
	return r.ProjectID != "" || r.Hidden || r.IdentityOverride != nil || r.Binding != nil || r.AI != nil || len(r.Integrated) > 0
}

// movedFrom is the match rule for MovedFrom and Reattach, applied to d.
func movedFrom(d *Data, id string) (RepoRecord, bool) { return movedMatch(d, id, false) }

// movedMatch is movedFrom; withData true also matches a found record that has data of its own, which Reattach
// needs to refuse it with ErrHasData instead of ErrNotFound.
func movedMatch(d *Data, id string, withData bool) (RepoRecord, bool) {
	cur := d.FindRepo(id)
	if cur == nil || cur.FirstCommit == "" || cur.MissingSince != nil || cur.FirstSeen == nil {
		return RepoRecord{}, false
	}
	// A found record that already has data of its own (project, hidden flag, identity override, ...) is not
	// asked about: answering "yes" would delete that data, so the question goes away instead (FU-18a).
	if !withData && cur.hasOwnData() {
		return RepoRecord{}, false
	}
	var found *RepoRecord
	for i := range d.Repos {
		r := &d.Repos[i]
		if r.ID == id || r.MissingSince == nil || r.FirstCommit != cur.FirstCommit || r.OriginURL != cur.OriginURL {
			continue
		}
		if cur.FirstSeen.Before(*r.MissingSince) { // the folder was already known when the old one vanished
			continue
		}
		declined := false
		for _, x := range cur.DeclinedMoves {
			if x == r.ID {
				declined = true
			}
		}
		if declined {
			continue
		}
		if found != nil {
			return RepoRecord{}, false
		}
		found = r
	}
	if found == nil {
		return RepoRecord{}, false
	}
	return *found, true
}

// MovedFrom looks for the record that repository id (a folder the app has just found) was moved from: a
// record marked missing with the same first commit and origin URL, that vanished no later than the scan that
// first saw this folder, and that the person has not already declined. It answers only when exactly one record
// matches; nothing is changed (the person is asked first, then Reattach or DeclineMove runs).
func (s *Store) MovedFrom(id string) (RepoRecord, bool) {
	d := s.Snapshot()
	return movedFrom(&d, id)
}

// DeclineMove records the answer "no, this folder is not oldID": MovedFrom skips the pair from then on.
func (s *Store) DeclineMove(newID, oldID string) error {
	err := s.Mutate(func(d *Data) error {
		cur := d.FindRepo(newID)
		if cur == nil {
			return fmt.Errorf("%w: repository %s", ErrNotFound, newID)
		}
		for _, x := range cur.DeclinedMoves {
			if x == oldID {
				return errNoChange
			}
		}
		cur.DeclinedMoves = append(cur.DeclinedMoves, oldID)
		return nil
	})
	if errors.Is(err, errNoChange) {
		return nil
	}
	return err
}

// Reattach answers "yes, this is the repository that moved here": the missing record oldID takes the folder of
// the record newID, loses its missing mark, and newID (made when the folder was first seen) is deleted. The
// old record's project, hidden flag, identity override, binding and test result are kept. It is refused unless
// MovedFrom would pick oldID for newID, and refused with ErrHasData when newID has a project, hidden flag,
// identity override, binding, AI defaults or integrated commits of its own (they would be lost).
func (s *Store) Reattach(oldID, newID string) error {
	err := s.Mutate(func(d *Data) error {
		if m, ok := movedMatch(d, newID, true); !ok || m.ID != oldID {
			return fmt.Errorf("%w: no unique moved repository for %s", ErrNotFound, newID)
		}
		old, cur := d.FindRepo(oldID), d.FindRepo(newID)
		if cur.hasOwnData() {
			return ErrHasData
		}
		old.Path = cur.Path
		old.MissingSince = nil
		old.OriginURL = cur.OriginURL
		if old.Provider == "" {
			old.Provider = cur.Provider
		}
		if old.LastTest == nil && cur.LastTest != nil {
			t := *cur.LastTest
			t.RepoID = old.ID
			old.LastTest = &t
		}
		kept := d.Repos[:0:0]
		for _, r := range d.Repos {
			if r.ID != newID {
				kept = append(kept, r)
			}
		}
		d.Repos = kept
		return nil
	})
	return err
}

// LastTests returns the saved connection test of every repository that has one, keyed by repository ID.
func (s *Store) LastTests() map[string]api.TestResult {
	out := map[string]api.TestResult{}
	d := s.Snapshot()
	for _, r := range d.Repos {
		if r.LastTest != nil {
			out[r.ID] = *r.LastTest
		}
	}
	return out
}

// SetLastTest saves the outcome of a connection test on the repository's record. The stderr detail is not
// persisted. An unknown ID is an error.
func (s *Store) SetLastTest(id string, r api.TestResult) error {
	r.RepoID, r.Detail = id, ""
	return s.updateRepo(id, func(rec *RepoRecord) { rec.LastTest = &r })
}

// ImportLegacyTests copies connection test results that 0.0.1 kept in settings.json under the path hash
// (legacy) into the matching records that have no result yet, and returns how many it copied. settings.json is
// only read by the caller and is never written. Running it again copies nothing already copied.
func (s *Store) ImportLegacyTests(legacy map[string]api.TestResult) (int, error) {
	if len(legacy) == 0 {
		return 0, nil
	}
	n := 0
	err := s.Mutate(func(d *Data) error {
		n = 0
		carried := map[string]int{}
		for _, r := range d.Repos {
			carried[r.LegacyID]++
		}
		for i := range d.Repos {
			r := &d.Repos[i]
			t, ok := legacy[r.LegacyID]
			// A hash carried by two records is ambiguous (a relocated folder's old path was reused): skip it.
			if !ok || r.LegacyID == "" || r.LastTest != nil || carried[r.LegacyID] > 1 {
				continue
			}
			t.RepoID, t.Detail = r.ID, ""
			r.LastTest = &t
			n++
		}
		if n == 0 {
			return errNoChange
		}
		return nil
	})
	if errors.Is(err, errNoChange) {
		return 0, nil
	}
	return n, err
}
