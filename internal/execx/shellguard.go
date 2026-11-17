package execx

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// loginShellOptIn is set by AllowLoginShellForTest. Without it a test binary never runs the owner's login
// shell to learn the PATH (internal/shellguard keeps the same rule for every other start).
var loginShellOptIn atomic.Bool

// loginShellEnabledLocked reports whether EnvPATH may start a login shell. mu is held.
func loginShellEnabledLocked() bool {
	if opts.DisableLoginShell {
		return false
	}
	return !testing.Testing() || loginShellOptIn.Load()
}

func resetPathCache() {
	mu.Lock()
	defer mu.Unlock()
	cachedPATH, pathReady, interactive, shellPath = "", false, false, ""
}

// AllowLoginShellForTest lets this test's EnvPATH run a login shell, which in a test binary is otherwise
// skipped. The shell reads shell files of the test's own: a new temp HOME and ZDOTDIR (the same folder) with
// empty .zshenv, .zprofile, .zshrc, .zlogin, .bash_profile and .bashrc, set with t.Setenv; BASH_ENV and ENV
// are cleared. It returns that folder so the test can write its own shell files. SHELL must name a shell the
// test chose (set it with t.Setenv first); otherwise the inherited SHELL runs. The opt-in has no effect while
// Options.DisableLoginShell is set (an earlier Configure or ContainerOptions): the test must Configure without it.
func AllowLoginShellForTest(t testing.TB) string {
	t.Helper()
	home := t.TempDir()
	for _, f := range []string{".zshenv", ".zprofile", ".zshrc", ".zlogin", ".bash_profile", ".bashrc"} {
		if err := os.WriteFile(filepath.Join(home, f), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", home)
	t.Setenv("BASH_ENV", "")
	t.Setenv("ENV", "")
	os.Unsetenv("BASH_ENV")
	os.Unsetenv("ENV")
	resetPathCache()
	loginShellOptIn.Store(true)
	t.Cleanup(func() {
		loginShellOptIn.Store(false)
		resetPathCache()
	})
	return home
}
