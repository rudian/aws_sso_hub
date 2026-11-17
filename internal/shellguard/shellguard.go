// Package shellguard keeps tests away from the owner's shell configuration. In a test binary, starting a
// shell as a login or interactive shell (zsh -l, bash -i, sh -lic, ...) is refused unless HOME (and, for zsh,
// ZDOTDIR) lies inside the temp folder, because such a shell reads ~/.zshenv, ~/.zprofile, ~/.zlogin and the
// like. A refusal is recorded (Refused) and returned as an error. In a production binary nothing changes:
// Check always returns nil.
//
// The same idea as internal/awsguard: testing.Testing() switches the guard on.
package shellguard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ErrRefused is wrapped by the error Check returns for a refused start.
var ErrRefused = errors.New("shellguard: refusing to start a login or interactive shell outside a temp HOME in a test binary")

// active reports whether the guard is on; a variable so a test can prove the production case.
var active = testing.Testing

var (
	mu      sync.Mutex
	refused []string
)

// Refused lists the refused commands (program and flags) since the process started, for tests.
func Refused() []string {
	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), refused...)
}

// ResetRefused forgets the recorded refusals (for tests of the guard itself).
func ResetRefused() {
	mu.Lock()
	defer mu.Unlock()
	refused = nil
}

var shells = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "fish": true, "dash": true, "ksh": true, "tcsh": true, "csh": true,
}

// IsShell reports whether program is a shell by its file name (zsh, bash, sh, fish and the like; ".exe" is ignored).
func IsShell(program string) bool {
	b := strings.ToLower(filepath.Base(strings.ReplaceAll(program, `\`, "/")))
	b = strings.TrimSuffix(b, ".exe")
	return shells[b]
}

// flags is what the leading arguments of a shell say.
type flags struct {
	login, interactive, command, operand, noRC bool
}

// valueOpts take the next argument as their value.
var valueOpts = map[string]bool{"--rcfile": true, "--init-file": true}

// fishValueOpts are the fish options that take the next argument as their value.
var fishValueOpts = map[string]bool{
	"--features": true, "--init-command": true, "--debug": true, "--debug-output": true, "--profile": true,
}

// noRCLetter is the short flag that turns the startup files off, per shell: zsh -f, fish -N, csh and tcsh -f.
// Any other letter means something else (zsh -N is AUTO_PUSHD, fish -f is --features).
func noRCLetter(name string, c rune) bool {
	switch name {
	case "zsh", "csh", "tcsh":
		return c == 'f'
	case "fish":
		return c == 'N'
	}
	return false
}

// parseFlags reads the leading arguments of the shell called name (lower case base name, may be empty).
func parseFlags(args []string, name string) flags {
	var f flags
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			f.operand = i+1 < len(args)
			return f
		}
		if a == "" || (a[0] != '-' && a[0] != '+') {
			f.operand = true
			return f
		}
		switch a {
		case "--login":
			f.login = true
			continue
		case "--interactive":
			f.interactive = true
			continue
		case "--no-rcs":
			f.noRC = f.noRC || name == "zsh"
			continue
		case "--no-config":
			f.noRC = f.noRC || name == "fish"
			continue
		}
		if valueOpts[a] || (name == "fish" && fishValueOpts[a]) {
			i++
			continue
		}
		if strings.HasPrefix(a, "--") {
			continue
		}
		for _, c := range a[1:] {
			switch c {
			case 'l':
				f.login = f.login || a[0] == '-'
			case 'i':
				f.interactive = f.interactive || a[0] == '-'
			case 'c':
				f.command = true
			case 'f', 'N':
				if noRCLetter(name, c) {
					f.noRC = f.noRC || a[0] == '-'
				} else if name == "fish" && c == 'f' && i+1 < len(args) {
					i++ // fish -f FEATURES
				}
			case 'C', 'd', 'p':
				if name == "fish" && i+1 < len(args) {
					i++ // fish -C COMMAND, -d CATEGORIES, -p FILE
				}
			case 'o', 'O':
				if i+1 < len(args) {
					i++
					if c == 'o' && a[0] == '-' && name != "fish" {
						switch args[i] {
						case "login":
							f.login = true
						case "interactive":
							f.interactive = true
						case "norcs":
							f.noRC = true
						}
					}
				}
			}
		}
	}
	return f
}

// LoginOrInteractive reports whether the shell arguments (without the program name) ask for a login or
// interactive shell: -l, -i, a combined flag such as -lic, --login, --interactive, -o login. Options that
// take a value (-o, -O, --rcfile) are skipped; scanning stops at the first argument that is not a flag.
func LoginOrInteractive(args []string) bool {
	f := parseFlags(args, "")
	return f.login || f.interactive
}

func envValue(env []string, key string) (string, bool) {
	val, ok := "", false
	for _, kv := range env {
		if k, v, found := strings.Cut(kv, "="); found && k == key {
			val, ok = v, true
		}
	}
	return val, ok
}

func resolve(p string) string {
	p = filepath.Clean(p)
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// InTemp reports whether path is inside os.TempDir() (symbolic links resolved on both sides).
func InTemp(path string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	root := resolve(os.TempDir())
	rel, err := filepath.Rel(root, resolve(path))
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// isShellFor reports whether argv0 is a shell: a known name, or the program $SHELL names (env, else the
// process environment), by base name or by symbolic-link-resolved path.
func isShellFor(argv0 string, env []string) bool {
	if IsShell(argv0) {
		return true
	}
	sh, ok := envValue(env, "SHELL")
	if !ok {
		sh = os.Getenv("SHELL")
	}
	if sh == "" {
		return false
	}
	if filepath.Base(sh) == filepath.Base(argv0) {
		return true
	}
	return filepath.IsAbs(sh) && filepath.IsAbs(argv0) && resolve(sh) == resolve(argv0)
}

func baseName(argv0 string) string {
	b := strings.ToLower(filepath.Base(strings.ReplaceAll(argv0, `\`, "/")))
	return strings.TrimSuffix(b, ".exe")
}

// evaluate decides whether a start would read the owner's shell files. terminal: the child runs on a
// terminal, so a shell with no command and no script is interactive.
func evaluate(argv0 string, args, env []string, terminal bool) (desc, bad string) {
	if !active() || !isShellFor(argv0, env) {
		return "", ""
	}
	name := baseName(argv0)
	f := parseFlags(args, name)
	interactive := f.interactive || (terminal && !f.command && !f.operand)
	switch name {
	case "zsh", "fish", "csh", "tcsh":
		// zsh reads $ZDOTDIR/.zshenv on every start, fish reads its config on every start.
		if f.noRC && !f.login && !f.interactive {
			return "", ""
		}
	default:
		if !f.login && !interactive {
			return "", ""
		}
	}
	if env == nil {
		env = os.Environ()
	}
	home, _ := envValue(env, "HOME")
	switch {
	case !InTemp(home):
		bad = "HOME=" + home
	default:
		if zd, ok := envValue(env, "ZDOTDIR"); ok && zd != "" && !InTemp(zd) {
			bad = "ZDOTDIR=" + zd
		} else if xdg, ok := envValue(env, "XDG_CONFIG_HOME"); ok && xdg != "" && name == "fish" && !InTemp(xdg) {
			bad = "XDG_CONFIG_HOME=" + xdg
		} else {
			for _, k := range []string{"BASH_ENV", "ENV"} {
				if v, ok := envValue(env, k); ok && v != "" && !InTemp(v) {
					bad = k + "=" + v
				}
			}
		}
	}
	if bad == "" {
		return "", ""
	}
	return strings.Join(append([]string{argv0}, args...), " "), bad
}

func record(desc string) {
	mu.Lock()
	refused = append(refused, desc)
	mu.Unlock()
}

func refuse(desc, bad string) error {
	return fmt.Errorf("%w: %s (%s)", ErrRefused, desc, bad)
}

// Check returns an error (and records it) when, in a test binary, argv0 is a shell that would read the owner's
// shell files (a login or interactive shell; zsh and fish on every start) while HOME, or ZDOTDIR,
// XDG_CONFIG_HOME (fish), BASH_ENV and ENV when set, is not inside the temp folder. env is the child's
// environment; nil means the process environment. It returns nil in a production binary.
func Check(argv0 string, args []string, env []string) error {
	desc, bad := evaluate(argv0, args, env, false)
	if bad == "" {
		return nil
	}
	record(desc)
	return refuse(desc, bad)
}

// CheckTerminal is Check for a child on a terminal: a shell with no -c and no script is interactive.
func CheckTerminal(argv0 string, args []string, env []string) error {
	desc, bad := evaluate(argv0, args, env, true)
	if bad == "" {
		return nil
	}
	record(desc)
	return refuse(desc, bad)
}

// Would is Check without recording: for an early test whose result a later Check may replace.
func Would(argv0 string, args []string, env []string) error {
	desc, bad := evaluate(argv0, args, env, false)
	if bad == "" {
		return nil
	}
	return refuse(desc, bad)
}

// RefuseInTest records and returns ErrRefused in a test binary, and nil in a production binary. A caller that
// would start the owner's shell indirectly (a terminal window that runs the login shell) calls it first.
func RefuseInTest(desc string) error {
	if !active() {
		return nil
	}
	record(desc)
	return fmt.Errorf("%w: %s", ErrRefused, desc)
}
