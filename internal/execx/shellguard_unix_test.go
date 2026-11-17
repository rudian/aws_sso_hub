//go:build !windows

package execx

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/shellguard"
)

// fakeLoginShell writes a script named zsh that records each start in marker and prints a PATH.
func fakeLoginShell(t *testing.T, marker string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "zsh")
	body := "#!/bin/sh\necho started >> '" + marker + "'\nprintf '" + pathMarker + "/fake/shell/bin" + pathMarker + "'\n"
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTestBinaryDoesNotRunALoginShellForPATHByDefault(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the login-shell PATH lookup is macOS only")
	}
	marker := filepath.Join(t.TempDir(), "marker")
	t.Setenv("SHELL", fakeLoginShell(t, marker))
	d := t.TempDir() // a temp HOME, so only the PATH skip can stop the fake shell
	t.Setenv("HOME", d)
	t.Setenv("ZDOTDIR", d)
	shellguard.ResetRefused()
	Configure(Options{}) // DisableLoginShell is not set
	t.Cleanup(func() { Configure(Options{DisableLoginShell: true}) })
	if p := EnvPATH(); strings.Contains(p, "/fake/shell/bin") {
		t.Fatalf("PATH holds the login shell's entry: %s", p)
	}
	retryInteractive()
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the login shell was started")
	}
	if len(shellguard.Refused()) != 0 {
		t.Fatalf("the skip should come before the guard: %v", shellguard.Refused())
	}
}

func TestAllowLoginShellForTestOptsIn(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the login-shell PATH lookup is macOS only")
	}
	marker := filepath.Join(t.TempDir(), "marker")
	t.Setenv("SHELL", fakeLoginShell(t, marker))
	Configure(Options{})
	t.Cleanup(func() { Configure(Options{DisableLoginShell: true}) })
	home := AllowLoginShellForTest(t)
	if h := os.Getenv("HOME"); h != home || os.Getenv("ZDOTDIR") != home || !shellguard.InTemp(home) {
		t.Fatalf("HOME=%q ZDOTDIR=%q home=%q", h, os.Getenv("ZDOTDIR"), home)
	}
	for _, f := range []string{".zshenv", ".zprofile", ".zshrc", ".zlogin", ".bash_profile", ".bashrc"} {
		if _, err := os.Stat(filepath.Join(home, f)); err != nil {
			t.Fatal(err)
		}
	}
	if p := EnvPATH(); !strings.Contains(p, "/fake/shell/bin") {
		t.Fatalf("the login shell's PATH is missing: %s", p)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("the login shell did not run: %v", err)
	}
}

func TestOptInStillRefusesANonTempHome(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the login-shell PATH lookup is macOS only")
	}
	marker := filepath.Join(t.TempDir(), "marker")
	t.Setenv("SHELL", fakeLoginShell(t, marker))
	Configure(Options{})
	t.Cleanup(func() { Configure(Options{DisableLoginShell: true}) })
	AllowLoginShellForTest(t)
	t.Setenv("HOME", "/Users/someone") // the test undoes the helper's HOME
	shellguard.ResetRefused()
	_ = EnvPATH()
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the shell started with a non-temp HOME")
	}
	if len(shellguard.Refused()) == 0 {
		t.Fatal("the refusal was not recorded")
	}
}

func TestCommandRefusesALoginShellWithANonTempHome(t *testing.T) {
	Configure(Options{DisableLoginShell: true})
	t.Setenv("HOME", "/Users/someone")
	shellguard.ResetRefused()
	for _, args := range [][]string{{"-l", "-c", "true"}, {"-lic", "true"}, {"-i", "-c", "true"}} {
		_, err := Run(Command(context.Background(), "/bin/zsh", args...), nil)
		if !errors.Is(err, shellguard.ErrRefused) {
			t.Fatalf("%v: err = %v", args, err)
		}
	}
	if err := Start("/bin/bash", []string{"-l", "-c", "true"}, "", nil, false); !errors.Is(err, shellguard.ErrRefused) {
		t.Fatalf("Start: %v", err)
	}
	// Run checks the environment the caller set after Command.
	cmd := Command(context.Background(), "/bin/zsh", "-l", "-c", "true")
	cmd.Err = nil
	cmd.Env = []string{"HOME=/Users/someone"}
	if _, err := Run(cmd, nil); !errors.Is(err, shellguard.ErrRefused) {
		t.Fatalf("Run: %v", err)
	}
	if len(shellguard.Refused()) < 5 {
		t.Fatalf("refused = %v", shellguard.Refused())
	}
}

func TestCommandAllowsAShellWithATempHomeAndPlainShells(t *testing.T) {
	Configure(Options{DisableLoginShell: true})
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", home)
	t.Setenv("BASH_ENV", "")
	res, err := Run(Command(context.Background(), "/bin/sh", "-l", "-c", "echo ok"), nil)
	if err != nil || strings.TrimSpace(string(res.Stdout)) != "ok" {
		t.Fatalf("%+v %v", res, err)
	}
	t.Setenv("HOME", "/Users/someone")
	if res, err = Run(Command(context.Background(), "/bin/sh", "-c", "echo ok"), nil); err != nil || strings.TrimSpace(string(res.Stdout)) != "ok" {
		t.Fatalf("%+v %v", res, err)
	}
}

// A refusal Command made on the process HOME does not stand when the caller then sets a temp HOME.
func TestRunDecidesOnTheFinalEnvironment(t *testing.T) {
	Configure(Options{DisableLoginShell: true})
	t.Setenv("HOME", "/Users/someone")
	shellguard.ResetRefused()
	home := t.TempDir()
	cmd := Command(context.Background(), "/bin/sh", "-l", "-c", "echo ok")
	cmd.Env = []string{"HOME=" + home, "ZDOTDIR=" + home, "PATH=/usr/bin:/bin"}
	res, err := Run(cmd, nil)
	if err != nil || strings.TrimSpace(string(res.Stdout)) != "ok" {
		t.Fatalf("%+v %v", res, err)
	}
	if len(shellguard.Refused()) != 0 {
		t.Fatalf("refused = %v", shellguard.Refused())
	}
	// A refused Command plus Run records once.
	shellguard.ResetRefused()
	if _, err := Run(Command(context.Background(), "/bin/sh", "-l", "-c", "true"), nil); !errors.Is(err, shellguard.ErrRefused) {
		t.Fatal(err)
	}
	if len(shellguard.Refused()) != 1 {
		t.Fatalf("refused = %v", shellguard.Refused())
	}
}

func TestCommandRefusesZshWhateverTheFlags(t *testing.T) {
	Configure(Options{DisableLoginShell: true})
	t.Setenv("HOME", "/Users/someone")
	shellguard.ResetRefused()
	if _, err := Run(Command(context.Background(), "/bin/zsh", "-c", "echo ran"), nil); !errors.Is(err, shellguard.ErrRefused) {
		t.Fatalf("zsh -c: %v", err)
	}
}

// In a production binary the guard is off and the login-shell PATH lookup runs: build a small program (not a
// test binary) and run it.
func TestProductionBinaryIsUnchanged(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the login-shell PATH lookup is macOS only")
	}
	gobin := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(gobin); err != nil {
		t.Skip("no go tool")
	}
	exe := filepath.Join(t.TempDir(), "inert")
	build := exec.Command(gobin, "build", "-o", exe, "./testdata/inert")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	home := t.TempDir()
	fake := fakeLoginShell(t, filepath.Join(t.TempDir(), "marker"))
	run := exec.Command(exe)
	run.Env = []string{"HOME=" + home, "ZDOTDIR=" + home, "SHELL=" + fake, "PATH=/usr/bin:/bin"}
	out, err := run.Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "true true" {
		t.Fatalf("production binary printed %q, want \"true true\"", got)
	}
}
