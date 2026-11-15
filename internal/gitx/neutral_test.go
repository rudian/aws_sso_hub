package gitx_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/gitx/gitxtest"
)

func TestVersionGate(t *testing.T) {
	if gitx.MinVersion != "2.39.1" {
		t.Errorf("MinVersion = %s", gitx.MinVersion)
	}
	old, _ := gitx.ParseVersion("git version 2.39.0")
	ok, _ := gitx.ParseVersion("git version 2.39.1")
	newer, _ := gitx.ParseVersion("git version 2.47.1 (Apple Git-999)")
	err := gitx.CheckVersion(old)
	if ae, isApp := err.(*api.AppError); !isApp || ae.Code != api.ErrGitTooOld {
		t.Errorf("2.39.0 must be too old: %v", err)
	}
	if err := gitx.CheckVersion(ok); err != nil {
		t.Errorf("2.39.1: %v", err)
	}
	if err := gitx.CheckVersion(newer); err != nil {
		t.Errorf("2.47.1: %v", err)
	}
}

// A repository above the private working directory must not be discovered by repository-less git calls.
func TestRepositoryLessGitIgnoresEnclosingRepository(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix permissions")
	}
	sb := gitxtest.New(t)
	ctx := context.Background()
	parent := t.TempDir()
	if res, err := sb.Git.Run(ctx, parent, nil, "init", "-q", "-b", "main"); err != nil || res.ExitCode != 0 {
		t.Fatalf("init: %v %s", err, res.Stderr)
	}
	if res, err := sb.Git.Run(ctx, parent, nil, "config", "--local", "awsssohub.marker", "1"); err != nil || res.ExitCode != 0 {
		t.Fatalf("config: %v %s", err, res.Stderr)
	}
	restore := gitx.SetNeutralParent(parent)
	t.Cleanup(restore)

	dir := gitx.NeutralDir()
	if filepath.Dir(dir) != resolved(t, parent) {
		t.Fatalf("neutral dir %q is not inside %q", dir, parent)
	}
	if st, err := os.Stat(dir); err != nil || st.Mode().Perm() != 0o700 {
		t.Errorf("neutral dir: %v %v", st, err)
	}
	// Sanity: the enclosing repository does hold the marker.
	if res, _ := sb.Git.Run(ctx, parent, nil, "config", "--get", "awsssohub.marker"); strings.TrimSpace(string(res.Stdout)) != "1" {
		t.Fatalf("fixture broken: %q", res.Stdout)
	}
	res, err := sb.Git.Run(ctx, "", nil, "config", "--get", "awsssohub.marker")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(res.Stdout)); got != "" || res.ExitCode == 0 {
		t.Errorf("the enclosing repository was discovered: %q (exit %d)", got, res.ExitCode)
	}
	if res, _ := sb.Git.Run(ctx, "", nil, "rev-parse", "--git-dir"); res.ExitCode == 0 {
		t.Errorf("rev-parse found a repository: %s", res.Stdout)
	}
	gitx.RemoveNeutralDir()
	if _, err := os.Stat(dir); err == nil {
		t.Error("RemoveNeutralDir left the folder behind")
	}
}

func resolved(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
