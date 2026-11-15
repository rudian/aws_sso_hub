//go:build windows

package store

import (
	"os"
	"testing"
	"time"
)

func holdDest(t *testing.T, f *File, d time.Duration) {
	t.Helper()
	h, err := os.Open(f.Path())
	if err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(d, func() { _ = h.Close() })
}

func TestWindowsRenameOverOpenFile(t *testing.T) {
	f := openV(t, t.TempDir(), 1)
	if err := f.Save(&doc{Name: "orig"}); err != nil {
		t.Fatal(err)
	}
	holdDest(t, f, 200*time.Millisecond)
	if err := f.Save(&doc{Name: "new"}); err != nil {
		t.Fatal(err)
	}
	var d doc
	if _, err := f.Load(&d); err != nil || d.Name != "new" {
		t.Fatal(err, d)
	}
}

func TestWindowsRenameHeldTooLong(t *testing.T) {
	f := openV(t, t.TempDir(), 1)
	if err := f.Save(&doc{Name: "orig"}); err != nil {
		t.Fatal(err)
	}
	holdDest(t, f, 3*time.Second)
	if err := f.Save(&doc{Name: "new"}); err == nil {
		t.Fatal("expected failure")
	}
	time.Sleep(3100 * time.Millisecond)
	var d doc
	if _, err := f.Load(&d); err != nil || d.Name != "orig" {
		t.Fatal(err, d)
	}
	if n := f.CleanTemp(); n != 0 {
		t.Fatalf("%d temp files left", n)
	}
}
