//go:build !windows

package tasks

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/aitools"
	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/notify"
	"github.com/rudian/codepass/internal/taskstate"
	"github.com/rudian/codepass/internal/terminal"
)

// realRig runs the service on the real terminal manager and real pseudo terminals, but with a fake tool: shell
// scripts in a temp folder, started through a login shell whose HOME and ZDOTDIR are a temp folder with its own
// shell files. Nothing of the owner's shell, tools or data folder is read.
type realRig struct {
	t     *testing.T
	svc   *Service
	mgr   *terminal.Manager
	data  string
	pids  string
	procs *fakeProcs
	awake *fakeAwake
}

func newRealRig(t *testing.T, cap int) *realRig {
	t.Helper()
	zsh := "/bin/zsh"
	if _, err := os.Stat(zsh); err != nil {
		t.Skip("no zsh")
	}
	home := t.TempDir()
	bin := t.TempDir()
	pids := t.TempDir()
	repo := t.TempDir()
	data := t.TempDir()
	// the fake tool: records its own and a background child's PID, then waits
	script := "#!/bin/sh\n" +
		"echo $$ > " + pids + "/$CODEPASS_TASK.tool\n" +
		"echo \"$@\" > " + pids + "/$CODEPASS_TASK.args\n" +
		"echo ready\n" +
		"sleep 600 &\n" +
		"echo $! > " + pids + "/$CODEPASS_TASK.child\n" +
		"wait\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".zshenv"), []byte("export PATH="+bin+":/usr/bin:/bin:/usr/sbin:/sbin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{".zprofile", ".zshrc", ".zlogin"} {
		_ = os.WriteFile(filepath.Join(home, f), nil, 0o600)
	}
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", home)
	t.Setenv("BASH_ENV", "")
	t.Setenv("ENV", "")
	base := []string{"HOME=" + home, "ZDOTDIR=" + home, "PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LANG=en_US.UTF-8"}
	mgr := terminal.NewManager(terminal.Config{DataDir: data, BaseEnv: func() []string { return base }, Grace: 300 * time.Millisecond})
	relay := &Relay{}
	tracker := taskstate.NewTracker(taskstate.Options{}, relay.OnChange)
	procs, awake := newFakeProcs(), &fakeAwake{}
	backend := notify.NewFake()
	nsvc := notify.New(notify.Options{Backend: backend, Go: func(f func()) { f() }})
	svc := New(Deps{
		DataDir: data, Terminals: mgr, Tracker: tracker, Relay: relay, Endpoint: filepath.Join(home, "x.sock"), Exe: "/nonexistent/codepass",
		Awake: awake, Procs: procs, Notify: nsvc, Repos: fakeRepos{"repo-1": {ID: "repo-1", Path: repo, Name: "acme_api"}},
		Launch: func(q aitools.LaunchRequest) (aitools.Launch, error) {
			q.Shell = zsh
			return aitools.BuildLaunch(q)
		},
		SoftCap: func() int { return cap }, PollEvery: time.Hour,
	})
	t.Cleanup(func() { svc.StopAll(); svc.Close() })
	return &realRig{t: t, svc: svc, mgr: mgr, data: data, pids: pids, procs: procs, awake: awake}
}

func (r *realRig) pid(task, kind string) int {
	r.t.Helper()
	var n int
	eventually(r.t, kind+" pid of "+task, func() bool {
		b, err := os.ReadFile(filepath.Join(r.pids, task+"."+kind))
		if err != nil {
			return false
		}
		v, err := strconv.Atoi(strings.TrimSpace(string(b)))
		n = v
		return err == nil && v > 0
	})
	return n
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func (r *realRig) gone(pid int) {
	r.t.Helper()
	eventually(r.t, "process "+strconv.Itoa(pid)+" to be gone", func() bool { return !alive(pid) })
}

func TestRealStopKillsTheToolAndTheProgramsItStarted(t *testing.T) {
	r := newRealRig(t, 6)
	tk, err := r.svc.Create(context.Background(), api.CreateTaskRequest{Repo: "repo-1", Tool: api.AIToolClaude, Description: "do it"})
	if err != nil {
		t.Fatal(err)
	}
	tool, child := r.pid(tk.ID, "tool"), r.pid(tk.ID, "child")
	b, _ := os.ReadFile(filepath.Join(r.pids, tk.ID+".args"))
	if !strings.Contains(string(b), "--session-id") || !strings.Contains(string(b), "do it") {
		t.Fatalf("args: %s", b)
	}
	if !alive(tool) || !alive(child) {
		t.Fatal("running")
	}
	got, err := r.svc.Stop(tk.ID)
	if err != nil || got.State != api.TaskStopped {
		t.Fatalf("%+v %v", got, err)
	}
	r.gone(tool)
	r.gone(child)
	if r.svc.TasksRunning() {
		t.Fatal("nothing runs")
	}
	// the terminal's saved scrollback belongs to the task's own folder
	eventually(t, "scrollback", func() bool {
		_, err := os.Stat(filepath.Join(r.data, "sessions", tk.ID, "scrollback"))
		return err == nil
	})
}

func TestRealStopAllLeavesNoProcessOfAnyTask(t *testing.T) {
	r := newRealRig(t, 6)
	var all []int
	var ids []string
	for i := 0; i < 3; i++ {
		tk, err := r.svc.Create(context.Background(), api.CreateTaskRequest{Repo: "repo-1", Tool: api.AIToolClaude, Description: "x"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, tk.ID)
		all = append(all, r.pid(tk.ID, "tool"), r.pid(tk.ID, "child"))
	}
	res := r.svc.StopAll()
	if res.Stopped != 3 {
		t.Fatalf("%+v", res)
	}
	for _, p := range all {
		r.gone(p)
	}
	for _, id := range ids {
		if got, _ := r.svc.Get(id); got.State != api.TaskStopped {
			t.Fatalf("%+v", got)
		}
	}
	if r.procs.forceAll != 1 {
		t.Fatal("ForceStopAll after the stops")
	}
	// after "stop all" a task resumes into its own session
	if _, err := r.svc.Resume(ids[0], false); err != nil {
		t.Fatal(err)
	}
	b := r.pid(ids[0], "child")
	_ = b
	eventually(t, "resume args", func() bool {
		a, _ := os.ReadFile(filepath.Join(r.pids, ids[0]+".args"))
		return strings.Contains(string(a), "--resume")
	})
}

func TestRealExitOfTheToolEndsTheTask(t *testing.T) {
	r := newRealRig(t, 6)
	tk, err := r.svc.Create(context.Background(), api.CreateTaskRequest{Repo: "repo-1", Tool: api.AIToolClaude, Description: "x"})
	if err != nil {
		t.Fatal(err)
	}
	tool := r.pid(tk.ID, "tool")
	if err := syscall.Kill(tool, syscall.SIGTERM); err != nil { // the tool we started, by its own PID
		t.Fatal(err)
	}
	eventually(t, "the task ended", func() bool { g, _ := r.svc.Get(tk.ID); return !g.Live })
	if g, _ := r.svc.Get(tk.ID); g.State != api.TaskFailed && g.State != api.TaskDone {
		t.Fatalf("%+v", g)
	}
	// the cleanup's StopAll stops what is left, through the session
}
