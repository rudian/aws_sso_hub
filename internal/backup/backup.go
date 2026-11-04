// Package backup keeps redacted copies of every file the app is about to modify
// and provides the atomic file writer used for all user-file writes.
package backup

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rudian/aws_sso_hub/internal/api"
)

// Placeholder replaces secret values in backups.
const Placeholder = "REDACTED-BY-AWS-SSO-HUB"

// Store manages <dir>/index.json and <dir>/<id>.bak.
type Store struct {
	dir       string
	retention func() int
	now       func() time.Time
	mu        sync.Mutex
	onSnap    func(api.BackupEntry)
}

// SetOnSnapshot registers a function called with every entry Snapshot stores. It runs while the store is
// locked, so it must be quick and must not call back into the store.
func (s *Store) SetOnSnapshot(f func(api.BackupEntry)) {
	s.mu.Lock()
	s.onSnap = f
	s.mu.Unlock()
}

// Open creates the backup directory (0700) if needed. retention returns 20, 50 or 0 (= keep all); nil means 20.
func Open(dir string, retention func() int, now func() time.Time) (*Store, error) {
	if retention == nil {
		retention = func() int { return 20 }
	}
	if now == nil {
		now = time.Now
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, api.NewError(api.ErrFileIO, "cannot create backup directory").WithParam("file", dir).WithDetail(err.Error())
	}
	return &Store{dir: dir, retention: retention, now: now}, nil
}

func (s *Store) Dir() string { return s.dir }

func (s *Store) indexPath() string { return filepath.Join(s.dir, "index.json") }

func (s *Store) readIndex() ([]api.BackupEntry, error) {
	b, err := os.ReadFile(s.indexPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var all []api.BackupEntry
	if err := json.Unmarshal(b, &all); err != nil {
		return nil, fmt.Errorf("backup index is not valid JSON: %w", err)
	}
	// An id becomes a file name inside the backup directory, so entries whose id does not have the shape
	// newID produces are ignored.
	out := all[:0]
	for _, e := range all {
		if validID(e.ID) {
			out = append(out, e)
		}
	}
	return out, nil
}

var idRE = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z-[a-z]+(-[a-z]+)*-[0-9a-f]{4}$`)

// validID reports whether id has the shape newID generates: UTC timestamp, backup kind, 4 hex digits.
func validID(id string) bool { return idRE.MatchString(id) }

func (s *Store) writeIndex(entries []api.BackupEntry) error {
	if entries == nil {
		entries = []api.BackupEntry{}
	}
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(s.indexPath(), b, 0o600)
}

func (s *Store) newID(kind api.BackupKind, t time.Time) string {
	for {
		var r [2]byte
		_, _ = rand.Read(r[:])
		id := fmt.Sprintf("%s-%s-%s", t.UTC().Format("20060102T150405Z"), kind, hex.EncodeToString(r[:]))
		if _, err := os.Stat(filepath.Join(s.dir, id+".bak")); errors.Is(err, fs.ErrNotExist) {
			return id
		}
	}
}

// Snapshot stores a redacted copy of path. A missing file yields a zero entry (ID "") and nil error.
func (s *Store) Snapshot(path, display string, kind api.BackupKind, reason api.BackupReason, params map[string]string) (api.BackupEntry, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return api.BackupEntry{}, nil
	}
	if err != nil {
		return api.BackupEntry{}, api.NewError(api.ErrFileIO, "cannot read file for backup").WithParam("file", display).WithDetail(err.Error())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out, redacted := Redact(kind, content)
	now := s.now().UTC()
	id := s.newID(kind, now)
	if err := WriteFileAtomic(filepath.Join(s.dir, id+".bak"), out, 0o600); err != nil {
		return api.BackupEntry{}, api.NewError(api.ErrFileIO, "cannot write backup").WithParam("file", s.dir).WithDetail(err.Error())
	}
	if params == nil {
		params = map[string]string{}
	}
	entry := api.BackupEntry{
		ID: id, CreatedAt: now.Format(time.RFC3339), Path: path, File: display, Kind: kind,
		Reason: reason, ReasonParams: params, Size: int64(len(out)), Redacted: redacted,
	}
	entries, err := s.readIndex()
	if err != nil {
		entries = nil // a corrupt index must not block a write; it is rebuilt from here on
	}
	entries = append(entries, entry)
	entries = s.applyRetention(entries, path)
	if err := s.writeIndex(entries); err != nil {
		return api.BackupEntry{}, api.NewError(api.ErrFileIO, "cannot write backup index").WithParam("file", s.dir).WithDetail(err.Error())
	}
	if s.onSnap != nil {
		s.onSnap(entry)
	}
	return entry, nil
}

func (s *Store) applyRetention(entries []api.BackupEntry, path string) []api.BackupEntry {
	keep := s.retention()
	if keep <= 0 {
		return entries
	}
	var idx []int
	for i, e := range entries {
		if e.Path == path {
			idx = append(idx, i)
		}
	}
	if len(idx) <= keep {
		return entries
	}
	drop := map[int]bool{}
	for _, i := range idx[:len(idx)-keep] {
		drop[i] = true
		_ = os.Remove(filepath.Join(s.dir, entries[i].ID+".bak"))
	}
	out := make([]api.BackupEntry, 0, len(entries)-len(drop))
	for i, e := range entries {
		if !drop[i] {
			out = append(out, e)
		}
	}
	return out
}

// List returns all entries, newest first.
func (s *Store) List() ([]api.BackupEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.readIndex()
	if err != nil {
		return nil, api.NewError(api.ErrFileIO, "cannot read backup index").WithParam("file", s.dir).WithDetail(err.Error())
	}
	out := make([]api.BackupEntry, 0, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.ReasonParams == nil {
			e.ReasonParams = map[string]string{}
		}
		out = append(out, e)
	}
	return out, nil
}

func (s *Store) find(id string) (api.BackupEntry, error) {
	entries, err := s.List()
	if err != nil {
		return api.BackupEntry{}, err
	}
	for _, e := range entries {
		if e.ID == id {
			return e, nil
		}
	}
	return api.BackupEntry{}, api.NewError(api.ErrBackupNotFound, "backup not found")
}

// Get returns the entry and the stored (redacted) content.
func (s *Store) Get(id string) (api.BackupEntry, []byte, error) {
	e, err := s.find(id)
	if err != nil {
		return api.BackupEntry{}, nil, err
	}
	b, err := os.ReadFile(filepath.Join(s.dir, e.ID+".bak"))
	if err != nil {
		return api.BackupEntry{}, nil, api.NewError(api.ErrBackupNotFound, "backup file missing")
	}
	return e, b, nil
}

// FilePath returns the absolute path of the .bak file for id.
func (s *Store) FilePath(id string) (string, error) {
	e, err := s.find(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.dir, e.ID+".bak"), nil
}

var awsSecretKeys = map[string]bool{"aws_access_key_id": true, "aws_secret_access_key": true, "aws_session_token": true, "aws_security_token": true}

// userinfoRE matches the userinfo part of any URL: scheme://<userinfo>@.
var userinfoRE = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.\-]*://)([^/\s@]*)@`)

// redactUserinfo replaces the secret part of every URL userinfo: "user:password" keeps the user name and
// hides the password, a bare "token" is hidden completely.
func redactUserinfo(line string) string {
	return userinfoRE.ReplaceAllStringFunc(line, func(m string) string {
		sub := userinfoRE.FindStringSubmatch(m)
		scheme, info := sub[1], sub[2]
		if info == "" {
			return m
		}
		if c := strings.IndexByte(info, ':'); c >= 0 {
			return scheme + info[:c] + ":" + Placeholder + "@"
		}
		return scheme + Placeholder + "@"
	})
}

// Redact applies the per-kind rules. AWS files hide the values of the key, secret and token entries; the
// key ends at the first "=" or ":" because both are accepted as separators. Git files hide entries whose key
// names a secret and every URL userinfo.
func Redact(kind api.BackupKind, content []byte) (out []byte, redacted bool) {
	lines := strings.Split(string(content), "\n")
	for i, l := range lines {
		nl := l
		sepSet := "="
		if kind == api.BackupAWSConfig || kind == api.BackupAWSCredentials {
			sepSet = "=:"
		}
		if eq := strings.IndexAny(l, sepSet); eq > 0 {
			key := strings.ToLower(strings.TrimSpace(l[:eq]))
			secret := false
			switch kind {
			case api.BackupAWSConfig, api.BackupAWSCredentials:
				secret = awsSecretKeys[key]
			case api.BackupGitGlobal, api.BackupGitLocal:
				secret = strings.Contains(key, "token") || strings.Contains(key, "password") || strings.Contains(key, "secret") || key == "extraheader"
			}
			if secret {
				j := eq + 1
				for j < len(l) && (l[j] == ' ' || l[j] == '\t') {
					j++
				}
				cr := ""
				if strings.HasSuffix(l, "\r") {
					cr = "\r"
				}
				if strings.TrimSuffix(l[j:], "\r") != "" {
					nl = l[:j] + Placeholder + cr
				}
			}
		}
		if kind == api.BackupGitGlobal || kind == api.BackupGitLocal {
			nl = redactUserinfo(nl)
		}
		if nl != l {
			redacted = true
			lines[i] = nl
		}
	}
	return []byte(strings.Join(lines, "\n")), redacted
}

var headerRE = regexp.MustCompile(`^\s*\[(.*)\]\s*$`)

// keyOf returns the lower-cased key of a "key = value" line, or the trimmed line when it has no "=".
func keyOf(l string) string {
	if eq := strings.IndexByte(l, '='); eq > 0 {
		return strings.ToLower(strings.TrimSpace(l[:eq]))
	}
	return strings.ToLower(strings.TrimSpace(l))
}

// MergeRedacted restores placeholder values from the current file. A line is matched by its section header,
// its key and its occurrence number among the lines with that key in that section, so repeated keys such as
// extraheader or insteadOf are restored in order. A line that still holds the placeholder afterwards is
// dropped and its section is reported in lostSections; a section header that holds the placeholder cannot
// be located in the current file, so the whole section is dropped.
func MergeRedacted(backupContent, current []byte) (merged []byte, lostSections []string, err error) {
	type slot struct {
		sec, key string
		n        int
	}
	cur := map[slot]string{}
	counts := map[[2]string]int{}
	sec := ""
	for _, l := range strings.Split(string(current), "\n") {
		if m := headerRE.FindStringSubmatch(strings.TrimSuffix(l, "\r")); m != nil {
			sec = strings.TrimSpace(m[1])
			continue
		}
		if strings.IndexByte(l, '=') > 0 {
			k := keyOf(l)
			cur[slot{sec, k, counts[[2]string{sec, k}]}] = l
			counts[[2]string{sec, k}]++
		}
	}
	var out []string
	seen := map[string]bool{}
	lose := func(name string) {
		if !seen[name] {
			seen[name] = true
			lostSections = append(lostSections, name)
		}
	}
	counts = map[[2]string]int{}
	sec = ""
	dropSection := false
	for _, l := range strings.Split(string(backupContent), "\n") {
		if m := headerRE.FindStringSubmatch(strings.TrimSuffix(l, "\r")); m != nil {
			sec = strings.TrimSpace(m[1])
			if strings.Contains(l, Placeholder) {
				dropSection = true
				lose(sec)
				continue
			}
			dropSection = false
			out = append(out, l)
			continue
		}
		if dropSection {
			continue
		}
		hasEq := strings.IndexByte(l, '=') > 0
		var k string
		var n int
		if hasEq {
			k = keyOf(l)
			n = counts[[2]string{sec, k}]
			counts[[2]string{sec, k}]++
		}
		if !strings.Contains(l, Placeholder) {
			out = append(out, l)
			continue
		}
		if hasEq {
			if cl, ok := cur[slot{sec, k, n}]; ok && !strings.Contains(cl, Placeholder) {
				out = append(out, cl)
				continue
			}
		}
		lose(sec)
	}
	if lostSections == nil {
		lostSections = []string{}
	}
	return []byte(strings.Join(out, "\n")), lostSections, nil
}

// WriteFileAtomic writes content to a temp file in the same directory, fsyncs and renames it over path.
// The final mode is the existing file's mode (defaultMode for a new file) with the group and other bits
// cleared, so a file that was readable by others is tightened to owner-only. A symlink at path is followed.
// On Windows the mode bits have no effect; secrecy there relies on the ACLs inherited from the user profile
// directories.
func WriteFileAtomic(path string, content []byte, defaultMode os.FileMode) error {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	mode := defaultMode
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	mode &^= 0o077
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	var r [4]byte
	_, _ = rand.Read(r[:])
	tmp := filepath.Join(dir, "."+filepath.Base(path)+".tmp-"+hex.EncodeToString(r[:]))
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	cleanup := func() { _ = os.Remove(tmp) }
	if _, err := f.Write(content); err != nil {
		f.Close()
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		cleanup()
		return err
	}
	return nil
}
