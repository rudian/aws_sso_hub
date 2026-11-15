package repos_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
)

// proxyToClosedPort sends all https traffic of the git calls to a closed loopback port, so no test traffic
// ever leaves the machine. GIT_CONFIG_COUNT is honoured because it is added after the environment is sanitised.
func proxyToClosedPort(g *gitx.Git) *gitx.Git {
	return g.WithEnv("GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.proxy", "GIT_CONFIG_VALUE_0=http://127.0.0.1:1",
		"https_proxy=", "HTTPS_PROXY=", "no_proxy=", "NO_PROXY=")
}

func TestConnectionTestIgnoresHostileRepositoryConfig(t *testing.T) {
	f := newFixture(t)
	repo := f.newRepo("hostile", ccURL("hostile_repo"))
	id := f.byName(f.state(), "hostile").ID
	if _, err := f.eng.Bind(context.Background(), id, "acme"); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(f.sb.Root, "marker")
	script := filepath.Join(f.sb.Root, "bin", "askpass-marker")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch '"+marker+"4'\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, ".git", "config"), strings.Join([]string{
		`[protocol "ext"]`, "\tallow = always",
		`[url "ext::sh -c touch% ` + marker + `1"]`, "\tinsteadOf = https://" + hostAP + "/",
		`[credential]`, "\thelper = !touch " + marker + "2",
		`[core]`, "\tsshCommand = touch " + marker + "3", "\taskPass = " + script,
		`[http]`, "\tsslVerify = false", "\tproxy = http://127.0.0.1:2",
		"",
	}, "\n"))
	f.gitHook = proxyToClosedPort
	res, err := f.eng.Test(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status == api.TestOK || res.Status == api.TestSignedOut {
		t.Errorf("unexpected status %s", res.Status)
	}
	for _, n := range []string{"1", "2", "3", "4"} {
		if _, err := os.Stat(marker + n); err == nil {
			t.Errorf("repository config ran code (marker %s)", n)
		}
	}
}

func fakeGitCountingLsRemote(t *testing.T, f *fixture) (marker string) {
	t.Helper()
	marker = filepath.Join(f.sb.Root, "ls-remote-ran")
	fake := filepath.Join(f.sb.Root, "bin", "fakegit")
	body := "#!/bin/sh\ncase \"$*\" in *ls-remote*) touch '" + marker + "';; esac\nexit 0\n"
	if err := os.WriteFile(fake, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	real := f.sb.Git
	f.gitHook = func(*gitx.Git) *gitx.Git { c := *real; c.Bin = fake; return &c }
	return marker
}

func TestConnectionTestRefusesSSHAndUnboundWithoutGit(t *testing.T) {
	f := newFixture(t)
	f.newRepo("sshrepo", "ssh://"+hostAP+"/v1/repos/ssh_repo")
	f.newRepo("plain", ccURL("plain_repo"))
	sshID := f.byName(f.state(), "sshrepo").ID
	plainID := f.byName(f.state(), "plain").ID
	marker := fakeGitCountingLsRemote(t, f)
	for name, id := range map[string]string{"ssh": sshID, "unbound https": plainID} {
		res, err := f.eng.Test(context.Background(), id)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.Status != api.TestError || res.Detail == "" {
			t.Errorf("%s: %+v", name, res)
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("git ls-remote ran for a remote that must be refused")
	}
}

func TestCloneRejectsRelativeParent(t *testing.T) {
	f := newFixture(t)
	if err := os.MkdirAll(filepath.Join(f.sb.Root, "rel"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(f.sb.Root)
	req := api.CloneRequest{Profile: "acme", Region: "us-east-1", RepoName: "x", ParentDir: "rel", FolderName: "x"}
	_, err := f.eng.PreviewClone(context.Background(), req)
	if ae := api.AsAppError(err); err == nil || ae.Code != api.ErrInvalidInput || ae.Params["field"] != "parentDir" {
		t.Errorf("preview: %v", err)
	}
	if _, err := f.eng.Clone(context.Background(), req); err == nil || api.AsAppError(err).Code != api.ErrInvalidInput {
		t.Errorf("clone: %v", err)
	}
}
