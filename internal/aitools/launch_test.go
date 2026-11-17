package aitools

import (
	"encoding/json"
	"errors"
	"github.com/rudian/codepass/internal/execx"
	"reflect"
	"strings"
	"testing"
)

const uuid1 = "0b9d3f2e-7c41-4a52-9f0e-3a1d2c4b5e66"

func tailAfterLaunch(argv []string) []string { // the tool's own arguments inside the login-shell form
	return argv[7:]
}

func TestLaunchClaude(t *testing.T) {
	l, err := BuildLaunch(LaunchRequest{
		Tool: Claude, GOOS: "darwin", Shell: "/bin/zsh", Dir: "/tmp/wt/acme/fix-login", TaskID: "t-1",
		SessionID: uuid1, PluginDirs: []string{"/data/tasks/t-1/plugin", "/data/second"}, FirstMessage: "Fix the login bug",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/bin/zsh", "-l", "-i", "-c", `"$@"`, "codepass", "claude",
		"--session-id", uuid1, "--plugin-dir", "/data/tasks/t-1/plugin", "--plugin-dir", "/data/second", "--", "Fix the login bug"}
	if !reflect.DeepEqual(l.Argv, want) {
		t.Fatalf("argv\n got %q\nwant %q", l.Argv, want)
	}
	if l.Dir != "/tmp/wt/acme/fix-login" || !reflect.DeepEqual(l.Env, []string{"CODEPASS_TASK=t-1"}) {
		t.Fatalf("dir/env: %q %q", l.Dir, l.Env)
	}
	for _, a := range l.Argv {
		if a == "--settings" {
			t.Fatal("hooks must travel as a plugin, never as --settings (two --settings do not merge)")
		}
	}
}

func TestLaunchClaudeResumeKeepsID(t *testing.T) {
	l, err := BuildLaunch(LaunchRequest{Tool: Claude, GOOS: "darwin", SessionID: uuid1, Resume: true, PluginDirs: []string{"/p"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(tailAfterLaunch(l.Argv), " "); got != "--resume "+uuid1+" --plugin-dir /p" {
		t.Fatal(got)
	}
	if _, err := BuildLaunch(LaunchRequest{Tool: Claude, GOOS: "darwin", SessionID: uuid1, Resume: true, FirstMessage: "x"}); !errors.Is(err, ErrBadMessage) {
		t.Fatalf("a resume with a new message: %v", err)
	}
	if _, err := BuildLaunch(LaunchRequest{Tool: Claude, GOOS: "darwin", Resume: true}); !errors.Is(err, ErrBadSessionID) {
		t.Fatalf("a resume without an ID: %v", err)
	}
}

func TestLaunchOpenCodeIsAlwaysStandalone(t *testing.T) {
	for _, req := range []LaunchRequest{
		{Tool: OpenCode},
		{Tool: OpenCode, SessionID: "ses_abc123"},
		{Tool: OpenCode, SessionID: "ses_abc123", Resume: true},
		{Tool: OpenCode, SessionID: "ses_abc123", FirstMessage: "Fix it"},
	} {
		req.GOOS = "darwin"
		l, err := BuildLaunch(req)
		if err != nil {
			t.Fatal(err)
		}
		if got := tailAfterLaunch(l.Argv); got[0] != "--standalone" {
			t.Fatalf("--standalone is not first: %q", got)
		}
	}
	l, _ := BuildLaunch(LaunchRequest{Tool: OpenCode, GOOS: "darwin", SessionID: "ses_abc123", FirstMessage: "Fix it"})
	if got := strings.Join(tailAfterLaunch(l.Argv), "|"); got != "--standalone|--session|ses_abc123|--prompt=Fix it" {
		t.Fatal(got)
	}
	if _, err := BuildLaunch(LaunchRequest{Tool: OpenCode, GOOS: "darwin", SessionID: "../x"}); !errors.Is(err, ErrBadSessionID) {
		t.Fatalf("bad ID: %v", err)
	}
}

func TestLaunchCodex(t *testing.T) {
	l, err := BuildLaunch(LaunchRequest{Tool: Codex, GOOS: "darwin", FirstMessage: "Fix it"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(tailAfterLaunch(l.Argv), "|"); got != "--|Fix it" {
		t.Fatal(got)
	}
	l, err = BuildLaunch(LaunchRequest{Tool: Codex, GOOS: "darwin", Resume: true, SessionID: uuid1, CodexConfig: []string{"features.hooks=true"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(tailAfterLaunch(l.Argv), "|"); got != "-c|features.hooks=true|resume|"+uuid1 {
		t.Fatal(got)
	}
	if _, err := BuildLaunch(LaunchRequest{Tool: Codex, GOOS: "darwin", SessionID: uuid1}); !errors.Is(err, ErrBadSessionID) {
		t.Fatalf("codex assigns no ID at the start: %v", err)
	}
}

// -c notify=[...] replaces a user's own notify, so it is used only when the user has none.
func TestLaunchCodexNotifyOnlyWhenTheUserHasNone(t *testing.T) {
	notify := []string{"/Apps/Codepass.app/Contents/MacOS/codepass", "report", `C:\x"y`}
	l, err := BuildLaunch(LaunchRequest{Tool: Codex, GOOS: "darwin", CodexNotify: notify})
	if err != nil {
		t.Fatal(err)
	}
	args := tailAfterLaunch(l.Argv)
	if len(args) != 2 || args[0] != "-c" || args[1] != `notify=["/Apps/Codepass.app/Contents/MacOS/codepass","report","C:\\x\"y"]` {
		t.Fatalf("%q", args)
	}
	l, err = BuildLaunch(LaunchRequest{Tool: Codex, GOOS: "darwin", CodexNotify: notify, CodexHasNotify: true, FirstMessage: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(tailAfterLaunch(l.Argv), "|"); got != "--|hi" || !l.NotifySkipped {
		t.Fatalf("a user's notify must stay: %q skipped=%v", got, l.NotifySkipped)
	}
}

func TestLaunchCodexRefusesSandboxOverrides(t *testing.T) {
	for _, kv := range []string{`sandbox_mode="danger-full-access"`, `approval_policy="never"`, `profiles.x.sandbox_mode="read-only"`, `sandbox_permissions=["a"]`, `x="danger-full-access"`, "no-equals", "bad key=1"} {
		if _, err := BuildLaunch(LaunchRequest{Tool: Codex, GOOS: "darwin", CodexConfig: []string{kv}}); !errors.Is(err, ErrBadConfig) {
			t.Errorf("%q: %v", kv, err)
		}
	}
}

// A description that is itself a forbidden flag is message text and nothing else, for every tool and platform.
func TestDescriptionThatIsAFlagReachesTheToolAsTextOnly(t *testing.T) {
	var descs []string
	for _, a := range forbiddenArgLists {
		descs = append(descs, strings.Join(a, " "), a[0])
	}
	descs = append(descs, "-", "--", "-- --", "---", "--prompt=x", "-c sandbox_mode=x")
	for _, goos := range []string{"darwin", "windows"} {
		for _, tool := range All() {
			for _, d := range descs {
				req := LaunchRequest{Tool: tool, GOOS: goos, ToolPath: `C:\bin\x.exe`, Dir: map[string]string{"darwin": "/tmp/wt", "windows": `C:\wt`}[goos], FirstMessage: d}
				if tool == Claude {
					req.SessionID = uuid1
					req.PluginDirs = []string{map[string]string{"darwin": "/p", "windows": `C:\p`}[goos]}
				}
				l, err := BuildLaunch(req)
				if err != nil {
					t.Fatalf("%s %s %q: %v", goos, tool, d, err)
				}
				args := l.Argv
				if goos == "darwin" {
					args = tailAfterLaunch(args)
				} else {
					args = args[1:]
				}
				if err := CheckArgs(args); err != nil {
					t.Fatalf("%s %s %q: %v", goos, tool, d, err)
				}
				found := 0
				want := d
				if tool == Claude && !strings.ContainsAny(d, " \t\n") {
					want = d + " " // a one-word message never equals a command name (claudeMessage)
				}
				for i, a := range args {
					if a == want && i > 0 && args[i-1] == "--" && tool != OpenCode {
						found++
					}
					if tool == OpenCode && a == "--prompt="+d {
						found++
					}
				}
				if found != 1 {
					t.Fatalf("%s %s: %q is not carried as one message token: %q", goos, tool, d, args)
				}
			}
		}
	}
}

func TestLaunchWindowsStartsTheProgramDirectly(t *testing.T) {
	l, err := BuildLaunch(LaunchRequest{Tool: OpenCode, GOOS: "windows", ToolPath: `C:\Tools\bin\opencode.cmd`, Dir: `C:\wt\acme`, FirstMessage: "x"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{`C:\Tools\bin\opencode.cmd`, "--standalone", "--prompt=x"}
	if !reflect.DeepEqual(l.Argv, want) {
		t.Fatalf("%q", l.Argv)
	}
	l, _ = BuildLaunch(LaunchRequest{Tool: Codex, GOOS: "windows", Lookup: func(string) (string, error) { return "", execx.ErrNotFound }})
	if !reflect.DeepEqual(l.Argv, []string{"codex"}) {
		t.Fatalf("%q", l.Argv)
	}
}

func TestLaunchAccountEnvironment(t *testing.T) {
	l, err := BuildLaunch(LaunchRequest{Tool: Claude, GOOS: "darwin", TaskID: "t1", AccountEnv: []string{"CLAUDE_CONFIG_DIR=/data/ai-accounts/claude/b"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(l.Env, []string{"CODEPASS_TASK=t1", "CLAUDE_CONFIG_DIR=/data/ai-accounts/claude/b"}) {
		t.Fatalf("%q", l.Env)
	}
	l, _ = BuildLaunch(LaunchRequest{Tool: Codex, GOOS: "darwin", AccountEnv: []string{"CODEX_HOME=/data/ai-accounts/codex/b"}})
	if !reflect.DeepEqual(l.Env, []string{"CODEX_HOME=/data/ai-accounts/codex/b"}) {
		t.Fatalf("%q", l.Env)
	}
	for _, bad := range []struct {
		tool ID
		env  string
	}{
		{Claude, "CODEX_HOME=/x"}, {Codex, "CLAUDE_CONFIG_DIR=/x"}, {OpenCode, "CLAUDE_CONFIG_DIR=/x"},
		{Claude, "CLAUDE_CONFIG_DIR=relative"}, {Claude, "ANTHROPIC_API_KEY=sk"}, {Claude, "CLAUDE_CONFIG_DIR"}, {Claude, "CLAUDE_CONFIG_DIR=/x\n/y"},
	} {
		if _, err := BuildLaunch(LaunchRequest{Tool: bad.tool, GOOS: "darwin", AccountEnv: []string{bad.env}}); !errors.Is(err, ErrBadAccount) {
			t.Errorf("%s %q: %v", bad.tool, bad.env, err)
		}
	}
}

func TestLaunchRefusesBadInput(t *testing.T) {
	cases := []LaunchRequest{
		{Tool: "vim"},
		{Tool: Claude, Dir: "relative/dir"},
		{Tool: Claude, Dir: "/tmp/a\nb"},
		{Tool: Claude, TaskID: "a b"},
		{Tool: Claude, TaskID: "$(x)"},
		{Tool: Claude, SessionID: "not-a-uuid"},
		{Tool: Claude, FirstMessage: "a\x00b"},
		{Tool: Claude, FirstMessage: strings.Repeat("x", MaxMessageBytes+1)},
		{Tool: Claude, PluginDirs: []string{"rel"}},
	}
	for i, c := range cases {
		c.GOOS = "darwin"
		if _, err := BuildLaunch(c); err == nil {
			t.Errorf("case %d: no error", i)
		}
	}
}

func TestOpenCodeConfigAddsToTheUsersPlugins(t *testing.T) {
	s, err := OpenCodeConfigContent("/data/oc-plugin", []string{"my-plugin", "/x/y", "my-plugin"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string][]string
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m["plugins"], []string{"my-plugin", "/x/y", "/data/oc-plugin"}) {
		t.Fatalf("%v", m)
	}
	l, err := BuildLaunch(LaunchRequest{Tool: OpenCode, GOOS: "darwin", OpenCodeConfig: s})
	if err != nil {
		t.Fatal(err)
	}
	if l.Env[len(l.Env)-1] != "OPENCODE_CONFIG_CONTENT="+s {
		t.Fatalf("%q", l.Env)
	}
	if _, err := BuildLaunch(LaunchRequest{Tool: OpenCode, GOOS: "darwin", OpenCodeConfig: "{nope"}); err == nil {
		t.Fatal("bad JSON accepted")
	}
}

func TestNewSessionID(t *testing.T) {
	a, _ := NewSessionID(Claude)
	b, _ := NewSessionID(Claude)
	if !reUUID.MatchString(a) || a == b || a[14] != '4' {
		t.Fatalf("%q %q", a, b)
	}
	o, _ := NewSessionID(OpenCode)
	if !reOCID.MatchString(o) {
		t.Fatal(o)
	}
	if c, _ := NewSessionID(Codex); c != "" {
		t.Fatal("codex has no ID to assign")
	}
	if _, err := NewSessionID("x"); err == nil {
		t.Fatal()
	}
}

func TestInstallCommands(t *testing.T) {
	want := map[string]map[ID]string{
		"darwin": {Claude: "brew install --cask claude-code", Codex: "brew install --cask codex", OpenCode: "brew install opencode"},
		"windows": {Claude: "winget install --id Anthropic.ClaudeCode -e", Codex: "winget install --id OpenAI.Codex -e",
			OpenCode: "npm install -g opencode-ai"},
	}
	for goos, m := range want {
		for id, cmd := range m {
			ins, ok := InstallFor(id, goos)
			if !ok || ins.Display() != cmd {
				t.Errorf("%s %s: %q", goos, id, ins.Display())
			}
		}
	}
	if _, ok := InstallFor(Claude, "linux"); ok {
		t.Fatal("linux")
	}
}

// Claude Code runs a subcommand when the first operand is a command name, even after "--". A one-word message
// therefore never reaches it as a bare word.
func TestLaunchClaudeOneWordMessageIsNotACommandName(t *testing.T) {
	for _, w := range []string{"update", "install", "doctor", "mcp", "config", "plugin", "help", "agents", "auth", "setup-token", "migrate-installer", "x"} {
		l, err := BuildLaunch(LaunchRequest{Tool: Claude, GOOS: "darwin", FirstMessage: w})
		if err != nil {
			t.Fatal(err)
		}
		args := tailAfterLaunch(l.Argv)
		if got := args[len(args)-1]; got != w+" " || args[len(args)-2] != "--" {
			t.Errorf("%s: %q", w, args)
		}
	}
	l, _ := BuildLaunch(LaunchRequest{Tool: Claude, GOOS: "darwin", FirstMessage: "update the docs"})
	if args := tailAfterLaunch(l.Argv); args[len(args)-1] != "update the docs" {
		t.Fatalf("a message with spaces is passed as it is: %q", args)
	}
	// Codex and OpenCode are not affected: their message forms never name a command
	l, _ = BuildLaunch(LaunchRequest{Tool: Codex, GOOS: "darwin", FirstMessage: "update"})
	if args := tailAfterLaunch(l.Argv); !reflect.DeepEqual(args, []string{"--", "update"}) {
		t.Fatalf("%q", args)
	}
	l, _ = BuildLaunch(LaunchRequest{Tool: OpenCode, GOOS: "darwin", FirstMessage: "update"})
	if args := tailAfterLaunch(l.Argv); !reflect.DeepEqual(args, []string{"--standalone", "--prompt=update"}) {
		t.Fatalf("%q", args)
	}
}

// OpenCode's config can carry the plugins the app injects and nothing else: "permission" is the config form of
// --auto.
func TestLaunchOpenCodeRefusesPermissionConfig(t *testing.T) {
	for _, cfg := range []string{
		`{"permission":"allow"}`, `{"permission":{"*":"allow"}}`, `{"permissions":[{"effect":"allow"}]}`,
		`{"plugins":["/p"],"permission":"allow"}`, `{"agent":{"build":{"permission":"allow"}}}`, `["plugins"]`, `{"theme":"x"}`,
	} {
		if _, err := BuildLaunch(LaunchRequest{Tool: OpenCode, GOOS: "darwin", OpenCodeConfig: cfg}); !errors.Is(err, ErrBadPlugin) {
			t.Errorf("%s: %v", cfg, err)
		}
	}
	if _, err := BuildLaunch(LaunchRequest{Tool: OpenCode, GOOS: "darwin", OpenCodeConfig: `{"plugins":["/p"]}`}); err != nil {
		t.Fatal(err)
	}
}

// Windows: a .cmd shim goes through cmd.exe, which re-reads the command line. Message text never goes there.
func TestLaunchWindowsBatchShimNeverGetsMessageText(t *testing.T) {
	shim := `C:\Tools\npm\opencode.cmd`
	none := func(string) bool { return false }
	for _, msg := range []string{`Fix the "Save & Close" button`, "50% done", "a | b", "a < b", "a ^ b", "wow!", "line one\nline two", "line one\r\nline two"} {
		if _, err := BuildLaunch(LaunchRequest{Tool: OpenCode, GOOS: "windows", ToolPath: shim, FirstMessage: msg, Exists: none}); !errors.Is(err, ErrBadMessage) {
			t.Errorf("%q: %v", msg, err)
		}
	}
	// plain text still launches, as the shim
	l, err := BuildLaunch(LaunchRequest{Tool: OpenCode, GOOS: "windows", ToolPath: shim, FirstMessage: "Fix the login bug", Exists: none})
	if err != nil || l.Argv[0] != shim {
		t.Fatalf("%q %v", l.Argv, err)
	}
	// with the package's entry point found, every message is fine and goes to node.exe, never to cmd.exe
	entry := `C:\Tools\npm\node_modules\opencode-ai\bin\opencode`
	has := func(p string) bool { return p == entry }
	l, err = BuildLaunch(LaunchRequest{Tool: OpenCode, GOOS: "windows", ToolPath: shim, FirstMessage: `Fix the "Save & Close" button` + "\nsecond line", Exists: has, Lookup: lookupTo("")})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{nodeExe, entry, "--standalone", "--prompt=Fix the \"Save & Close\" button\nsecond line"}
	if !reflect.DeepEqual(l.Argv, want) {
		t.Fatalf("%q", l.Argv)
	}
	has = func(p string) bool { return p == entry || p == `C:\Tools\npm\node.exe` }
	l, _ = BuildLaunch(LaunchRequest{Tool: OpenCode, GOOS: "windows", ToolPath: shim, Exists: has})
	if l.Argv[0] != `C:\Tools\npm\node.exe` {
		t.Fatalf("%q", l.Argv)
	}
	// a real program is started as it is, whatever the message holds
	l, err = BuildLaunch(LaunchRequest{Tool: Claude, GOOS: "windows", ToolPath: `C:\bin\claude.exe`, FirstMessage: `a & b "c"`})
	if err != nil || l.Argv[0] != `C:\bin\claude.exe` {
		t.Fatalf("%q %v", l.Argv, err)
	}
}
