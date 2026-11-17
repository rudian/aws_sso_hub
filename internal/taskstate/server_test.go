//go:build !windows

package taskstate

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func shortDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "ts")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return d
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout")
}

type recSink struct {
	mu sync.Mutex
	rs []Report
}

func (s *recSink) Apply(r Report) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rs = append(s.rs, r)
	return true
}
func (s *recSink) n() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.rs) }

func TestSocketIsPrivateAndCarriesAReport(t *testing.T) {
	dir := shortDir(t)
	srv, err := Listen(Endpoint(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	sink := &recSink{}
	srv.Serve(sink, nil)
	fi, err := os.Stat(srv.Endpoint())
	if err != nil || fi.Mode().Perm() != 0o600 || fi.Mode()&os.ModeSocket == 0 {
		t.Fatalf("socket mode: %v %v", fi, err)
	}
	di, _ := os.Stat(filepath.Dir(srv.Endpoint()))
	if di.Mode().Perm() != 0o700 {
		t.Fatalf("folder mode %v", di.Mode())
	}
	if err := Send(srv.Endpoint(), Report{Task: "t1", Tool: ToolClaude, Event: "Stop", Session: "s1", At: 5}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return sink.n() == 1 })
	if sink.rs[0].Task != "t1" || sink.rs[0].Event != "Stop" {
		t.Fatalf("%+v", sink.rs[0])
	}
}

func TestOtherUsersAreDropped(t *testing.T) {
	old := peerUID
	defer func() { peerUID = old }()
	cases := map[string]func(*net.UnixConn) (uint32, error){
		"another user": func(*net.UnixConn) (uint32, error) { return uint32(os.Geteuid()) + 1, nil },
		"no creds":     func(*net.UnixConn) (uint32, error) { return 0, fmt.Errorf("no creds") },
	}
	for name, stub := range cases {
		var calls atomic.Int32
		peerUID = func(c *net.UnixConn) (uint32, error) { calls.Add(1); return stub(c) }
		srv, err := Listen(Endpoint(shortDir(t))) // the stub is read once, here
		if err != nil {
			t.Fatal(err)
		}
		sink := &recSink{}
		srv.Serve(sink, nil)
		_ = Send(srv.Endpoint(), Report{Task: "t1", Tool: ToolClaude, Event: "Stop"})
		waitFor(t, func() bool { return calls.Load() > 0 })
		time.Sleep(100 * time.Millisecond)
		srv.Close()
		if sink.n() != 0 {
			t.Fatalf("%s: report accepted", name)
		}
	}
}

func TestRealPeerCredentialsOfOwnProcess(t *testing.T) {
	srv, err := Listen(Endpoint(shortDir(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	sink := &recSink{}
	srv.Serve(sink, nil)
	_ = Send(srv.Endpoint(), Report{Task: "t1", Tool: ToolClaude, Event: "Stop"})
	waitFor(t, func() bool { return sink.n() == 1 }) // the real check passed for the same user
}

func TestStaleSocketReplacedLiveOneRefused(t *testing.T) {
	dir := shortDir(t)
	ep := Endpoint(dir)
	if err := os.MkdirAll(filepath.Dir(ep), 0o700); err != nil {
		t.Fatal(err)
	}
	// A dead app leaves a socket file behind.
	l, _ := net.Listen("unix", ep)
	l.(*net.UnixListener).SetUnlinkOnClose(false)
	l.Close()
	srv, err := Listen(ep)
	if err != nil {
		t.Fatalf("stale: %v", err)
	}
	defer srv.Close()
	if _, err := Listen(ep); err != ErrInUse {
		t.Fatalf("live: %v", err)
	}
	srv.Serve(&recSink{}, nil)
}

func TestFolderOfAnotherModeOrALinkIsRefused(t *testing.T) {
	dir := shortDir(t)
	target := filepath.Join(dir, "real")
	_ = os.MkdirAll(target, 0o700)
	_ = os.Symlink(target, filepath.Join(dir, "run"))
	if _, err := Listen(Endpoint(dir)); err == nil {
		t.Fatal("a symlinked run folder was accepted")
	}
}

func TestLongDataPathUsesAShortSocket(t *testing.T) {
	ep := Endpoint("/" + strings.Repeat("a", 200))
	if len(ep) > maxSocketPath {
		t.Fatalf("%d", len(ep))
	}
}

func TestProblemsNeverCarryMessageText(t *testing.T) {
	srv, err := Listen(Endpoint(shortDir(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	var mu sync.Mutex
	var probs []string
	sink := &recSink{}
	srv.Serve(sink, func(p string) { mu.Lock(); probs = append(probs, p); mu.Unlock() })
	raw := func(b string) {
		c, err := net.Dial("unix", srv.Endpoint())
		if err != nil {
			t.Fatal(err)
		}
		c.Write([]byte(b))
		c.Close()
	}
	raw(`{"task": "` + secret)
	raw(`{"task":"t","tool":"claude","event":"PostToolUse","prompt":"` + secret + `"}`)
	raw(strings.Repeat("x", maxWireLen+10))
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(probs) == 3 })
	for _, p := range probs {
		if strings.Contains(p, "SECRET") || strings.Contains(p, "task") {
			t.Fatalf("problem text %q", p)
		}
	}
	if sink.n() != 0 {
		t.Fatal("bad message reached the sink")
	}
}

func TestEndToEndPromptNeverReachesChangesOrProblems(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	tr := NewTracker(Options{}, func(i Info) { mu.Lock(); seen = append(seen, fmt.Sprintf("%+v", i)); mu.Unlock() })
	tr.Register("t1", ToolClaude, ModeHooks)
	var named string
	var wg sync.WaitGroup
	wg.Add(1)
	tr.ExpectFirstPrompt("t1", func(p string) { named = p; wg.Done() })
	srv, err := Listen(Endpoint(shortDir(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	srv.Serve(tr, func(p string) { mu.Lock(); seen = append(seen, p); mu.Unlock() })

	o := SanitizeOptions{Task: "t1", WantPrompt: true}
	r, _ := Sanitize(ToolClaude, []byte(`{"hook_event_name":"UserPromptSubmit","session_id":"s1","permission_mode":"plan","prompt":"`+secret+`"}`), o)
	if err := Send(srv.Endpoint(), r); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if named != secret {
		t.Fatalf("naming call got %q", named)
	}
	waitFor(t, func() bool { i, _ := tr.Get("t1"); return i.State == StateRunning })
	mu.Lock()
	defer mu.Unlock()
	for _, s := range seen {
		if strings.Contains(s, "SECRET") {
			t.Fatalf("prompt reached a state change: %s", s)
		}
	}
}

func TestSendWithoutAnAppFailsFast(t *testing.T) {
	start := time.Now()
	if err := Send(Endpoint(shortDir(t)), Report{Task: "t", Tool: ToolClaude, Event: "Stop"}); err == nil {
		t.Fatal("expected an error")
	}
	if time.Since(start) > time.Second {
		t.Fatal("slow")
	}
}
