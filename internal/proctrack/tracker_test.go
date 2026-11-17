package proctrack

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func fakeExists(t *testing.T, paths ...string) {
	t.Helper()
	old := pathExists
	pathExists = func(p string) bool {
		for _, x := range paths {
			if x == p {
				return true
			}
		}
		return false
	}
	t.Cleanup(func() { pathExists = old })
}

func TestParsePS(t *testing.T) {
	fakeExists(t, "/Applications/Visual Studio Code.app/Contents/MacOS/Electron")
	out := []byte(`    1     0     1     0 Mon Nov  3 08:00:00 2026   0.0  11000 /sbin/launchd
  501     1   501     0 Mon Nov 17 10:23:45 2026  12.5   2048 -zsh
  777   501   777     0 Mon Nov 17 10:24:01 2026   0.3  51200 /Applications/Visual Studio Code.app/Contents/MacOS/Electron
  garbage line
  888   501   888     0 Mon Nov 17 99:99:99 2026   0.3  51200 /bin/bad
`)
	got, err := ParsePS(out, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("parsed %d rows: %+v", len(got), got)
	}
	if got[1].PID != 501 || got[1].Name() != "zsh" || got[1].PPID != 1 || got[1].RSS != 2048*1024 || got[1].CPU != 12.5 {
		t.Errorf("row 1: %+v", got[1])
	}
	if got[2].Comm != "/Applications/Visual Studio Code.app/Contents/MacOS/Electron" || got[2].Name() != "Electron" {
		t.Errorf("a path with spaces: %+v", got[2])
	}
	if want := time.Date(2026, 11, 17, 10, 23, 45, 0, time.UTC).Unix(); got[1].Start != want {
		t.Errorf("start %d, want %d", got[1].Start, want)
	}
}

// Two programs, one session: a `sleep 600 &` and a server that called setsid, both left behind after the
// shell and the tool exited and were re-parented to launchd.
func TestSetsidAndReparentedAreLeftovers(t *testing.T) {
	f := newFakeSys(
		pr(100, 1, 100, 10, "/bin/zsh"),        // leader
		pr(110, 100, 110, 11, "/usr/bin/node"), // the tool
		pr(120, 110, 120, 12, "/bin/sleep"),    // sleep 600 &
		pr(130, 110, 130, 12, "/usr/bin/node"), // server; will call setsid
	)
	g := newFakeGroup(100, 110, 120, 130)
	tr := newTestTracker(t, f, "")
	tr.Track("s1", g)
	if err := tr.Tick(); err != nil {
		t.Fatal(err)
	}
	if got := pids(tr.Leftovers("s1")); !reflect.DeepEqual(got, []int{100, 110, 120, 130}) {
		t.Fatalf("live set %v", got)
	}
	// The server leaves the terminal session (setsid) and the tool, shell and terminal go away; the survivors
	// are re-parented to launchd.
	g.set(120)
	f.set(pr(130, 1, 130, 12, "/usr/bin/node"))
	f.set(pr(120, 1, 120, 12, "/bin/sleep"))
	f.del(100)
	f.del(110)
	close(g.done)
	if err := tr.Tick(); err != nil {
		t.Fatal(err)
	}
	r := tr.Leftovers("s1")
	if r.Live || r.Count != 2 || !reflect.DeepEqual(pids(r), []int{120, 130}) {
		t.Fatalf("after close: live=%v %v", r.Live, pids(r))
	}
	if tr.Active() != 2 {
		t.Errorf("Active = %d", tr.Active())
	}
	res, err := tr.ForceStop("s1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Found != 2 || res.Stopped != 2 || len(res.Survivors) != 0 {
		t.Errorf("result %+v", res)
	}
	if !reflect.DeepEqual(res.Names, []string{"node", "sleep"}) {
		t.Errorf("names %v", res.Names)
	}
	if len(f.procs) != 0 {
		t.Errorf("left in the table: %v", f.procs)
	}
	if tr.Leftovers("s1").Count != 0 || len(tr.All()) != 0 {
		t.Error("the session stayed in the tracker after its last program ended")
	}
}

// Review question: can ForceStop signal a process the session did not start?
func TestNeverTracksOrSignalsStrangers(t *testing.T) {
	f := newFakeSys(
		pr(100, 1, 100, 10, "/bin/zsh"),
		pr(120, 100, 120, 12, "/bin/sleep"),
		// The owner's own Terminal runs the same shell binary; its tree is unrelated.
		pr(200, 1, 200, 5, "/bin/zsh"),
		pr(210, 200, 210, 6, "/bin/sleep"),
		// Another session's program.
		pr(300, 1, 300, 7, "/bin/zsh"),
		pr(310, 300, 310, 8, "/usr/bin/node"),
		// A stranger in the same process group as a tracked program.
		pr(121, 1, 120, 13, "/bin/cat"),
	)
	tr := newTestTracker(t, f, "")
	tr.Track("mine", newFakeGroup(100, 120))
	tr.Track("other", newFakeGroup(300, 310))
	if err := tr.Tick(); err != nil {
		t.Fatal(err)
	}
	if got := pids(tr.Leftovers("mine")); !reflect.DeepEqual(got, []int{100, 120}) {
		t.Fatalf("mine = %v", got)
	}
	if got := pids(tr.Leftovers("other")); !reflect.DeepEqual(got, []int{300, 310}) {
		t.Fatalf("other = %v", got)
	}
	if _, err := tr.ForceStop("mine"); err != nil {
		t.Fatal(err)
	}
	for _, pid := range []int{200, 210, 300, 310, 121} {
		if f.signalled(pid) {
			t.Errorf("pid %d was signalled", pid)
		}
	}
	for _, s := range f.signals {
		if s.pid == -120 {
			t.Error("the group of 120 was signalled although a stranger shares it")
		}
	}
	if _, ok := f.procs[121]; !ok {
		t.Error("the stranger in the shared group died")
	}
	if _, ok := f.procs[120]; ok {
		t.Error("the tracked program survived")
	}
}

// A child of a tracked program is added only when it started no earlier than its parent: a PID that was
// reused (same number, later start) is a different process.
func TestPIDReuseWithLaterStartTime(t *testing.T) {
	f := newFakeSys(pr(100, 1, 100, 10, "/bin/zsh"), pr(120, 100, 120, 12, "/bin/sleep"))
	g := newFakeGroup(100, 120)
	tr := newTestTracker(t, f, "")
	tr.Track("s", g)
	_ = tr.Tick()
	// 120 ends; the number is handed to an unrelated program (parent 1) with a later start time.
	g.set(100)
	f.set(pr(120, 1, 120, 99, "/usr/bin/vim"))
	_ = tr.Tick()
	if got := pids(tr.Leftovers("s")); !reflect.DeepEqual(got, []int{100}) {
		t.Fatalf("the reused PID entered the set: %v", got)
	}
	// Even a stale entry is never signalled when the start time changed under it.
	f2 := newFakeSys(pr(100, 1, 100, 10, "/bin/zsh"), pr(120, 100, 120, 12, "/bin/sleep"))
	tr2 := newTestTracker(t, f2, "")
	g2 := newFakeGroup(100, 120)
	tr2.Track("s", g2)
	_ = tr2.Tick()
	g2.set(100) // the system no longer reports 120 as a member of the session: its number now belongs to a stranger
	f2.set(pr(120, 1, 120, 99, "/usr/bin/vim"))
	f2.del(100)
	if _, err := tr2.ForceStop("s"); err != nil {
		t.Fatal(err)
	}
	if f2.signalled(120) {
		t.Error("ForceStop signalled a reused PID")
	}
	if _, ok := f2.procs[120]; !ok {
		t.Error("the stranger with the reused PID died")
	}
}

func TestForceStopEscalatesToSIGKILL(t *testing.T) {
	f := newFakeSys(pr(100, 1, 100, 10, "/bin/zsh"), pr(120, 100, 120, 12, "/bin/stubborn"))
	f.ignoreTerm[120] = true
	tr := newTestTracker(t, f, "")
	tr.Track("s", newFakeGroup(100, 120))
	_ = tr.Tick()
	start := time.Now()
	res, err := tr.ForceStop("s")
	if err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took < 250*time.Millisecond {
		t.Errorf("SIGKILL came after %v, before the grace period ended", took)
	}
	if res.Stopped != 2 || len(res.Survivors) != 0 {
		t.Errorf("result %+v", res)
	}
	var term, kill bool
	for _, s := range f.signals {
		if s.pid == -120 || s.pid == 120 {
			term = term || s.sig == syscall.SIGTERM
			kill = kill || s.sig == syscall.SIGKILL
		}
	}
	if !term || !kill {
		t.Errorf("SIGTERM=%v SIGKILL=%v: %v", term, kill, f.signals)
	}
}

// A program that cannot be stopped is reported, with its PID.
func TestForceStopReportsSurvivors(t *testing.T) {
	f := newFakeSys(pr(120, 1, 120, 12, "/bin/zombieish"))
	tr := newTestTracker(t, f, "")
	tr.o.Signal = func(pid int, sig syscall.Signal) error { return nil } // nothing ever dies
	tr.Track("s", newFakeGroup(120))
	_ = tr.Tick()
	res, _ := tr.ForceStop("s")
	if len(res.Survivors) != 1 || res.Survivors[0].PID != 120 || res.Stopped != 0 {
		t.Errorf("result %+v", res)
	}
}

func TestOneListingPerTick(t *testing.T) {
	f := newFakeSys(pr(100, 1, 100, 10, "/bin/zsh"), pr(120, 100, 120, 12, "/bin/sleep"), pr(121, 120, 120, 12, "/bin/sleep"))
	tr := newTestTracker(t, f, "")
	tr.Track("a", newFakeGroup(100))
	tr.Track("b", newFakeGroup())
	for i := 1; i <= 3; i++ {
		_ = tr.Tick()
		if f.reads != i {
			t.Fatalf("after %d ticks: %d listings", i, f.reads)
		}
	}
	if got := pids(tr.Leftovers("a")); !reflect.DeepEqual(got, []int{100, 120, 121}) {
		t.Errorf("a = %v", got)
	}
}

func TestNoTickWithoutSessions(t *testing.T) {
	f := newFakeSys()
	tr := newTestTracker(t, f, "")
	_ = tr.Tick()
	if f.reads != 0 {
		t.Error("listed the process table with nothing tracked")
	}
}

func TestPersistAndRestore(t *testing.T) {
	dir := t.TempDir()
	f := newFakeSys(pr(100, 1, 100, 10, "/bin/zsh"), pr(120, 100, 120, 12, "/usr/local/bin/vite --secret-token=abc"))
	tr := newTestTracker(t, f, dir)
	tr.Track("s1", newFakeGroup(100, 120))
	_ = tr.Tick()
	file := filepath.Join(dir, "sessions", "leftovers.json")
	st, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode().Perm())
	}
	b, _ := os.ReadFile(file)
	if !strings.Contains(string(b), `"name": "vite"`) || !strings.Contains(string(b), `"pid": 120`) {
		t.Errorf("file lacks the set: %s", b)
	}
	if strings.Contains(string(b), "--secret-token") || strings.Contains(string(b), "/usr/local/bin") {
		t.Errorf("the file holds a command line or a path: %s", b)
	}

	// Next start: 100 is gone, 120 survived, and the number 130 was reused with another start time.
	f2 := newFakeSys(pr(120, 1, 120, 12, "/usr/local/bin/vite"), pr(100, 1, 100, 77, "/bin/zsh"))
	tr2 := newTestTracker(t, f2, dir)
	n, err := tr2.Restore()
	if err != nil || n != 1 {
		t.Fatalf("restored %d, %v", n, err)
	}
	r := tr2.Leftovers("s1")
	if r.Live || !reflect.DeepEqual(pids(r), []int{120}) {
		t.Fatalf("restored %+v", r)
	}
	if tr2.Active() != 1 {
		t.Errorf("Active = %d", tr2.Active())
	}
	res, err := tr2.ForceStop("s1")
	if err != nil || res.Stopped != 1 {
		t.Fatalf("stop: %+v %v", res, err)
	}
	if f2.signalled(100) {
		t.Error("a PID with a different start time was signalled")
	}
	b, _ = os.ReadFile(file)
	if strings.Contains(string(b), `"pid"`) {
		t.Errorf("the file still lists programs after the stop: %s", b)
	}
}

func TestFileOnlyChangesWhenTheSetChanges(t *testing.T) {
	dir := t.TempDir()
	f := newFakeSys(pr(100, 1, 100, 10, "/bin/zsh"))
	tr := newTestTracker(t, f, dir)
	tr.Track("s", newFakeGroup(100))
	_ = tr.Tick()
	file := filepath.Join(dir, "sessions", "leftovers.json")
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(file, old, old)
	p := f.procs[100]
	p.CPU = 55
	f.set(p)
	_ = tr.Tick()
	st, _ := os.Stat(file)
	if !st.ModTime().Before(time.Now().Add(-30 * time.Minute)) {
		t.Error("the file was rewritten for a CPU change")
	}
}

func TestRestoreDamagedFile(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "sessions"), 0o700)
	_ = os.WriteFile(filepath.Join(dir, "sessions", "leftovers.json"), []byte("{not json"), 0o600)
	tr := newTestTracker(t, newFakeSys(), dir)
	if n, err := tr.Restore(); n != 0 || err != nil {
		t.Errorf("restore: %d %v", n, err)
	}
}

func TestWindowsJobStop(t *testing.T) {
	f := newFakeSys(pr(120, 1, 0, 12, "C:\\x\\node.exe"))
	yes := true
	tr := New(Options{Source: f, UseJob: &yes, Signal: func(int, syscall.Signal) error { t.Error("signalled on the job path"); return nil },
		Hard: 200 * time.Millisecond, Poll: 5 * time.Millisecond})
	g := newFakeGroup(120)
	killJob := func() { f.del(120); g.set() }
	tr.Track("s", &killingGroup{fakeGroup: g, onKill: killJob})
	_ = tr.Tick()
	res, err := tr.ForceStop("s")
	if err != nil || res.Stopped != 1 || len(res.Survivors) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	if g.killed != 1 {
		t.Errorf("KillSession called %d times", g.killed)
	}
}

type killingGroup struct {
	*fakeGroup
	onKill func()
}

func (k *killingGroup) KillSession() error { _ = k.fakeGroup.KillSession(); k.onKill(); return nil }

// Review: the saved name comes from the program alone. On macOS comm is argv[0], which programs rewrite to carry
// their arguments.
func TestNameNeverHoldsArguments(t *testing.T) {
	fakeExists(t, "/Applications/Visual Studio Code.app/Contents/MacOS/Electron")
	out := []byte(`  10  1  10  0 Mon Nov 17 10:00:00 2026  0.0  100 npm exec vite --port 9996 --strictPort
  11  1  11  0 Mon Nov 17 10:00:00 2026  0.0  100 redis-server 127.0.0.1:6390
  12  1  12  0 Mon Nov 17 10:00:00 2026  0.0  100 /usr/local/bin/vite --token=probe-not-a-secret
  13  1  13  0 Mon Nov 17 10:00:00 2026  0.0  100 -zsh
  14  1  14  0 Mon Nov 17 10:00:00 2026  0.0  100 /Applications/Visual Studio Code.app/Contents/MacOS/Electron
`)
	got, err := ParsePS(out, time.UTC)
	if err != nil || len(got) != 5 {
		t.Fatalf("%v %v", got, err)
	}
	for i, want := range []string{"npm", "redis-server", "vite", "zsh", "Electron"} {
		if got[i].Name() != want {
			t.Errorf("row %d: name %q, want %q (comm %q)", i, got[i].Name(), want, got[i].Comm)
		}
	}
}

// OnChange may call back into the tracker: it is never called with a tracker lock held.
func TestOnChangeMayCallBack(t *testing.T) {
	dir := t.TempDir()
	f := newFakeSys(pr(100, 1, 100, 10, "/bin/zsh"), pr(120, 100, 120, 12, "/bin/sleep"))
	tr := newTestTracker(t, f, dir)
	calls := 0
	tr.SetOnChange(func() {
		calls++
		if calls > 3 {
			return
		}
		_ = tr.Tick()
		_, _ = tr.ForceStop("none")
		tr.Forget("none")
		_, _ = tr.Restore()
	})
	done := make(chan struct{})
	go func() {
		tr.Track("s", newFakeGroup(100, 120))
		_ = tr.Tick()
		tr.MarkClosed("s")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("OnChange calling back into the tracker deadlocked")
	}
	if calls == 0 {
		t.Error("OnChange never ran")
	}
}

// Review round 2: OnChange may call ForceStop and ForceStopAll for a session that still has programs.
func TestOnChangeMayForceStopLiveSession(t *testing.T) {
	f := newFakeSys(pr(100, 1, 100, 10, "/bin/zsh"), pr(120, 100, 120, 12, "/bin/sleep"))
	tr := newTestTracker(t, f, t.TempDir())
	var calls atomic.Int32
	tr.SetOnChange(func() {
		if calls.Add(1) > 6 {
			return
		}
		_, _ = tr.ForceStop("s")
		_, _ = tr.ForceStopAll()
	})
	tr.Track("s", newFakeGroup(100, 120))
	done := make(chan struct{})
	go func() {
		_ = tr.Tick()
		_, _ = tr.ForceStop("s")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("OnChange calling ForceStop deadlocked")
	}
	if calls.Load() == 0 {
		t.Error("OnChange never ran")
	}
}

// Review: "Stop all and quit" waits about one Grace plus Hard, not one per session.
func TestForceStopAllRunsSessionsTogether(t *testing.T) {
	f := newFakeSys()
	tr := newTestTracker(t, f, "")
	for i := 0; i < 3; i++ {
		base := 100 + i*10
		f.set(pr(base, 1, base, 10, "/bin/zsh"))
		f.set(pr(base+1, base, base+1, 11, "/bin/sleep"))
		f.ignoreTerm[base], f.ignoreTerm[base+1] = true, true
		tr.Track(string(rune('a'+i)), newFakeGroup(base, base+1))
	}
	_ = tr.Tick()
	start := time.Now()
	rs, err := tr.ForceStopAll()
	took := time.Since(start)
	if err != nil || len(rs) != 3 {
		t.Fatalf("%v %v", rs, err)
	}
	for _, r := range rs {
		if r.Found != 2 || r.Stopped != 2 || len(r.Survivors) != 0 {
			t.Errorf("%+v", r)
		}
	}
	// Grace is 300 ms: one after the other would take 900 ms or more.
	if took > 700*time.Millisecond {
		t.Errorf("ForceStopAll took %v for 3 stubborn sessions", took)
	}
}

// Review: Forget and MarkClosed reach leftovers.json at once.
func TestForgetAndMarkClosedPersist(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sessions", "leftovers.json")
	f := newFakeSys(pr(100, 1, 100, 10, "/bin/zsh"))
	tr := newTestTracker(t, f, dir)
	tr.Track("s", newFakeGroup(100))
	_ = tr.Tick()
	tr.MarkClosed("s")
	if b, _ := os.ReadFile(file); !strings.Contains(string(b), "closedAt") {
		t.Errorf("MarkClosed not saved: %s", b)
	}
	tr.Forget("s")
	if b, _ := os.ReadFile(file); strings.Contains(string(b), `"pid"`) {
		t.Errorf("Forget left the set in the file: %s", b)
	}
	tr2 := newTestTracker(t, f, dir)
	if n, _ := tr2.Restore(); n != 0 {
		t.Errorf("restored %d after Forget", n)
	}
}

// Review: a track while Run waits in the idle interval is seen at once.
func TestRunTicksSoonAfterTrack(t *testing.T) {
	f := newFakeSys(pr(100, 1, 100, 10, "/bin/zsh"))
	tr := New(Options{Source: f, Live: time.Hour, Idle: time.Hour, Signal: f.signal})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { tr.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	time.Sleep(50 * time.Millisecond) // Run is now in its hour-long wait
	tr.Track("s", newFakeGroup(100))
	waitUntil(t, func() bool { return tr.Leftovers("s").Count == 1 })
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}

func TestCommandsOnDemandMatchStartTime(t *testing.T) {
	f := newFakeSys(pr(100, 1, 100, 10, "/bin/zsh"), pr(120, 100, 120, 12, "/usr/local/bin/node"))
	var asked []int
	reads := 0
	tr := New(Options{Source: f, Signal: f.signal, Commands: func(p []int) ([]CommandLine, error) {
		asked = p
		reads++
		return []CommandLine{{PID: 100, Start: 10, Line: "-zsh"}, {PID: 120, Start: 12, Line: "node ./node_modules/.bin/vite --port 5173"}, {PID: 120, Start: 99, Line: "other"}}, nil
	}})
	tr.Track("s", newFakeGroup(100, 120))
	_ = tr.Tick()
	if reads != 0 {
		t.Fatal("command lines read on a tick")
	}
	got, err := tr.Commands("s")
	if err != nil || reads != 1 || !reflect.DeepEqual(asked, []int{100, 120}) {
		t.Fatalf("%v %v asked %v", got, err, asked)
	}
	if len(got) != 2 || got[1].Line != "node ./node_modules/.bin/vite --port 5173" {
		t.Errorf("%+v", got)
	}
}

func TestParseCommands(t *testing.T) {
	out := []byte(`  120 Mon Nov 17 10:00:12 2026 node ./node_modules/.bin/vite --port 5173
  130 Mon Nov 17 10:00:13 2026 sh -c "npm run dev"
  bad
`)
	got := ParseCommands(out, time.UTC)
	if len(got) != 2 || got[0].PID != 120 || got[1].Line != `sh -c "npm run dev"` {
		t.Errorf("%+v", got)
	}
}

// Review: a damaged file is set aside and replaced; a file of a newer version is never touched.
func TestRestoreSetsDamagedFileAsideAndKeepsNewer(t *testing.T) {
	dir := t.TempDir()
	sd := filepath.Join(dir, "sessions")
	_ = os.MkdirAll(sd, 0o700)
	file := filepath.Join(sd, "leftovers.json")
	_ = os.WriteFile(file, []byte("{not json"), 0o600)
	f := newFakeSys(pr(100, 1, 100, 10, "/bin/zsh"))
	tr := newTestTracker(t, f, dir)
	if n, err := tr.Restore(); n != 0 || err != nil {
		t.Fatalf("restore: %d %v", n, err)
	}
	if m, _ := filepath.Glob(file + ".corrupt-*"); len(m) != 1 {
		t.Errorf("the damaged file was not set aside: %v", m)
	}
	tr.Track("s", newFakeGroup(100))
	_ = tr.Tick()
	if b, _ := os.ReadFile(file); !strings.Contains(string(b), `"schemaVersion": 1`) || !strings.Contains(string(b), `"pid": 100`) {
		t.Errorf("no fresh file: %s", b)
	}

	dir2 := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir2, "sessions"), 0o700)
	file2 := filepath.Join(dir2, "sessions", "leftovers.json")
	newer := []byte(`{"schemaVersion": 99, "sessions": []}`)
	_ = os.WriteFile(file2, newer, 0o600)
	tr2 := newTestTracker(t, f, dir2)
	if _, err := tr2.Restore(); err == nil {
		t.Error("a newer file was accepted")
	}
	tr2.Track("s", newFakeGroup(100))
	_ = tr2.Tick()
	if b, _ := os.ReadFile(file2); string(b) != string(newer) {
		t.Errorf("the newer file was changed: %s", b)
	}
}
