// Package execx finds the aws and git binaries, builds the PATH that GUI-launched apps lack, and runs child
// processes with a sanitised environment.
package execx

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Options configures PATH handling. Configure is called once at startup.
type Options struct {
	ExtraEnv          []string // appended last (paths.ChildEnv())
	PreferredDirs     []string // nil = platform defaults
	DisableLoginShell bool     // tests
}

// ErrNotFound is returned when a binary cannot be located.
var ErrNotFound = exec.ErrNotFound

// ErrTerminated is returned by Run when the process was killed (context ended or signal).
var ErrTerminated = errors.New("execx: process terminated")

var (
	mu          sync.Mutex
	opts        Options
	cachedPATH  string
	pathReady   bool
	interactive bool // the "-i -l" login shell attempt was made
	shellPath   string
)

// Configure sets the options and resets the cached PATH. Tests may call it again.
func Configure(o Options) {
	mu.Lock()
	defer mu.Unlock()
	opts = o
	cachedPATH = ""
	pathReady = false
	interactive = false
	shellPath = ""
}

func isWindows() bool { return runtime.GOOS == "windows" }

func platformPreferredDirs() []string {
	if isWindows() {
		var dirs []string
		add := func(base string, rest ...string) {
			if base != "" {
				dirs = append(dirs, filepath.Join(append([]string{base}, rest...)...))
			}
		}
		add(os.Getenv("ProgramFiles"), "Amazon", "AWSCLIV2")
		add(os.Getenv("ProgramFiles"), "Git", "cmd")
		add(os.Getenv("LOCALAPPDATA"), "Programs", "Git", "cmd")
		add(os.Getenv("LOCALAPPDATA"), "Programs", "Amazon", "AWSCLIV2") // per-user install location
		add(os.Getenv("USERPROFILE"), "scoop", "shims")
		return dirs
	}
	dirs := []string{"/opt/homebrew/bin", "/usr/local/bin"}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		dirs = append(dirs, filepath.Join(h, ".local", "bin"))
	}
	return append(dirs, "/usr/local/aws-cli")
}

func fallbackDirs() []string {
	if isWindows() {
		root := os.Getenv("SystemRoot")
		if root == "" {
			return nil
		}
		return []string{filepath.Join(root, "System32"), root}
	}
	return []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"}
}

// Dedupe removes duplicate entries (case-insensitive when fold is true) and empty strings, keeping order.
func Dedupe(in []string, fold bool) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		k := s
		if fold {
			k = strings.ToLower(s)
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
	}
	return out
}

const pathMarker = "__AWSSSOHUB_PATH__"

// loginShellPATH asks the user's login shell for its PATH (macOS only).
func loginShellPATH(interactiveShell bool, timeout time.Duration) string {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	args := []string{"-l", "-c"}
	if interactiveShell {
		args = []string{"-i", "-l", "-c"}
	}
	args = append(args, `printf "`+pathMarker+`%s`+pathMarker+`" "$PATH"`)
	cmd := exec.CommandContext(ctx, sh, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stdin = nil
	if err := cmd.Run(); err != nil && out.Len() == 0 {
		return ""
	}
	s := out.String()
	i := strings.Index(s, pathMarker)
	if i < 0 {
		return ""
	}
	s = s[i+len(pathMarker):]
	j := strings.Index(s, pathMarker)
	if j < 0 {
		return ""
	}
	return s[:j]
}

func buildPATHLocked() string {
	var parts []string
	if opts.PreferredDirs != nil {
		parts = append(parts, opts.PreferredDirs...)
	} else {
		parts = append(parts, platformPreferredDirs()...)
	}
	if runtime.GOOS == "darwin" && !opts.DisableLoginShell {
		if shellPath == "" {
			shellPath = loginShellPATH(false, 3*time.Second)
		}
		parts = append(parts, filepath.SplitList(shellPath)...)
	}
	parts = append(parts, filepath.SplitList(os.Getenv("PATH"))...)
	parts = append(parts, fallbackDirs()...)
	return strings.Join(Dedupe(parts, isWindows()), string(os.PathListSeparator))
}

// EnvPATH returns the PATH used for lookups and for every child process.
func EnvPATH() string {
	mu.Lock()
	defer mu.Unlock()
	if !pathReady {
		cachedPATH = buildPATHLocked()
		pathReady = true
	}
	return cachedPATH
}

// retryInteractive recomputes the PATH once using an interactive login shell (macOS only). Reports whether it
// changed anything.
func retryInteractive() bool {
	mu.Lock()
	defer mu.Unlock()
	if runtime.GOOS != "darwin" || opts.DisableLoginShell || interactive {
		return false
	}
	interactive = true
	p := loginShellPATH(true, 5*time.Second)
	if p == "" || p == shellPath {
		return false
	}
	shellPath = p
	cachedPATH = buildPATHLocked()
	pathReady = true
	return true
}

func isExecutable(path string) bool {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return false
	}
	if isWindows() {
		return true
	}
	return st.Mode().Perm()&0o111 != 0
}

// LookPath searches EnvPATH for name. The returned path is not symlink-resolved.
func LookPath(name string) (string, error) {
	if strings.ContainsAny(name, `/\`) {
		if filepath.IsAbs(name) && isExecutable(name) {
			return name, nil
		}
		return "", ErrNotFound
	}
	exts := []string{""}
	if isWindows() {
		exts = []string{".exe", ".cmd", ".bat", ".com"}
		if filepath.Ext(name) != "" {
			exts = []string{""}
		}
	}
	for _, dir := range filepath.SplitList(EnvPATH()) {
		// A relative entry (".", "bin") would resolve against the working directory.
		if dir == "" || !filepath.IsAbs(dir) {
			continue
		}
		for _, ext := range exts {
			p := filepath.Join(dir, name+ext)
			if isExecutable(p) {
				return p, nil
			}
		}
	}
	return "", ErrNotFound
}

var stripped = map[string]bool{
	"GIT_ASKPASS": true, "SSH_ASKPASS": true, "AWS_ACCESS_KEY_ID": true, "AWS_SECRET_ACCESS_KEY": true,
	"AWS_SESSION_TOKEN": true, "AWS_PROFILE": true, "AWS_DEFAULT_PROFILE": true,

	// Git variables that redirect repository discovery, configuration or program execution. GIT_CONFIG_GLOBAL
	// is kept on purpose: the app reads and writes the global config it names.
	"GIT_DIR": true, "GIT_WORK_TREE": true, "GIT_INDEX_FILE": true, "GIT_OBJECT_DIRECTORY": true,
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": true, "GIT_COMMON_DIR": true, "GIT_NAMESPACE": true,
	"GIT_CEILING_DIRECTORIES": true, "GIT_CONFIG_PARAMETERS": true, "GIT_CONFIG_COUNT": true,
	"GIT_SSH": true, "GIT_SSH_COMMAND": true, "GIT_PROXY_COMMAND": true, "GIT_EXEC_PATH": true,
}

// strippedPrefixes lists variable families removed from the child environment.
var strippedPrefixes = []string{"GIT_CONFIG_KEY_", "GIT_CONFIG_VALUE_"}

func isStrippedEnv(k string) bool {
	if stripped[k] || k == "PATH" || k == "AWS_PAGER" || k == "GIT_TERMINAL_PROMPT" || k == "GCM_INTERACTIVE" {
		return true
	}
	for _, p := range strippedPrefixes {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

// ChildEnv returns the environment for child processes.
func ChildEnv() []string {
	base := os.Environ()
	env := make([]string, 0, len(base)+8)
	for _, kv := range base {
		k := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			k = kv[:i]
		}
		if isWindows() {
			k = strings.ToUpper(k)
		}
		if isStrippedEnv(k) {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "PATH="+EnvPATH(), "AWS_PAGER=", "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
	mu.Lock()
	extra := append([]string(nil), opts.ExtraEnv...)
	mu.Unlock()
	return append(env, extra...)
}

// Command builds a child process with the sanitised environment. On Windows the console window is hidden.
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = ChildEnv()
	hideWindow(cmd)
	return cmd
}

// Start launches a long-lived program (a terminal window) without waiting for it. The child gets the
// sanitised environment plus extraEnv, runs in dir (when set) and, with newConsole on Windows, in its own
// console window. Without newConsole the process is started as is: the program is a GUI application such
// as Windows Terminal that shows its own window, and a hidden-window start state would be inherited by it.
// The process is reaped in the background.
func Start(name string, args []string, dir string, extraEnv []string, newConsole bool) error {
	cmd := startCmd(name, args, dir, extraEnv, newConsole)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// startCmd builds the command for Start.
func startCmd(name string, args []string, dir string, extraEnv []string, newConsole bool) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Env = append(ChildEnv(), extraEnv...)
	cmd.Dir = dir
	if newConsole {
		showNewConsole(cmd)
	}
	return cmd
}

// Result is the outcome of a finished process.
type Result struct {
	Stdout, Stderr []byte
	ExitCode       int
}

// Run executes cmd with stdin and returns its captured output. The error is non-nil only when the process
// could not start or was killed (context ended); a non-zero exit code is reported in Result.
func Run(cmd *exec.Cmd, stdin []byte) (Result, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	err := cmd.Run()
	res := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err == nil {
		return res, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.ExitCode = ee.ExitCode()
		if res.ExitCode == -1 {
			return res, ErrTerminated
		}
		return res, nil
	}
	res.ExitCode = -1
	return res, err
}

func find(name, override string) (string, error) {
	if override != "" {
		if ValidateToolPath(name, override, runtime.GOOS) != nil {
			return "", ErrNotFound
		}
		return override, nil
	}
	if isWindows() {
		name += ".exe"
	}
	if p, err := LookPath(name); err == nil {
		return p, nil
	}
	if retryInteractive() {
		return LookPath(name)
	}
	return "", ErrNotFound
}

// FindAWS locates the aws binary (override first). The result is not symlink-resolved.
func FindAWS(override string) (string, error) { return find("aws", override) }

// FindGit locates git. On macOS /usr/bin/git is the CLT shim and is accepted only when xcode-select -p succeeds.
func FindGit(ctx context.Context, override string) (string, error) {
	p, err := find("git", override)
	if err != nil {
		return "", err
	}
	if override == "" && runtime.GOOS == "darwin" && p == "/usr/bin/git" {
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		res, rerr := Run(Command(c, "xcode-select", "-p"), nil)
		if rerr != nil || res.ExitCode != 0 {
			return "", ErrNotFound
		}
	}
	return p, nil
}

// HelperPath formats a binary path for use inside a git credential helper value: forward slashes on windows.
func HelperPath(p, goos string) string {
	if goos == "windows" {
		return strings.ReplaceAll(p, `\`, "/")
	}
	return p
}
