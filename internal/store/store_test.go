package store

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type doc struct {
	SchemaVersion int      `json:"schemaVersion"`
	Name          string   `json:"name"`
	Items         []string `json:"items"`
}

func openV(t *testing.T, dir string, cur int) *File {
	t.Helper()
	f, err := Open(filepath.Join(dir, "d.json"), Spec{Current: cur, Migrations: map[int]Migration{
		1: func(m map[string]any) error { m["name"] = m["title"]; delete(m, "title"); return nil },
	}})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestRoundTripAndMissing(t *testing.T) {
	f := openV(t, t.TempDir(), 1)
	var d doc
	if found, err := f.Load(&d); found || err != nil {
		t.Fatalf("missing: %v %v", found, err)
	}
	if err := f.Save(&doc{Name: "a", Items: []string{"x", "y"}}); err != nil {
		t.Fatal(err)
	}
	var got doc
	if found, err := f.Load(&got); !found || err != nil {
		t.Fatal(found, err)
	}
	if got.Name != "a" || len(got.Items) != 2 || got.SchemaVersion != 1 {
		t.Fatalf("%+v", got)
	}
	if fi, _ := os.Stat(f.Path()); runtimeUnix() && fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}
}

func TestMigrationBacksUpOnceAndOnFirstWrite(t *testing.T) {
	dir := t.TempDir()
	v1 := openV(t, dir, 1)
	orig := `{"schemaVersion":1,"title":"old","items":["z"]}`
	if err := os.WriteFile(v1.Path(), []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	v2 := openV(t, dir, 2)
	var d doc
	if _, err := v2.Load(&d); err != nil {
		t.Fatal(err)
	}
	if d.Name != "old" || d.SchemaVersion != 2 {
		t.Fatalf("%+v", d)
	}
	// Loading alone writes nothing.
	if b, _ := os.ReadFile(v2.Path()); string(b) != orig {
		t.Fatal("load rewrote the file")
	}
	if _, err := os.Stat(v2.BackupPath(1)); err == nil {
		t.Fatal("backup before first write")
	}
	d.Items = append(d.Items, "w")
	if err := v2.Save(&d); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(v2.BackupPath(1)); string(b) != orig {
		t.Fatalf("backup content %q", b)
	}
	if b, _ := os.ReadFile(v2.Path()); !strings.Contains(string(b), `"schemaVersion": 2`) {
		t.Fatalf("not upgraded: %s", b)
	}
	// A later save does not rewrite the backup.
	_ = os.WriteFile(v2.BackupPath(1), []byte("keep"), 0o600)
	if err := v2.Save(&d); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(v2.BackupPath(1)); string(b) != "keep" {
		t.Fatal("backup rewritten")
	}
}

func TestSaveWithoutLoadStillBacksUp(t *testing.T) {
	dir := t.TempDir()
	v1 := openV(t, dir, 1)
	orig := `{"schemaVersion":1,"title":"old"}`
	_ = os.WriteFile(v1.Path(), []byte(orig), 0o600)
	v2 := openV(t, dir, 2)
	if err := v2.Save(&doc{Name: "n"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(v2.BackupPath(1)); string(b) != orig {
		t.Fatalf("backup %q", b)
	}
}

func TestNewerVersionRefused(t *testing.T) {
	dir := t.TempDir()
	f := openV(t, dir, 1)
	newer := `{"schemaVersion":7,"future":true}`
	_ = os.WriteFile(f.Path(), []byte(newer), 0o600)
	var d doc
	_, err := f.Load(&d)
	var nv *NewerVersionError
	if !errors.Is(err, ErrNewerVersion) || !errors.As(err, &nv) || nv.FileVersion != 7 || nv.AppVersion != 1 {
		t.Fatalf("load: %v", err)
	}
	if err := f.Save(&doc{Name: "x"}); !errors.Is(err, ErrNewerVersion) {
		t.Fatalf("save: %v", err)
	}
	if err := f.Update(&d, func() error { return nil }); !errors.Is(err, ErrNewerVersion) {
		t.Fatalf("update: %v", err)
	}
	if b, _ := os.ReadFile(f.Path()); string(b) != newer {
		t.Fatal("file changed")
	}
	// The file turning newer between Load and Save is caught too.
	g := openV(t, t.TempDir(), 1)
	_ = g.Save(&doc{Name: "a"})
	_ = os.WriteFile(g.Path(), []byte(newer), 0o600)
	if err := g.Save(&doc{Name: "b"}); !errors.Is(err, ErrNewerVersion) {
		t.Fatalf("race: %v", err)
	}
}

func TestCorruptRefusedAndQuarantine(t *testing.T) {
	f := openV(t, t.TempDir(), 1)
	for _, c := range []string{"{not json", `{"name":"no version"}`, `{"schemaVersion":0}`} {
		_ = os.WriteFile(f.Path(), []byte(c), 0o600)
		var d doc
		if _, err := f.Load(&d); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("%q: %v", c, err)
		}
		if err := f.Save(&d); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("save %q: %v", c, err)
		}
	}
	dst, err := f.Quarantine(&doc{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(&doc{Name: "fresh"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Quarantine(&doc{}); err == nil {
		t.Fatal("quarantined a healthy file")
	}
}

func TestCrashBetweenTempWriteAndRename(t *testing.T) {
	dir := t.TempDir()
	f := openV(t, dir, 1)
	if err := f.Save(&doc{Name: "before"}); err != nil {
		t.Fatal(err)
	}
	f.beforeMove = func() { panic("crash") }
	func() {
		defer func() { _ = recover() }()
		_ = f.Save(&doc{Name: "after"})
	}()
	f.beforeMove = nil
	var d doc
	if _, err := f.Load(&d); err != nil || d.Name != "before" {
		t.Fatalf("old content lost: %v %+v", err, d)
	}
	if n := f.CleanTemp(); n != 1 {
		t.Fatalf("cleaned %d temp files", n)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("leftover %s", e.Name())
		}
	}
	if err := f.Save(&doc{Name: "again"}); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentUpdates(t *testing.T) {
	f := openV(t, t.TempDir(), 1)
	var wg sync.WaitGroup
	const n = 40
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var d doc
			if err := f.Update(&d, func() error { d.Items = append(d.Items, "x"); return nil }); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var d doc
	if _, err := f.Load(&d); err != nil || len(d.Items) != n {
		t.Fatalf("lost updates: %v %d", err, len(d.Items))
	}
	entries, _ := os.ReadDir(filepath.Dir(f.Path()))
	if len(entries) != 1 {
		t.Fatalf("stray files: %v", entries)
	}
}

func TestOpenRequiresMigrationChain(t *testing.T) {
	if _, err := Open("x", Spec{Current: 3, Migrations: map[int]Migration{1: func(map[string]any) error { return nil }}}); err == nil {
		t.Fatal("gap accepted")
	}
	if _, err := Open("x", Spec{Current: 0}); err == nil {
		t.Fatal("zero accepted")
	}
}

func TestFailedUpdateDoesNotWrite(t *testing.T) {
	f := openV(t, t.TempDir(), 1)
	_ = f.Save(&doc{Name: "a"})
	var d doc
	boom := errors.New("boom")
	if err := f.Update(&d, func() error { d.Name = "b"; return boom }); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	var got doc
	_, _ = f.Load(&got)
	if got.Name != "a" {
		t.Fatal("written despite error")
	}
}

func TestBadBodyIsDamaged(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d.json")
	_ = os.WriteFile(p, []byte(`{"schemaVersion":1,"n":"not a number"}`), 0o600)
	f, _ := Open(p, Spec{Current: 1})
	var v struct {
		N int `json:"n"`
	}
	if _, err := f.Load(&v); !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
	if err := f.Save(&v); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("save: %v", err)
	}
	if _, err := f.Quarantine(&v); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(&v); err != nil {
		t.Fatal(err)
	}
	f2, _ := Open(p, Spec{Current: 1})
	if ok, err := f2.Load(&v); !ok || err != nil {
		t.Fatal(ok, err)
	}
}

func TestTwoFilesSamePathShareLock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d.json")
	type doc struct {
		N int `json:"n"`
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		for k := 0; k < 2; k++ {
			f, _ := Open(p, Spec{Current: 1})
			wg.Add(1)
			go func() {
				defer wg.Done()
				var d doc
				if err := f.Update(&d, func() error { d.N++; return nil }); err != nil {
					t.Error(err)
				}
			}()
		}
	}
	wg.Wait()
	f, _ := Open(p, Spec{Current: 1})
	var d doc
	_, _ = f.Load(&d)
	if d.N != 40 {
		t.Fatalf("kept %d of 40", d.N)
	}
}

func TestCleanTempWaitsForInFlightWrite(t *testing.T) {
	dir := t.TempDir()
	a := openV(t, dir, 1)
	b := openV(t, dir, 1)
	done := make(chan struct{})
	returned := make(chan struct{})
	a.beforeMove = func() {
		go func() {
			b.CleanTemp()
			close(returned)
		}()
		select {
		case <-returned:
			t.Error("CleanTemp finished while a write was in flight")
		case <-time.After(100 * time.Millisecond):
		}
		close(done)
	}
	if err := a.Save(&doc{Name: "x"}); err != nil {
		t.Fatalf("save lost to concurrent CleanTemp: %v", err)
	}
	<-done
	<-returned
}

func TestRenameRetriesThenSucceeds(t *testing.T) {
	oldR, oldN := osRename, renameAttempts
	defer func() { osRename, renameAttempts = oldR, oldN }()
	renameAttempts = 5
	calls := 0
	osRename = func(a, b string) error {
		calls++
		if calls <= 2 {
			return errors.New("sharing violation")
		}
		return os.Rename(a, b)
	}
	f := openV(t, t.TempDir(), 1)
	if err := f.Save(&doc{Name: "new"}); err != nil {
		t.Fatal(err)
	}
	var d doc
	if ok, err := f.Load(&d); !ok || err != nil || d.Name != "new" || calls != 3 {
		t.Fatal(ok, err, d, calls)
	}
}

func TestRenameAlwaysFailsKeepsOriginal(t *testing.T) {
	oldR, oldN := osRename, renameAttempts
	defer func() { osRename, renameAttempts = oldR, oldN }()
	f := openV(t, t.TempDir(), 1)
	if err := f.Save(&doc{Name: "orig"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(f.Path())
	renameAttempts = 3
	osRename = func(a, b string) error { return errors.New("locked") }
	if err := f.Save(&doc{Name: "new"}); err == nil {
		t.Fatal("expected error")
	}
	after, _ := os.ReadFile(f.Path())
	if string(before) != string(after) {
		t.Fatal("original changed")
	}
	if n := f.CleanTemp(); n != 0 {
		t.Fatalf("%d temp files left", n)
	}
}

func TestUpdateResetsReusedValue(t *testing.T) {
	type m struct {
		SchemaVersion int            `json:"schemaVersion"`
		Keys          map[string]int `json:"keys"`
	}
	p := filepath.Join(t.TempDir(), "m.json")
	a, _ := Open(p, Spec{Current: 1})
	b, _ := Open(p, Spec{Current: 1})
	if err := a.Save(&m{Keys: map[string]int{"x": 1, "y": 2}}); err != nil {
		t.Fatal(err)
	}
	var v m
	if err := a.Update(&v, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := b.Update(&m{}, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	var other m
	if err := b.Update(&other, func() error { delete(other.Keys, "x"); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := a.Update(&v, func() error { v.Keys["z"] = 3; return nil }); err != nil {
		t.Fatal(err)
	}
	var got m
	_, _ = a.Load(&got)
	if _, back := got.Keys["x"]; back || got.Keys["z"] != 3 || got.Keys["y"] != 2 {
		t.Fatalf("%+v", got.Keys)
	}
}

func TestUpdatePrefilledValueNoFile(t *testing.T) {
	f := openV(t, t.TempDir(), 1)
	v := doc{Name: "stale", Items: []string{"old"}}
	if err := f.Update(&v, func() error { v.Name = "fresh"; return nil }); err != nil {
		t.Fatal(err)
	}
	var got doc
	_, _ = f.Load(&got)
	if got.Name != "fresh" || len(got.Items) != 0 {
		t.Fatalf("%+v", got)
	}
	if err := f.Update(doc{}, func() error { return nil }); err == nil {
		t.Fatal("non-pointer accepted")
	}
}

func TestQuarantineStaleFlagAndNameCollision(t *testing.T) {
	now := func() time.Time { return time.Unix(1700000000, 0) }
	p := filepath.Join(t.TempDir(), "d.json")
	a, _ := Open(p, Spec{Current: 1, Now: now})
	b, _ := Open(p, Spec{Current: 1, Now: now})
	_ = os.WriteFile(p, []byte("{bad"), 0o600)
	var d doc
	if _, err := a.Load(&d); !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
	first, err := b.Quarantine(&doc{})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Save(&doc{Name: "healthy"}); err != nil {
		t.Fatal(err)
	}
	// a still has a stale corrupt flag; the healthy file must not be moved.
	if _, err := a.Quarantine(&doc{}); err == nil {
		t.Fatal("healthy file quarantined")
	}
	// A second damaged file in the same second does not replace the first quarantined copy.
	_ = os.WriteFile(p, []byte("{bad2"), 0o600)
	second, err := a.Quarantine(&doc{})
	if err != nil || second == first {
		t.Fatal(second, err)
	}
	if b1, _ := os.ReadFile(first); string(b1) != "{bad" {
		t.Fatalf("first copy lost: %q", b1)
	}
}

// FU-02b: the suffix search stops on a name it cannot look at, and after a cap, instead of looping.
func TestQuarantineStopsOnAnUnreadableNameAndCapsTheSuffix(t *testing.T) {
	now := func() time.Time { return time.Unix(1700000000, 0) }
	run := func(t *testing.T, p string) error {
		f, _ := Open(p, Spec{Current: 1, Now: now})
		if err := os.WriteFile(p, []byte("{bad"), 0o600); err != nil {
			t.Fatal(err)
		}
		var d doc
		if _, err := f.Load(&d); !errors.Is(err, ErrCorrupt) {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { _, err := f.Quarantine(&doc{}); done <- err }()
		select {
		case err := <-done:
			return err
		case <-time.After(20 * time.Second):
			t.Fatal("Quarantine does not end")
			return nil
		}
	}
	t.Run("cap", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "d.json")
		base := p + ".corrupt-1700000000"
		_ = os.WriteFile(base, nil, 0o600)
		for i := 1; i <= maxQuarantineSuffix; i++ {
			_ = os.WriteFile(base+"-"+strconv.Itoa(i), nil, 0o600)
		}
		if err := run(t, p); err == nil || !strings.Contains(err.Error(), "already exist") {
			t.Fatalf("%v", err)
		}
		if b, _ := os.ReadFile(p); string(b) != "{bad" {
			t.Fatal("the damaged file must stay where it is")
		}
	})
	t.Run("a name that cannot be looked at", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("name length limits differ")
		}
		// the damaged file's own name is just inside the limit; the quarantine name is not
		p := filepath.Join(t.TempDir(), strings.Repeat("d", 240)+".json")
		if err := run(t, p); err == nil {
			t.Fatal("an error is expected")
		}
		if _, err := os.Stat(p); err != nil {
			t.Fatal("the damaged file must stay where it is")
		}
	})
}
