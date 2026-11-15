package update

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPreflight(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "Codepass.app")
	exe := filepath.Join(app, "Contents", "MacOS", "codepass")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := installTarget(exe, "darwin"); got != app {
		t.Errorf("target = %q", got)
	}
	if got := installTarget(exe, "windows"); got != exe {
		t.Errorf("windows target = %q", got)
	}
	if err := Preflight(exe, "darwin"); err != nil {
		t.Fatalf("writable folder: %v", err)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 1 {
		t.Errorf("the write probe left something behind: %v", ents)
	}

	// App Translocation
	tr := filepath.Join(dir, "AppTranslocation", "ABC", "d", "Codepass.app", "Contents", "MacOS", "codepass")
	var pe *PreflightError
	if err := Preflight(tr, "darwin"); !errors.As(err, &pe) || pe.Kind != BlockTranslocated {
		t.Errorf("translocation: %v", err)
	}
	tr2 := "/private/var/folders/xx/T/AppTranslocation/ABC/d/Codepass.app/Contents/MacOS/codepass"
	if err := Preflight(tr2, "darwin"); !errors.As(err, &pe) || pe.Kind != BlockTranslocated {
		t.Errorf("real translocation path: %v", err)
	}

	// not writable: names the folder
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("directory permissions are not enforced here")
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)
	err := Preflight(exe, "darwin")
	if !errors.As(err, &pe) || pe.Kind != BlockNotWritable || pe.Folder != dir {
		t.Errorf("read-only folder: %v", err)
	}
}

func TestHelperLogs(t *testing.T) {
	dir := t.TempDir()
	rec := filepath.Join(t.TempDir(), "data", "restarts")
	old := time.Now().Add(-48 * time.Hour)
	mk := func(name string, mtime time.Time) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(p, mtime, mtime)
		return p
	}
	mine := mk("wails-update-123.log", old)
	running := mk("wails-update-456.log", time.Now()) // recorded, but a helper may be running now
	other := mk("wails-update-789.log", old)          // another app's helper log: never recorded here
	selfLog := mk("wails-update-31337.log", old)      // our own pid
	_ = os.Mkdir(filepath.Join(dir, "wails-update-99.log"), 0o700)
	keep := mk("other.log", old)
	for _, pid := range []int{123, 456, 99, 31337, 555} { // 555 has no file
		if err := RecordRestart(rec, pid); err != nil {
			t.Fatal(err)
		}
	}
	got := cleanHelperLogs(rec, dir, 31337, time.Now(), time.Hour)
	if len(got) != 1 || got[0] != mine {
		t.Fatalf("removed %v", got)
	}
	for _, p := range []string{running, other, selfLog, keep, filepath.Join(dir, "wails-update-99.log")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s was removed", p)
		}
	}
	// only the entries that may still be written to stay recorded
	if left := readPIDRecord(rec); len(left) != 2 || left[0] != 456 || left[1] != 31337 {
		t.Errorf("record = %v", left)
	}
	// once they are old enough, they go and the record is cleared
	got = cleanHelperLogs(rec, dir, 1, time.Now().Add(3*time.Hour), time.Hour)
	if len(got) != 2 {
		t.Errorf("second pass removed %v", got)
	}
	if _, err := os.Stat(rec); !errors.Is(err, os.ErrNotExist) {
		t.Error("the record was not cleared")
	}
	if _, err := os.Stat(other); err != nil {
		t.Error("another app's log was removed")
	}
	// no record: nothing is touched
	if got := cleanHelperLogs(rec, dir, 1, time.Now().Add(9*time.Hour), time.Hour); len(got) != 0 {
		t.Errorf("without a record: %v", got)
	}
	if err := RecordRestart("", 5); err == nil {
		t.Error("empty record path accepted")
	}
}

func TestRejectAppleDouble(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "X.app", "Contents", "MacOS"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "X.app", "Contents", "MacOS", "x"), []byte("x"), 0o755)
	if err := rejectAppleDouble(root); err != nil {
		t.Fatalf("clean tree: %v", err)
	}
	_ = os.WriteFile(filepath.Join(root, "X.app", "Contents", "MacOS", "._x"), []byte("x"), 0o644)
	err := rejectAppleDouble(root)
	if err == nil || !strings.Contains(err.Error(), "AppleDouble") {
		t.Fatalf("AppleDouble entry accepted: %v", err)
	}
}

func TestRemoveStagingOnlyRemovesWailsStaging(t *testing.T) {
	install := t.TempDir()
	stage := filepath.Join(install, "wails-update-123")
	_ = os.MkdirAll(filepath.Join(stage, "Codepass.app"), 0o755)
	removeStaging(filepath.Join(stage, "Codepass.app"), install)
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Error("staging folder not removed")
	}
	// wrong prefix, wrong parent: nothing is removed
	other := filepath.Join(install, "Documents")
	_ = os.MkdirAll(filepath.Join(other, "Codepass.app"), 0o755)
	removeStaging(filepath.Join(other, "Codepass.app"), install)
	elsewhere := t.TempDir()
	st2 := filepath.Join(elsewhere, "wails-update-9")
	_ = os.MkdirAll(st2, 0o755)
	removeStaging(filepath.Join(st2, "Codepass.app"), install)
	for _, p := range []string{other, st2} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s removed", p)
		}
	}
}

func TestDesignatedRequirement(t *testing.T) {
	r := DesignatedRequirement(BundleID, TeamID)
	for _, want := range []string{
		`anchor apple generic`, `identifier "com.rudian.awsssohub"`,
		`certificate 1[field.1.2.840.113635.100.6.2.6] exists`,
		`certificate leaf[field.1.2.840.113635.100.6.1.13] exists`,
		`certificate leaf[subject.OU] = "XC34P8TNZ6"`,
	} {
		if !strings.Contains(r, want) {
			t.Errorf("requirement lacks %s", want)
		}
	}
	g := ProductionGate()
	if !g.RequireGatekeeper || g.Requirement != r || g.BundleID != BundleID {
		t.Errorf("production gate: %+v", g)
	}
}

type fakeBackend struct {
	val   []byte
	found bool
	err   error
}

func (f *fakeBackend) read() ([]byte, bool, error) { return f.val, f.found, f.err }
func (f *fakeBackend) write(b []byte) error {
	f.val, f.found = append([]byte(nil), b...), true
	return f.err
}
func (f *fakeBackend) remove() error { f.val, f.found = nil, false; return f.err }

func TestTokenStore(t *testing.T) {
	tok := "github_pat_" + strings.Repeat("A1_-", 8)
	f := &fakeBackend{}
	s := secretStore{b: f}
	if got, err := s.Get(); err != nil || got != "" {
		t.Errorf("not found must be empty: %q %v", got, err)
	}
	if err := s.Set(tok); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "short", "has space " + strings.Repeat("a", 30), "semi;colon" + strings.Repeat("a", 30), "new\nline" + strings.Repeat("a", 30), strings.Repeat("a", 300)} {
		if err := s.Set(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if string(f.val) != tok {
		t.Error("a refused token overwrote the stored one")
	}
	if got, err := s.Get(); err != nil || got != tok {
		t.Errorf("get: %q %v", got, err)
	}
	f.err = errors.New("keychain read failed (status -25293)")
	if _, err := s.Get(); err == nil || strings.Contains(err.Error(), tok) {
		t.Errorf("read failure: %v", err)
	}
	f.err = nil
	f.val = []byte("garbage with spaces")
	if _, err := s.Get(); err == nil {
		t.Error("a value that is not a token was returned")
	}
	if err := s.Delete(); err != nil {
		t.Errorf("delete: %v", err)
	}
	if _, err := (unsupportedStore{}).Get(); !errors.Is(err, ErrTokenStoreUnsupported) {
		t.Errorf("unsupported: %v", err)
	}
}

func TestHTTPClientHasNoOverallTimeout(t *testing.T) {
	c := NewHTTPClient(nil)
	if c.Timeout != 0 {
		t.Errorf("overall timeout %v cuts a slow download off", c.Timeout)
	}
}
