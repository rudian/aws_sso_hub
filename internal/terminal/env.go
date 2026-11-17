package terminal

import (
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/pty"
)

// BuildEnv builds the complete, explicit environment of a session's child (plan 4.7). It starts from base (the
// app's own environment, so PATH, HOME, proxies and the like carry over), removes what must never reach a
// task, sets TERM, COLORTERM and a UTF-8 LANG, and lets extra override by name.
//
// Removed from base: every AWSSSOHUB_*, WAILS_* and CODEPASS_* variable (the app's own; a task gets its own
// CODEPASS_TASK through extra), TERM and COLORTERM (set below), PWD and OLDPWD (the shell recomputes them).
// Extra may carry anything except AWSSSOHUB_* and WAILS_*: those are dropped from it too.
//
// goos "" means the running OS (names are case-insensitive on Windows).
func BuildEnv(base []string, goos string, extra []string) []string {
	if goos == "" {
		goos = runtime.GOOS
	}
	win := goos == "windows"
	norm := func(k string) string {
		if win {
			return strings.ToUpper(k)
		}
		return k
	}
	var order []string
	vals := map[string]string{}
	names := map[string]string{} // normalised key -> spelling
	set := func(kv string) {
		i := strings.IndexByte(kv, '=')
		if i <= 0 { // no name, or a Windows "=C:" drive entry
			return
		}
		k := kv[:i]
		n := norm(k)
		if _, ok := vals[n]; !ok {
			order = append(order, n)
		}
		vals[n], names[n] = kv[i+1:], k
	}
	for _, kv := range base {
		if k := keyOf(kv); !inheritable(norm(k)) {
			continue
		}
		set(kv)
	}
	set("TERM=xterm-256color")
	set("COLORTERM=truecolor")
	if !utf8Locale(vals[norm("LANG")]) {
		set("LANG=en_US.UTF-8")
	}
	for _, kv := range extra {
		if n := norm(keyOf(kv)); strings.HasPrefix(n, "AWSSSOHUB_") || strings.HasPrefix(n, "WAILS_") {
			continue
		}
		set(kv)
	}
	out := make([]string, 0, len(order))
	for _, n := range order {
		out = append(out, names[n]+"="+vals[n])
	}
	return out
}

func keyOf(kv string) string {
	if i := strings.IndexByte(kv, '='); i >= 0 {
		return kv[:i]
	}
	return kv
}

func inheritable(k string) bool {
	switch k {
	case "TERM", "COLORTERM", "PWD", "OLDPWD":
		return false
	}
	k = strings.ToUpper(k)
	for _, p := range [...]string{"AWSSSOHUB_", "WAILS_", "CODEPASS_"} {
		if strings.HasPrefix(k, p) {
			return false
		}
	}
	return true
}

func utf8Locale(v string) bool {
	v = strings.ToLower(v)
	return strings.Contains(v, "utf-8") || strings.Contains(v, "utf8")
}

// LaunchArgv is the argument list that starts a tool in a session (plan 4.7, point 3). Unix: the login-shell
// form, so the owner's PATH, version managers and shell functions apply, and every value travels as its own
// word, never joined into shell text. Windows: the tool's executable directly.
func LaunchArgv(goos, shell, tool string, args ...string) []string {
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos == "windows" {
		return append([]string{tool}, args...)
	}
	return pty.LaunchArgv(shell, tool, args...)
}

// CheckProgram refuses, on Windows, a program that is a batch file (.cmd or .bat). CreateProcess runs those
// through cmd.exe, which re-parses the command line with its own rules ("&", "|", "%VAR%" and quotes act
// there), so an argument such as a task description could break the call or run commands. Plan 4.7 allows no
// task value to be spliced into command text, so the real executable has to be launched. lookPath resolves
// argv0 the way pty.Start does (exec.LookPath when nil); a program that cannot be found passes here and fails
// in pty.Start with its own error. Other systems are never refused.
func CheckProgram(goos, argv0 string, lookPath func(string) (string, error)) error {
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos != "windows" {
		return nil
	}
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	path, err := lookPath(argv0)
	if err != nil {
		path = argv0
	}
	l := strings.ToLower(strings.TrimRight(path, " ."))
	if strings.HasSuffix(l, ".cmd") || strings.HasSuffix(l, ".bat") {
		return api.NewError(api.ErrInvalidInput, "%s is a batch file: start the program inside it directly", path).WithParam("field", "argv")
	}
	return nil
}

// DefaultShell is the login shell for the launch form: $SHELL when it is an absolute path to a file, else
// /bin/zsh on macOS and /bin/sh elsewhere.
func DefaultShell(goos, shellEnv string) string {
	if goos == "" {
		goos = runtime.GOOS
	}
	if shellEnv != "" && strings.HasPrefix(shellEnv, "/") {
		if st, err := os.Stat(shellEnv); err == nil && !st.IsDir() {
			return shellEnv
		}
	}
	if goos == "darwin" {
		return "/bin/zsh"
	}
	return "/bin/sh"
}
