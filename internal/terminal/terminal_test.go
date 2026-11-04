package terminal

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/execx"
)

func invalidField(err error, field string) bool {
	ae, ok := err.(*api.AppError)
	return ok && ae.Code == api.ErrInvalidInput && ae.Params["field"] == field
}

// isInvalidDir: the folder syntax cannot be passed to a terminal safely.
func isInvalidDir(err error) bool { return invalidField(err, "dir") }

func TestDarwinPassesFolderAsArgument(t *testing.T) {
	dir := "/Users/alex/my code/o'brien"
	for _, app := range []api.TerminalApp{api.TerminalAuto, api.TerminalMacTerminal, api.TerminalITerm2} {
		l, err := Build(Request{App: app, Dir: dir, Profile: "initech-admin", GOOS: "darwin"})
		if err != nil || l.Name != "osascript" || !l.Wait {
			t.Fatalf("%s: %+v %v", app, l, err)
		}
		n := len(l.Args)
		if l.Args[n-2] != dir || l.Args[n-1] != "initech-admin" {
			t.Errorf("%s: folder and profile must be the last two arguments: %q", app, l.Args)
		}
		for i := 0; i < n-2; i++ {
			if l.Args[i] != "-e" && (strings.Contains(l.Args[i], dir) || strings.Contains(l.Args[i], "initech-admin")) {
				t.Errorf("%s: script text contains the folder or profile: %q", app, l.Args[i])
			}
		}
		script := strings.Join(l.Args[:n-2], "\n")
		for _, want := range []string{"on run argv", "quoted form of (item 1 of argv)", "quoted form of (item 2 of argv)", "end run"} {
			if !strings.Contains(script, want) {
				t.Errorf("%s: script lacks %q", app, want)
			}
		}
	}
	term, _ := Build(Request{App: api.TerminalMacTerminal, Dir: dir, Profile: "p", GOOS: "darwin"})
	auto, _ := Build(Request{App: api.TerminalAuto, Dir: dir, Profile: "p", GOOS: "darwin"})
	if strings.Join(term.Args, "\x00") != strings.Join(auto.Args, "\x00") {
		t.Error("terminal != auto")
	}
	it, _ := Build(Request{App: api.TerminalITerm2, Dir: dir, Profile: "p", GOOS: "darwin"})
	if !strings.Contains(strings.Join(it.Args, "\n"), `tell application "iTerm"`) || !strings.Contains(strings.Join(it.Args, "\n"), "write text") {
		t.Errorf("iTerm script: %q", it.Args)
	}
}

func TestDarwinRejectsShellSensitiveFolders(t *testing.T) {
	for _, dir := range []string{`/tmp/a\';open -a Calculator;#`, "/tmp/a!b", `/tmp/a"b`, "/tmp/a\nb", "/tmp/a\rb", "/tmp/a\x00b", "/tmp/a\u2028b", "/tmp/a\u0085b", "relative/dir", `C:\x`} {
		for _, app := range []api.TerminalApp{api.TerminalAuto, api.TerminalITerm2} {
			_, err := Build(Request{App: app, Dir: dir, Profile: "p", GOOS: "darwin"})
			if !isInvalidDir(err) {
				t.Errorf("%q (%s): %v", dir, app, err)
			}
		}
	}
	// Characters that quoted form handles stay allowed.
	for _, dir := range []string{"/tmp/x y", "/tmp/a$b", "/tmp/a`b", "/tmp/o'brien", "/tmp/a;b", "/tmp/a&b"} {
		if _, err := Build(Request{Dir: dir, Profile: "p", GOOS: "darwin"}); err != nil {
			t.Errorf("%q: %v", dir, err)
		}
	}
}

var hostileWindowsDirs = []string{
	`C:\repos\x&calc`, `C:\repos\x;calc`, `C:\repos\x’;calc;’`, `C:\R&D\repo`, `C:\repos\%PATH%`, `C:\repos\it's`, `C:\x y`,
}

func TestWindowsNeverPutsFolderIntoCommandText(t *testing.T) {
	old := lookPath
	t.Cleanup(func() { lookPath = old })
	lookPath = func(string) (string, error) { return "", execx.ErrNotFound }
	t.Setenv("SystemRoot", `C:\Windows`)
	for _, dir := range hostileWindowsDirs {
		for _, app := range []api.TerminalApp{api.TerminalAuto, api.TerminalCmd, api.TerminalPowerShell, api.TerminalWindowsTerminal} {
			l, err := Build(Request{App: app, Dir: dir, Profile: "initech-admin", GOOS: "windows"})
			if err != nil {
				t.Fatalf("%q %s: %v", dir, app, err)
			}
			if strings.Contains(l.Name, dir) {
				t.Errorf("%q %s: name contains the folder", dir, app)
			}
			for _, a := range l.Args {
				if strings.Contains(a, dir) {
					t.Errorf("%q %s: argument contains the folder: %q", dir, app, a)
				}
			}
			if l.Dir != dir || !l.NewConsole || l.Wait {
				t.Errorf("%q %s: %+v", dir, app, l)
			}
			if len(l.Env) != 1 || l.Env[0] != "AWS_PROFILE=initech-admin" {
				t.Errorf("%q %s: env %v", dir, app, l.Env)
			}
		}
	}
	cmd, _ := Build(Request{App: api.TerminalCmd, Dir: `C:\x`, Profile: "p", GOOS: "windows"})
	if cmd.Name != `C:\Windows\System32\cmd.exe` || strings.Join(cmd.Args, " ") != "/k" {
		t.Errorf("cmd: %+v", cmd)
	}
	ps, _ := Build(Request{App: api.TerminalPowerShell, Dir: `C:\x`, Profile: "p", GOOS: "windows"})
	if ps.Name != `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe` || strings.Join(ps.Args, " ") != "-NoExit" {
		t.Errorf("powershell: %+v", ps)
	}
}

func TestWindowsTerminalFolderOnlyAfterDashD(t *testing.T) {
	old := lookPath
	t.Cleanup(func() { lookPath = old })
	lookPath = func(string) (string, error) { return `C:\Users\alex\AppData\Local\Microsoft\WindowsApps\wt.exe`, nil }
	t.Setenv("SystemRoot", `C:\Windows`)
	for _, app := range []api.TerminalApp{api.TerminalAuto, api.TerminalWindowsTerminal} {
		dir := `C:\repos\x&calc`
		l, err := Build(Request{App: app, Dir: dir, Profile: "p", GOOS: "windows"})
		if err != nil || !strings.HasSuffix(l.Name, "wt.exe") {
			t.Fatalf("%s: %+v %v", app, l, err)
		}
		if strings.Join(l.Args, "|") != `-d|`+dir+`|powershell.exe|-NoExit|-Command|$env:AWS_PROFILE='p'` {
			t.Errorf("%s: %q", app, l.Args)
		}
		for i, a := range l.Args {
			if strings.Contains(a, dir) && !(i == 1 && l.Args[0] == "-d") {
				t.Errorf("%s: folder outside -d: %q", app, a)
			}
		}
		if l.Dir != dir || len(l.Env) != 1 {
			t.Errorf("%s: %+v", app, l)
		}
		// wt splits on ";": fall back to a direct PowerShell console.
		l, err = Build(Request{App: app, Dir: `C:\repos\x;calc`, Profile: "p", GOOS: "windows"})
		if err != nil || !strings.HasSuffix(l.Name, `powershell.exe`) || !l.NewConsole || strings.Join(l.Args, " ") != "-NoExit" {
			t.Errorf("%s with ';': %+v %v", app, l, err)
		}
	}
}

func TestWindowsRejectsBadFolders(t *testing.T) {
	for _, dir := range []string{"C:\\x\ny", "C:\\x\x00", `relative\dir`, `/tmp/x`, "C:\\x\u2029y"} {
		if _, err := Build(Request{Dir: dir, Profile: "p", GOOS: "windows"}); !isInvalidDir(err) {
			t.Errorf("%q: %v", dir, err)
		}
	}
}

func TestRejectsUnsafeProfile(t *testing.T) {
	for _, p := range []string{"a'b", `a"b`, "a b", "$(x)", "a;b", "", "a`b"} {
		for _, goos := range []string{"darwin", "windows"} {
			dir := "/tmp"
			if goos == "windows" {
				dir = `C:\tmp`
			}
			_, err := Build(Request{Dir: dir, Profile: p, GOOS: goos})
			if ae, ok := err.(*api.AppError); !ok || ae.Code != api.ErrInvalidInput {
				t.Errorf("%q on %s: %v", p, goos, err)
			}
		}
	}
	if _, err := Build(Request{Dir: "/tmp", Profile: "p", GOOS: "linux"}); err == nil {
		t.Error("linux must be unsupported")
	}
}

func TestEmptyDirMeansHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	l, err := Build(Request{Profile: "p", GOOS: "darwin"})
	if err != nil || l.Args[len(l.Args)-2] != home {
		t.Errorf("%v %v", l.Args, err)
	}
}

func TestOpenChecksFolderExists(t *testing.T) {
	err := Open(context.Background(), Request{Dir: "/definitely/not/here", Profile: "p", GOOS: "darwin"})
	if !invalidField(err, "dirMissing") {
		t.Errorf("a missing folder is reported as such, not as an unsafe name: %v", err)
	}
	f, _ := os.CreateTemp(t.TempDir(), "file")
	f.Close()
	if err := Open(context.Background(), Request{Dir: f.Name(), Profile: "p", GOOS: "darwin"}); !invalidField(err, "dirMissing") {
		t.Errorf("file instead of folder: %v", err)
	}
}

// zsh replaces an unquoted word that starts with "=" (=ls) by a command path, so the profile typed into the
// login shell goes through "quoted form of" like the folder.
func TestDarwinQuotesProfileForTheShell(t *testing.T) {
	for _, app := range []api.TerminalApp{api.TerminalAuto, api.TerminalITerm2} {
		l, err := Build(Request{App: app, Dir: "/tmp", Profile: "=ls", GOOS: "darwin"})
		if err != nil {
			t.Fatal(err)
		}
		script := strings.Join(l.Args[:len(l.Args)-2], "\n")
		if !strings.Contains(script, "set p to quoted form of (item 2 of argv)") || !strings.Contains(script, `export AWS_PROFILE=" & p`) {
			t.Errorf("%s: the profile is not quoted:\n%s", app, script)
		}
		if strings.Contains(script, "& (item 2 of argv)") {
			t.Errorf("%s: the profile is typed unquoted:\n%s", app, script)
		}
		if l.Args[len(l.Args)-1] != "=ls" {
			t.Errorf("%s: the profile must travel as an argument: %q", app, l.Args)
		}
	}
}
