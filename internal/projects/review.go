package projects

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// This file holds what the repository engine needs for "newly found" repositories, hiding, projects and
// dismissed suggestions (F1-11). Every write goes through Store.Mutate like the rest of the registry.

// IDsForFound is IDsForPaths for a scan: it also reports which records it created, makes the new ones pending
// when pendingNew is set (they wait for the person to show or hide them), and fixes a cached provider that no
// longer matches the remote. at is the scan's start time. created is parallel to paths.
func (s *Store) IDsForFound(paths []string, provider Provider, at time.Time, pendingNew bool) (ids []string, created []bool, err error) {
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			return nil, nil, errors.New("projects: empty path")
		}
	}
	if at.IsZero() {
		at = s.now()
	}
	at = at.UTC()
	ids = make([]string, len(paths))
	created = make([]bool, len(paths))
	err = s.Mutate(func(d *Data) error {
		changed := false
		for i, p := range paths {
			created[i] = false
			if r, respell := d.findPath(p); r != nil {
				ids[i] = r.ID
				if respell {
					r.Path, changed = filepath.Clean(p), true
				}
				if provider != "" && r.Provider != provider {
					r.Provider, changed = provider, true
				}
				continue
			}
			seen := at
			r := RepoRecord{ID: NewID("r_"), LegacyID: LegacyID(p), Path: filepath.Clean(p), Provider: provider, FirstSeen: &seen, Pending: pendingNew}
			d.Repos = append(d.Repos, r)
			ids[i], created[i] = r.ID, true
			changed = true
		}
		if !changed {
			return errNoChange
		}
		return nil
	})
	if err != nil && !errors.Is(err, errNoChange) {
		return nil, nil, err
	}
	return ids, created, nil
}

// ResolvePending answers the one-time review: the repositories in show join the list, the ones in hide are
// hidden (they can be unhidden later). Both leave the pending set. An unknown ID is skipped.
func (s *Store) ResolvePending(show, hide []string) error {
	err := s.Mutate(func(d *Data) error {
		changed := false
		for _, id := range show {
			if r := d.FindRepo(id); r != nil && r.Pending {
				r.Pending, changed = false, true
			}
		}
		for _, id := range hide {
			if r := d.FindRepo(id); r != nil && (r.Pending || !r.Hidden) {
				r.Pending, r.Hidden, changed = false, true, true
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

// RenameProject changes a project's name.
func (s *Store) RenameProject(id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("projects: empty name")
	}
	return s.Mutate(func(d *Data) error {
		p := d.FindProject(id)
		if p == nil {
			return fmt.Errorf("%w: project %s", ErrNotFound, id)
		}
		p.Name = name
		return nil
	})
}

// RemoveProject deletes a project; its repositories become ungrouped. Nothing on disk is touched.
func (s *Store) RemoveProject(id string) error {
	return s.Mutate(func(d *Data) error {
		if d.FindProject(id) == nil {
			return fmt.Errorf("%w: project %s", ErrNotFound, id)
		}
		kept := d.Projects[:0:0]
		for _, p := range d.Projects {
			if p.ID != id {
				kept = append(kept, p)
			}
		}
		d.Projects = kept
		for i := range d.Repos {
			if d.Repos[i].ProjectID == id {
				d.Repos[i].ProjectID = ""
			}
		}
		return nil
	})
}

// SetProjectCollapsed remembers whether a project section is folded.
func (s *Store) SetProjectCollapsed(id string, collapsed bool) error {
	err := s.Mutate(func(d *Data) error {
		p := d.FindProject(id)
		if p == nil {
			return fmt.Errorf("%w: project %s", ErrNotFound, id)
		}
		if p.Collapsed == collapsed {
			return errNoChange
		}
		p.Collapsed = collapsed
		return nil
	})
	if errors.Is(err, errNoChange) {
		return nil
	}
	return err
}

// DismissSuggestion remembers that the person said no to a project suggestion; key is "prefix:<p>" or
// "folder:<path>".
func (s *Store) DismissSuggestion(key string) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("projects: empty suggestion key")
	}
	err := s.Mutate(func(d *Data) error {
		for _, k := range d.DismissedSuggestions {
			if k == key {
				return errNoChange
			}
		}
		d.DismissedSuggestions = append(d.DismissedSuggestions, key)
		return nil
	})
	if errors.Is(err, errNoChange) {
		return nil
	}
	return err
}
