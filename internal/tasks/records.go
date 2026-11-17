package tasks

import (
	"errors"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/rudian/codepass/internal/store"
)

// SchemaVersion is the version of tasks.json this app writes.
const SchemaVersion = 1

// File names under the data folder.
const tasksFile = "tasks.json"

// Persistent status of a record. A record that was "live" when the app went away is "stopped" at the next start.
const (
	statusLive    = "live"
	statusDone    = "done"
	statusFailed  = "failed"
	statusStopped = "stopped"
)

// record is one task in tasks.json. It never holds the description (it is the person's text and goes only to
// the tool), a command line, an environment or any terminal text.
type record struct {
	ID         string    `json:"id"`
	Repo       string    `json:"repo"` // the repository's stable ID (F1-18)
	Tool       string    `json:"tool"`
	Title      string    `json:"title"`
	Slug       string    `json:"slug"`
	NamedBy    string    `json:"namedBy"` // ai | fallback | pending | manual
	Number     int       `json:"number"`
	Where      string    `json:"where"`
	Dir        string    `json:"dir"`
	SessionIDs []string  `json:"sessionIds"` // newest last; every ID the task has had (plan 4.8a)
	Status     string    `json:"status"`
	ExitCode   *int      `json:"exitCode,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	StartedAt  time.Time `json:"startedAt"`
	EndedAt    time.Time `json:"endedAt,omitempty"`
	Resumed    bool      `json:"resumed,omitempty"`
}

type fileData struct {
	SchemaVersion int            `json:"schemaVersion"`
	Tasks         []record       `json:"tasks"`
	Counters      map[string]int `json:"counters"` // repository ID -> the last "New task <n>"
	// StillRunningNoticed: the "Codepass is still running" notification was sent once (design 4.4: once ever).
	StillRunningNoticed bool `json:"stillRunningNoticed,omitempty"`
}

// recStore is tasks.json. A damaged file is moved aside and replaced by an empty one; a file written by a newer
// app is left alone and this run keeps its records in memory only.
type recStore struct {
	f   *store.File // nil: memory only
	log *slog.Logger
	bad bool // a save failure was already logged
}

func openRecords(dataDir string, now func() time.Time, log *slog.Logger) (*recStore, fileData) {
	d := fileData{SchemaVersion: SchemaVersion, Counters: map[string]int{}}
	rs := &recStore{log: log}
	if dataDir == "" {
		return rs, d
	}
	f, err := store.Open(filepath.Join(dataDir, tasksFile), store.Spec{Current: SchemaVersion, Now: now})
	if err != nil {
		log.Warn("tasks.json cannot be opened; the tasks stay in memory", "err", err)
		return rs, d
	}
	f.CleanTemp()
	var loaded fileData
	_, err = f.Load(&loaded)
	if errors.Is(err, store.ErrCorrupt) {
		moved, qerr := f.Quarantine(&fileData{SchemaVersion: SchemaVersion})
		if qerr != nil {
			log.Warn("tasks.json is damaged and cannot be moved aside; the tasks stay in memory", "err", qerr)
			return rs, d
		}
		log.Warn("tasks.json was damaged; moved aside and starting a new one", "movedTo", moved)
		loaded, err = fileData{}, nil
	}
	if err != nil {
		log.Warn("tasks.json cannot be used; the tasks stay in memory", "err", err)
		return rs, d
	}
	if loaded.Counters == nil {
		loaded.Counters = map[string]int{}
	}
	loaded.SchemaVersion = SchemaVersion
	rs.f = f
	return rs, loaded
}

func (r *recStore) save(d fileData) {
	if r.f == nil {
		return
	}
	d.SchemaVersion = SchemaVersion
	if err := r.f.Save(&d); err != nil && !r.bad {
		r.bad = true
		r.log.Warn("tasks.json cannot be written", "err", err)
	}
}
