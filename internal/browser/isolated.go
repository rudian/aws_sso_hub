package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Isolated is a browser instance started for one sign-in with its own, empty profile directory. It shares
// no cookies with the user's browser or with another sign-in, so the sign-in page can never pick up a web
// session of another IAM Identity Center user. Even private windows don't give that: all private windows of
// a browser share one session while any of them is open. Close ends the instance and deletes the profile.
type Isolated struct {
	o    *Opener
	goos string
	dir  string
	pid  int // Windows: the started browser process. macOS finds the processes by their profile directory.
	once sync.Once
	err  error
}

// Dir is the profile directory of the instance.
func (w *Isolated) Dir() string { return w.dir }

const (
	psPath       = "/bin/ps"
	userDataFlag = "--user-data-dir="
	// closeWait is how long Close waits for the browser to quit after asking it to, before it forces it.
	closeWait = 5 * time.Second
	// localState is the file a Chromium-based browser writes into the root of the profile it runs on.
	localState = "Local State"
	// userDataDirPolicy is the browser policy that moves the profile and overrides --user-data-dir.
	userDataDirPolicy = "UserDataDir"
)

// isolatedArgs are the switches for a Chromium-based browser started on its own profile: no first-run
// pages, no default-browser question and no search-engine choice screen in front of the sign-in page.
func isolatedArgs(kind browserKind, dir, rawURL string) []string {
	return []string{userDataFlag + dir, "--no-first-run", "--no-default-browser-check",
		"--disable-search-engine-choice-screen", kind.flag, rawURL}
}

// OpenIsolated opens rawURL in a private window of a new instance of the default browser that uses dir, an
// empty directory, as its profile. (nil, nil): the default browser cannot be started this way (only Chrome,
// Edge, Brave and Chromium are), its private mode is turned off by policy, a UserDataDir policy would put it
// on another profile anyway, or the system is not supported; nothing was launched and the caller falls back
// to OpenPrivate.
func (o *Opener) OpenIsolated(ctx context.Context, rawURL, dir string) (*Isolated, error) {
	if err := CheckURL(rawURL); err != nil {
		return nil, err
	}
	goos := o.goos()
	abs := filepath.IsAbs(dir)
	if goos == "windows" {
		abs = winAbs(dir)
	}
	if !abs || strings.ContainsAny(dir, "\x00\r\n") {
		return nil, errors.New("browser: the profile directory must be an absolute path")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	switch goos {
	case "darwin":
		id, err := o.defaultMacBundle(ctx)
		if err != nil {
			return nil, err
		}
		kind, ok := macKind(id)
		if !ok || !kind.isolate || o.macPolicyDisabled(ctx, kind) {
			return nil, nil
		}
		if _, set := o.macPolicy(ctx, kind, userDataDirPolicy); set {
			return nil, nil
		}
		// -n starts a new instance; the browser keeps it separate because the profile directory differs.
		args := append([]string{"-n", "-b", id, "--args"}, isolatedArgs(kind, dir, rawURL)...)
		if _, err := o.output(ctx, openPath, args...); err != nil {
			return nil, fmt.Errorf("cannot start %s: %w", kind.name, err)
		}
		return &Isolated{o: o, goos: goos, dir: dir}, nil
	case "windows":
		exe, kind, ok, err := o.windowsBrowser(ctx)
		if err != nil || !ok || !kind.isolate {
			return nil, err
		}
		if _, set := o.winPolicyString(kind.winPolicy, userDataDirPolicy); set {
			return nil, nil
		}
		pid, err := o.startPID(ctx, exe, isolatedArgs(kind, dir, rawURL)...)
		if err != nil {
			return nil, fmt.Errorf("cannot start %s: %w", kind.name, err)
		}
		return &Isolated{o: o, goos: goos, dir: dir, pid: pid}, nil
	}
	return nil, nil
}

// Close quits the browser instance (gracefully first, then by force) and deletes its profile directory.
// It is safe to call more than once.
func (w *Isolated) Close(ctx context.Context) error {
	w.once.Do(func() {
		if ctx == nil {
			ctx = context.Background()
		}
		w.o.endProfile(ctx, w.goos, w.dir, w.pid)
		w.err = removeAllRetry(ctx, w.dir)
	})
	return w.err
}

// RemoveStale deletes the profile directories under root that keep(dir) does not claim, left behind by an
// earlier run (for example after a crash). On macOS it first ends the browser still running on one; on
// Windows, where the processes of an earlier run cannot be told apart safely, a profile still in use is kept.
func (o *Opener) RemoveStale(ctx context.Context, root string, keep func(dir string) bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	goos := o.goos()
	for _, e := range entries {
		dir := filepath.Join(root, e.Name())
		if !e.IsDir() || (keep != nil && keep(dir)) {
			continue
		}
		if goos == "windows" {
			// Windows refuses to rename a folder while a running program holds files in it.
			gone := dir + ".removing"
			if os.Rename(dir, gone) == nil {
				_ = removeAllRetry(ctx, gone)
			}
			continue
		}
		o.endProfile(ctx, goos, dir, 0)
		_ = removeAllRetry(ctx, dir)
	}
}

// profileUsed reports whether a browser has written its profile into dir, waiting up to profileWait for a
// browser that has just started. A browser that a policy put on another profile never does, and must not be
// ended: it may be the user's own browser.
func profileUsed(ctx context.Context, dir string) bool {
	deadline := time.Now().Add(profileWait)
	for {
		if _, err := os.Stat(filepath.Join(dir, localState)); err == nil {
			return true
		}
		if !time.Now().Before(deadline) || ctx.Err() != nil {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// endProfile ends the browser processes that use dir as their profile.
func (o *Opener) endProfile(ctx context.Context, goos, dir string, pid int) {
	switch goos {
	case "darwin":
		pids := o.profilePIDs(ctx, dir)
		if len(pids) == 0 || !profileUsed(ctx, dir) {
			return
		}
		for _, p := range pids {
			_ = o.signal(p, false)
		}
		deadline := time.Now().Add(closeWait)
		for time.Now().Before(deadline) && ctx.Err() == nil {
			time.Sleep(200 * time.Millisecond)
			if pids = o.profilePIDs(ctx, dir); len(pids) == 0 {
				return
			}
		}
		for _, p := range pids {
			_ = o.signal(p, true)
		}
	case "windows":
		if pid <= 0 {
			return
		}
		defer o.release(pid)
		// Only while the started browser still runs (its kept handle reserves the id) and runs on dir.
		if !o.running(pid) || !profileUsed(ctx, dir) {
			return
		}
		// /T ends the browser's child processes (renderers, GPU) with it.
		_, _ = o.output(ctx, taskkillPath(), "/PID", strconv.Itoa(pid), "/T", "/F")
	}
}

// profilePIDs lists the current user's processes whose command line names dir as the profile directory.
func (o *Opener) profilePIDs(ctx context.Context, dir string) []int {
	out, err := o.output(ctx, psPath, "-axww", "-o", "pid=,uid=,command=")
	if err != nil {
		return nil
	}
	return parsePS(string(out), os.Getuid(), dir)
}

// parsePS picks from `ps -o pid=,uid=,command=` output the processes of uid that use dir as their profile.
func parsePS(out string, uid int, dir string) []int {
	flag := userDataFlag + dir
	var pids []int
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		owner, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil || owner != uid || pid <= 0 {
			continue
		}
		// The flag must be a whole argument: dir must not just be a prefix of another directory.
		cmd := " " + strings.Join(f[2:], " ") + " "
		if strings.Contains(cmd, " "+flag+" ") {
			pids = append(pids, pid)
		}
	}
	return pids
}

// removeAllRetry deletes dir. A browser that has just quit can still hold files for a moment (Windows).
func removeAllRetry(ctx context.Context, dir string) error {
	var err error
	for i := 0; i < 10; i++ {
		if err = os.RemoveAll(dir); err == nil || ctx.Err() != nil {
			return err
		}
		time.Sleep(300 * time.Millisecond)
	}
	return err
}

// profileWait is how long Close waits for a just-started browser to write its profile (a variable for tests).
var profileWait = 3 * time.Second

func (o *Opener) goos() string {
	if o.GOOS != "" {
		return o.GOOS
	}
	return runtime.GOOS
}

func (o *Opener) signal(pid int, force bool) error {
	if o.Signal != nil {
		return o.Signal(pid, force)
	}
	return signalPID(pid, force)
}

func (o *Opener) running(pid int) bool {
	if o.Running != nil {
		return o.Running(pid)
	}
	return processRunning(pid)
}

func (o *Opener) release(pid int) {
	if o.Running == nil {
		releaseProcess(pid)
	}
}

func (o *Opener) winPolicyString(subkey, name string) (string, bool) {
	if subkey == "" {
		return "", false
	}
	if o.WinPolicyString != nil {
		return o.WinPolicyString(subkey, name)
	}
	return winPolicyString(subkey, name)
}

func (o *Opener) startPID(ctx context.Context, name string, args ...string) (int, error) {
	if o.StartPID != nil {
		return o.StartPID(ctx, name, args...)
	}
	return startProcess(name, args)
}

func taskkillPath() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return root + `\System32\taskkill.exe`
}
