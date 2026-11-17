package app

import (
	"io"
	"time"

	"github.com/rudian/codepass/internal/taskstate"
)

// Report mode (plan 4.8): the app's own binary started by an AI tool's hook as `<exe> report <tool>`. It reads
// the hook payload, reduces it to the fields taskstate.Report allows, sends that to the running app over the
// local socket and exits at once. It never waits for the app, never prints, never logs and always exits 0 (a
// hook's non-zero exit can block the tool). Nothing but the reduced report leaves this function.

const stdinCap = 8 << 20 // a Stop payload can hold a long assistant message; the cap only bounds memory

// IsReportMode says whether the command line asks for report mode. main.go calls it before anything else
// starts (no paths, no log file, no services).
func IsReportMode(args []string) bool { return len(args) >= 2 && args[1] == "report" }

// RunReportMode runs report mode. args are the arguments after "report": `claude`, `codex`, `opencode`
// (payload on stdin) or `codex-notify <json>` (payload in the last argument). getenv reads the task's
// environment. It returns the process exit code, always 0.
func RunReportMode(args []string, stdin io.Reader, getenv func(string) string) int {
	task, sock := getenv(taskstate.EnvTask), getenv(taskstate.EnvSocket)
	if len(args) == 0 || task == "" || sock == "" {
		return 0
	}
	opt := taskstate.SanitizeOptions{
		Task:       task,
		Queue:      getenv(taskstate.EnvQueue) == "1",
		WantPrompt: getenv(taskstate.EnvNamePrompt) == "1",
	}
	var (
		r  taskstate.Report
		ok bool
	)
	switch args[0] {
	case "codex-notify":
		if len(args) < 2 {
			return 0
		}
		r, ok = taskstate.SanitizeCodexNotify([]byte(args[len(args)-1]), opt)
	case string(taskstate.ToolClaude), string(taskstate.ToolCodex), string(taskstate.ToolOpenCode):
		payload := readLimited(stdin)
		r, ok = taskstate.Sanitize(taskstate.Tool(args[0]), payload, opt)
	}
	if ok {
		_ = taskstate.Send(sock, r) // the app may not be running: nothing to do then
	}
	return 0
}

// readLimited reads stdin with a size cap and a time cap, so a hook whose stdin never closes still exits.
func readLimited(r io.Reader) []byte {
	if r == nil {
		return nil
	}
	ch := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(io.LimitReader(r, stdinCap))
		ch <- b
	}()
	select {
	case b := <-ch:
		return b
	case <-time.After(2 * time.Second):
		return nil
	}
}
