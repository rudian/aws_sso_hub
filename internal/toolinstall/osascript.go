package toolinstall

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strconv"
)

// The one privileged step. macOS shows its own password window; the app never sees the password.
//
//   - osascript is started with a fixed environment (PATH only); nothing of the app's own environment is inherited.
//   - The script runs under /usr/bin/env -i /bin/bash --noprofile --norc -c, so even a variable that reached the
//     shell (BASH_ENV, an exported function, SHELLOPTS with PS4) cannot change what root runs.
//   - The script text, the package path and every other value are AppleScript arguments (argv), and each one goes
//     through `quoted form of` before it is part of the shell command. No value is spliced into shell text.

// osascriptEnv is the whole environment of the osascript child.
var osascriptEnv = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"}

// OSAScriptPath is the only osascript the app runs.
const OSAScriptPath = "/usr/bin/osascript"

// appleScriptLines is the AppleScript, one -e per line. Items of argv: 1 the shell script text, 2 the package
// path, 3 the Team ID, 4 the size, 5 the sha256, 6 the mode, 7 the prompt text.
var appleScriptLines = []string{
	`on run argv`,
	`set sh to "/usr/bin/env -i /bin/bash --noprofile --norc -c " & quoted form of (item 1 of argv) & " codepass-pkg " & quoted form of (item 2 of argv) & " " & quoted form of (item 3 of argv) & " " & quoted form of (item 4 of argv) & " " & quoted form of (item 5 of argv) & " " & quoted form of (item 6 of argv)`,
	`do shell script sh with prompt (item 7 of argv) with administrator privileges`,
	`end run`,
}

// PkgRequest is what the privileged step needs to know.
type PkgRequest struct {
	Path   string // the downloaded package (user-owned, untrusted)
	Team   string // the pinned Team ID
	Size   int64  // expected bytes; <= 0 when unknown
	Sha256 string // expected digest in hex; "" when unknown
	Mode   string // "install" or "verify"
	Prompt string // the sentence macOS shows in its password window
}

// osascriptArgs builds the argument list of the osascript call. script is the shell text (pkgScript).
func osascriptArgs(script string, r PkgRequest) []string {
	args := make([]string, 0, 2*len(appleScriptLines)+9)
	for _, l := range appleScriptLines {
		args = append(args, "-e", l)
	}
	size, sha := "-", "-"
	if r.Size > 0 {
		size = strconv.FormatInt(r.Size, 10)
	}
	if r.Sha256 != "" {
		sha = r.Sha256
	}
	mode := r.Mode
	if mode == "" {
		mode = "install"
	}
	return append(args, "--", script, r.Path, r.Team, size, sha, mode, r.Prompt)
}

// PrivResult is the outcome of the osascript call.
type PrivResult struct {
	Exit   int    // osascript's exit status
	Stdout string // only meaningful when Exit is 0; osascript turns newlines into carriage returns
	Stderr string // "<line>:<col>: execution error: <the script's stderr> (<exit code>)"
}

// runOSAScript runs osascript at path (tests pass a fake) with the fixed environment.
func runOSAScript(ctx context.Context, path string, r PkgRequest) (PrivResult, error) {
	cmd := exec.CommandContext(ctx, path, osascriptArgs(pkgScript, r)...)
	cmd.Env = append([]string(nil), osascriptEnv...)
	cmd.Dir = "/"
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	res := PrivResult{Stdout: so.String(), Stderr: se.String()}
	if err == nil {
		return res, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.Exit = ee.ExitCode()
		if res.Exit == -1 {
			return res, ctx.Err()
		}
		return res, nil
	}
	res.Exit = -1
	return res, err
}
