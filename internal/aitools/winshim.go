package aitools

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/rudian/codepass/internal/execx"
)

// Windows runs a .cmd or .bat file through cmd.exe, which parses the command line again by its own rules: it
// expands %NAME%, does not treat \" as an escaped quote (so a "&" after a quote starts a second command), and
// ends the command at a line break. Go's own quoting does not cover that (the os/exec documentation names batch
// files as the exception). Detection finds OpenCode's npm shim, opencode.cmd. So a task description or naming
// prompt never goes to such a file:
//
//  1. the package's own entry point is started with node.exe, which takes the command line as Go composes it;
//  2. when that entry is not found, a call whose arguments hold a character cmd.exe treats as syntax is refused
//     (ErrBadMessage; the naming call falls back).

// unsafeForCmd are the characters cmd.exe reads as syntax even inside the quotes Go adds (and a quote, which
// it does not treat as escaped). unsafeApp is the same set without the quote: the app's own arguments, such
// as a Codex "-c notify=[...]" override, hold quotes and no other operator.
const (
	unsafeForCmd = "\"%!^&|<>()\r\n"
	unsafeApp    = "%!^&|<>()\r\n"
)

func isBatchFile(p string) bool {
	l := strings.ToLower(p)
	return strings.HasSuffix(l, ".cmd") || strings.HasSuffix(l, ".bat")
}

// shimEntry is where npm puts a package's entry point beside its shim: <shim folder>\<path>.
var shimEntry = map[ID]string{
	OpenCode: `node_modules\opencode-ai\bin\opencode`,
	Codex:    `node_modules\@openai\codex\bin\codex.js`,
	Claude:   `node_modules\@anthropic-ai\claude-code\cli.js`,
}

func shimDir(p string) string {
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		return p[:i]
	}
	return ""
}

// ShimEnv is the file system the shim lookup uses; the zero value is the real one.
type ShimEnv struct {
	Exists func(string) bool                 // nil = os.Stat
	Lookup func(name string) (string, error) // nil = execx.LookPath (what the starter uses)
}

// WindowsCommand returns the argv that starts prog (of tool) with args on Windows. A bare name is resolved
// the way the starter does (PATH and PATHEXT), so a name that finds opencode.cmd is treated as that shim. A real
// program is started as it is. A .cmd or .bat shim is replaced by node.exe and the package's entry point when
// found; otherwise it is started only when no argument holds a character cmd.exe treats as syntax. text is the
// message or naming prompt inside args ("" = none): it may not hold a quote either, and its refusal is
// ErrBadMessage; another argument is refused with ErrBadConfig.
func WindowsCommand(tool ID, prog string, args []string, text string, env ShimEnv) ([]string, error) {
	if env.Exists == nil {
		env.Exists = func(p string) bool { st, err := os.Stat(p); return err == nil && !st.IsDir() }
	}
	if env.Lookup == nil {
		if runtime.GOOS == "windows" {
			env.Lookup = execx.LookPath
		} else {
			// A Windows launch built on another OS (a test) keeps the bare name: it never reaches the host's PATH.
			env.Lookup = func(string) (string, error) { return "", execx.ErrNotFound }
		}
	}
	if !strings.ContainsAny(prog, `\/`) && !isBatchFile(prog) {
		if p, err := env.Lookup(prog); err == nil && p != "" {
			prog = p
		}
	}
	if !isBatchFile(prog) {
		return append([]string{prog}, args...), nil
	}
	if dir, rel := shimDir(prog), shimEntry[tool]; dir != "" && rel != "" {
		if entry := dir + `\` + rel; env.Exists(entry) {
			node := ""
			if n := dir + `\node.exe`; env.Exists(n) {
				node = n
			} else if p, err := env.Lookup("node"); err == nil && p != "" && !isBatchFile(p) {
				node = p
			}
			if node != "" {
				return append([]string{node, entry}, args...), nil
			}
			// node is not beside the shim and does not resolve to a program: the entry is unusable, so the
			// batch-file rules below apply.
		}
	}
	for _, a := range args {
		if text != "" && strings.Contains(a, text) {
			if strings.ContainsAny(a, unsafeForCmd) {
				return nil, fmt.Errorf("%w: the program is a batch file (%s) and the text holds a character cmd.exe would read as a command", ErrBadMessage, prog)
			}
		} else if strings.ContainsAny(a, unsafeApp) {
			return nil, fmt.Errorf("%w: the program is a batch file (%s) and an argument holds a character cmd.exe would read as a command", ErrBadConfig, prog)
		}
	}
	return append([]string{prog}, args...), nil
}
