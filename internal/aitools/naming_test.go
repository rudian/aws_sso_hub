package aitools

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestParseNamingTable(t *testing.T) {
	long := strings.Repeat("a", 60)
	cases := []struct {
		name, out, desc string
		ok              bool
		title, slug     string
	}{
		{"english", `{"title":"Fix login bug","slug":"fix-login-bug"}`, "x", true, "Fix login bug", "fix-login-bug"},
		{"chinese title english slug", `{"title":"修复登录页面的崩溃","slug":"fix-login-crash"}`, "x", true, "修复登录页面的崩溃", "fix-login-crash"},
		{"mixed", `{"title":"给 API 加 rate limit","slug":"add-api-rate-limit"}`, "x", true, "给 API 加 rate limit", "add-api-rate-limit"},
		{"emoji title", `{"title":"🚀 Ship it","slug":"ship-it"}`, "x", true, "🚀 Ship it", "ship-it"},
		{"fenced", "```json\n{\"title\":\"Fix it\",\"slug\":\"fix-it\"}\n```", "x", true, "Fix it", "fix-it"},
		{"sentence around", "Sure! Here you go: {\"title\":\"Fix it\",\"slug\":\"fix-it\"} Hope that helps", "x", true, "Fix it", "fix-it"},
		{"noise before", "welcome to zsh {not json}\n{\"title\":\"Fix it\",\"slug\":\"fix-it\"}", "x", true, "Fix it", "fix-it"},
		{"title too long is cut", `{"title":"` + strings.Repeat("长", 80) + `","slug":"x"}`, "x", true, strings.Repeat("长", 40), "x"},
		{"title with newline", "{\"title\":\"Fix\\nthe\\tbug\",\"slug\":\"fix-bug\"}", "x", true, "Fix the bug", "fix-bug"},
		{"slug too long is rebuilt", `{"title":"T","slug":"` + strings.Repeat("word-", 20) + `"}`, "x", true, "T", strings.Repeat("word-", 7) + "word"},
		{"one huge word", `{"title":"T","slug":"` + long + long + `"}`, "x", true, "T", long[:40]},
		{"slug with branch-breaking chars", `{"title":"Fix it","slug":"Fix..it/now @{x}.lock"}`, "x", true, "Fix it", "fix-it-now-x-lock"},
		{"slug uppercase and spaces", `{"title":"Fix it","slug":"Fix Login Bug"}`, "x", true, "Fix it", "fix-login-bug"},
		{"slug chinese only: from the title", `{"title":"Add search box","slug":"添加搜索"}`, "x", true, "Add search box", "add-search-box"},
		{"slug and title chinese: from the description", `{"title":"添加搜索","slug":"添加搜索"}`, "add search please", true, "添加搜索", "add-search-please"},
		{"nothing ascii anywhere", `{"title":"添加搜索","slug":""}`, "添加搜索", true, "添加搜索", "task-7"},
		{"slug missing", `{"title":"Fix it"}`, "x", true, "Fix it", "fix-it"},
		{"quoted title", `{"title":"\"Fix it\"","slug":"fix-it"}`, "x", true, "Fix it", "fix-it"},
		{"empty answer", ``, "x", false, "", ""},
		{"not json", `Fix the login bug`, "x", false, "", ""},
		{"json array", `["a"]`, "x", false, "", ""},
		{"broken json", `{"title":"Fix it","slug":`, "x", false, "", ""},
		{"empty title", `{"title":"  ","slug":"fix-it"}`, "x", false, "", ""},
		{"title is not a string", `{"title":5,"slug":"fix-it"}`, "x", false, "", ""},
		{"control characters only", `{"title":"\u0000\u0007","slug":"fix-it"}`, "x", false, "", ""},
		{"punctuation only title", `{"title":"...","slug":"fix-it"}`, "x", false, "", ""},
		// codex exec echoes the prompt it was given before its answer: the answer is the last object
		{"codex transcript with the echoed prompt", "user\n" + NamingPrompt + "Fix the login bug\ncodex\n{\"title\":\"Fix login bug\",\"slug\":\"fix-login-bug\"}\n", "Fix the login bug", true, "Fix login bug", "fix-login-bug"},
		{"codex transcript with an example object in the prompt", "user\nReply like {\"title\": \"...\", \"slug\": \"...\"}\ncodex\n{\"title\":\"Fix it\",\"slug\":\"fix-it\"}", "x", true, "Fix it", "fix-it"},
		{"an object without a title does not win", "{\"title\":\"Fix it\",\"slug\":\"fix-it\"}\n{\"tokens\": 12}", "x", true, "Fix it", "fix-it"},
		{"a nested title is not the answer", "{\"title\":\"Outer\",\"slug\":\"outer\",\"meta\":{\"title\":\"Inner\"}}", "x", true, "Outer", "outer"},
	}
	for _, c := range cases {
		got, ok := ParseNaming(c.out, c.desc, 7)
		if ok != c.ok {
			t.Errorf("%s: ok=%v", c.name, ok)
			continue
		}
		if !ok {
			continue
		}
		if got.Title != c.title || got.Slug != c.slug || !got.FromAI {
			t.Errorf("%s: %+v want %q %q", c.name, got, c.title, c.slug)
		}
		if !ValidSlug(got.Slug) || utf8.RuneCountInString(got.Title) > MaxTitle {
			t.Errorf("%s: invalid result %+v", c.name, got)
		}
	}
}

func TestFallbackNamingTable(t *testing.T) {
	cases := []struct {
		desc, title, slug string
	}{
		{"Fix the login bug", "Fix the login bug", "fix-the-login-bug"},
		{"修复登录页面的崩溃", "修复登录页面的崩溃", "task-3"},
		{"给 API 加 rate limit\nand more lines", "给 API 加 rate limit", "api-rate-limit-and-more-lines"},
		{"🚀🚀🚀", "🚀🚀🚀", "task-3"},
		{strings.Repeat("word ", 30), strings.TrimSpace(strings.Repeat("word ", 8)), strings.Repeat("word-", 7) + "word"},
		{"\n\n  second line first\n", "second line first", "second-line-first"},
		{strings.Repeat("x", 100), strings.Repeat("x", 40), strings.Repeat("x", 40)},
		{"--dangerously-skip-permissions", "--dangerously-skip-permissions", "dangerously-skip-permissions"},
		{"Fix: a/b..c @{x}.lock", "Fix: a/b..c @{x}.lock", "fix-a-b-c-x-lock"},
		{"   \t ", "", "task-3"},
	}
	for _, c := range cases {
		got := FallbackNaming(c.desc, 3)
		if c.title == "" {
			c.title = "task-3"
		}
		if got.Title != c.title || got.Slug != c.slug || got.FromAI {
			t.Errorf("%q: %+v want %q %q", c.desc, got, c.title, c.slug)
		}
		if !ValidSlug(got.Slug) {
			t.Errorf("%q: slug %q", c.desc, got.Slug)
		}
	}
}

func TestEmptyDescriptionSkipsTheCall(t *testing.T) {
	ran := false
	nm := Namer{GOOS: "darwin", Run: func(context.Context, []string, string, []string) ([]byte, int, error) { ran = true; return nil, 0, nil }}
	for _, d := range []string{"", "  \n\t"} {
		if got := nm.Name(context.Background(), Claude, d, 4); got.Slug != "new-task-4" || got.Title != "New task 4" || got.Reason != "empty" {
			t.Fatalf("%+v", got)
		}
	}
	nm.Lang = "zh-CN"
	if got := nm.Name(context.Background(), Claude, "", 4); got.Title != "新任务 4" || got.Slug != "new-task-4" {
		t.Fatalf("%+v", got)
	}
	if ran {
		t.Fatal("an empty description must not call the tool")
	}
}

func TestNamingArgsPerTool(t *testing.T) {
	p := "PROMPT"
	// no tools: Claude Code's built-in tools off and no MCP server; Codex read-only; OpenCode denied by config
	c, _ := NamingArgs(Claude, p, "", "")
	if !reflect.DeepEqual(c, []string{"-p", "--model", "haiku", "--tools", "", "--strict-mcp-config", "--no-session-persistence", "--output-format", "text", "--", p}) {
		t.Fatalf("%q", c)
	}
	x, _ := NamingArgs(Codex, p, "", "")
	if !reflect.DeepEqual(x, []string{"exec", "--skip-git-repo-check", "--ephemeral", "--sandbox", "read-only", "--color", "never", "--", p}) {
		t.Fatalf("%q", x)
	}
	x, _ = NamingArgs(Codex, p, "m-small", "/n/answer.txt")
	if !strings.Contains(strings.Join(x, " "), "--output-last-message /n/answer.txt -m m-small -- PROMPT") {
		t.Fatalf("%q", x)
	}
	o, _ := NamingArgs(OpenCode, p, "", "")
	// --archived: the session the call creates stays out of the person's session list
	if !reflect.DeepEqual(o, []string{"run", "--standalone", "--archived", "--", p}) {
		t.Fatalf("%q", o)
	}
	for _, a := range [][]string{c, x, o} {
		if err := CheckArgs(a); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHelperEnvLeavesOutAPIKeys(t *testing.T) {
	base := []string{"PATH=/x", "ANTHROPIC_API_KEY=sk-fake", "OPENAI_API_KEY=sk-fake2", "anthropic_api_key=low", "AWSSSOHUB_SANDBOX=/s", "WAILS_X=1", "HOME=/h", "ANTHROPIC_MODEL=m", "CODEX_API_KEY=sk-c", "anthropic_auth_token=t", "OPENCODE_CONFIG_CONTENT={}"}
	got := HelperEnv(base, []string{"CLAUDE_CONFIG_DIR=/acc"})
	want := []string{"PATH=/x", "HOME=/h", "ANTHROPIC_MODEL=m", "CLAUDE_CONFIG_DIR=/acc", "NO_COLOR=1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%q", got)
	}
}

// The naming call with a fake runner: argv, folder, environment and the fallbacks.
func TestNamerFakeRunner(t *testing.T) {
	root := t.TempDir()
	var gotArgv, gotEnv []string
	var gotDir string
	var entries int
	run := func(ctx context.Context, argv []string, dir string, env []string) ([]byte, int, error) {
		gotArgv, gotEnv, gotDir = argv, env, dir
		es, _ := os.ReadDir(dir)
		entries = len(es)
		return []byte(`{"title":"Fix login","slug":"fix-login"}`), 0, nil
	}
	nm := Namer{GOOS: "darwin", Shell: "/bin/zsh", WorkRoot: root, Run: run,
		BaseEnv: func() []string { return []string{"PATH=/x", "ANTHROPIC_API_KEY=sk-fake", "OPENAI_API_KEY=sk-fake"} }}
	got := nm.Name(context.Background(), Claude, "--dangerously-skip-permissions fix login", 1)
	if got.Title != "Fix login" || got.Slug != "fix-login" || !got.FromAI {
		t.Fatalf("%+v", got)
	}
	for _, kv := range gotEnv {
		if strings.HasPrefix(kv, "ANTHROPIC_API_KEY") || strings.HasPrefix(kv, "OPENAI_API_KEY") {
			t.Fatalf("API key in the naming call's environment: %q", gotEnv)
		}
	}
	if entries != 0 || filepath.Dir(gotDir) != root {
		t.Fatalf("the working folder must be empty and under the data folder: %q (%d entries)", gotDir, entries)
	}
	if _, err := os.Stat(gotDir); !os.IsNotExist(err) {
		t.Fatal("the working folder is removed afterwards")
	}
	if gotArgv[0] != "/bin/zsh" || gotArgv[5] != "codepass" || gotArgv[6] != "claude" || gotArgv[len(gotArgv)-2] != "--" {
		t.Fatalf("%q", gotArgv)
	}
	if !strings.HasSuffix(gotArgv[len(gotArgv)-1], "--dangerously-skip-permissions fix login") || !strings.HasPrefix(gotArgv[len(gotArgv)-1], NamingPrompt) {
		t.Fatal("the description follows the fixed prompt, as one argument")
	}
	if err := CheckArgs(gotArgv[7:]); err != nil {
		t.Fatal(err)
	}
}

func TestNamerFallbacks(t *testing.T) {
	mk := func(out string, exit int, err error) Namer {
		return Namer{GOOS: "darwin", WorkRoot: t.TempDir(), BaseEnv: func() []string { return nil },
			Run: func(context.Context, []string, string, []string) ([]byte, int, error) { return []byte(out), exit, err }}
	}
	for _, c := range []struct {
		nm     Namer
		reason string
	}{
		{mk("not json", 0, nil), "bad-answer"},
		{mk("", 127, nil), "missing"},
		{mk("not signed in", 1, nil), "failed"},
		{mk("", -1, os.ErrNotExist), "missing"},
		{mk(`{"title":"","slug":"x"}`, 0, nil), "bad-answer"},
	} {
		got := c.nm.Name(context.Background(), Codex, "Fix the login bug", 2)
		if got.FromAI || got.Reason != c.reason || got.Title != "Fix the login bug" || got.Slug != "fix-the-login-bug" {
			t.Errorf("%s: %+v", c.reason, got)
		}
	}
	// the budget: a runner that waits for its context
	nm := Namer{GOOS: "darwin", WorkRoot: t.TempDir(), Budget: 100 * time.Millisecond, BaseEnv: func() []string { return nil },
		Run: func(ctx context.Context, _ []string, _ string, _ []string) ([]byte, int, error) {
			<-ctx.Done()
			return nil, -1, ctx.Err()
		}}
	start := time.Now()
	got := nm.Name(context.Background(), OpenCode, "x y", 1)
	if got.Reason != "timeout" || got.FromAI || time.Since(start) > 2*time.Second {
		t.Fatalf("%+v", got)
	}
}

// The naming call through a real login shell and a fake claude: a profile that exports an API key does not
// leak it to the tool, and a description that looks like a flag is only text.
func TestNamerThroughLoginShell(t *testing.T) {
	h := newFakeHome(t)
	envDump := filepath.Join(h.Home, "env.txt")
	cwdDump := filepath.Join(h.Home, "cwd.txt")
	h.tool("claude", `env > `+envDump+`; pwd > `+cwdDump+`; echo '{"title":"登录修复","slug":"fix-login"}'`)
	h.zshrc(`export ANTHROPIC_API_KEY=sk-fake-from-profile OPENAI_API_KEY=sk-fake2 CODEX_API_KEY=sk-fake4 ANTHROPIC_AUTH_TOKEN=tok-fake`)
	nm := Namer{GOOS: "darwin", Shell: "/bin/zsh", WorkRoot: filepath.Join(h.Home, "naming"), Budget: 20 * time.Second,
		BaseEnv: func() []string {
			return append(h.Env(), "ANTHROPIC_API_KEY=sk-fake-from-app", "OPENAI_API_KEY=sk-fake3", "CODEX_API_KEY=sk-fake5", "ANTHROPIC_AUTH_TOKEN=tok2", "AWSSSOHUB_SANDBOX=/s", "WAILS_X=1")
		}}
	got := nm.Name(context.Background(), Claude, "--dangerously-skip-permissions", 9)
	if !got.FromAI || got.Title != "登录修复" || got.Slug != "fix-login" {
		t.Fatalf("%+v", got)
	}
	env, _ := os.ReadFile(envDump)
	for _, bad := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "OPENAI_API_KEY", "CODEX_API_KEY", "AWSSSOHUB_", "WAILS_"} {
		if strings.Contains(string(env), bad) {
			t.Fatalf("%s reached the tool", bad)
		}
	}
	cwd, _ := os.ReadFile(cwdDump)
	if !strings.Contains(string(cwd), "naming-") {
		t.Fatalf("cwd %q", cwd)
	}
	call := strings.Join(h.calls(), "\n") // the prompt has line breaks
	// every flag comes before the "--", the description once, after it
	i := strings.Index(call, " [--] ")
	if i < 0 || strings.Count(call, "--dangerously-skip-permissions") != 1 || !strings.Contains(call[i:], "--dangerously-skip-permissions") {
		t.Fatalf("%q", call)
	}
	if !strings.HasPrefix(call, "claude [-p] [--model] [haiku] [--tools] [] [--strict-mcp-config] [--no-session-persistence]") {
		t.Fatalf("%q", call)
	}
}

// The fixed prompt must not hold an object that parses: a tool that echoes it must not name every task after it.
func TestNamingPromptHoldsNoParsableObject(t *testing.T) {
	if got := jsonObjects(NamingPrompt); len(got) != 0 {
		t.Fatalf("the prompt contains %q", got)
	}
	if _, ok := ParseNaming(NamingPrompt+"Fix it", "Fix it", 1); ok {
		t.Fatal("the echoed prompt alone is not an answer")
	}
}

func TestNamerCodexReadsItsLastMessageFile(t *testing.T) {
	var argv, env []string
	nm := Namer{GOOS: "darwin", Shell: "/bin/zsh", WorkRoot: t.TempDir(), BaseEnv: func() []string { return []string{"PATH=/x", "CODEX_API_KEY=sk-c"} },
		Run: func(ctx context.Context, a []string, dir string, e []string) ([]byte, int, error) {
			argv, env = a, e
			for i, x := range a {
				if x == "--output-last-message" {
					os.WriteFile(a[i+1], []byte(`{"title":"Fix login","slug":"fix-login"}`), 0o600)
				}
			}
			// the mixed output has the echoed prompt, whose example must not be taken
			return []byte("user\n" + NamingPrompt + "Fix the login\n"), 0, nil
		}}
	got := nm.Name(context.Background(), Codex, "Fix the login", 1)
	if !got.FromAI || got.Title != "Fix login" || got.Slug != "fix-login" {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(strings.Join(argv, " "), "--sandbox read-only") || !strings.Contains(strings.Join(argv, " "), "--output-last-message ") {
		t.Fatalf("%q", argv)
	}
	if !strings.Contains(argv[4], "unset ANTHROPIC_API_KEY ANTHROPIC_AUTH_TOKEN OPENAI_API_KEY CODEX_API_KEY;") {
		t.Fatalf("the unset: %q", argv[4])
	}
	for _, kv := range env {
		if strings.HasPrefix(kv, "CODEX_API_KEY") {
			t.Fatalf("CODEX_API_KEY reached the call: %q", env)
		}
	}
	// no answer file: the call falls back
	nm.Run = func(context.Context, []string, string, []string) ([]byte, int, error) {
		return []byte(`{"title":"From the mixed output","slug":"x"}`), 0, nil
	}
	if got := nm.Name(context.Background(), Codex, "Fix the login", 1); got.FromAI || got.Reason != "bad-answer" {
		t.Fatalf("%+v", got)
	}
}

func TestNamerOpenCodeDeniesEveryPermission(t *testing.T) {
	var env []string
	nm := Namer{GOOS: "darwin", Shell: "/bin/zsh", WorkRoot: t.TempDir(),
		BaseEnv: func() []string { return []string{"PATH=/x", "OPENCODE_CONFIG_CONTENT={\"permission\":\"allow\"}"} },
		Run: func(ctx context.Context, a []string, dir string, e []string) ([]byte, int, error) {
			env = e
			return []byte(`{"title":"Fix","slug":"fix"}`), 0, nil
		}}
	nm.Name(context.Background(), OpenCode, "Fix it", 1)
	n := 0
	for _, kv := range env {
		if strings.HasPrefix(kv, "OPENCODE_CONFIG_CONTENT=") {
			n++
			if kv != "OPENCODE_CONFIG_CONTENT="+OpenCodeNamingConfig {
				t.Fatalf("%q", kv)
			}
		}
	}
	if n != 1 {
		t.Fatalf("%d config variables in %q", n, env)
	}
}

// Windows: OpenCode's npm shim is a .cmd file; the naming prompt has line breaks, so it can never go through
// cmd.exe, and the package's entry point is used when it is there.
func TestNamerWindowsNeverSendsThePromptThroughCmd(t *testing.T) {
	var got []string
	run := func(ctx context.Context, a []string, dir string, e []string) ([]byte, int, error) {
		got = a
		return []byte(`{"title":"Fix","slug":"fix"}`), 0, nil
	}
	nm := Namer{GOOS: "windows", ToolPath: `C:\Tools\npm\opencode.cmd`, WorkRoot: t.TempDir(), BaseEnv: func() []string { return nil }, Run: run,
		Exists: func(string) bool { return false }}
	if r := nm.Name(context.Background(), OpenCode, "Fix it", 1); r.FromAI || got != nil {
		t.Fatalf("a prompt with line breaks went to a .cmd file: %q %+v", got, r)
	}
	nm.Lookup = lookupTo("")
	nm.Exists = func(p string) bool { return p == `C:\Tools\npm\node_modules\opencode-ai\bin\opencode` }
	if r := nm.Name(context.Background(), OpenCode, "Fix it", 1); !r.FromAI || got[0] != nodeExe || !strings.HasSuffix(got[1], `opencode-ai\bin\opencode`) {
		t.Fatalf("%q %+v", got, r)
	}
}
