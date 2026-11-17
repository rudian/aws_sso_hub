package terminal

import (
	"bytes"
	goparser "go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/pty"
)

func waitFor(t *testing.T, what string, d time.Duration, f func() bool) {
	t.Helper()
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		if f() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestReaderNeverBlocksOnAConsumerThatNeverReads(t *testing.T) {
	// Emit blocks forever, as Wails does when its 64-event queue is full, and nobody calls Read.
	block := make(chan struct{})
	defer close(block)
	var emitted int
	var mu sync.Mutex
	m, ch := fakeManager(Config{LogCap: 1 << 20, Emit: func(string, any) {
		mu.Lock()
		emitted++
		mu.Unlock()
		<-block
	}})
	if _, err := m.Start(StartOptions{ID: "t1", Argv: []string{"x"}, Dir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	f := <-ch
	chunk := strings.Repeat("0123456789abcdef", 1024) // 16 KB
	t0 := time.Now()
	for i := 0; i < 1600; i++ { // 25 MB through the reader
		f.feed(chunk)
	}
	waitFor(t, "the reader to take everything", 20*time.Second, func() bool { return m.StatusOf("t1").TotalRead == int64(1600*len(chunk)) })
	t.Logf("25 MB read in %v with a stuck consumer; longest append+notify %v", time.Since(t0), m.StatusOf("t1").LongestAppend)
	st := m.StatusOf("t1")
	if st.Tail != st.TotalRead {
		t.Fatalf("tail %d total %d", st.Tail, st.TotalRead)
	}
	if st.Trims == 0 || st.Base == 0 {
		t.Fatal("expected the log to have been trimmed")
	}
	r := m.Read("t1", 0, 0)
	if r.Gap != st.Base || len(r.Prologue) == 0 {
		t.Fatalf("a late page must get a gap and a prologue: %+v", r.Result.Gap)
	}
	mu.Lock()
	n := emitted
	mu.Unlock()
	if n != 1 {
		t.Fatalf("only the pinger may be stuck in Emit, once: %d", n)
	}
}

func TestPingsAreCoalescedAndEndWithDone(t *testing.T) {
	var mu sync.Mutex
	var pings []api.TerminalPing
	m, ch := fakeManager(Config{PingEvery: 30 * time.Millisecond, Emit: func(name string, d any) {
		if name != api.EventTerminalPing {
			t.Errorf("event %q", name)
		}
		mu.Lock()
		pings = append(pings, d.(api.TerminalPing))
		mu.Unlock()
	}})
	if _, err := m.Start(StartOptions{ID: "t1", Argv: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	f := <-ch
	for i := 0; i < 500; i++ {
		f.feed("x")
	}
	waitFor(t, "output", 5*time.Second, func() bool { return m.StatusOf("t1").Tail == 500 })
	time.Sleep(100 * time.Millisecond)
	f.eof()
	waitFor(t, "the done ping", 5*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(pings) > 0 && pings[len(pings)-1].Done
	})
	mu.Lock()
	defer mu.Unlock()
	if len(pings) > 20 {
		t.Fatalf("500 chunks must not give %d events", len(pings))
	}
	last := pings[len(pings)-1]
	if last.Session != "t1" || last.Tail != 500 {
		t.Fatalf("%+v", last)
	}
	for _, p := range pings[:len(pings)-1] {
		if p.Done {
			t.Fatal("done before the end")
		}
	}
}

func TestReadDoneOnlyAfterEverythingWasRead(t *testing.T) {
	m, ch := fakeManager(Config{})
	if _, err := m.Start(StartOptions{ID: "t1", Argv: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	f := <-ch
	f.feed("hello world")
	f.eof()
	waitFor(t, "end", 5*time.Second, func() bool { return m.StatusOf("t1").Ended })
	r := m.Read("t1", 0, 5)
	if string(r.Data) != "hello" || r.Done {
		t.Fatalf("%+v done=%v", r.Result, r.Done)
	}
	r = m.Read("t1", r.Next, 100)
	if string(r.Data) != " world" || !r.Done {
		t.Fatalf("%q done=%v", r.Data, r.Done)
	}
	if r := m.Read("nope", 0, 10); !r.Done || len(r.Data) != 0 {
		t.Fatal("unknown session is reported as done")
	}
	if r := m.Read("t1", 0, 1<<30); len(r.Data) != 11 { // max is capped, not honoured beyond the hard cap
		t.Fatal(len(r.Data))
	}
}

func TestWriteIsQueuedInOrderAndNeverBlocks(t *testing.T) {
	m, ch := fakeManager(Config{})
	if _, err := m.Start(StartOptions{ID: "t1", Argv: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	f := <-ch
	f.blockIn = make(chan struct{}) // the child does not read its input
	done := make(chan error, 1)
	go func() {
		var err error
		for i := 0; i < 5; i++ {
			err = m.Write("t1", []byte{byte('a' + i)})
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Write blocked on a child that does not read")
	}
	// too much pending input is refused, not buffered without end
	if err := m.Write("t1", bytes.Repeat([]byte("z"), maxPendingInput)); err == nil {
		t.Fatal("expected a refusal")
	}
	close(f.blockIn)
	waitFor(t, "input", 5*time.Second, func() bool { return f.input() == "abcde" })
}

func TestResizeAndClosedConsole(t *testing.T) {
	m, ch := fakeManager(Config{})
	if _, err := m.Start(StartOptions{ID: "t1", Argv: []string{"x"}, Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	f := <-ch
	if err := m.Resize("t1", 132, 43); err != nil {
		t.Fatal(err)
	}
	if c, r := f.size(); c != 132 || r != 43 {
		t.Fatal(c, r)
	}
	f.resizeEr = pty.ErrClosed
	if err := m.Resize("t1", 100, 30); err != nil {
		t.Fatalf("a closed console must be swallowed: %v", err)
	}
	if err := m.Resize("t1", 0, 30); err == nil {
		t.Fatal("zero columns")
	}
	if err := m.Resize("t1", 100, 100000); err == nil {
		t.Fatal("huge rows")
	}
	if err := m.Resize("unknown", 100, 30); err != nil {
		t.Fatal(err)
	}
}

func TestStartRefusesBadInputAndRunningDuplicates(t *testing.T) {
	m, ch := fakeManager(Config{})
	for _, id := range []string{"", "../x", "a/b", ".hidden", strings.Repeat("a", 65), "a b"} {
		if _, err := m.Start(StartOptions{ID: id, Argv: []string{"x"}}); err == nil {
			t.Errorf("id %q accepted", id)
		}
	}
	if _, err := m.Start(StartOptions{ID: "ok"}); err == nil {
		t.Error("empty argv")
	}
	if _, err := m.Start(StartOptions{ID: "ok", Argv: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	f := <-ch
	if _, err := m.Start(StartOptions{ID: "ok", Argv: []string{"x"}}); err == nil {
		t.Error("a running session must not be replaced")
	}
	f.eof()
	waitFor(t, "end", 5*time.Second, func() bool { return m.StatusOf("ok").Ended })
	if err := m.Release("ok"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(StartOptions{ID: "ok", Argv: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	<-ch
}

func TestStartPassesTheExplicitEnvironment(t *testing.T) {
	var got pty.Options
	m := NewManager(Config{
		BaseEnv:  func() []string { return []string{"PATH=/usr/bin", "AWSSSOHUB_SANDBOX=/x", "WAILS_A=1", "TERM=dumb"} },
		ExtraEnv: []string{"HOME=/tmp/sandbox-home"},
		GOOS:     "darwin",
		Start: func(o pty.Options) (pty.Session, error) {
			got = o
			return newFakePTY(), nil
		},
	})
	if _, err := m.Start(StartOptions{ID: "t1", Argv: []string{"claude"}, Env: []string{"CODEPASS_TASK=t1"}, Dir: "/tmp/repo"}); err != nil {
		t.Fatal(err)
	}
	if got.Env == nil {
		t.Fatal("Env must be explicit: nil would inherit the app's environment")
	}
	e := envMap(got.Env)
	if e["TERM"] != "xterm-256color" || e["CODEPASS_TASK"] != "t1" || e["HOME"] != "/tmp/sandbox-home" || e["PATH"] != "/usr/bin" {
		t.Fatalf("%v", e)
	}
	for k := range e {
		if strings.HasPrefix(k, "AWSSSOHUB_") || strings.HasPrefix(k, "WAILS_") {
			t.Fatalf("%s leaked", k)
		}
	}
	if got.Dir != "/tmp/repo" || got.Cols != 120 || got.Rows != 30 {
		t.Fatalf("%+v", got)
	}
}

func TestStopKillsThenReleasesTheConsole(t *testing.T) {
	m, ch := fakeManager(Config{})
	if _, err := m.Start(StartOptions{ID: "t1", Argv: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	f := <-ch
	if err := m.Stop("t1"); err != nil {
		t.Fatal(err)
	}
	if f.killed.Load() != 1 || !f.closed.Load() || !m.StatusOf("t1").Ended {
		t.Fatal("kill, close, ended")
	}
	if err := m.Stop("t1"); err != nil { // twice is fine
		t.Fatal(err)
	}
	if err := m.Stop("unknown"); err != nil {
		t.Fatal(err)
	}
}

// A package that reads terminal bytes must not be able to write them to the app log: it imports no logger.
func TestNoLogging(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := goparser.ParseFile(token.NewFileSet(), name, nil, goparser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, im := range f.Imports {
			switch p := strings.Trim(im.Path.Value, `"`); {
			case p == "log", p == "log/slog", strings.Contains(p, "logx"):
				t.Errorf("%s imports %s", name, p)
			}
		}
	}
}

func TestScrollbackPathRejectsBadIDs(t *testing.T) {
	d := t.TempDir()
	if p := ScrollbackPath(d, "t-1"); p != filepath.Join(d, "sessions", "t-1", "scrollback") {
		t.Fatal(p)
	}
	for _, id := range []string{"", "..", "../x", "a/b", `a\b`, ".", "Task-1", "a.", "a.b", "_x", "-x", "é"} {
		if ScrollbackPath(d, id) != "" {
			t.Errorf("%q", id)
		}
	}
	if ScrollbackPath("", "ok") != "" {
		t.Error("no data folder")
	}
}

func TestScrollbackWrittenAtEndWithMode0600AndReadableAfterRestart(t *testing.T) {
	data := t.TempDir()
	m, ch := fakeManager(Config{DataDir: data, ScrollbackCap: 1 << 20})
	if _, err := m.Start(StartOptions{ID: "t-1", Argv: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	f := <-ch
	f.feed("\x1b[?1049hbuild output\r\n")
	f.eof()
	path := filepath.Join(data, "sessions", "t-1", "scrollback")
	waitFor(t, "scrollback", 5*time.Second, func() bool { _, err := os.Stat(path); return err == nil })
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0o600 && !windows() {
		t.Fatalf("mode %v", st.Mode().Perm())
	}
	if d, _ := os.Stat(filepath.Dir(path)); d.Mode().Perm() != 0o700 && !windows() {
		t.Fatalf("dir mode %v", d.Mode().Perm())
	}
	if ents, _ := os.ReadDir(filepath.Dir(path)); len(ents) != 1 {
		t.Fatalf("temporary files left behind: %v", ents)
	}
	// "restart": a new manager reads the ended session from the file
	m2 := NewManager(Config{DataDir: data})
	r := m2.Read("t-1", 0, 0)
	if string(r.Data) != "\x1b[?1049hbuild output\r\n" || !r.Done {
		t.Fatalf("%q done=%v", r.Data, r.Done)
	}
	if r := m2.Read("t-2", 0, 0); !r.Done || len(r.Data) != 0 {
		t.Fatal("no file, no data")
	}
}

func TestScrollbackIsCappedAndKeepsTheModes(t *testing.T) {
	data := t.TempDir()
	m, ch := fakeManager(Config{DataDir: data, ScrollbackCap: 2000})
	if _, err := m.Start(StartOptions{ID: "t-1", Argv: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	f := <-ch
	f.feed("\x1b[?1049h\x1b[?2004h")
	for i := 0; i < 100; i++ {
		f.feed(strings.Repeat("line\r\n", 50))
	}
	f.eof()
	path := filepath.Join(data, "sessions", "t-1", "scrollback")
	waitFor(t, "scrollback", 5*time.Second, func() bool { _, err := os.Stat(path); return err == nil })
	b, _ := os.ReadFile(path)
	if len(b) > 2000+400+sbHeaderLen || len(b) < 1000 {
		t.Fatalf("file is %d bytes", len(b))
	}
	from, pro, body := decodeScrollback(b)
	if from == 0 || len(body) == 0 || !bytes.HasPrefix(pro, []byte("\x1b[?")) || !strings.Contains(string(pro), "\x1b[?1049h") || !strings.Contains(string(pro), "\x1b[?2004h") {
		t.Fatalf("the file must hold the mode prologue and the offset of its data: from=%d %q", from, pro)
	}
}

func TestScrollbackSettingOffWritesNothingAndRemovesAnOldFile(t *testing.T) {
	data := t.TempDir()
	var on atomic.Bool
	on.Store(true)
	m, ch := fakeManager(Config{DataDir: data, ScrollbackEnabled: on.Load})
	run := func() {
		if _, err := m.Start(StartOptions{ID: "t-1", Argv: []string{"x"}}); err != nil {
			t.Fatal(err)
		}
		f := <-ch
		f.feed("secret output")
		f.eof()
		waitFor(t, "end", 5*time.Second, func() bool { return m.StatusOf("t-1").Ended })
		time.Sleep(50 * time.Millisecond)
	}
	run()
	path := filepath.Join(data, "sessions", "t-1", "scrollback")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	_ = m.Release("t-1")
	on.Store(false)
	run()
	if _, err := os.Stat(path); err == nil {
		t.Fatal("the setting is off: the old file must be gone")
	}
}

func windows() bool { return os.PathSeparator == '\\' }
