package projects

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rudian/codepass/internal/store"
)

var (
	ErrNotFound  = errors.New("projects: not found")
	ErrDuplicate = errors.New("projects: path already registered")
)

// LegacyID is the 0.0.1 repository ID: the first 16 hex digits of SHA-1 over the cleaned path. It exists only
// to match old data; new code must use RepoRecord.ID.
func LegacyID(path string) string {
	sum := sha1.Sum([]byte(filepath.Clean(path)))
	return hex.EncodeToString(sum[:])[:16]
}

// NewID returns a random 128-bit ID with a short prefix ("r_", "p_").
func NewID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand does not fail on supported platforms
	}
	return prefix + hex.EncodeToString(b[:])
}

// Store is projects.json. Safe for concurrent use.
type Store struct {
	mu  sync.Mutex
	f   *store.File
	d   Data
	now func() time.Time
}

// Open reads projects.json (an absent file is an empty store, nothing is created until the first write).
// store.ErrNewerVersion and store.ErrCorrupt are returned as is and the file is left untouched.
func Open(path string) (*Store, error) {
	return open(path, time.Now)
}

func open(path string, now func() time.Time) (*Store, error) {
	f, err := store.Open(path, store.Spec{Current: SchemaVersion, Now: now})
	if err != nil {
		return nil, err
	}
	s := &Store{f: f, now: now}
	f.CleanTemp()
	if _, err := f.Load(&s.d); err != nil {
		return nil, err
	}
	s.d.SchemaVersion = SchemaVersion
	return s, nil
}

// Quarantine moves a damaged projects.json (unusable header, or a body that does not decode) aside to
// "<name>.corrupt-<unix>" and returns the new path, so Open works again with an empty store.
func Quarantine(path string) (string, error) {
	f, err := store.Open(path, store.Spec{Current: SchemaVersion})
	if err != nil {
		return "", err
	}
	return f.Quarantine(&Data{})
}

// Snapshot returns a deep copy of the current data.
func (s *Store) Snapshot() Data {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clone(s.d)
}

// Mutate runs fn on the data and saves if fn returns nil. On any error the in-memory data is rolled back.
// The file is re-read under a per-path lock shared by every Store on the path in this process, so writers
// do not lose each other's changes. Another process is kept out by the single-instance guard, not here.
func (s *Store) Mutate(fn func(d *Data) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil { // in-memory store (OpenMemory): nothing is read or written
		next := clone(s.d)
		next.SchemaVersion = SchemaVersion
		if err := fn(&next); err != nil {
			return err
		}
		s.d = next
		return nil
	}
	next := Data{}
	err := s.f.Update(&next, func() error {
		next.SchemaVersion = SchemaVersion
		return fn(&next)
	})
	if err != nil {
		return err
	}
	s.d = next
	return nil
}

func clone(d Data) Data {
	out := Data{SchemaVersion: d.SchemaVersion}
	if len(d.DismissedSuggestions) > 0 {
		out.DismissedSuggestions = append([]string(nil), d.DismissedSuggestions...)
	}
	if len(d.ImportedLegacy) > 0 {
		out.ImportedLegacy = append([]string(nil), d.ImportedLegacy...)
	}
	out.Projects = append([]Project(nil), d.Projects...)
	for i := range out.Projects {
		if p := d.Projects[i].Identity; p != nil {
			c := *p
			out.Projects[i].Identity = &c
		}
	}
	for _, r := range d.Repos {
		c := r
		if r.LastTest != nil {
			t := *r.LastTest
			c.LastTest = &t
		}
		if r.Binding != nil {
			b := *r.Binding
			c.Binding = &b
		}
		if r.IdentityOverride != nil {
			i := *r.IdentityOverride
			c.IdentityOverride = &i
		}
		if r.AI != nil {
			a := *r.AI
			c.AI = &a
		}
		if r.MissingSince != nil {
			t := *r.MissingSince
			c.MissingSince = &t
		}
		if r.FirstSeen != nil {
			t := *r.FirstSeen
			c.FirstSeen = &t
		}
		c.DeclinedMoves = append([]string(nil), r.DeclinedMoves...)
		if len(c.DeclinedMoves) == 0 {
			c.DeclinedMoves = nil
		}
		if r.Integrated != nil {
			c.Integrated = make(map[string]string, len(r.Integrated))
			for k, v := range r.Integrated {
				c.Integrated[k] = v
			}
		}
		out.Repos = append(out.Repos, c)
	}
	return out
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if a == b {
		return true
	}
	return pathFold && strings.EqualFold(a, b)
}

// FindByPath returns the record for a folder.
func (d *Data) FindByPath(path string) *RepoRecord {
	for i := range d.Repos {
		if samePath(d.Repos[i].Path, path) {
			return &d.Repos[i]
		}
	}
	return nil
}

// findPath is FindByPath for a folder that is being registered: an exact match wins; on macOS and Windows a
// match that differs only in case is accepted unless both folders exist and are different files (a
// case-sensitive volume holding two repositories). It never writes; the second result is true when the match
// came from folding and the stored spelling differs from path.
func (d *Data) findPath(path string) (*RepoRecord, bool) {
	clean := filepath.Clean(path)
	for i := range d.Repos {
		if d.Repos[i].Path == clean {
			return &d.Repos[i], false
		}
	}
	if !pathFold {
		return nil, false
	}
	for i := range d.Repos {
		r := &d.Repos[i]
		if !strings.EqualFold(r.Path, clean) {
			continue
		}
		a, errA := os.Stat(r.Path)
		b, errB := os.Stat(clean)
		if errA == nil && errB == nil {
			if !os.SameFile(a, b) {
				continue
			}
			return r, true
		}
		if errA != nil && errB == nil {
			// a case-only rename on a case-sensitive volume: the stored spelling is gone and the other one is
			// there, so it is the same folder under its new name (the record keeps its data)
			return r, true
		}
		return r, false
	}
	return nil, false
}

// FindRepo returns the record with this ID.
func (d *Data) FindRepo(id string) *RepoRecord {
	for i := range d.Repos {
		if d.Repos[i].ID == id {
			return &d.Repos[i]
		}
	}
	return nil
}

// FindByLegacyID returns the record whose 0.0.1 ID matches. A hash that more than one record carries (a folder
// that was relocated and whose old path was then used by another repository) is ambiguous: nothing is returned.
func (d *Data) FindByLegacyID(legacy string) *RepoRecord {
	if legacy == "" {
		return nil
	}
	var found *RepoRecord
	for i := range d.Repos {
		if d.Repos[i].LegacyID == legacy {
			if found != nil {
				return nil
			}
			found = &d.Repos[i]
		}
	}
	return found
}

// FindProject returns the project with this ID.
func (d *Data) FindProject(id string) *Project {
	for i := range d.Projects {
		if d.Projects[i].ID == id {
			return &d.Projects[i]
		}
	}
	return nil
}

// AddProject appends a project with a fresh ID and the next order number.
func (s *Store) AddProject(name string) (Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, errors.New("projects: empty name")
	}
	var p Project
	err := s.Mutate(func(d *Data) error {
		order := 0
		for _, q := range d.Projects {
			if q.Order >= order {
				order = q.Order + 1
			}
		}
		p = Project{ID: NewID("p_"), Name: name, Order: order, CreatedAt: s.now().UTC().Format(time.RFC3339)}
		d.Projects = append(d.Projects, p)
		return nil
	})
	return p, err
}

// EnsureRepo returns the record for a path, creating it (random ID, LegacyID from the path) when new.
func (s *Store) EnsureRepo(path string, provider Provider) (RepoRecord, bool, error) {
	if strings.TrimSpace(path) == "" {
		return RepoRecord{}, false, errors.New("projects: empty path")
	}
	var out RepoRecord
	created := false
	err := s.Mutate(func(d *Data) error {
		if r, _ := d.findPath(path); r != nil {
			out = *r
			return errNoChange
		}
		at := s.now().UTC()
		r := RepoRecord{ID: NewID("r_"), LegacyID: LegacyID(path), Path: filepath.Clean(path), Provider: provider, FirstSeen: &at}
		d.Repos = append(d.Repos, r)
		out, created = r, true
		return nil
	})
	if errors.Is(err, errNoChange) {
		return out, false, nil
	}
	return out, created, err
}

var errNoChange = errors.New("no change")

// SetRepoPath records a new folder for the same repository (rename or move). It is Relocate: the ID never
// changes and a "missing" mark is cleared.
func (s *Store) SetRepoPath(id, newPath string) error { return s.Relocate(id, newPath) }

// SetHidden sets or clears the hidden flag.
// A flag that already has the value is not written: a caller that only wants "shown" must not fail because
// projects.json cannot be written.
func (s *Store) SetHidden(id string, hidden bool) error {
	err := s.Mutate(func(d *Data) error {
		r := d.FindRepo(id)
		if r == nil {
			return fmt.Errorf("%w: repository %s", ErrNotFound, id)
		}
		if r.Hidden == hidden {
			return errNoChange
		}
		r.Hidden = hidden
		return nil
	})
	if errors.Is(err, errNoChange) {
		return nil
	}
	return err
}

// SetProject moves a repository into a project ("" removes it from any project).
func (s *Store) SetProject(repoID, projectID string) error {
	return s.Mutate(func(d *Data) error {
		r := d.FindRepo(repoID)
		if r == nil {
			return fmt.Errorf("%w: repository %s", ErrNotFound, repoID)
		}
		if projectID != "" && d.FindProject(projectID) == nil {
			return fmt.Errorf("%w: project %s", ErrNotFound, projectID)
		}
		r.ProjectID = projectID
		return nil
	})
}

// SetIntegrated records the last remote commit the user integrated for "<remote>/<branch>".
func (s *Store) SetIntegrated(id, remoteBranch, sha string) error {
	return s.updateRepo(id, func(r *RepoRecord) {
		if r.Integrated == nil {
			r.Integrated = map[string]string{}
		}
		r.Integrated[remoteBranch] = sha
	})
}

func (s *Store) updateRepo(id string, fn func(*RepoRecord)) error {
	return s.Mutate(func(d *Data) error {
		r := d.FindRepo(id)
		if r == nil {
			return fmt.Errorf("%w: repository %s", ErrNotFound, id)
		}
		fn(r)
		return nil
	})
}
