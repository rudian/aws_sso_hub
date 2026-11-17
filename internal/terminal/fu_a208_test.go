package terminal

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/pty"
)

// FU-A2-02b: after a resume the archive cache never serves the previous run.
func TestArchiveCacheNeverServesThePreviousRunAfterAResume(t *testing.T) {
	dir := t.TempDir()
	m, ch := fakeManager(Config{DataDir: dir, ScrollbackCap: 1 << 20})
	if _, err := m.Start(StartOptions{ID: "t-1", Argv: []string{"x"}, Dir: dir}); err != nil {
		t.Fatal(err)
	}
	f := <-ch
	f.feed("OLD RUN OUTPUT")
	waitFor(t, "old output", time.Second, func() bool { return m.StatusOf("t-1").Tail >= 14 })
	f.eof()
	waitFor(t, "the old scrollback file", 2*time.Second, func() bool {
		_, err := os.Stat(ScrollbackPath(dir, "t-1"))
		return err == nil
	})
	if err := m.Release("t-1"); err != nil {
		t.Fatal(err)
	}
	if r := m.Read("t-1", 0, 0); !strings.Contains(string(r.Data), "OLD RUN OUTPUT") { // loads and caches the archive
		t.Fatalf("archive: %q", r.Data)
	}

	// The second run: the Read during the spawn window must not re-cache the old file.
	gate := make(chan struct{})
	started := make(chan struct{})
	f2 := newFakePTY()
	m.cfg.Start = func(pty.Options) (pty.Session, error) {
		close(started)
		<-gate
		return f2, nil
	}
	go func() { _, _ = m.Start(StartOptions{ID: "t-1", Argv: []string{"x"}, Dir: dir}) }()
	<-started
	if r := m.Read("t-1", 0, 0); !strings.Contains(string(r.Data), "OLD RUN OUTPUT") {
		t.Fatalf("during the spawn window the old file is still what there is: %q", r.Data)
	}
	close(gate)
	waitFor(t, "the new session", time.Second, func() bool { return m.get("t-1") != nil })
	f2.feed("NEW RUN OUTPUT")
	waitFor(t, "new output", time.Second, func() bool {
		return m.StatusOf("t-1").Tail > 0 && strings.Contains(string(m.Read("t-1", 0, 0).Data), "NEW RUN OUTPUT")
	})
	m.mu.Lock()
	cached := m.archives["t-1"]
	m.mu.Unlock()
	if cached != nil {
		t.Fatal("the old run's archive is still cached while the new run lives")
	}
	// End it and release at once: a read before the new file is written must not keep the old file either.
	f2.eof()
	if err := m.Stop("t-1"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the session to end", time.Second, func() bool { return m.get("t-1").Ended() })
	_ = m.Release("t-1")
	waitFor(t, "the new file", 2*time.Second, func() bool {
		r := m.Read("t-1", 0, 0)
		return strings.Contains(string(r.Data), "NEW RUN OUTPUT")
	})
	if r := m.Read("t-1", 0, 0); strings.Contains(string(r.Data), "OLD RUN OUTPUT") {
		t.Fatalf("the previous run is served: %q", r.Data)
	}
}

// An archive loaded from the file before a drop is not cached after it: the old file is read, then the new run's
// save replaces it and drops the cache (the same steps as saveScrollback), and the next Read sees the new file.
func TestArchiveLoadedBeforeADropIsNotCached(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(Config{DataDir: dir})
	path := ScrollbackPath(dir, "t-1")
	if err := writeFile0600(path, encodeScrollback(0, nil, []byte("OLD RUN OUTPUT"))); err != nil {
		t.Fatal(err)
	}
	m.afterArchiveRead = func(id string) {
		m.afterArchiveRead = nil
		if err := writeFile0600(path, encodeScrollback(0, nil, []byte("NEW RUN OUTPUT"))); err != nil {
			t.Error(err)
		}
		m.mu.Lock()
		m.dropArchiveLocked(id)
		m.mu.Unlock()
	}
	if r := m.Read("t-1", 0, 0); !strings.Contains(string(r.Data), "OLD RUN OUTPUT") {
		t.Fatalf("the first read answers from the file it loaded: %q", r.Data)
	}
	m.mu.Lock()
	cached := m.archives["t-1"]
	m.mu.Unlock()
	if cached != nil {
		t.Fatal("an archive loaded before a drop must not be cached")
	}
	if r := m.Read("t-1", 0, 0); !strings.Contains(string(r.Data), "NEW RUN OUTPUT") || strings.Contains(string(r.Data), "OLD") {
		t.Fatalf("the next read must see the new run: %q", r.Data)
	}
}

// FU-A2-02d: a reused ID's log starts one byte past the old tail, so a page that followed the old run to its
// end gets gap > 0 and the default-mode prologue.
func TestReusedIDStartsPastTheOldTail(t *testing.T) {
	m, ch := fakeManager(Config{})
	dir := t.TempDir()
	if _, err := m.Start(StartOptions{ID: "t-1", Argv: []string{"x"}, Dir: dir}); err != nil {
		t.Fatal(err)
	}
	f := <-ch
	f.feed("\x1b[?1049h\x1b[?1003h\x1b[?1006hvim")
	waitFor(t, "output", time.Second, func() bool { return m.StatusOf("t-1").Tail > 0 })
	oldTail := m.StatusOf("t-1").Tail
	f.eof()
	waitFor(t, "end", time.Second, func() bool { return m.get("t-1").Ended() })
	if _, err := m.Start(StartOptions{ID: "t-1", Argv: []string{"x"}, Dir: dir}); err != nil {
		t.Fatal(err)
	}
	r := m.Read("t-1", oldTail, 0)
	if r.Gap != 1 || len(r.Prologue) == 0 {
		t.Fatalf("a page at the old tail must get a gap and a prologue: gap %d prologue %q", r.Gap, r.Prologue)
	}
	if strings.Contains(string(r.Prologue), "1049h") || strings.Contains(string(r.Prologue), "1003h") {
		t.Fatalf("the prologue must not carry the old run's modes: %q", r.Prologue)
	}
}

func TestOnInputSeesWritesOfLiveSessions(t *testing.T) {
	m, ch := fakeManager(Config{})
	if _, err := m.Start(StartOptions{ID: "t-1", Argv: []string{"x"}, Dir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	<-ch
	var got []string
	m.SetOnInput(func(id string, b []byte) { got = append(got, id+":"+string(b)) })
	_ = m.Write("t-1", []byte("a\r"))
	_ = m.Write("nope", []byte("b"))
	if len(got) != 1 || got[0] != "t-1:a\r" {
		t.Fatalf("got %q", got)
	}
}
