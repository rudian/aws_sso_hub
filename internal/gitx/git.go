// Package gitx wraps the git command line for the app: remote parsing, credential-config resolution that mirrors
// git's own matching, bind/unbind, clone, connection tests and repository scanning. Nothing here talks to AWS.
package gitx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/execx"
	"github.com/rudian/aws_sso_hub/internal/redact"
)

// MinVersion is the oldest supported git. Older releases have known code-execution flaws in repository and
// submodule handling, and 2.26 introduced the `config --show-scope` the app relies on.
const MinVersion = "2.39.1"

// ProfileNameRE mirrors awsconfig.ProfileNameRE (the git side must not import AWS-side packages).
var ProfileNameRE = regexp.MustCompile(`^[A-Za-z0-9_.@+=,-]{1,64}$`)

// RegionRE mirrors awsconfig.RegionRE.
var RegionRE = regexp.MustCompile(`^[a-z]{2}(-gov|-iso[a-z]*)?-[a-z]+-[0-9]{1,2}$`)

// Git is a handle on one git binary. Env is appended to every child environment (tests, sandbox mode).
type Git struct {
	Bin     string
	Version [3]int
	Env     []string
}

var reGitVersion = regexp.MustCompile(`git version (\d+)\.(\d+)\.(\d+)`)

// ParseVersion extracts the version from `git --version` output.
func ParseVersion(out string) ([3]int, bool) {
	m := reGitVersion.FindStringSubmatch(out)
	if m == nil {
		return [3]int{}, false
	}
	var v [3]int
	for i := 0; i < 3; i++ {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return v, true
}

func less(a, b [3]int) bool {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func fmtVersion(v [3]int) string {
	return strconv.Itoa(v[0]) + "." + strconv.Itoa(v[1]) + "." + strconv.Itoa(v[2])
}

// New locates git (bin "" = search), checks its version and returns a handle.
// Errors are *api.AppError (E_GIT_MISSING, E_GIT_TOO_OLD).
func New(ctx context.Context, bin string, env []string) (*Git, error) {
	if bin == "" {
		p, err := execx.FindGit(ctx, "")
		if err != nil {
			return nil, api.NewError(api.ErrGitMissing, "git was not found")
		}
		bin = p
	} else if _, err := os.Stat(bin); err != nil {
		return nil, api.NewError(api.ErrGitMissing, "git was not found")
	}
	g := &Git{Bin: bin, Env: append([]string(nil), env...)}
	res, err := g.Run(ctx, "", nil, "--version")
	if err != nil {
		return nil, api.NewError(api.ErrGitMissing, "git could not be started").WithDetail(err.Error())
	}
	v, ok := ParseVersion(string(res.Stdout))
	if res.ExitCode != 0 || !ok {
		return nil, api.NewError(api.ErrGitMissing, "git --version gave no usable output").WithDetail(redact.Tail(string(res.Stderr), 3))
	}
	g.Version = v
	if err := checkVersion(v); err != nil {
		return nil, err
	}
	return g, nil
}

// checkVersion returns E_GIT_TOO_OLD for versions below MinVersion.
func checkVersion(v [3]int) error {
	min, _ := ParseVersion("git version " + MinVersion)
	if less(v, min) {
		return api.NewError(api.ErrGitTooOld, "git %s is older than %s", fmtVersion(v), MinVersion).
			WithParam("version", fmtVersion(v)).WithParam("min", MinVersion)
	}
	return nil
}

// WithEnv returns a copy of g whose environment gets extra entries appended (later entries win).
func (g *Git) WithEnv(extra ...string) *Git {
	c := *g
	c.Env = append(append([]string(nil), g.Env...), extra...)
	return &c
}

func (g *Git) command(ctx context.Context, dir string, args ...string) *exec.Cmd {
	full := args
	if dir != "" {
		full = append([]string{"-C", dir}, args...)
	}
	cmd := execx.Command(ctx, g.Bin, full...)
	cmd.Env = append(cmd.Env, g.Env...)
	cmd.Env = append(cmd.Env, "LC_ALL=C")
	if dir == "" {
		// Outside any repository: global/system config only. The process runs in a private empty folder and
		// git may not look above it, so no repository (and no local config) is ever discovered.
		nd := neutralDir()
		cmd.Dir = nd
		cmd.Env = append(cmd.Env, "GIT_CEILING_DIRECTORIES="+filepath.Dir(nd))
	}
	return cmd
}

var neutral struct {
	sync.Mutex
	dir    string
	parent string // "" = the system temp folder; tests point it elsewhere
}

// neutralDir returns the private empty working directory for repository-less git calls, creating it on first
// use (mode 0700). If it cannot be created the system temp folder is used.
func neutralDir() string {
	neutral.Lock()
	defer neutral.Unlock()
	if neutral.dir != "" {
		if st, err := os.Stat(neutral.dir); err == nil && st.IsDir() {
			return neutral.dir
		}
	}
	d, err := os.MkdirTemp(neutral.parent, "awsssohub-git-")
	if err != nil {
		return os.TempDir()
	}
	if r, rerr := filepath.EvalSymlinks(d); rerr == nil {
		d = r
	}
	neutral.dir = d
	return d
}

// RemoveNeutralDir deletes the private working directory created for repository-less git calls.
func RemoveNeutralDir() {
	neutral.Lock()
	defer neutral.Unlock()
	if neutral.dir != "" {
		_ = os.Remove(neutral.dir)
		neutral.dir = ""
	}
}

// Run executes git with LC_ALL=C. dir "" means no -C (the process runs in a neutral directory).
func (g *Git) Run(ctx context.Context, dir string, stdin []byte, args ...string) (execx.Result, error) {
	return execx.Run(g.command(ctx, dir, args...), stdin)
}

// runOK runs git and maps unexpected exit codes to E_GIT_FAILED. okCodes lists non-zero exit codes that are fine.
func (g *Git) runOK(ctx context.Context, dir string, stdin []byte, okCodes []int, args ...string) (execx.Result, error) {
	res, err := g.Run(ctx, dir, stdin, args...)
	if err != nil {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		return res, api.NewError(api.ErrGitFailed, "git could not run").WithDetail(err.Error())
	}
	if res.ExitCode == 0 {
		return res, nil
	}
	for _, c := range okCodes {
		if res.ExitCode == c {
			return res, nil
		}
	}
	return res, api.NewError(api.ErrGitFailed, "git %s failed (exit %d)", firstWord(args), res.ExitCode).
		WithDetail(redact.Tail(string(res.Stderr), 3))
}

func firstWord(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}
