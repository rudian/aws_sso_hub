package aitools

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rudian/codepass/internal/execx"
)

// DefaultDetectTimeout is the budget for one tool. An interactive login shell loads the person's whole profile
// (version managers, completion) before it runs anything.
const DefaultDetectTimeout = 10 * time.Second

const atMarker = "__CODEPASS_AT__"

// Detection is what was found for one tool.
type Detection struct {
	ID        ID
	Installed bool
	Version   string
	Path      string // absolute path of the program; "" for a shell function or alias
	Via       string // "file", "function" or ""
	Note      string // why detection failed in an unusual way
}

// DetectOptions wires detection to the machine. The zero value is the real thing.
type DetectOptions struct {
	GOOS    string
	Shell   string        // login shell (macOS); "" = $SHELL from Env, else /bin/zsh
	Env     []string      // the complete environment of the shell; nil = execx.ChildEnv()
	Timeout time.Duration // per tool; 0 = DefaultDetectTimeout
	Run     RunFunc       // nil = RealRun
	// Lookup finds a program on the running app's PATH (Windows, where no login shell exists); nil = execx.LookPath.
	Lookup func(name string) (string, error)
}

// LoginShell is the shell the app starts every tool through: shell when given, else SHELL from env, else
// /bin/zsh. It is accepted only when its base name runs `-l -i -c '"$@"'` as the launch form needs (sh, bash,
// zsh, ksh, dash); anything else (fish, nushell, a shell in an odd place) becomes /bin/zsh, so Detect, the
// naming call and a task launch always agree on one working shell.
func LoginShell(shell string, env []string) string {
	if shell == "" {
		for _, kv := range env {
			if v, ok := strings.CutPrefix(kv, "SHELL="); ok && v != "" {
				shell = v
			}
		}
	}
	base := shell
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	switch base {
	case "sh", "bash", "zsh", "ksh", "dash":
		if shell != "" && execx.IsAbsPath(shell, "darwin") && !execx.HasControlChars(shell) {
			return shell
		}
	}
	return "/bin/zsh"
}

func (o *DetectOptions) defaults() {
	if o.GOOS == "" {
		o.GOOS = runtimeGOOS
	}
	if o.Env == nil {
		o.Env = execx.ChildEnv()
	}
	o.Shell = LoginShell(o.Shell, o.Env)
	if o.Timeout <= 0 {
		o.Timeout = DefaultDetectTimeout
	}
	if o.Run == nil {
		o.Run = RealRun
	}
	if o.Lookup == nil {
		o.Lookup = execx.LookPath
	}
}

var reVersion = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?(?:[-+][0-9A-Za-z.\-+]+)?`)

// ParseVersion picks the first version number out of a tool's `--version` text ("2.1.325 (Claude Code)",
// "codex-cli 0.3.1", "2.3.0").
func ParseVersion(out string) string { return reVersion.FindString(out) }

// Detect looks for every tool at once, each with its own time budget. Only `--version` is ever run
// (`opencode auth list` would start OpenCode's shared background service; `opencode --version` does not).
//
// macOS: through the person's login shell ($SHELL -l -i -c, the same form a task launches with), so PATH,
// version managers and a `claude` shell function of their own apply. The shell prints where the name resolves
// ("command -v"), then runs "<tool> --version". Windows: the program is found on PATH and run directly.
func Detect(ctx context.Context, o DetectOptions) []Detection {
	o.defaults()
	ids := All()
	out := make([]Detection, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = detectOne(ctx, o, id)
		}()
	}
	wg.Wait()
	return out
}

// DetectOne looks for one tool.
func DetectOne(ctx context.Context, o DetectOptions, id ID) Detection {
	o.defaults()
	return detectOne(ctx, o, id)
}

func detectOne(ctx context.Context, o DetectOptions, id ID) Detection {
	d := Detection{ID: id}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	if o.GOOS == "windows" {
		p, err := o.Lookup(string(id))
		if err != nil {
			return d
		}
		out, exit, err := o.Run(ctx, []string{p, "--version"}, "", o.Env)
		if err != nil {
			d.Note = runNote(ctx, err)
			return d
		}
		d.Path, d.Via = p, "file"
		d.Version = ParseVersion(string(out))
		d.Installed = exit == 0 && d.Version != ""
		if exit != 0 || d.Version == "" {
			d.Via, d.Note = "", "the program did not print a version"
		}
		return d
	}
	// The shell text is a constant; the tool name and "--version" travel as separate arguments.
	argv := []string{o.Shell, "-l", "-i", "-c", `printf '` + atMarker + `%s\n' "$(command -v "$1")"; "$@"`, "codepass", string(id), "--version"}
	out, exit, err := o.Run(ctx, argv, "", o.Env)
	if err != nil {
		d.Note = runNote(ctx, err)
		// The marker may already be in the output (a profile that hangs after the tool ran is unlikely, but a
		// slow tool is not): fall through to what was read.
		if len(out) == 0 {
			return d
		}
	}
	loc, rest, ok := splitMarker(string(out))
	if !ok {
		if d.Note == "" {
			d.Note = "the login shell gave no answer"
		}
		return d
	}
	if loc == "" {
		return d // not found: the shell says "command not found" on its own
	}
	aliased := strings.HasPrefix(loc, "alias ")
	if aliased {
		// "$@" never expands an alias, so the launch runs the program behind it, if there is one. The shell
		// prints the alias even when the program is on PATH: what "--version" printed decides.
		if v := ParseVersion(rest); err != nil || exit != 0 || v == "" {
			d.Note = "the name is a shell alias, which Codepass cannot run; make it a shell function or install the program"
			return d
		}
		d.Via = "file"
		d.Version = ParseVersion(rest)
		d.Note = "the name is also a shell alias; its flags do not apply to tasks"
		d.Installed = true
		return d
	}
	if execx.IsAbsPath(loc, o.GOOS) {
		d.Path, d.Via = loc, "file"
	} else {
		d.Via = "function" // a function of the person's own: its flags come before ours
	}
	d.Version = ParseVersion(rest)
	if err != nil || exit != 0 || d.Version == "" {
		if d.Note == "" {
			d.Note = "the program did not print a version"
		}
		d.Path, d.Via, d.Version = "", "", ""
		return d
	}
	d.Installed = true
	return d
}

func runNote(ctx context.Context, err error) string {
	if ctx.Err() != nil {
		return "timeout"
	}
	return "cannot start: " + err.Error()
}

// splitMarker finds the marker line in the shell's output (a profile may print its own lines before it) and
// returns the location after it and the text that follows the line.
func splitMarker(s string) (loc, rest string, ok bool) {
	i := strings.Index(s, atMarker)
	if i < 0 {
		return "", "", false
	}
	s = s[i+len(atMarker):]
	line, rest, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line), rest, true
}

// HomeEnv returns an environment for a shell whose HOME and ZDOTDIR are a folder of the caller's choosing. It
// is a helper for tests and probes: the person's own shell files never run.
func HomeEnv(home, path string) []string {
	return []string{"HOME=" + home, "ZDOTDIR=" + home, "PATH=" + path, "SHELL=/bin/zsh", "LANG=en_US.UTF-8", "TERM=dumb"}
}
