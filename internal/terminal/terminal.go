// Package terminal opens the user's terminal application in a folder with AWS_PROFILE set.
//
// Build turns a request into a Launch without running anything. The folder is never spliced into text that a
// shell or script interpreter parses, with one unavoidable exception on macOS: the terminal types a "cd" line
// into the user's login shell, so folders containing characters that shells treat specially inside single
// quotes (backslash, "!" and the double quote) are refused there.
package terminal

import (
	"context"
	"os"
	"runtime"
	"strings"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/execx"
	"github.com/rudian/aws_sso_hub/internal/gitx"
)

// Request selects the terminal, directory and profile. GOOS "" means the running OS.
type Request struct {
	App                api.TerminalApp
	Dir, Profile, GOOS string
}

// Launch describes the process that opens the terminal.
type Launch struct {
	Name string
	Args []string
	// Dir is the working directory of the new process ("" = inherit).
	Dir string
	// Env holds only the extra variables; the runner appends them to the sanitised child environment.
	Env []string
	// NewConsole asks Windows to give the process its own console window.
	NewConsole bool
	// Wait is true when the launcher exits on its own (osascript) and its result should be checked.
	Wait bool
}

var lookPath = execx.LookPath // replaced in tests

func resolveDir(dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", api.NewError(api.ErrInternal, "cannot determine the home directory").WithDetail(err.Error())
	}
	return h, nil
}

func badDir() error {
	return api.NewError(api.ErrInvalidInput, "the folder cannot be opened in a terminal safely").WithParam("field", "dir")
}

// checkDir rejects folders that are not absolute paths of the target OS or that contain control characters.
// It does not touch the file system, so the result depends only on its arguments.
func checkDir(dir, goos string) error {
	if !execx.IsAbsPath(dir, goos) || execx.HasControlChars(dir) {
		return badDir()
	}
	return nil
}

func systemRoot() string {
	if r := os.Getenv("SystemRoot"); r != "" {
		return strings.TrimRight(r, `\/`)
	}
	return `C:\Windows`
}

// Build returns the launch description for the request. It validates the profile and the folder syntax but
// does not check that the folder exists (Open does).
func Build(req Request) (Launch, error) {
	if !gitx.ProfileNameRE.MatchString(req.Profile) {
		return Launch{}, api.NewError(api.ErrInvalidInput, "invalid profile name").WithParam("field", "profile")
	}
	goos := req.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos != "darwin" && goos != "windows" {
		return Launch{}, api.NewError(api.ErrUnsupportedOS, "terminal launching is not supported on %s", goos)
	}
	dir, err := resolveDir(req.Dir)
	if err != nil {
		return Launch{}, err
	}
	if err := checkDir(dir, goos); err != nil {
		return Launch{}, err
	}
	if goos == "darwin" {
		return buildDarwin(req.App, dir, req.Profile)
	}
	return buildWindows(req.App, dir, req.Profile), nil
}

// buildDarwin passes the folder and profile to osascript as arguments of a run handler, so neither appears in
// the script text. "quoted form of" produces the single-quoted shell word that is typed into the terminal;
// the profile is quoted too, because zsh would turn an unquoted word starting with "=" into a command path.
func buildDarwin(app api.TerminalApp, dir, profile string) (Launch, error) {
	// Characters that the user's login shell would interpret even inside the quoted word.
	if strings.ContainsAny(dir, `\!"`) {
		return Launch{}, badDir()
	}
	var script []string
	if app == api.TerminalITerm2 {
		script = []string{
			`on run argv`,
			`set d to quoted form of (item 1 of argv)`,
			`set p to quoted form of (item 2 of argv)`,
			`tell application "iTerm"`,
			`set w to (create window with default profile)`,
			`tell current session of w to write text "cd " & d & " && export AWS_PROFILE=" & p`,
			`activate`,
			`end tell`,
			`end run`,
		}
	} else {
		script = []string{
			`on run argv`,
			`set d to quoted form of (item 1 of argv)`,
			`set p to quoted form of (item 2 of argv)`,
			`tell application "Terminal" to do script "cd " & d & " && export AWS_PROFILE=" & p`,
			`tell application "Terminal" to activate`,
			`end run`,
		}
	}
	args := make([]string, 0, 2*len(script)+2)
	for _, line := range script {
		args = append(args, "-e", line)
	}
	args = append(args, dir, profile)
	return Launch{Name: "osascript", Args: args, Wait: true}, nil
}

// buildWindows never puts the folder into command text: the new process starts in it (Dir). Windows Terminal
// is the one exception, where "-d" takes the folder as its own argument; wt splits its command line on ";",
// so a folder containing one falls back to a plain PowerShell console.
func buildWindows(app api.TerminalApp, dir, profile string) Launch {
	root := systemRoot()
	envProfile := []string{"AWS_PROFILE=" + profile}
	cmdExe := Launch{Name: root + `\System32\cmd.exe`, Args: []string{"/k"}, Dir: dir, Env: envProfile, NewConsole: true}
	psExe := Launch{Name: root + `\System32\WindowsPowerShell\v1.0\powershell.exe`, Args: []string{"-NoExit"}, Dir: dir, Env: envProfile, NewConsole: true}
	switch app {
	case api.TerminalCmd:
		return cmdExe
	case api.TerminalPowerShell:
		return psExe
	}
	if wt, err := lookPath("wt.exe"); err == nil && !strings.Contains(dir, ";") {
		return Launch{
			Name: wt,
			Args: []string{"-d", dir, "powershell.exe", "-NoExit", "-Command", "$env:AWS_PROFILE='" + profile + "'"},
			Dir:  dir,
			Env:  envProfile,
		}
	}
	return psExe
}

// Open validates the folder and starts the terminal.
func Open(ctx context.Context, req Request) error {
	dir, err := resolveDir(req.Dir)
	if err != nil {
		return err
	}
	if st, serr := os.Stat(dir); serr != nil || !st.IsDir() {
		return api.NewError(api.ErrInvalidInput, "the folder does not exist").WithParam("field", "dirMissing")
	}
	req.Dir = dir
	l, err := Build(req)
	if err != nil {
		return err
	}
	if !l.Wait {
		if serr := execx.Start(l.Name, l.Args, l.Dir, l.Env, l.NewConsole); serr != nil {
			return api.NewError(api.ErrInternal, "cannot start the terminal").WithDetail(serr.Error())
		}
		return nil
	}
	cmd := execx.Command(ctx, l.Name, l.Args...)
	cmd.Dir = l.Dir
	cmd.Env = append(cmd.Env, l.Env...)
	res, rerr := execx.Run(cmd, nil)
	if rerr != nil {
		return api.NewError(api.ErrInternal, "cannot start the terminal").WithDetail(rerr.Error())
	}
	if res.ExitCode != 0 {
		return api.NewError(api.ErrInternal, "terminal launcher failed (exit %d)", res.ExitCode).WithDetail(string(res.Stderr))
	}
	return nil
}
