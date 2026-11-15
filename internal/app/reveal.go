package app

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/execx"
)

// revealCommand builds the file-manager command that shows path selected in its parent folder. Revealing
// (instead of opening) matters on macOS: opening a folder named "Tools.app" would launch the bundle.
//
// name and args are the process to run; cmdLine, when non-empty, is the exact Windows command line (explorer
// parses its own arguments, so Go's argument quoting must not be applied). The path is used verbatim: no
// environment expansion, no shell.
func revealCommand(goos, path string) (name string, args []string, cmdLine string, err error) {
	if !execx.IsAbsPath(path, goos) {
		return "", nil, "", api.NewError(api.ErrInvalidInput, "the path must be absolute").WithParam("field", "path")
	}
	if execx.HasControlChars(path) || strings.Contains(path, `"`) {
		return "", nil, "", api.NewError(api.ErrInvalidInput, "the path contains characters that are not allowed").WithParam("field", "path")
	}
	if goos == runtime.GOOS {
		if _, serr := os.Lstat(path); serr != nil {
			return "", nil, "", api.NewError(api.ErrFileIO, "path not found").WithParam("file", path)
		}
	}
	switch goos {
	case "darwin":
		return "/usr/bin/open", []string{"-R", path}, "", nil
	case "windows":
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		return strings.TrimRight(root, `\/`) + `\explorer.exe`, nil, `explorer.exe /select,"` + path + `"`, nil
	}
	return "", nil, "", api.NewError(api.ErrUnsupportedOS, "unsupported system")
}

// revealPath runs the file manager for path and waits for it to hand over (10 s at most).
func revealPath(path string) error {
	name, args, cmdLine, err := revealCommand(runtime.GOOS, path)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = execx.ChildEnv()
	setCmdLine(cmd, cmdLine)
	runErr := cmd.Run()
	if runtime.GOOS == "windows" {
		return nil // explorer reports success with a non-zero exit code
	}
	if runErr != nil {
		return api.NewError(api.ErrInternal, "cannot open the file manager").WithDetail(runErr.Error())
	}
	return nil
}
