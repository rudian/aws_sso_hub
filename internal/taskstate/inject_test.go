package taskstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudePluginFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	task := t.TempDir()
	dir, err := writeClaudePlugin(task, "/Applications/Codepass.app/Contents/MacOS/codepass", "darwin")
	if err != nil {
		t.Fatal(err)
	}
	var man map[string]string
	b, _ := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	if json.Unmarshal(b, &man) != nil || man["name"] == "" || man["version"] == "" {
		t.Fatalf("manifest: %s", b)
	}
	var f struct {
		Hooks map[string][]struct {
			Matcher string
			Hooks   []struct {
				Type, Command string
				Timeout       int
			}
		}
	}
	b, _ = os.ReadFile(filepath.Join(dir, "hooks", "hooks.json"))
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	want := []string{"SessionStart", "UserPromptSubmit", "Notification", "PermissionRequest", "Stop", "StopFailure", "SessionEnd", "PreToolUse"}
	if len(f.Hooks) != len(want) {
		t.Fatalf("events: %v", f.Hooks)
	}
	for _, ev := range want {
		g := f.Hooks[ev]
		if len(g) != 1 || len(g[0].Hooks) != 1 {
			t.Fatalf("%s: %+v", ev, g)
		}
		h := g[0].Hooks[0]
		if h.Type != "command" || h.Timeout <= 0 || h.Timeout > 10 || !strings.HasSuffix(h.Command, " report claude") {
			t.Fatalf("%s: %+v", ev, h)
		}
		if (ev == "PreToolUse") != (g[0].Matcher == "AskUserQuestion") || (ev != "PreToolUse" && g[0].Matcher != "") {
			t.Fatalf("%s matcher %q", ev, g[0].Matcher)
		}
	}
	for _, bad := range []string{"PostToolUse", "SubagentStop", "PreCompact"} {
		if _, ok := f.Hooks[bad]; ok {
			t.Fatalf("%s subscribed", bad)
		}
	}
	if fi, _ := os.Stat(filepath.Join(dir, "hooks", "hooks.json")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}
	// Nothing was written into the home folder.
	if ents, _ := os.ReadDir(home); len(ents) != 0 {
		t.Fatalf("home touched: %v", ents)
	}
	if got := ClaudePluginArgs(dir); got[0] != "--plugin-dir" || got[1] != dir {
		t.Fatal(got)
	}
	p, err := WriteClaudeSettingsFallback(task, "/x/codepass")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); !strings.Contains(string(b), `"AskUserQuestion"`) {
		t.Fatal("fallback settings")
	}
}

func TestReportCommandQuoting(t *testing.T) {
	cases := []struct{ goos, exe, want string }{
		{"darwin", "/Applications/Codepass.app/Contents/MacOS/codepass", "/Applications/Codepass.app/Contents/MacOS/codepass report claude"},
		{"darwin", "/Users/alex dev/it's/codepass", `'/Users/alex dev/it'\''s/codepass' report claude`},
		{"windows", `C:\Program Files\Codepass\codepass.exe`, `"C:\Program Files\Codepass\codepass.exe" "report" "claude"`},
	}
	for _, c := range cases {
		if got := ReportCommand(c.goos, c.exe, "report", "claude"); got != c.want {
			t.Errorf("%s: %q want %q", c.goos, got, c.want)
		}
	}
}

func TestOpenCodePluginAndEnv(t *testing.T) {
	dir, err := WriteOpenCodePlugin(t.TempDir(), `/opt/my "app"/codepass`)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "index.js"))
	s := string(b)
	for _, must := range []string{`"/opt/my \"app\"/codepass"`, `"session.execution.started"`, `"permission.asked"`, "export default", `["report", "opencode"]`} {
		if !strings.Contains(s, must) {
			t.Errorf("plugin lacks %s", must)
		}
	}
	if strings.Contains(s, "inbox") || strings.Contains(s, ".text") || strings.Contains(s, "message") {
		t.Error("plugin reads prompt or message text")
	}
	env := OpenCodeConfigEnv(dir)
	var cfg map[string][]string
	if !strings.HasPrefix(env, "OPENCODE_CONFIG_CONTENT=") || json.Unmarshal([]byte(strings.TrimPrefix(env, "OPENCODE_CONFIG_CONTENT=")), &cfg) != nil || cfg["plugins"][0] != dir {
		t.Fatal(env)
	}
}

func TestCodexNotifyArgsAndTaskEnv(t *testing.T) {
	a := CodexNotifyArgs(`C:\Program Files\codepass.exe`)
	if a[0] != "-c" || a[1] != `notify=["C:\\Program Files\\codepass.exe","report","codex-notify"]` {
		t.Fatal(a)
	}
	env := TaskEnv("t1", "/s", true, true)
	if len(env) != 4 || env[0] != "CODEPASS_TASK=t1" || env[1] != "CODEPASS_SOCK=/s" {
		t.Fatal(env)
	}
	if len(TaskEnv("t1", "/s", false, false)) != 2 {
		t.Fatal("extras without arming")
	}
}
