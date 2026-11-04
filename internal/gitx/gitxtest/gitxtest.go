// Package gitxtest builds isolated real-git sandboxes for tests: private HOME and global config, no system
// config, no terminal prompts, an askpass spy that records any password prompt, fake aws binaries and spy
// credential helpers. Imported only by _test.go files. Unix only (scripts use /bin/sh); tests skip elsewhere.
package gitxtest

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rudian/aws_sso_hub/internal/execx"
	"github.com/rudian/aws_sso_hub/internal/gitx"
)

// Sandbox is one isolated environment.
type Sandbox struct {
	Root, Home, GlobalConfig string
	Env                      []string
	Git                      *gitx.Git

	spyScript, askpassLog, spyLog string
}

// New creates a sandbox under t.TempDir(). It skips the test when git (or a POSIX shell) is unavailable.
func New(t testing.TB) *Sandbox {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("gitxtest uses POSIX shell scripts")
	}
	execx.Configure(execx.Options{DisableLoginShell: true})
	root := t.TempDir()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	s := &Sandbox{Root: root, Home: filepath.Join(root, "home")}
	s.GlobalConfig = filepath.Join(s.Home, ".gitconfig")
	for _, d := range []string{s.Home, filepath.Join(s.Home, ".aws"), filepath.Join(root, "bin")} {
		must(t, os.MkdirAll(d, 0o755))
	}
	s.spyLog = filepath.Join(root, "spy.log")
	s.askpassLog = filepath.Join(root, "askpass.log")
	askpass := filepath.Join(root, "bin", "askpass-spy")
	writeScript(t, askpass, "#!/bin/sh\necho \"!!! ASKPASS $1\" >> '"+s.askpassLog+"'\nexit 1\n")
	s.spyScript = filepath.Join(root, "bin", "spy-helper")
	writeScript(t, s.spyScript, "#!/bin/sh\necho \"spy[$1] $2\" >> '"+s.spyLog+"'\ncat >/dev/null\n")
	s.Env = []string{
		"HOME=" + s.Home,
		"USERPROFILE=" + s.Home,
		"XDG_CONFIG_HOME=" + filepath.Join(s.Home, ".config"),
		"AWS_CONFIG_FILE=" + filepath.Join(s.Home, ".aws", "config"),
		"AWS_SHARED_CREDENTIALS_FILE=" + filepath.Join(s.Home, ".aws", "credentials"),
		"GIT_CONFIG_GLOBAL=" + s.GlobalConfig,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=" + askpass,
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	}
	g, err := gitx.New(context.Background(), "", s.Env)
	if err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	s.Git = g
	return s
}

func must(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func writeScript(t testing.TB, path, body string) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(path), 0o755))
	must(t, os.WriteFile(path, []byte(body), 0o755))
}

// InitRepo creates <Root>/<rel> as a git repository with the given remotes and returns its absolute path.
func (s *Sandbox) InitRepo(t testing.TB, rel string, remotes map[string]string) string {
	t.Helper()
	dir := filepath.Join(s.Root, rel)
	must(t, os.MkdirAll(dir, 0o755))
	ctx := context.Background()
	if res, err := s.Git.Run(ctx, dir, nil, "init", "-q", "-b", "main"); err != nil || res.ExitCode != 0 {
		t.Fatalf("git init: %v %s", err, res.Stderr)
	}
	for name, url := range remotes {
		if res, err := s.Git.Run(ctx, dir, nil, "remote", "add", name, url); err != nil || res.ExitCode != 0 {
			t.Fatalf("git remote add: %v %s", err, res.Stderr)
		}
	}
	return dir
}

// WriteGlobal replaces the sandbox's global git config.
func (s *Sandbox) WriteGlobal(t testing.TB, content string) {
	t.Helper()
	must(t, os.WriteFile(s.GlobalConfig, []byte(content), 0o644))
}

// FakeAWS writes an executable named "aws" into dir and returns its absolute path. mode "ok" answers
// `codecommit credential-helper ... get` with fake credentials; "fail" prints an SSO-style error and exits 255.
// Both answer --version.
func (s *Sandbox) FakeAWS(t testing.TB, dir, mode string) string {
	t.Helper()
	body := "#!/bin/sh\n" +
		"for a in \"$@\"; do last=\"$a\"; done\n" +
		"if [ \"$1\" = \"--version\" ]; then echo 'aws-cli/2.40.0 Python/3.13.5 Darwin/24.6.0 source/arm64'; exit 0; fi\n"
	switch mode {
	case "ok":
		body += "cat >/dev/null\n" +
			"if [ \"$last\" = get ]; then echo 'username=ASIAIOSFODNN7EXAMPLE%FAKESESSIONTOKEN'; echo 'password=20261104T031902Zdeadbeef'; fi\n" +
			"exit 0\n"
	case "fail":
		body += "cat >/dev/null\n" +
			"echo 'aws: [ERROR]: Error loading SSO Token: Token for fake-portal does not exist' >&2\n" +
			"exit 255\n"
	default:
		t.Fatalf("FakeAWS: unknown mode %q", mode)
	}
	p := filepath.Join(dir, "aws")
	writeScript(t, p, body)
	return p
}

// SpyHelper returns a credential.helper value that appends "spy[<label>] <op>" to the spy log and supplies
// no credentials.
func (s *Sandbox) SpyHelper(t testing.TB, label string) string {
	t.Helper()
	return s.spyScript + " " + label
}

// SpyLog returns the spy log content ("" if nothing was logged).
func (s *Sandbox) SpyLog(t testing.TB) string { return readOrEmpty(s.spyLog) }

// AskpassLog returns what the askpass spy recorded ("" = git never prompted).
func (s *Sandbox) AskpassLog(t testing.TB) string { return readOrEmpty(s.askpassLog) }

// ResetLogs empties both logs.
func (s *Sandbox) ResetLogs(t testing.TB) {
	t.Helper()
	_ = os.Remove(s.spyLog)
	_ = os.Remove(s.askpassLog)
}

func readOrEmpty(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

// CredentialFill runs `git credential fill` for url inside repoDir ("" = outside any repository).
func (s *Sandbox) CredentialFill(t testing.TB, repoDir, url string) (stdout, stderr string, exit int) {
	return s.CredentialFillEnv(t, repoDir, url)
}

// CredentialFillEnv is CredentialFill with extra environment entries applied last (e.g. a Finder-like PATH).
func (s *Sandbox) CredentialFillEnv(t testing.TB, repoDir, url string, extraEnv ...string) (stdout, stderr string, exit int) {
	t.Helper()
	proto, rest, ok := strings.Cut(url, "://")
	if !ok {
		t.Fatalf("bad url %q", url)
	}
	host, path, _ := strings.Cut(rest, "/")
	in := "protocol=" + proto + "\nhost=" + host + "\n"
	if path != "" {
		in += "path=" + path + "\n"
	}
	in += "\n"
	args := []string{"credential", "fill"}
	if repoDir != "" {
		args = append([]string{"-C", repoDir}, args...)
	}
	cmd := exec.Command(s.Git.Bin, args...)
	cmd.Env = append(cleanEnv(), s.Env...)
	cmd.Env = append(cmd.Env, "LC_ALL=C")
	cmd.Env = append(cmd.Env, extraEnv...)
	cmd.Dir = s.Root
	cmd.Stdin = strings.NewReader(in)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	exit = 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exit = ee.ExitCode()
		} else {
			t.Fatalf("credential fill: %v", err)
		}
	}
	return so.String(), se.String(), exit
}

func cleanEnv() []string {
	var out []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "GIT_") || strings.HasPrefix(k, "AWS_") || k == "SSH_ASKPASS" {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// CommitFile creates a commit with one file in repoDir and returns the commit SHA.
func (s *Sandbox) CommitFile(t testing.TB, repoDir, name, content string) string {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(repoDir, name), []byte(content), 0o644))
	ctx := context.Background()
	for _, a := range [][]string{{"add", name}, {"commit", "-q", "-m", "add " + name}} {
		if res, err := s.Git.Run(ctx, repoDir, nil, a...); err != nil || res.ExitCode != 0 {
			t.Fatalf("git %v: %v %s", a, err, res.Stderr)
		}
	}
	res, err := s.Git.Run(ctx, repoDir, nil, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(res.Stdout))
}

// InitBare creates a bare repository at <Root>/<rel> that has one commit on main and returns its path.
func (s *Sandbox) InitBare(t testing.TB, rel string) string {
	t.Helper()
	work := s.InitRepo(t, rel+"-work", nil)
	s.CommitFile(t, work, "README.md", "hello\n")
	bare := filepath.Join(s.Root, rel)
	ctx := context.Background()
	if res, err := s.Git.Run(ctx, "", nil, "clone", "-q", "--bare", work, bare); err != nil || res.ExitCode != 0 {
		t.Fatalf("bare clone: %v %s", err, res.Stderr)
	}
	return bare
}
