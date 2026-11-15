package identity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/execx"
	"github.com/rudian/codepass/internal/netx"
)

// Terminal environment contract (shared with milestone 2, which builds the AI session terminal):
//
// A terminal started by the app runs the person's own login shell, interactive: `<shell> -l -i` (see
// ShellArgs), with the environment TerminalEnv(home, proxyEnv) builds (plan 4.7 step 4: TERM, COLORTERM, a
// UTF-8 LANG, the proxy variables, AWSSSOHUB_* and WAILS_* removed). That shell sources the person's
// profiles, so a GIT_AUTHOR_EMAIL exported in ~/.zshrc reaches git inside the terminal. ProbeShellEnv starts
// the same shell, the same way, with the same TerminalEnv, and reads back only the identity-related variables
// (IdentityEnvNames), so `git var GIT_AUTHOR_IDENT` can be answered for the environment a session gets.
// Milestone 2 must therefore use the same shell, the same ShellArgs and the same TerminalEnv builder; it
// differs from the probe only in the working folder (the probe runs in the home folder, a session in the task
// folder, so a profile that reacts to the current folder, such as direnv, can differ). On Windows no profile
// is run (PowerShell's profile is not probed): the process environment is what is reported.

// IdentityEnvNames are the variables the probe reads (names only; the values never reach a log).
var IdentityEnvNames = []string{
	"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "EMAIL",
	"GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM", "GIT_CONFIG_NOSYSTEM", "GIT_CONFIG_COUNT", "XDG_CONFIG_HOME",
}

// identityVarNames are the ones that decide a commit's author or committer.
var identityVarNames = []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "EMAIL"}

// forwardNames are the probed variables passed on to the git processes that answer for the terminal.
// GIT_CONFIG_COUNT (and KEY/VALUE) is deliberately not forwarded: it can set any config key, and a read must
// never be steered by it. It is only detected and reported.
var forwardNames = []string{
	"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "EMAIL",
	"GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM", "GIT_CONFIG_NOSYSTEM", "XDG_CONFIG_HOME",
}

// ShellArgs returns the arguments (before the script) with which the app starts a shell for its terminals and
// for the identity probe: login and interactive, so every profile file is read.
func ShellArgs(shell string) []string { return []string{"-l", "-i"} }

// TerminalEnv builds the environment of an AI session terminal (plan 4.7 step 4) and of the identity probe:
// the app's own environment with AWSSSOHUB_* and WAILS_* removed, TERM=xterm-256color, COLORTERM=truecolor, a
// UTF-8 LANG (added when missing or not UTF-8: apps started from Finder get none), the proxy variables
// replaced by proxyEnv (netx Config.ChildEnv), PATH widened as for other children, and HOME / USERPROFILE
// pinned to home (never the process's own idea of it). It deliberately does not carry the background-git
// settings of execx.ChildEnv (GIT_TERMINAL_PROMPT=0, GCM_INTERACTIVE=never, stripped askpass and AWS
// variables): the terminal is interactive, and git in it must be able to ask for credentials.
func TerminalEnv(home string, proxyEnv []string) []string {
	return terminalEnvFrom(os.Environ(), home, proxyEnv)
}

func terminalEnvFrom(base []string, home string, proxyEnv []string) []string {
	win := os.PathSeparator == '\\'
	drop := map[string]bool{"TERM": true, "COLORTERM": true, "PATH": true}
	lang := ""
	env := make([]string, 0, len(base)+8)
	for _, kv := range netx.StripProxyEnv(base) {
		k, v, _ := strings.Cut(kv, "=")
		if win {
			k = strings.ToUpper(k)
		}
		if strings.HasPrefix(k, "AWSSSOHUB_") || strings.HasPrefix(k, "WAILS_") || drop[k] || (home != "" && (k == "HOME" || k == "USERPROFILE")) {
			continue
		}
		if k == "LANG" {
			lang = v
			continue
		}
		env = append(env, kv)
	}
	if l := strings.ToLower(lang); lang == "" || !(strings.Contains(l, "utf-8") || strings.Contains(l, "utf8")) {
		lang = "en_US.UTF-8"
	}
	env = append(env, "TERM=xterm-256color", "COLORTERM=truecolor", "LANG="+lang, "PATH="+execx.EnvPATH())
	env = append(env, proxyEnv...)
	if home != "" {
		env = append(env, "HOME="+home, "USERPROFILE="+home)
	}
	return env
}

func shellName(shell string) string {
	b := strings.ToLower(filepath.Base(shell))
	return strings.TrimSuffix(b, ".exe")
}

const (
	probeStart = "@@CPV@@"
	probeEnd   = "@@CPE@@"
	// probeDone ends the script. Its presence, not the shell's exit status, says the script ran to the end:
	// the last variable of the list is usually not exported, which leaves a non-zero status behind.
	probeDone = "@@CPDONE@@"
)

var probeRE = regexp.MustCompile(`(?s)@@CPV@@([A-Z0-9_]+)=(.*?)@@CPE@@`)

// probeScript is a constant: no value is ever spliced into it.
func probeScript(shell string) string {
	names := strings.Join(IdentityEnvNames, " ")
	done := `printf '\n` + probeDone + `\n'`
	if shellName(shell) == "fish" {
		return `for n in ` + names + `; if set -qx $n; printf '\n` + probeStart + `%s=%s` + probeEnd + `\n' $n "$$n"; end; end; ` + done
	}
	return `for n in ` + names + `; do if v=$(command printenv "$n"); then printf '\n` + probeStart + `%s=%s` + probeEnd + `\n' "$n" "$v"; fi; done; ` + done
}

// ProbeOptions configures ProbeShellEnv.
type ProbeOptions struct {
	Shell   string
	Env     []string // base environment of the shell
	Dir     string   // working folder ("" = the app's)
	Timeout time.Duration
}

// ErrNoShell is returned when no shell can be run (Windows, or no shell found).
var ErrNoShell = errors.New("identity: no shell to probe")

// ProbeFailure is returned when the shell ran but did not answer.
type ProbeFailure struct {
	Kind api.ProbeErrorKind
	Exit int // the shell's exit status, when it had one
}

func (p *ProbeFailure) Error() string {
	return "identity: shell probe " + string(p.Kind) + " (exit " + itoa(p.Exit) + ")"
}

// ProbeShellEnv runs the login shell and returns the identity variables it exports. Only names in
// IdentityEnvNames are read. Values holding a newline or NUL are dropped.
func ProbeShellEnv(ctx context.Context, o ProbeOptions) (map[string]string, error) {
	if o.Shell == "" {
		return nil, ErrNoShell
	}
	if st, err := os.Stat(o.Shell); err != nil || st.IsDir() {
		return nil, ErrNoShell
	}
	if o.Timeout <= 0 {
		o.Timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	args := append(ShellArgs(o.Shell), "-c", probeScript(o.Shell))
	cmd := execx.Command(ctx, o.Shell, args...)
	cmd.Env = append([]string(nil), o.Env...)
	if isDir(o.Dir) {
		cmd.Dir = o.Dir
	}
	// A profile that starts a background program keeps the output pipe open; do not wait for it forever.
	cmd.WaitDelay = 500 * time.Millisecond
	res, err := execx.Run(cmd, nil)
	if !strings.Contains(string(res.Stdout), probeDone) {
		switch {
		case ctx.Err() != nil:
			return nil, &ProbeFailure{Kind: api.ProbeTimeout}
		case err != nil && res.ExitCode <= 0:
			return nil, &ProbeFailure{Kind: api.ProbeFailed}
		}
		return nil, &ProbeFailure{Kind: api.ProbeFailed, Exit: res.ExitCode}
	}
	out := map[string]string{}
	allowed := map[string]bool{}
	for _, n := range IdentityEnvNames {
		allowed[n] = true
	}
	for _, m := range probeRE.FindAllStringSubmatch(string(res.Stdout), -1) {
		if !allowed[m[1]] || strings.ContainsAny(m[2], "\n\x00") {
			continue
		}
		out[m[1]] = m[2]
	}
	return out, nil
}

// envFromList reads the identity variables from an environment list (Windows, or when no shell can run).
func envFromList(env []string) map[string]string {
	out := map[string]string{}
	want := map[string]bool{}
	for _, n := range IdentityEnvNames {
		want[n] = true
	}
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if ok && want[k] {
			out[k] = v // later entries win
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// profileCandidates lists the startup files a shell reads, for the "open that file" hint.
func profileCandidates(shell, home string, env []string) []string {
	zdot := home
	xdg := filepath.Join(home, ".config")
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "ZDOTDIR="); ok && v != "" {
			zdot = v
		}
		if v, ok := strings.CutPrefix(kv, "XDG_CONFIG_HOME="); ok && v != "" {
			xdg = v
		}
	}
	switch shellName(shell) {
	case "zsh":
		return []string{filepath.Join(zdot, ".zshenv"), filepath.Join(zdot, ".zprofile"), filepath.Join(zdot, ".zshrc"), filepath.Join(zdot, ".zlogin")}
	case "bash":
		return []string{filepath.Join(home, ".bash_profile"), filepath.Join(home, ".bash_login"), filepath.Join(home, ".profile"), filepath.Join(home, ".bashrc")}
	case "fish":
		return []string{filepath.Join(xdg, "fish", "config.fish")}
	}
	return []string{filepath.Join(home, ".profile")}
}

var exportRE = regexp.MustCompile(`(?m)^\s*(?:export\s+|typeset\s+-x\s+|declare\s+-x\s+|set\s+-[gUx]+\s+)?(GIT_AUTHOR_NAME|GIT_AUTHOR_EMAIL|GIT_COMMITTER_NAME|GIT_COMMITTER_EMAIL|EMAIL)(?:=|\s)`)

// scanProfiles returns the candidate files whose text assigns one of the identity variables. It is a hint
// for "open that file" only; whether a variable is really exported is decided by running the shell.
func scanProfiles(files []string, names map[string]bool) map[string][]string {
	hits := map[string][]string{} // variable -> files
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil || st.IsDir() || st.Size() > 1<<20 {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, m := range exportRE.FindAllStringSubmatch(string(b), -1) {
			if len(names) == 0 || names[m[1]] {
				hits[m[1]] = append(hits[m[1]], f)
			}
		}
	}
	return hits
}
