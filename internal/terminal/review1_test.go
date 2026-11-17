package terminal

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/pty"
)

// A stream that stays inside an ESC or CSI sequence (digits, NULs, intermediates, newlines) has no ground point
// to cut at; the parser ends such a sequence after maxSeq bytes, so the log stays near its cap.
func TestEndlessEscapeOrCSIDoesNotPinTheLog(t *testing.T) {
	filler := map[string][]byte{
		"CSI digits":        append([]byte("\x1b["), bytes.Repeat([]byte("1"), 1)...),
		"ESC NULs":          []byte("\x1b\x00"),
		"ESC intermediates": []byte("\x1b "),
		"CSI newlines":      []byte("\x1b[\n"),
		"CSI semicolons":    []byte("\x1b[;"),
		"CSI ignored":       []byte("\x1b[1;2;3;4;5;6;7;8$<"),
	}
	for name, head := range filler {
		l := newLog(1000, 64)
		l.Append(head[:len(head)-1])
		unit := head[len(head)-1:]
		if name == "CSI digits" {
			unit = []byte("1")
		}
		for i := 0; i < 200; i++ {
			l.Append(bytes.Repeat(unit, 2000))
		}
		if sz := l.Size(); sz > 1000+1000/8+64+maxSeq+2000 {
			t.Errorf("%s: %d bytes held after 400000 bytes (cap 1000)", name, sz)
		}
		if l.Trims() == 0 {
			t.Errorf("%s: never trimmed", name)
		}
	}
}

// The scrollback file stays within the cap plus the look-back for a clear, even when a clear sits just inside
// the old window (cap/8 of the whole log instead of a share of the file cap).
func TestNeverGroundStreamsAreTrimmed(t *testing.T) {
	const cap int64 = 1 << 20
	mk := func(unit string, n int) []byte { return bytes.Repeat([]byte(unit), n) }
	cases := map[string][]byte{
		"ESC":            mk("\x1b", 10<<20),
		"ESC LF":         mk("\x1b\n", 5<<20),
		"ESC [ 1":        mk("\x1b[1", 4<<20),
		"OSC then ESC ]": mk("\x1b]0;\x1b]", 4<<20),
		"UTF-8 lead C8":  mk("\xc8", 10<<20),
		"UTF-8 lead E0":  mk("\xe0", 10<<20),
	}
	for name, data := range cases {
		l := newLog(int(cap), 4096)
		for off := 0; off < len(data); off += 64 << 10 {
			l.Append(data[off:min(off+64<<10, len(data))])
		}
		slack := int64(maxSeq)
		if strings.Contains(name, "OSC") {
			slack = int64(maxStr)
		}
		if sz := l.Size(); sz > cap+cap/8+4096+slack {
			t.Errorf("%s: %d bytes held (cap %d)", name, sz, cap)
		}
		if l.Trims() == 0 {
			t.Errorf("%s: never trimmed", name)
		}
	}
}

func TestSnapshotLookBackIsAShareOfTheFileCap(t *testing.T) {
	l := newLog(1<<20, 4096)
	l.Append([]byte(strings.Repeat("a", 150<<10)))
	l.Append([]byte("\x1b[2J"))
	l.Append([]byte(strings.Repeat("b", 100<<10)))
	const max = 20000
	data, pro, from := l.Snapshot(max)
	if len(data) > max+max/8 {
		t.Fatalf("snapshot holds %d bytes for a cap of %d", len(data), max)
	}
	if from == 0 || len(pro) == 0 {
		t.Fatalf("from=%d prologue=%d", from, len(pro))
	}
	// a clear just inside the look-back is still preferred
	l = newLog(1<<20, 4096)
	l.Append([]byte(strings.Repeat("a", 100<<10)))
	l.Append([]byte("\x1b[2J"))
	l.Append([]byte(strings.Repeat("b", max+max/16)))
	data, _, _ = l.Snapshot(max)
	if !bytes.HasPrefix(data, []byte("\x1b[2J")) || len(data) > max+max/8 {
		t.Fatalf("%d bytes, starts %q", len(data), data[:min(len(data), 8)])
	}
}

func lines(from, to int) string {
	var b strings.Builder
	for i := from; i < to; i++ {
		fmt.Fprintf(&b, "line %04d\r\n", i)
	}
	return b.String()
}

func endSession(t *testing.T, m *Manager, f *fakePTY, id string) {
	t.Helper()
	f.eof()
	waitFor(t, "end", 5*time.Second, func() bool { return m.StatusOf(id).Ended })
}

// A page's old offset must never read other bytes: after Release the archive keeps absolute offsets.
func TestOffsetsSurviveReleaseAndRestart(t *testing.T) {
	data := t.TempDir()
	m, ch := fakeManager(Config{DataDir: data, LogCap: 8192, ScrollbackCap: 3000})
	if _, err := m.Start(StartOptions{ID: "t-1", Argv: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	f := <-ch
	f.feed("\x1b[?2004h" + lines(0, 2000))
	waitFor(t, "output", 5*time.Second, func() bool { return m.StatusOf("t-1").Tail == 8+2000*11 })
	live := m.Read("t-1", 0, 1<<20)
	tail := live.Tail
	endSession(t, m, f, "t-1")
	path := filepath.Join(data, "sessions", "t-1", "scrollback")
	waitFor(t, "scrollback", 5*time.Second, func() bool { _, err := os.Stat(path); return err == nil })
	if err := m.Release("t-1"); err != nil {
		t.Fatal(err)
	}
	for name, mm := range map[string]*Manager{"released": m, "restarted": NewManager(Config{DataDir: data, LogCap: 4096})} {
		stale := live.From + 100 // a page's offset: inside the live log of that time, before the archive's start
		r := mm.Read("t-1", stale, 1<<20)
		if r.Tail != tail {
			t.Fatalf("%s: tail %d, want %d", name, r.Tail, tail)
		}
		if r.Gap == 0 || r.From != stale+r.Gap {
			t.Fatalf("%s: from %d gap %d for offset %d", name, r.From, r.Gap, stale)
		}
		if !bytes.Equal(r.Data, live.Data[r.From-live.From:]) {
			t.Fatalf("%s: the archive reads other bytes than the live log had at offset %d", name, r.From)
		}
		// an offset the archive still holds returns exactly the bytes the live log had there
		off := r.From + 7
		if got, want := mm.Read("t-1", off, 50), (live.Data[off-live.From:][:50]); !bytes.Equal(got.Data, want) || got.Gap != 0 {
			t.Fatalf("%s: offset %d reads other bytes: %q vs %q gap %d", name, off, got.Data, want, got.Gap)
		}
		// a page at an offset before the archive's start gets a gap and a prologue that restores the modes
		r = mm.Read("t-1", live.From, 10)
		if r.Gap == 0 || !strings.Contains(string(r.Prologue), "\x1b[?2004h") {
			t.Fatalf("%s: gap=%d prologue=%q", name, r.Gap, r.Prologue)
		}
	}
}

// A reused ID continues its offsets: a page holding an offset of the previous run gets a gap, never the middle
// of the new run.
func TestReusedIDContinuesItsOffsets(t *testing.T) {
	data := t.TempDir()
	m, ch := fakeManager(Config{DataDir: data})
	if _, err := m.Start(StartOptions{ID: "t-1", Argv: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	f := <-ch
	f.feed(strings.Repeat("old run ", 1000))
	waitFor(t, "output", 5*time.Second, func() bool { return m.StatusOf("t-1").Tail == 8000 })
	endSession(t, m, f, "t-1")
	path := filepath.Join(data, "sessions", "t-1", "scrollback")
	waitFor(t, "scrollback", 5*time.Second, func() bool { _, err := os.Stat(path); return err == nil })
	for _, release := range []bool{false, true} {
		if release {
			if err := m.Release("t-1"); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := m.Start(StartOptions{ID: "t-1", Argv: []string{"x"}}); err != nil {
			t.Fatal(err)
		}
		f = <-ch
		before := m.StatusOf("t-1")
		f.feed("new run\r\n")
		waitFor(t, "output", 5*time.Second, func() bool { return m.StatusOf("t-1").Tail > before.Tail })
		r := m.Read("t-1", 5000, 1<<20)
		if r.Gap == 0 || r.From != before.Base || !strings.HasPrefix(string(r.Data), "new run") {
			t.Fatalf("release=%v: stale page got from=%d gap=%d %q (base %d)", release, r.From, r.Gap, r.Data, before.Base)
		}
		if r := m.Read("t-1", before.Base, 100); r.Gap != 0 || string(r.Data) != "new run\r\n" {
			t.Fatalf("%+v", r)
		}
		endSession(t, m, f, "t-1")
		waitFor(t, "scrollback", 5*time.Second, func() bool { st, err := os.Stat(path); return err == nil && st.Size() > 0 })
		time.Sleep(50 * time.Millisecond)
	}
	// a fresh manager (restart) continues from the file's tail as well
	m2, ch2 := fakeManager(Config{DataDir: data})
	if _, err := m2.Start(StartOptions{ID: "t-1", Argv: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	<-ch2
	if b := m2.StatusOf("t-1").Base; b < 8000 {
		t.Fatalf("a restarted app must not reuse the offsets of the file: base %d", b)
	}
}

func TestConcurrentStartsOfOneIDSucceedOnce(t *testing.T) {
	var started atomic.Int32
	m := NewManager(Config{Start: func(pty.Options) (pty.Session, error) {
		started.Add(1)
		time.Sleep(100 * time.Millisecond)
		return newFakePTY(), nil
	}})
	var wg sync.WaitGroup
	var ok atomic.Int32
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := m.Start(StartOptions{ID: "t-1", Argv: []string{"x"}}); err == nil {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || started.Load() != 1 {
		t.Fatalf("%d Starts succeeded, %d terminals created", ok.Load(), started.Load())
	}
	// a failed start gives the ID back
	m2 := NewManager(Config{Start: func(pty.Options) (pty.Session, error) { return nil, fmt.Errorf("boom") }})
	for i := 0; i < 2; i++ {
		if _, err := m2.Start(StartOptions{ID: "t-1", Argv: []string{"x"}}); err == nil || strings.Contains(err.Error(), "already running") {
			t.Fatalf("%v", err)
		}
	}
}

func TestArchiveCacheIsBounded(t *testing.T) {
	data := t.TempDir()
	for i := 0; i < 3*maxArchives; i++ {
		p := filepath.Join(data, "sessions", fmt.Sprintf("t-%d", i), "scrollback")
		if err := writeFile0600(p, encodeScrollback(0, nil, []byte(fmt.Sprintf("task %d\r\n", i)))); err != nil {
			t.Fatal(err)
		}
	}
	m := NewManager(Config{DataDir: data})
	for i := 0; i < 3*maxArchives; i++ {
		if r := m.Read(fmt.Sprintf("t-%d", i), 0, 100); string(r.Data) != fmt.Sprintf("task %d\r\n", i) {
			t.Fatalf("%q", r.Data)
		}
		m.mu.Lock()
		n, o := len(m.archives), len(m.archOrder)
		m.mu.Unlock()
		if n > maxArchives || o != n {
			t.Fatalf("%d archives, %d in the order list", n, o)
		}
	}
	// the most recently read stays, and an evicted one loads again
	if r := m.Read("t-0", 0, 100); string(r.Data) != "task 0\r\n" {
		t.Fatalf("%q", r.Data)
	}
}

func TestProxyEnvReplacesInheritedProxyVariables(t *testing.T) {
	var got pty.Options
	proxy := []string{"HTTPS_PROXY=http://proxy.example.com:3128", "https_proxy=http://proxy.example.com:3128", "NO_PROXY=localhost", "no_proxy=localhost"}
	cfg := func(pe func() []string) *Manager {
		return NewManager(Config{
			GOOS: "darwin",
			BaseEnv: func() []string {
				return []string{"PATH=/usr/bin", "http_proxy=http://old.example.com:1", "ALL_PROXY=socks5://old.example.com:2"}
			},
			ProxyEnv: pe,
			Start:    func(o pty.Options) (pty.Session, error) { got = o; return newFakePTY(), nil },
		})
	}
	if _, err := cfg(func() []string { return proxy }).Start(StartOptions{ID: "t-1", Argv: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	e := envMap(got.Env)
	if e["HTTPS_PROXY"] != "http://proxy.example.com:3128" || e["https_proxy"] != e["HTTPS_PROXY"] || e["NO_PROXY"] != "localhost" || e["PATH"] != "/usr/bin" {
		t.Fatalf("%v", e)
	}
	if _, ok := e["http_proxy"]; ok {
		t.Errorf("an inherited proxy variable survived next to the detected proxy: %v", e)
	}
	if _, ok := e["ALL_PROXY"]; ok {
		t.Errorf("ALL_PROXY survived: %v", e)
	}
	// nothing detected: the inherited variables stay
	if _, err := cfg(func() []string { return nil }).Start(StartOptions{ID: "t-1", Argv: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	if e := envMap(got.Env); e["http_proxy"] != "http://old.example.com:1" {
		t.Fatalf("%v", e)
	}
}

func TestCheckProgramRefusesBatchFilesOnWindows(t *testing.T) {
	look := func(m map[string]string) func(string) (string, error) {
		return func(s string) (string, error) {
			if p, ok := m[s]; ok {
				return p, nil
			}
			return "", fmt.Errorf("not found")
		}
	}
	found := look(map[string]string{
		"opencode":     `C:\Users\alex\AppData\Roaming\npm\opencode.CMD`,
		"old":          `C:\x\old.bat`,
		"claude":       `C:\Users\alex\.local\bin\claude.exe`,
		"opencode.exe": `C:\x\opencode.exe`,
	})
	for _, p := range []string{"opencode", "old"} {
		if err := CheckProgram("windows", p, found); err == nil {
			t.Errorf("%s: a batch file must be refused on Windows", p)
		}
	}
	for _, p := range []string{"claude", "opencode.exe", "missing"} {
		if err := CheckProgram("windows", p, found); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	if err := CheckProgram("darwin", "opencode", found); err != nil {
		t.Errorf("only Windows is affected: %v", err)
	}
	// the manager applies it before it starts anything
	var started atomic.Bool
	m := NewManager(Config{GOOS: "windows", LookPath: found, Start: func(pty.Options) (pty.Session, error) { started.Store(true); return newFakePTY(), nil }})
	if _, err := m.Start(StartOptions{ID: "t-1", Argv: []string{"opencode", "--prompt=a&calc"}}); err == nil || started.Load() {
		t.Fatalf("started=%v err=%v", started.Load(), err)
	}
}
