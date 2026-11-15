package execx

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrDevToolsMissing is returned for a macOS developer-tool stub when the Command Line Tools are not
// installed. Running the stub would open the "install developer tools" dialog. It matches ErrNotFound with
// errors.Is, so a caller that only asks "is the tool there?" needs no new case.
var ErrDevToolsMissing = fmt.Errorf("%w: developer tools are not installed", ErrNotFound)

// devToolStubs are the programs in /usr/bin that macOS ships as shims for the Command Line Tools. Without the
// tools, running any of them starts the installer dialog (checklist item 10).
var devToolStubs = map[string]bool{
	"git": true, "make": true, "clang": true, "clang++": true, "cc": true, "c++": true, "gcc": true, "g++": true,
	"python": true, "python3": true, "pip3": true, "xcrun": true, "swift": true, "swiftc": true, "lldb": true,
	"ld": true, "ar": true, "nm": true, "strip": true, "otool": true, "lipo": true, "ranlib": true, "svn": true,
	"svnadmin": true, "m4": true, "gm4": true, "bison": true, "flex": true, "git-lfs": true, "gitk": true,
	"git-receive-pack": true, "git-upload-pack": true, "git-shell": true, "git-upload-archive": true,
	"xcodebuild": true, "instruments": true, "simctl": true, "dsymutil": true, "size": true, "strings": true,
}

// Variables so tests can point the rule at a folder of fake stubs and a fake xcode-select.
var (
	stubDir        = "/usr/bin"
	xcodeSelectBin = "/usr/bin/xcode-select"
	stubGOOS       = runtime.GOOS
)

// IsDevToolStub reports whether path is one of the developer-tool shims in /usr/bin (macOS only). A symlink is
// resolved first, so /usr/local/bin/git -> /usr/bin/git is treated like the stub itself.
func IsDevToolStub(path string) bool {
	if stubGOOS != "darwin" || path == "" {
		return false
	}
	is := func(p string) bool { return filepath.Dir(p) == stubDir && devToolStubs[filepath.Base(p)] }
	if is(filepath.Clean(path)) {
		return true
	}
	if real, err := filepath.EvalSymlinks(path); err == nil && is(real) {
		return true
	}
	return false
}

// DevToolsInstalled reports whether `xcode-select -p` prints an existing folder other than "/". It is the only
// question that is safe to ask on a Mac without Command Line Tools: xcode-select itself never opens the dialog.
// The child gets a fixed environment, because DEVELOPER_DIR would change the answer.
func DevToolsInstalled(ctx context.Context) bool {
	if stubGOOS != "darwin" {
		return true
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, xcodeSelectBin, "-p")
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" || dir == "/" || !filepath.IsAbs(dir) {
		return false
	}
	st, err := os.Stat(dir)
	return err == nil && st.IsDir()
}

// GuardDevToolStub returns ErrDevToolsMissing when path is a developer-tool stub and the tools are not
// installed. Every caller that is about to run something it did not install itself asks this first. For any
// other path it does nothing.
func GuardDevToolStub(ctx context.Context, path string) error {
	if !IsDevToolStub(path) {
		return nil
	}
	if DevToolsInstalled(ctx) {
		return nil
	}
	return ErrDevToolsMissing
}

// LookPathGuarded is LookPath with the stub rule applied to the result.
func LookPathGuarded(ctx context.Context, name string) (string, error) {
	p, err := LookPath(name)
	if err != nil {
		return "", err
	}
	if err := GuardDevToolStub(ctx, p); err != nil {
		return "", err
	}
	return p, nil
}

// SetDevToolStubRoot points the stub rule at a folder of fake stubs, a fake xcode-select and a pretend OS, and
// returns the function that puts the real values back. It exists so tests in other packages (the tools gate) can
// prove that no stub runs while the developer tools are missing. Production code never calls it.
func SetDevToolStubRoot(dir, xcodeSelect, goos string) (restore func()) {
	oldDir, oldXS, oldOS := stubDir, xcodeSelectBin, stubGOOS
	stubDir, xcodeSelectBin, stubGOOS = dir, xcodeSelect, goos
	return func() { stubDir, xcodeSelectBin, stubGOOS = oldDir, oldXS, oldOS }
}
