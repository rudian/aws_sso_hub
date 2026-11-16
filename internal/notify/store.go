package notify

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/rudian/codepass/internal/store"
)

// SchemaVersion is the version of notifications.json this app writes.
const SchemaVersion = 1

// fileData is notifications.json (plan 4.15 "Storage"). It holds whether the app asked the system for
// permission and, per portal and kind, the key of the last notice sent. No text of any notification and no
// secret: a key is the portal's sign-in time.
type fileData struct {
	SchemaVersion int                          `json:"schemaVersion"`
	Asked         bool                         `json:"asked"`
	Sent          map[string]map[string]string `json:"sent"` // portal -> kind -> key
}

// Store is notifications.json. A damaged file is moved aside and replaced; a file written by a newer app is
// left alone and this run keeps its records in memory. Whatever happens, the store never returns an error to
// its callers: a broken notifications.json must not stop the app or the sign-in loop.
type Store struct {
	mu   sync.Mutex
	f    *store.File // nil: memory only
	d    fileData
	log  *slog.Logger
	warn bool // a save failure was already logged
}

// OpenStore opens (or starts) notifications.json at path. path "" gives a store that lives in memory only.
func OpenStore(path string, now func() time.Time, log *slog.Logger) *Store {
	if log == nil {
		log = slog.New(slog.NewTextHandler(discard{}, nil))
	}
	s := &Store{log: log, d: fileData{Sent: map[string]map[string]string{}}}
	if path == "" {
		return s
	}
	f, err := store.Open(path, store.Spec{Current: SchemaVersion, Now: now})
	if err != nil {
		log.Warn("notifications.json cannot be opened; records stay in memory", "err", err)
		return s
	}
	f.CleanTemp()
	var d fileData
	_, err = f.Load(&d)
	if errors.Is(err, store.ErrCorrupt) {
		moved, qerr := f.Quarantine(&fileData{})
		if qerr != nil {
			log.Warn("notifications.json is damaged and cannot be moved aside; records stay in memory", "err", qerr)
			return s
		}
		log.Warn("notifications.json was damaged; moved aside and starting a new one", "movedTo", moved)
		d = fileData{}
		err = nil
	}
	if err != nil {
		// newer version, unreadable: never written to, never replaced
		log.Warn("notifications.json cannot be used; records stay in memory", "err", err)
		return s
	}
	if d.Sent == nil {
		d.Sent = map[string]map[string]string{}
	}
	s.d, s.f = d, f
	return s
}

type discard struct{}

func (discard) Write(b []byte) (int, error) { return len(b), nil }

// save writes the file; failures are logged once and otherwise ignored. Caller holds s.mu.
func (s *Store) save() {
	if s.f == nil {
		return
	}
	if err := s.f.Save(&s.d); err != nil && !s.warn {
		s.warn = true
		s.log.Warn("notifications.json cannot be written", "err", err)
	}
}

// Asked reports whether the app has already asked the system for permission to notify.
func (s *Store) Asked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.d.Asked
}

// SetAsked records that the permission question was put to the system.
func (s *Store) SetAsked() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.d.Asked {
		return
	}
	s.d.Asked = true
	s.save()
}

// Last returns the key of the last notice of that kind sent for the portal ("" = none).
func (s *Store) Last(portal, kind string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.d.Sent[portal][kind]
}

// Mark records the key of the notice just sent (or deliberately skipped) for the portal.
func (s *Store) Mark(portal, kind, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.d.Sent[portal][kind] == key {
		return
	}
	if s.d.Sent[portal] == nil {
		s.d.Sent[portal] = map[string]string{}
	}
	s.d.Sent[portal][kind] = key
	s.save()
}

// Reset forgets the portal's records (a successful sign-in or a sign-out starts a new life of the portal).
func (s *Store) Reset(portal string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.d.Sent[portal]; !ok {
		return
	}
	delete(s.d.Sent, portal)
	s.save()
}
