//go:build !windows

package app

import (
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/taskstate"
)

type collect struct {
	mu sync.Mutex
	rs []taskstate.Report
}

func (c *collect) Apply(r taskstate.Report) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rs = append(c.rs, r)
	return true
}
func (c *collect) get() []taskstate.Report {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]taskstate.Report(nil), c.rs...)
}

func startServer(t *testing.T) (*taskstate.Server, *collect) {
	t.Helper()
	d, err := os.MkdirTemp("", "rm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	srv, err := taskstate.Listen(taskstate.Endpoint(d))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	c := &collect{}
	srv.Serve(c, nil)
	return srv, c
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func wait(t *testing.T, f func() bool) {
	t.Helper()
	for i := 0; i < 200 && !f(); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if !f() {
		t.Fatal("timeout")
	}
}

func TestIsReportMode(t *testing.T) {
	if !IsReportMode([]string{"codepass", "report", "claude"}) || IsReportMode([]string{"codepass"}) || IsReportMode([]string{"codepass", "x", "report"}) {
		t.Fatal("dispatch")
	}
}

func TestReportModeForwardsOnlyTheReducedFields(t *testing.T) {
	srv, c := startServer(t)
	e := env(map[string]string{taskstate.EnvTask: "t1", taskstate.EnvSocket: srv.Endpoint()})
	payload := `{"hook_event_name":"UserPromptSubmit","session_id":"s-1","permission_mode":"plan","prompt":"SECRET prompt for acme","cwd":"/Users/alex/acme"}`
	if code := RunReportMode([]string{"claude"}, strings.NewReader(payload), e); code != 0 {
		t.Fatal(code)
	}
	wait(t, func() bool { return len(c.get()) == 1 })
	r := c.get()[0]
	if r.Prompt != "" || r.Task != "t1" || r.Session != "s-1" || r.Mode != "plan" || r.Event != "UserPromptSubmit" {
		t.Fatalf("%+v", r)
	}
}

func TestReportModeArmedPromptAndNotify(t *testing.T) {
	srv, c := startServer(t)
	e := env(map[string]string{taskstate.EnvTask: "t1", taskstate.EnvSocket: srv.Endpoint(), taskstate.EnvNamePrompt: "1"})
	RunReportMode([]string{"claude"}, strings.NewReader(`{"hook_event_name":"UserPromptSubmit","prompt":"fix the login"}`), e)
	RunReportMode([]string{"codex-notify", `{"type":"agent-turn-complete","thread-id":"th1","last-assistant-message":"SECRET"}`}, nil, e)
	wait(t, func() bool { return len(c.get()) == 2 })
	rs := c.get()
	if rs[0].Prompt != "fix the login" && rs[1].Prompt != "fix the login" {
		t.Fatalf("%+v", rs)
	}
	for _, r := range rs {
		if r.Event == taskstate.EventCodexTurnComplete && (r.Session != "th1" || r.Tool != taskstate.ToolCodex) {
			t.Fatalf("%+v", r)
		}
	}
}

func TestReportModeNeverFailsOrWaits(t *testing.T) {
	start := time.Now()
	// No app listening, no task, no socket, garbage, a stdin that never closes: always 0, quickly.
	pr, pw := io.Pipe()
	defer pw.Close()
	cases := []struct {
		args []string
		in   io.Reader
		env  map[string]string
	}{
		{[]string{"claude"}, strings.NewReader(`{"hook_event_name":"Stop"}`), map[string]string{taskstate.EnvTask: "t", taskstate.EnvSocket: "/nonexistent/x.sock"}},
		{[]string{"claude"}, strings.NewReader(`{"hook_event_name":"Stop"}`), map[string]string{}},
		{[]string{"claude"}, strings.NewReader("garbage"), map[string]string{taskstate.EnvTask: "t", taskstate.EnvSocket: "/nonexistent/x.sock"}},
		{nil, nil, map[string]string{taskstate.EnvTask: "t", taskstate.EnvSocket: "/x"}},
		{[]string{"codex-notify"}, nil, map[string]string{taskstate.EnvTask: "t", taskstate.EnvSocket: "/x"}},
		{[]string{"other"}, nil, map[string]string{taskstate.EnvTask: "t", taskstate.EnvSocket: "/x"}},
		{[]string{"claude"}, pr, map[string]string{taskstate.EnvTask: "t", taskstate.EnvSocket: "/x"}},
	}
	for i, c := range cases {
		if code := RunReportMode(c.args, c.in, env(c.env)); code != 0 {
			t.Errorf("case %d: exit %d", i, code)
		}
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %v", d)
	}
}
