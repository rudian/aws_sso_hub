// Package store keeps one versioned JSON data file (projects.json, accounts.json, tasks.json).
//
// Every file carries a top-level "schemaVersion". Opening an older file migrates it forward in memory;
// the first write after a migration first copies the original bytes to a backup next to the file. A file
// whose version is newer than the app understands is reported (ErrNewerVersion) and never overwritten.
// Writes are atomic: temp file in the same folder, fsync, rename. The store never touches settings.json.
package store

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	// ErrNewerVersion: the file was written by a newer app. Reads and writes are refused.
	ErrNewerVersion = errors.New("data file was written by a newer version of the app")
	// ErrCorrupt: the file is not valid JSON or has no usable schemaVersion. Writes are refused until Quarantine.
	ErrCorrupt = errors.New("data file is damaged")
)

// NewerVersionError carries the versions involved. errors.Is(err, ErrNewerVersion) is true.
type NewerVersionError struct {
	Path        string
	FileVersion int
	AppVersion  int
}

func (e *NewerVersionError) Error() string {
	return fmt.Sprintf("%s has schema version %d, this app understands up to %d", filepath.Base(e.Path), e.FileVersion, e.AppVersion)
}
func (e *NewerVersionError) Is(target error) bool { return target == ErrNewerVersion }

// Migration upgrades a decoded document from version N to N+1 in place (the map key in Spec.Migrations is N).
type Migration func(doc map[string]any) error

// Spec describes a data file.
type Spec struct {
	Current    int               // the version this app writes (>= 1)
	Migrations map[int]Migration // from-version -> step; must cover every version below Current that can exist on disk
	// Now is optional (tests); used for backup and quarantine names.
	Now func() time.Time
}

// File is one data file. Safe for concurrent use inside a process.
type File struct {
	path string
	spec Spec

	mu         *sync.Mutex // shared by every File opened on the same path in this process
	corrupt    bool        // the last Load found a body that does not decode; Save is refused until Quarantine
	origBytes  []byte      // bytes of an older-version file as read, until the pre-migration backup is made
	origVer    int
	beforeMove func() // test hook: runs after the temp file is written, before the rename
}

// Open prepares a file. It does not read or create anything.
func Open(path string, spec Spec) (*File, error) {
	if spec.Current < 1 {
		return nil, errors.New("store: Spec.Current must be >= 1")
	}
	for v := 1; v < spec.Current; v++ {
		if spec.Migrations[v] == nil {
			return nil, fmt.Errorf("store: no migration from version %d", v)
		}
	}
	if spec.Now == nil {
		spec.Now = time.Now
	}
	return &File{path: path, spec: spec, mu: pathLock(path)}, nil
}

var (
	locksMu sync.Mutex
	locks   = map[string]*sync.Mutex{}
)

// pathLock returns the one mutex for a path, so two File objects on the same path never interleave.
func pathLock(path string) *sync.Mutex {
	key := path
	if abs, err := filepath.Abs(path); err == nil {
		key = abs
	}
	key = filepath.Clean(key)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		key = strings.ToLower(key)
	}
	locksMu.Lock()
	defer locksMu.Unlock()
	m := locks[key]
	if m == nil {
		m = &sync.Mutex{}
		locks[key] = m
	}
	return m
}

// Path returns the file path.
func (f *File) Path() string { return f.path }

type header struct {
	SchemaVersion *int `json:"schemaVersion"`
}

// Load reads the file into v (a pointer to a struct). found is false when the file does not exist (v is untouched).
//
// v must be a fresh (zero) value: the file is decoded over whatever v holds, so a field that the file leaves
// out keeps the old value (a map or slice is merged into, not replaced). Callers that want defaults for a
// missing field set them after Load, or decode into a new value each time.
func (f *File) Load(v any) (found bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.loadLocked(v)
}

func (f *File) loadLocked(v any) (bool, error) {
	ok, err := f.loadInner(v)
	f.corrupt = errors.Is(err, ErrCorrupt)
	return ok, err
}

func (f *File) loadInner(v any) (bool, error) {
	b, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		f.origBytes, f.origVer = nil, 0
		return false, nil
	}
	if err != nil {
		return false, err
	}
	ver, err := f.version(b)
	if err != nil {
		return false, err
	}
	if ver == f.spec.Current {
		f.origBytes, f.origVer = nil, 0
		if err := json.Unmarshal(b, v); err != nil {
			return false, fmt.Errorf("%w: %v", ErrCorrupt, err)
		}
		return true, nil
	}
	var doc map[string]any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return false, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	for from := ver; from < f.spec.Current; from++ {
		if err := f.spec.Migrations[from](doc); err != nil {
			return false, fmt.Errorf("store: migrate %s from version %d: %w", filepath.Base(f.path), from, err)
		}
		doc["schemaVersion"] = from + 1
	}
	mb, err := json.Marshal(doc)
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(mb, v); err != nil {
		return false, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	f.origBytes, f.origVer = b, ver
	return true, nil
}

// version reads schemaVersion and applies the newer / corrupt rules.
func (f *File) version(b []byte) (int, error) {
	var h header
	if err := json.Unmarshal(b, &h); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if h.SchemaVersion == nil || *h.SchemaVersion < 1 {
		return 0, fmt.Errorf("%w: missing schemaVersion", ErrCorrupt)
	}
	if *h.SchemaVersion > f.spec.Current {
		return 0, &NewerVersionError{Path: f.path, FileVersion: *h.SchemaVersion, AppVersion: f.spec.Current}
	}
	return *h.SchemaVersion, nil
}

// Save writes v (a struct or pointer to one) atomically with schemaVersion set to Spec.Current. If the file on
// disk is newer than the app or damaged, nothing is written. The first save after a migration backs up the
// original bytes to "<name>.v<N>.bak" (kept; never overwritten).
func (f *File) Save(v any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.saveLocked(v)
}

func (f *File) saveLocked(v any) error {
	if f.corrupt {
		return fmt.Errorf("%w: refusing to overwrite", ErrCorrupt)
	}
	cur, err := os.ReadFile(f.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		f.origBytes, f.origVer = nil, 0
	case err != nil:
		return err
	default:
		ver, verr := f.version(cur)
		if verr != nil {
			return verr
		}
		if ver < f.spec.Current && f.origBytes == nil {
			// Older file we have not loaded (or changed under us): back up what is on disk now.
			f.origBytes, f.origVer = cur, ver
		}
	}
	body, err := f.encode(v)
	if err != nil {
		return err
	}
	if f.origBytes != nil {
		if err := f.backupLocked(); err != nil {
			return fmt.Errorf("store: backup before migration: %w", err)
		}
	}
	if err := f.writeAtomic(body); err != nil {
		return err
	}
	f.origBytes, f.origVer = nil, 0
	f.corrupt = false
	return nil
}

func (f *File) encode(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("store: value must encode to a JSON object: %w", err)
	}
	m["schemaVersion"] = json.RawMessage(fmt.Sprint(f.spec.Current))
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// BackupPath is where the pre-migration copy of a version-n file goes.
func (f *File) BackupPath(n int) string { return fmt.Sprintf("%s.v%d.bak", f.path, n) }

func (f *File) backupLocked() error {
	p := f.BackupPath(f.origVer)
	if _, err := os.Stat(p); err == nil {
		return nil // an earlier backup of that version exists; keep the oldest
	}
	return f.writeAtomicTo(p, f.origBytes, nil)
}

// Update loads the file into v (a non-nil pointer), runs fn, and saves v if fn returns nil, all under the
// store lock, so concurrent read-modify-write calls do not lose updates. v is reset to its zero value first,
// so after loading it holds exactly the file's content, or the zero value when there is no file; a value
// reused across calls never carries stale fields or map keys.
func (f *File) Update(v any, fn func() error) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return errors.New("store: Update needs a non-nil pointer")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	rv.Elem().SetZero()
	if _, err := f.loadLocked(v); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	return f.saveLocked(v)
}

// maxQuarantineSuffix bounds the "-1", "-2", ... suffixes Quarantine tries for a name that is taken.
const maxQuarantineSuffix = 1000

// Quarantine renames a damaged file to "<name>.corrupt-<unix>" (with -1, -2, ... when that name is taken, so
// an earlier quarantined copy is never replaced) so a fresh file can be started. v is a pointer to the type the
// file decodes into; damage is decided under the lock by loading into a fresh value of that type: the header
// is unusable or the body does not decode. It refuses a healthy or newer-version file.
func (f *File) Quarantine(v any) (string, error) {
	t := reflect.TypeOf(v)
	if t == nil || t.Kind() != reflect.Pointer {
		return "", errors.New("store: Quarantine needs a pointer to the target type")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, err := os.Stat(f.path); err != nil {
		return "", err
	}
	_, lerr := f.loadLocked(reflect.New(t.Elem()).Interface())
	if !errors.Is(lerr, ErrCorrupt) {
		return "", errors.New("store: file is not damaged")
	}
	base := fmt.Sprintf("%s.corrupt-%d", f.path, f.spec.Now().Unix())
	dst := base
	for i := 1; ; i++ {
		_, err := os.Lstat(dst)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return "", err // a name that cannot even be looked at: stop, never loop on it
		}
		if i > maxQuarantineSuffix {
			return "", fmt.Errorf("store: %d quarantined copies of %s already exist", maxQuarantineSuffix, filepath.Base(f.path))
		}
		dst = fmt.Sprintf("%s-%d", base, i)
	}
	if err := renameReplace(f.path, dst); err != nil {
		return "", err
	}
	f.corrupt = false
	f.origBytes, f.origVer = nil, 0
	return dst, nil
}

func (f *File) writeAtomic(body []byte) error {
	return f.writeAtomicTo(f.path, body, f.beforeMove)
}

func (f *File) writeAtomicTo(path string, body []byte, beforeMove func()) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	var r [6]byte
	_, _ = rand.Read(r[:])
	tmp := filepath.Join(dir, "."+filepath.Base(path)+".tmp-"+hex.EncodeToString(r[:]))
	tf, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	fail := func(err error) error { _ = tf.Close(); _ = os.Remove(tmp); return err }
	if _, err := tf.Write(body); err != nil {
		return fail(err)
	}
	if err := tf.Sync(); err != nil {
		return fail(err)
	}
	if err := tf.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if beforeMove != nil {
		beforeMove()
	}
	if err := renameReplace(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	syncDir(dir)
	return nil
}

var (
	osRename       = os.Rename // replaced in tests
	renameAttempts = defaultRenameAttempts()
)

func defaultRenameAttempts() int {
	if runtime.GOOS == "windows" {
		return 20
	}
	return 1
}

// renameReplace renames over an existing file. On Windows the rename fails with a sharing violation while
// another process (a virus scanner, an indexer, a second reader) holds the destination open without
// FILE_SHARE_DELETE, so it is retried for about a second.
func renameReplace(from, to string) error {
	var err error
	for i := 0; i < renameAttempts; i++ {
		if err = osRename(from, to); err == nil {
			return nil
		}
		if i < renameAttempts-1 {
			time.Sleep(time.Duration(10+i*5) * time.Millisecond)
		}
	}
	return err
}

// syncDir makes the rename durable; best effort, a no-op where directories cannot be opened (Windows).
func syncDir(dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}

// CleanTemp removes leftover temp files from a crash between write and rename. Only files of this data file
// ("."+name+".tmp-<hex>") are touched.
//
// It takes the per-path lock: every writer holds it from temp creation to rename, so only crash leftovers
// are removed, never a temp file of a write in progress.
func (f *File) CleanTemp() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	dir, base := filepath.Split(f.path)
	if dir == "" {
		dir = "."
	}
	entries, _ := os.ReadDir(dir)
	prefix := "." + base + ".tmp-"
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) && os.Remove(filepath.Join(dir, e.Name())) == nil {
			n++
		}
	}
	return n
}
