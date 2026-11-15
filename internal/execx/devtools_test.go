package execx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// stubWorld builds a folder of fake developer-tool stubs that write a marker file when they run, plus a fake
// xcode-select whose answer the test chooses. It points the package rule at them for the length of the test.
func stubWorld(t *testing.T, xcodeSelectScript string) (dir, marker string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	dir, _ = filepath.EvalSymlinks(t.TempDir())
	marker = filepath.Join(t.TempDir(), "ran")
	for _, n := range []string{"git", "make", "clang", "python3", "xcrun"} {
		body := "#!/bin/sh\necho " + n + " >> '" + marker + "'\necho 'git version 2.50.0'\n"
		if err := os.WriteFile(filepath.Join(dir, n), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	xs := filepath.Join(t.TempDir(), "xcode-select")
	if err := os.WriteFile(xs, []byte("#!/bin/sh\n"+xcodeSelectScript+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldDir, oldXS, oldOS := stubDir, xcodeSelectBin, stubGOOS
	stubDir, xcodeSelectBin, stubGOOS = dir, xs, "darwin"
	t.Cleanup(func() { stubDir, xcodeSelectBin, stubGOOS = oldDir, oldXS, oldOS })
	Configure(Options{PreferredDirs: []string{dir}, DisableLoginShell: true})
	t.Cleanup(func() { Configure(Options{}) })
	return dir, marker
}

func ran(marker string) bool { _, err := os.Stat(marker); return err == nil }

func TestStubNeverRunsWithoutDeveloperTools(t *testing.T) {
	cases := map[string]string{
		"xcode-select fails":      `echo "xcode-select: error: unable to get active developer directory" >&2; exit 2`,
		"prints /":                `echo /`,
		"prints nothing":          `exit 0`,
		"prints a missing folder": `echo /no/such/folder/for/codepass`,
		"prints a relative path":  `echo Developer`,
	}
	for name, script := range cases {
		t.Run(name, func(t *testing.T) {
			dir, marker := stubWorld(t, script)
			ctx := context.Background()
			if DevToolsInstalled(ctx) {
				t.Fatal("DevToolsInstalled must be false")
			}
			if _, err := FindGit(ctx, ""); err != ErrNotFound {
				t.Errorf("FindGit = %v, want ErrNotFound", err)
			}
			// a user-chosen path to the stub is held to the same rule
			if p, err := FindGit(ctx, filepath.Join(dir, "git")); err == nil {
				t.Errorf("FindGit(override stub) = %q", p)
			}
			for _, n := range []string{"git", "make", "clang", "python3", "xcrun"} {
				if err := GuardDevToolStub(ctx, filepath.Join(dir, n)); !errors.Is(err, ErrDevToolsMissing) || !errors.Is(err, ErrNotFound) {
					t.Errorf("Guard(%s) = %v", n, err)
				}
				if _, err := LookPathGuarded(ctx, n); !errors.Is(err, ErrDevToolsMissing) {
					t.Errorf("LookPathGuarded(%s) = %v", n, err)
				}
			}
			if ran(marker) {
				t.Fatal("a developer-tool stub was executed while xcode-select -p failed")
			}
		})
	}
}

func TestStubAllowedWhenDeveloperToolsExist(t *testing.T) {
	tools := t.TempDir()
	dir, marker := stubWorld(t, "echo '"+tools+"'")
	ctx := context.Background()
	if !DevToolsInstalled(ctx) {
		t.Fatal("DevToolsInstalled must be true for an existing folder")
	}
	p, err := FindGit(ctx, "")
	if err != nil || p != filepath.Join(dir, "git") {
		t.Fatalf("FindGit = %q, %v", p, err)
	}
	if ran(marker) {
		t.Fatal("finding git must not run it")
	}
}

func TestStubRuleIgnoresOtherPlacesAndSystems(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	dir, marker := stubWorld(t, "exit 2")
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "git"), []byte("#!/bin/sh\necho x >> '"+marker+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A git outside the stub folder (Homebrew's, for example) is never held back.
	if IsDevToolStub(filepath.Join(other, "git")) {
		t.Error("git outside /usr/bin is not a stub")
	}
	if IsDevToolStub(filepath.Join(dir, "aws")) {
		t.Error("aws is not a stub")
	}
	// A symlink to a stub is a stub.
	link := filepath.Join(other, "mygit")
	if err := os.Symlink(filepath.Join(dir, "git"), link); err != nil {
		t.Fatal(err)
	}
	if !IsDevToolStub(link) {
		t.Error("a link to the stub is the stub")
	}
	stubGOOS = "linux"
	if IsDevToolStub(filepath.Join(dir, "git")) || !DevToolsInstalled(context.Background()) {
		t.Error("the rule is macOS only")
	}
}
