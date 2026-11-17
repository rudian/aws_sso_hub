package aitools

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/execx"
)

// nodeExe is where a node on PATH resolves in these tests (not beside the shim).
const nodeExe = `C:\Program Files\nodejs\node.exe`

func lookupTo(p string) func(string) (string, error) {
	return func(n string) (string, error) {
		if n == "node" {
			return nodeExe, nil
		}
		return p, nil
	}
}

func lookupMap(m map[string]string) func(string) (string, error) {
	return func(n string) (string, error) {
		if p, ok := m[n]; ok {
			return p, nil
		}
		return "", execx.ErrNotFound
	}
}

// A bare name that the starter would resolve to a .cmd shim is treated as that shim.
func TestWindowsBareNameResolvesToShim(t *testing.T) {
	shim := `C:\Tools\npm\opencode.cmd`
	none := func(string) bool { return false }
	_, err := BuildLaunch(LaunchRequest{Tool: OpenCode, GOOS: "windows", FirstMessage: `Fix the "Save & Close" button`, Exists: none, Lookup: lookupTo(shim)})
	if !errors.Is(err, ErrBadMessage) {
		t.Fatalf("%v", err)
	}
	entry := `C:\Tools\npm\node_modules\opencode-ai\bin\opencode`
	l, err := BuildLaunch(LaunchRequest{Tool: OpenCode, GOOS: "windows", FirstMessage: `Fix the "Save & Close" button`,
		Exists: func(p string) bool { return p == entry }, Lookup: lookupTo(shim)})
	want := []string{nodeExe, entry, "--standalone", `--prompt=Fix the "Save & Close" button`}
	if err != nil || !reflect.DeepEqual(l.Argv, want) {
		t.Fatalf("%q %v", l.Argv, err)
	}

	var got []string
	run := func(ctx context.Context, a []string, dir string, e []string) ([]byte, int, error) {
		got = a
		return []byte(`{"title":"Fix","slug":"fix"}`), 0, nil
	}
	nm := Namer{GOOS: "windows", WorkRoot: t.TempDir(), BaseEnv: func() []string { return nil }, Run: run, Exists: none, Lookup: lookupTo(shim)}
	if r := nm.Name(context.Background(), OpenCode, `Fix "it"`, 1); r.FromAI || got != nil {
		t.Fatalf("%q %+v", got, r)
	}
	nm.Exists = func(p string) bool { return p == entry }
	if r := nm.Name(context.Background(), OpenCode, `Fix "it"`, 1); !r.FromAI || got[0] != nodeExe || got[1] != entry {
		t.Fatalf("%q %+v", got, r)
	}
}

// The app's own -c override holds quotes and no cmd.exe operator: an npm Codex shim takes it, with node.exe
// when the entry exists.
func TestWindowsCodexShimTakesAppArguments(t *testing.T) {
	shim := `C:\Users\alex\AppData\Roaming\npm\codex.cmd`
	notify := []string{`C:\Program Files\Codepass\codepass.exe`, "report"}
	none := func(string) bool { return false }
	l, err := BuildLaunch(LaunchRequest{Tool: Codex, GOOS: "windows", ToolPath: shim, CodexNotify: notify, Exists: none})
	if err != nil || l.Argv[0] != shim {
		t.Fatalf("%q %v", l.Argv, err)
	}
	// a quote in the message is still refused; an operator in an app argument is ErrBadConfig
	if _, err := BuildLaunch(LaunchRequest{Tool: Codex, GOOS: "windows", ToolPath: shim, FirstMessage: `say "hi"`, Exists: none}); !errors.Is(err, ErrBadMessage) {
		t.Fatalf("%v", err)
	}
	if _, err := BuildLaunch(LaunchRequest{Tool: Codex, GOOS: "windows", ToolPath: shim, CodexNotify: []string{`C:\a&b\x.exe`, "report"}, Exists: none}); !errors.Is(err, ErrBadConfig) {
		t.Fatalf("%v", err)
	}
	entry := `C:\Users\alex\AppData\Roaming\npm\node_modules\@openai\codex\bin\codex.js`
	l, err = BuildLaunch(LaunchRequest{Tool: Codex, GOOS: "windows", ToolPath: shim, CodexNotify: notify, FirstMessage: `say "hi" & go`, Exists: func(p string) bool { return p == entry }, Lookup: lookupTo("")})
	if err != nil || l.Argv[0] != nodeExe || l.Argv[1] != entry {
		t.Fatalf("%q %v", l.Argv, err)
	}
	cl := `C:\Users\alex\AppData\Roaming\npm\claude.cmd`
	ce := `C:\Users\alex\AppData\Roaming\npm\node_modules\@anthropic-ai\claude-code\cli.js`
	l, err = BuildLaunch(LaunchRequest{Tool: Claude, GOOS: "windows", ToolPath: cl, FirstMessage: `a & b`, Exists: func(p string) bool { return p == ce }, Lookup: lookupTo("")})
	if err != nil || !strings.HasSuffix(l.Argv[1], "cli.js") {
		t.Fatalf("%q %v", l.Argv, err)
	}
}

// With no node.exe beside the shim, "node" is resolved through Lookup; a batch file there is unusable.
func TestWindowsNodeBesideNothingIsResolved(t *testing.T) {
	shim := `C:\Users\alex\AppData\Roaming\npm\opencode.cmd`
	entry := `C:\Users\alex\AppData\Roaming\npm\node_modules\opencode-ai\bin\opencode`
	only := func(p string) bool { return p == entry }
	msg := `Fix the "Save & Close" button`
	_, err := BuildLaunch(LaunchRequest{Tool: OpenCode, GOOS: "windows", ToolPath: shim, FirstMessage: msg, Exists: only,
		Lookup: lookupMap(map[string]string{"node": `C:\mise\shims\node.cmd`})})
	if !errors.Is(err, ErrBadMessage) {
		t.Fatalf("%v", err)
	}
	node := `C:\Program Files\nodejs\node.exe`
	l, err := BuildLaunch(LaunchRequest{Tool: OpenCode, GOOS: "windows", ToolPath: shim, FirstMessage: msg, Exists: only,
		Lookup: lookupMap(map[string]string{"node": node})})
	if err != nil || l.Argv[0] != node || l.Argv[1] != entry {
		t.Fatalf("%q %v", l.Argv, err)
	}
	nm := Namer{GOOS: "windows", WorkRoot: t.TempDir(), BaseEnv: func() []string { return nil }, Exists: only,
		Run: func(context.Context, []string, string, []string) ([]byte, int, error) {
			t.Fatal("ran")
			return nil, 0, nil
		},
		Lookup: lookupMap(map[string]string{"opencode": shim, "node": `C:\mise\shims\node.cmd`})}
	if r := nm.Name(context.Background(), OpenCode, `Fix "it"`, 1); r.FromAI {
		t.Fatalf("%+v", r)
	}
}

// A Windows request built on another OS with no Lookup keeps the bare name and never reaches the host.
func TestWindowsWithoutLookupKeepsBareNameOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("host lookup is the real one on Windows")
	}
	argv, err := WindowsCommand(Codex, "codex", []string{"x"}, "", ShimEnv{})
	if err != nil || !reflect.DeepEqual(argv, []string{"codex", "x"}) {
		t.Fatalf("%q %v", argv, err)
	}
}
