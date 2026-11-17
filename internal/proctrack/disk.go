package proctrack

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rudian/codepass/internal/store"
)

// leftovers.json (plan 4.8b "After a crash"): the tracked set of every session, so that after a crash the
// next start can list what survived. PID, start time and command NAME only: never a command line.

const leftoversFile = "leftovers.json"

type diskProc struct {
	PID   int    `json:"pid"`
	Start int64  `json:"start"`
	Name  string `json:"name"`
}

type diskSession struct {
	ID       string     `json:"id"`
	ClosedAt string     `json:"closedAt,omitempty"` // RFC 3339; empty while the session is live
	Procs    []diskProc `json:"procs"`
}

type diskDoc struct {
	Sessions []diskSession `json:"sessions"`
}

type diskFile struct {
	f        *store.File
	disabled bool // the file was written by a newer app: leave it alone
}

func openDisk(dataDir string) *diskFile {
	if dataDir == "" {
		return nil
	}
	dir := filepath.Join(dataDir, "sessions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil
	}
	f, err := store.Open(filepath.Join(dir, leftoversFile), store.Spec{Current: 1})
	if err != nil {
		return nil
	}
	return &diskFile{f: f}
}

// snapshotLocked is the persisted form of every non-empty set. The caller holds t.mu.
func (t *Tracker) snapshotLocked() diskDoc {
	var d diskDoc
	for _, s := range t.sess {
		if len(s.entries) == 0 {
			continue
		}
		ds := diskSession{ID: s.id}
		if !s.live() {
			ds.ClosedAt = s.closedAt.UTC().Format(time.RFC3339)
		}
		for _, e := range s.entries {
			ds.Procs = append(ds.Procs, diskProc{PID: e.PID, Start: e.Start, Name: e.Name})
		}
		sort.Slice(ds.Procs, func(i, j int) bool { return ds.Procs[i].PID < ds.Procs[j].PID })
		d.Sessions = append(d.Sessions, ds)
	}
	sort.Slice(d.Sessions, func(i, j int) bool { return d.Sessions[i].ID < d.Sessions[j].ID })
	return d
}

func signature(d diskDoc) string {
	var b strings.Builder
	for _, s := range d.Sessions {
		fmt.Fprintf(&b, "%s|%s", s.ID, s.ClosedAt)
		for _, p := range s.Procs {
			fmt.Fprintf(&b, "|%d@%d:%s", p.PID, p.Start, p.Name)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// persist writes the file when the set changed since the last write. A write error is not fatal: the
// tracker works from memory and tries again at the next change.
func (t *Tracker) persist(d diskDoc) {
	if t.disk == nil || t.disk.disabled {
		return
	}
	sig := signature(d)
	if sig == t.saved {
		return
	}
	if d.Sessions == nil {
		d.Sessions = []diskSession{}
	}
	if err := t.disk.f.Save(&d); err == nil {
		t.saved = sig
	}
}

// Restore reads leftovers.json at start and adopts every program that is still alive with the same PID and
// start time as a closed session. It returns how many it adopted. Call it once, before Run. A damaged file
// is set aside; a file of a newer version is left alone.
func (t *Tracker) Restore() (int, error) {
	if t.disk == nil {
		return 0, nil
	}
	t.tickMu.Lock()
	n, err := t.restoreLocked()
	t.tickMu.Unlock()
	if n > 0 {
		t.changed() // after the lock: the callback may call back into the tracker
	}
	return n, err
}

func (t *Tracker) restoreLocked() (int, error) {
	var d diskDoc
	found, err := t.disk.f.Load(&d)
	switch {
	case errors.Is(err, store.ErrNewerVersion):
		t.disk.disabled = true
		return 0, err
	case errors.Is(err, store.ErrCorrupt):
		_, _ = t.disk.f.Quarantine(&d)
		return 0, nil
	case err != nil:
		return 0, err
	case !found:
		return 0, nil
	}
	var want []int
	for _, s := range d.Sessions {
		for _, p := range s.Procs {
			want = append(want, p.PID)
		}
	}
	if len(want) == 0 {
		t.persist(diskDoc{})
		return 0, nil
	}
	list, err := t.o.Source.Read(want)
	if err != nil {
		return 0, err
	}
	table := make(map[int]Proc, len(list))
	for _, p := range list {
		table[p.PID] = p
	}
	adopted := 0
	t.mu.Lock()
	now := t.o.Now()
	for _, ds := range d.Sessions {
		if t.sess[ds.ID] != nil {
			continue
		}
		s := &session{id: ds.ID, entries: map[int]*entry{}, closedAt: now}
		if c, err := time.Parse(time.RFC3339, ds.ClosedAt); err == nil {
			s.closedAt = c
		}
		for _, p := range ds.Procs {
			cur, ok := table[p.PID]
			if !ok || cur.Start != p.Start || p.PID <= 1 || p.PID == os.Getpid() {
				continue
			}
			s.entries[p.PID] = &entry{Process: Process{PID: p.PID, Start: p.Start, Name: nameOf(p.Name), PPID: cur.PPID, CPU: cur.CPU, RSS: cur.RSS}, pgid: cur.PGID}
			adopted++
		}
		if len(s.entries) > 0 {
			t.sess[s.id] = s
		}
	}
	snap := t.snapshotLocked()
	t.mu.Unlock()
	t.persist(snap)
	return adopted, nil
}
