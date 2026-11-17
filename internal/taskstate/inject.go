package taskstate

import (
	"os"
	"path/filepath"
	"strings"
)

// TaskEnv returns the environment entries the launcher adds to the child: the task ID, the socket and, when
// armed, the one-shot prompt hand-over and the queue marker. Report mode reads exactly these.
func TaskEnv(task, endpoint string, namePrompt, queue bool) []string {
	env := []string{EnvTask + "=" + task, EnvSocket + "=" + endpoint}
	if namePrompt {
		env = append(env, EnvNamePrompt+"=1")
	}
	if queue {
		env = append(env, EnvQueue+"=1")
	}
	return env
}

// ReportCommand is the hook command line: the app's own binary in report mode. exe is quoted for the shell
// that runs hooks (sh on Unix, cmd-style on Windows); nothing from a task is part of it.
func ReportCommand(goos, exe string, args ...string) string {
	parts := []string{quoteArg(goos, exe)}
	for _, a := range args {
		parts = append(parts, quoteArg(goos, a))
	}
	return strings.Join(parts, " ")
}

func quoteArg(goos, s string) string {
	if goos == "windows" {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-./:=") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// writePrivate writes a file with mode 0600 inside folders of mode 0700 (the data folder holds nothing a
// stranger should read).
func writePrivate(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
