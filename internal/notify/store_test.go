package notify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreKeepsKeysAcrossARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.json")
	s := OpenStore(path, nil, nil)
	if s.Asked() || s.Last("acme", RecordHeadsUp) != "" {
		t.Fatal("a new store is empty")
	}
	s.SetAsked()
	s.Mark("acme", RecordHeadsUp, "2026-11-16T01:00:00Z")
	s.Mark("acme", RecordEnded, "2026-11-16T01:00:00Z")
	s.Mark("globex", RecordHeadsUp, "2026-11-16T02:00:00Z")

	again := OpenStore(path, nil, nil)
	if !again.Asked() || again.Last("acme", RecordHeadsUp) != "2026-11-16T01:00:00Z" || again.Last("acme", RecordEnded) == "" || again.Last("globex", RecordHeadsUp) == "" {
		t.Fatalf("lost after a restart: %+v", again.d)
	}
	again.Reset("acme")
	if again.Last("acme", RecordHeadsUp) != "" || again.Last("globex", RecordHeadsUp) == "" {
		t.Fatal("Reset forgets one portal only")
	}
	b, _ := os.ReadFile(path)
	text := string(b)
	if !strings.Contains(text, `"schemaVersion": 1`) || strings.Contains(text, "token") {
		t.Fatalf("file: %s", text)
	}
}

// A damaged file never stops anything: it is moved aside and the store starts fresh.
func TestStoreQuarantinesADamagedFile(t *testing.T) {
	for name, body := range map[string]string{
		"not json":          "{{{{",
		"no schema version": `{"asked": true}`,
		"wrong shape":       `{"schemaVersion": 1, "sent": 7}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "notifications.json")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			s := OpenStore(path, nil, nil)
			if s.Asked() {
				t.Fatal("fresh store")
			}
			s.Mark("acme", RecordEnded, "k") // writes a new file
			matches, _ := filepath.Glob(path + ".corrupt-*")
			if len(matches) != 1 {
				t.Fatalf("damaged file not kept aside: %v", matches)
			}
			if OpenStore(path, nil, nil).Last("acme", RecordEnded) != "k" {
				t.Fatal("the new file works")
			}
		})
	}
}

// A file from a newer app is never overwritten; this run keeps its records in memory.
func TestStoreLeavesANewerFileAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.json")
	const body = `{"schemaVersion": 99, "asked": true, "future": 1}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s := OpenStore(path, nil, nil)
	s.SetAsked()
	s.Mark("acme", RecordEnded, "k")
	if s.Last("acme", RecordEnded) != "k" {
		t.Fatal("memory still works")
	}
	if b, _ := os.ReadFile(path); string(b) != body {
		t.Fatalf("a newer file was rewritten: %s", b)
	}
}

// A store that cannot write (the folder is a file) keeps working in memory.
func TestStoreSurvivesAWriteFailure(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := OpenStore(filepath.Join(blocker, "notifications.json"), nil, nil)
	s.Mark("acme", RecordHeadsUp, "k")
	s.SetAsked()
	if s.Last("acme", RecordHeadsUp) != "k" || !s.Asked() {
		t.Fatal("memory still works")
	}
}

func TestMemoryStore(t *testing.T) {
	s := OpenStore("", nil, nil)
	s.Mark("acme", RecordHeadsUp, "k")
	if s.Last("acme", RecordHeadsUp) != "k" {
		t.Fatal()
	}
}
