package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/api"
)

func TestRevealCommandDarwinNeverOpensBundles(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "Tools.app")
	if err := os.MkdirAll(filepath.Join(bundle, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "Contents", "Info.plist"), []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	name, args, cmdLine, err := revealCommand(runtime.GOOS, bundle)
	if runtime.GOOS != "darwin" {
		t.Skip("darwin command")
	}
	if err != nil || name != "/usr/bin/open" || cmdLine != "" {
		t.Fatalf("%q %q %v", name, cmdLine, err)
	}
	if len(args) != 2 || args[0] != "-R" || args[1] != bundle {
		t.Errorf("args = %q, want [-R %s]", args, bundle)
	}
}

func TestRevealCommandKeepsEnvironmentReferences(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin command")
	}
	t.Setenv("HOME", t.TempDir())
	p := filepath.Join(t.TempDir(), "$HOME")
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatal(err)
	}
	_, args, _, err := revealCommand("darwin", p)
	if err != nil || args[1] != p || !strings.Contains(args[1], "$HOME") {
		t.Errorf("%q %v", args, err)
	}
}

func TestRevealCommandRejectsBadPaths(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin command")
	}
	dir := t.TempDir()
	for name, p := range map[string]string{
		"relative":   "some/dir",
		"quote":      filepath.Join(dir, `a"b`),
		"newline":    filepath.Join(dir, "a\nb"),
		"nul":        filepath.Join(dir, "a\x00b"),
		"missing":    filepath.Join(dir, "missing"),
		"empty":      "",
		"option":     "-R",
		"separators": "\u2028",
	} {
		if _, _, _, err := revealCommand("darwin", p); err == nil {
			t.Errorf("%s: %q must be refused", name, p)
		}
	}
	if _, _, _, err := revealCommand("darwin", filepath.Join(dir, "missing")); err == nil || err.(*api.AppError).Code != api.ErrFileIO {
		t.Errorf("missing path should be a file error: %v", err)
	}
}

func TestRevealCommandWindows(t *testing.T) {
	t.Setenv("SystemRoot", `C:\Windows`)
	name, args, cmdLine, err := revealCommand("windows", `C:\x\Tools`)
	if err != nil {
		t.Fatal(err)
	}
	if cmdLine != `explorer.exe /select,"C:\x\Tools"` || name != `C:\Windows\explorer.exe` || len(args) != 0 {
		t.Errorf("%q %q %q", name, args, cmdLine)
	}
	for _, p := range []string{`C:\x"y`, "C:\\x\ny", `relative\dir`, `/tmp/x`} {
		if _, _, _, err := revealCommand("windows", p); err == nil {
			t.Errorf("%q must be refused", p)
		}
	}
}

func TestCheckOpenableLoopbackOnlyInSandbox(t *testing.T) {
	loop := "http://127.0.0.1:8080/x"
	if err := checkOpenable(loop, false); err == nil {
		t.Error("loopback http must fail outside sandbox mode")
	}
	if err := checkOpenable(loop, true); err != nil {
		t.Errorf("loopback http in sandbox mode: %v", err)
	}
	for _, sandbox := range []bool{false, true} {
		if err := checkOpenable("https://acme.awsapps.com/start", sandbox); err != nil {
			t.Errorf("https (sandbox=%v): %v", sandbox, err)
		}
		if err := checkOpenable("http://acme.awsapps.com/start", sandbox); err == nil {
			t.Errorf("remote http must fail (sandbox=%v)", sandbox)
		}
		if err := checkOpenable("file:///etc/passwd", sandbox); err == nil {
			t.Errorf("file URL must fail (sandbox=%v)", sandbox)
		}
	}
}

// The Wails file-explorer helper expands environment variables and opens directories; it must not be used.
func TestNoWailsFileExplorerCalls(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	needle := "OpenFile" + "Manager("
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), needle) {
			t.Errorf("%s calls the Wails file explorer helper", f)
		}
	}
}
