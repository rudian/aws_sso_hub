//go:build !windows

package aitools

import (
	"context"
	"strings"
	"testing"
)

// A user's own `claude` function passes flags of its own. Run the launch argv through a real login shell: the
// function's flags come first, and then ours (session ID, both app plugin folders, "--", the message).
func TestLaunchThroughAUsersClaudeFunction(t *testing.T) {
	h := newFakeHome(t)
	h.tool("claude", `:`)
	h.zshrc(`claude() { command claude --plugin-dir /user/plug --settings '{"x":1}' "$@"; }`)
	l, err := BuildLaunch(LaunchRequest{
		Tool: Claude, GOOS: "darwin", Shell: "/bin/zsh", Dir: h.Home, TaskID: "t-1", SessionID: uuid1,
		PluginDirs: []string{"/data/tasks/t-1/plugin", "/data/second"}, FirstMessage: "Fix the login bug",
	})
	if err != nil {
		t.Fatal(err)
	}
	out, exit, err := RealRun(context.Background(), l.Argv, h.Home, append(h.Env(), l.Env...))
	if err != nil || exit != 0 {
		t.Fatalf("%v exit %d: %s", err, exit, out)
	}
	got := h.calls()
	if len(got) != 1 {
		t.Fatalf("%q", got)
	}
	want := "claude [--plugin-dir] [/user/plug] [--settings] [{\"x\":1}] [--session-id] [" + uuid1 + "] [--plugin-dir] [/data/tasks/t-1/plugin] [--plugin-dir] [/data/second] [--] [Fix the login bug]"
	if got[0] != want {
		t.Fatalf("\n got %s\nwant %s", got[0], want)
	}
	if strings.Count(got[0], "[--settings]") != 1 {
		t.Fatal("the app must not add a --settings of its own")
	}
}
