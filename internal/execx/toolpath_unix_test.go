//go:build !windows

package execx

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

func TestGroupTrusted(t *testing.T) {
	cases := []struct {
		gid  uint32
		goos string
		want bool
	}{
		{0, "darwin", true}, {80, "darwin", true}, {20, "darwin", false},
		{0, "linux", true}, {80, "linux", false}, {1000, "linux", false},
	}
	for _, c := range cases {
		if got := groupTrusted(c.gid, c.goos); got != c.want {
			t.Errorf("groupTrusted(%d, %s) = %v, want %v", c.gid, c.goos, got, c.want)
		}
	}
}

// A folder like Homebrew's prefix (mode 0775, owned by the user, group admin) holds a trusted program.
func TestValidateToolPathAcceptsAdminGroupFolder(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the admin group (gid 80) is a macOS convention")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	cellar := filepath.Join(root, "Cellar", "bin")
	for _, d := range []string{bin, cellar} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chown(bin, -1, 80); err != nil {
		t.Skipf("the current user is not in the admin group: %v", err)
	}
	if err := os.Chmod(bin, 0o775); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(cellar, "aws")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(bin, "aws")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := ValidateToolPath("aws", link, runtime.GOOS); err != nil {
		t.Errorf("symlink in a 0775 user:admin folder: %v", err)
	}
	// The same folder holding the program itself.
	direct := filepath.Join(bin, "git")
	if err := os.WriteFile(direct, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateToolPath("git", direct, runtime.GOOS); err != nil {
		t.Errorf("program in a 0775 user:admin folder: %v", err)
	}
	// World-writable stays refused whatever the group is.
	if err := os.Chmod(bin, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := ValidateToolPath("git", direct, runtime.GOOS); err == nil {
		t.Error("a world-writable folder must be refused")
	}
}

// A group-writable folder whose group is not a trusted one (for example staff or a shared group) is refused.
func TestValidateToolPathRejectsUntrustedGroupFolder(t *testing.T) {
	dir := t.TempDir()
	st, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); !ok || groupTrusted(sys.Gid, runtime.GOOS) {
		t.Skip("the temp folder already belongs to a trusted group")
	}
	if err := os.Chmod(dir, 0o775); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "aws")
	if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateToolPath("aws", p, runtime.GOOS); err == nil {
		t.Error("a group-writable folder of an untrusted group must be refused")
	}
}

func TestValidateToolPathAcceptsRootOwnedTool(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("relies on /usr/bin/git")
	}
	if _, err := os.Stat("/usr/bin/git"); err != nil {
		t.Skip("no /usr/bin/git")
	}
	if err := ValidateToolPath("git", "/usr/bin/git", runtime.GOOS); err != nil {
		t.Errorf("/usr/bin/git: %v", err)
	}
}
