//go:build windows

package update

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyBundleWindows(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "codepass.exe")
	buildExe(t, exe, [4]uint16{1, 0, 0, 2}, nil)
	if err := VerifyBundle(exe, "1.0.0-rc.2", Gate{}); err != nil {
		t.Fatalf("matching exe refused: %v", err)
	}
	err := VerifyBundle(exe, "1.0.0-rc.3", Gate{})
	if !IsRefusal(err) || RefusalCode(err) != CodeBundleVersion {
		t.Fatalf("version mismatch: %v", err)
	}
	if err := VerifyBundle(dir, "1.0.0-rc.2", Gate{}); RefusalCode(err) != CodeBundleInvalid {
		t.Fatalf("directory: %v", err)
	}
	txt := filepath.Join(dir, "codepass.txt")
	_ = os.WriteFile(txt, []byte("x"), 0o644)
	if err := VerifyBundle(txt, "1.0.0-rc.2", Gate{}); RefusalCode(err) != CodeBundleInvalid {
		t.Fatalf("not an exe: %v", err)
	}
	if err := VerifyBundle(filepath.Join(dir, "none.exe"), "1.0.0", Gate{}); RefusalCode(err) != CodeBundleInvalid {
		t.Fatalf("missing: %v", err)
	}
	if !GateSupported() {
		t.Error("gate not supported on windows")
	}
}
