package toolinstall

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeOSA writes a program that stands in for /usr/bin/osascript: it records its arguments (one per line, with
// the NUL-free text of each) and its whole environment, prints the given stdout and stderr, and exits.
func fakeOSA(t *testing.T, dir string, exit int, stdout, stderr string, onOK string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script")
	}
	p := filepath.Join(dir, "osascript")
	body := "#!/bin/sh\n" +
		"i=0; for a in \"$@\"; do printf '%s' \"$a\" > '" + dir + "/osa-arg-'$i; i=$((i+1)); done\n" +
		"echo $i > '" + dir + "/osa-argc'\n" +
		"/usr/bin/env > '" + dir + "/osa-env'\n" +
		"pwd > '" + dir + "/osa-pwd'\n"
	if exit == 0 && onOK != "" {
		body += onOK + "\n"
	}
	body += "printf '%s' '" + stdout + "'\nprintf '%s' '" + stderr + "' >&2\nexit " + itoa(exit) + "\n"
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func osaArgs(t *testing.T, dir string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "osa-argc"))
	if err != nil {
		t.Fatalf("osascript did not run: %v", err)
	}
	n := 0
	for _, c := range strings.TrimSpace(string(b)) {
		n = n*10 + int(c-'0')
	}
	out := make([]string, n)
	for i := range out {
		a, _ := os.ReadFile(filepath.Join(dir, "osa-arg-"+itoa(i)))
		out[i] = string(a)
	}
	return out
}

func TestOSAScriptCallShape(t *testing.T) {
	dir := t.TempDir()
	fake := fakeOSA(t, dir, 1, "", "42:99: execution error: User canceled. (-128)", "")
	// The app's own environment is full of things that must not reach osascript.
	t.Setenv("BASH_ENV", "/tmp/evil")
	t.Setenv("SHELLOPTS", "xtrace")
	t.Setenv("PS4", "$(touch x)")
	t.Setenv("HTTPS_PROXY", "http://user:secret@proxy.example.com:8080")
	t.Setenv("HOME", "/Users/example")
	res, err := runOSAScript(context.Background(), fake, PkgRequest{
		Path: "/Users/example/Library/Application Support/Codepass/it's \"odd\" $(touch x); `id`.pkg",
		Team: HomebrewTeamID, Size: 154292112, Sha256: strings.Repeat("a", 64), Mode: "install", Prompt: "Codepass needs your password to install Homebrew.",
	})
	if err != nil || res.Exit != 1 || !strings.Contains(res.Stderr, "(-128)") {
		t.Fatalf("%+v %v", res, err)
	}
	env, _ := os.ReadFile(filepath.Join(dir, "osa-env"))
	envLines := strings.Fields(strings.TrimSpace(string(env)))
	// env(1) itself may add nothing; the child must hold the one fixed PATH and no more (a shell may add PWD/_/SHLVL).
	for _, l := range envLines {
		k, _, _ := strings.Cut(l, "=")
		switch k {
		case "PATH", "PWD", "_", "SHLVL", "OLDPWD", "__CF_USER_TEXT_ENCODING":
		default:
			t.Errorf("osascript inherited %s", k)
		}
	}
	if !strings.Contains(string(env), "PATH=/usr/bin:/bin:/usr/sbin:/sbin") {
		t.Errorf("env: %s", env)
	}
	if strings.Contains(string(env), "secret") || strings.Contains(string(env), "BASH_ENV") {
		t.Errorf("the app's environment leaked: %s", env)
	}
	if pwd, _ := os.ReadFile(filepath.Join(dir, "osa-pwd")); strings.TrimSpace(string(pwd)) != "/" {
		t.Errorf("working folder %q", pwd)
	}

	args := osaArgs(t, dir)
	// -e <line> x4, then "--", then the seven values.
	if len(args) != 2*len(appleScriptLines)+1+7 {
		t.Fatalf("argc = %d", len(args))
	}
	for i, l := range appleScriptLines {
		if args[2*i] != "-e" || args[2*i+1] != l {
			t.Errorf("AppleScript line %d: %q %q", i, args[2*i], args[2*i+1])
		}
	}
	vals := args[2*len(appleScriptLines)+1:]
	if args[2*len(appleScriptLines)] != "--" {
		t.Error("missing --")
	}
	if vals[0] != pkgScript {
		t.Error("the shell text is not the embedded constant")
	}
	if vals[1] != "/Users/example/Library/Application Support/Codepass/it's \"odd\" $(touch x); `id`.pkg" || vals[2] != "927JGANW46" ||
		vals[3] != "154292112" || vals[4] != strings.Repeat("a", 64) || vals[5] != "install" || vals[6] == "" {
		t.Errorf("values: %q", vals[1:])
	}
	// Unknown size and digest are "-".
	osaFresh := fakeOSA(t, t.TempDir(), 0, "", "", "")
	_ = osaFresh
	a := osascriptArgs(pkgScript, PkgRequest{Path: "/x.pkg", Team: HomebrewTeamID})
	if tail := a[len(a)-5:]; tail[1] != "-" || tail[2] != "-" || tail[3] != "install" {
		t.Errorf("defaults: %q", tail)
	}
}

// Every value that reaches the shell command goes through `quoted form of`; nothing else is spliced in.
func TestAppleScriptQuotesEveryArgument(t *testing.T) {
	var cmdLine string
	for _, l := range appleScriptLines {
		if strings.HasPrefix(l, "set sh to") {
			cmdLine = l
		}
	}
	if cmdLine == "" {
		t.Fatal("no command line in the AppleScript")
	}
	for i := 1; i <= 6; i++ {
		if !strings.Contains(cmdLine, "quoted form of (item "+itoa(i)+" of argv)") {
			t.Errorf("item %d is not quoted", i)
		}
	}
	// No bare "item N of argv" outside quoted form of.
	stripped := cmdLine
	for i := 1; i <= 6; i++ {
		stripped = strings.ReplaceAll(stripped, "quoted form of (item "+itoa(i)+" of argv)", "")
	}
	if strings.Contains(stripped, "argv") {
		t.Errorf("an argument is used without quoted form of: %s", stripped)
	}
	if !strings.Contains(cmdLine, `"/usr/bin/env -i /bin/bash --noprofile --norc -c "`) {
		t.Error("the shell is not started clean")
	}
	if strings.Contains(strings.Join(appleScriptLines, "\n"), "POSIX file") || strings.Contains(strings.Join(appleScriptLines, "\n"), "read ") {
		t.Error("the script must never be read from a file")
	}
	// The prompt is not a shell value.
	if !strings.Contains(strings.Join(appleScriptLines, "\n"), "with prompt (item 7 of argv) with administrator privileges") {
		t.Error("the administrator clause changed")
	}
}

// osacompile only compiles; it runs nothing and shows no window.
func TestAppleScriptCompiles(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	osac, err := exec.LookPath("osacompile")
	if err != nil {
		t.Skip("no osacompile")
	}
	args := []string{"-o", filepath.Join(t.TempDir(), "x.scpt")}
	for _, l := range appleScriptLines {
		args = append(args, "-e", l)
	}
	if out, err := exec.Command(osac, args...).CombinedOutput(); err != nil {
		t.Fatalf("does not compile: %v\n%s", err, out)
	}
}

func TestNoPasswordAskedByTheApp(t *testing.T) {
	// The script and the Go code never read a password: no stdin, no "sudo", no "-S".
	for _, banned := range []string{"sudo", "read -s", "read -sp", "askpass", "--stdin"} {
		if strings.Contains(pkgScript, banned) {
			t.Errorf("the script mentions %q", banned)
		}
	}
	for _, l := range appleScriptLines {
		if strings.Contains(l, "password") && !strings.Contains(l, "with prompt") {
			t.Errorf("AppleScript handles a password: %s", l)
		}
	}
}
