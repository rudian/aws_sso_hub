package gitx_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/gitx/gitxtest"
)

const (
	host   = "git-codecommit.us-east-1.amazonaws.com"
	ccURL  = "https://" + host + "/v1/repos/acme_cs_portal"
	fillCC = ccURL
)

func spec(repo, awsPath, profile string) gitx.BindSpec {
	return gitx.BindSpec{RepoDir: repo, Hosts: []string{host}, Profile: profile, AccountID: "111122223333", AWSPath: awsPath, Lang: "en", GOOS: "darwin"}
}

func readConfig(t *testing.T, repo string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repo, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func managedBlock(cfg string) string {
	i := strings.Index(cfg, "[credential \"https://")
	if i < 0 {
		return ""
	}
	return cfg[i:]
}

func TestBindWritesExactBlock(t *testing.T) {
	sb := gitxtest.New(t)
	repo := sb.InitRepo(t, "r", map[string]string{"origin": ccURL})
	if err := sb.Git.Bind(context.Background(), spec(repo, "/opt/homebrew/bin/aws", "acme")); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/bind_block.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got := managedBlock(readConfig(t, repo)); got != string(want) {
		t.Errorf("block differs\n--- got\n%s\n--- want\n%s", got, want)
	}
}

func TestBindIdempotent(t *testing.T) {
	sb := gitxtest.New(t)
	repo := sb.InitRepo(t, "r", map[string]string{"origin": ccURL})
	s := spec(repo, "/opt/homebrew/bin/aws", "acme")
	if err := sb.Git.Bind(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	first := readConfig(t, repo)
	if err := sb.Git.Bind(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if second := readConfig(t, repo); second != first {
		t.Errorf("second bind changed the config:\n%s\n---\n%s", first, second)
	}
	// Re-bind to another profile replaces the old one completely.
	s.Profile = "globex"
	if err := sb.Git.Bind(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	cfg := readConfig(t, repo)
	if strings.Contains(cfg, "'acme'") || strings.Contains(cfg, "profile = acme") || strings.Count(cfg, "[credential") != 1 {
		t.Errorf("rebind left old data:\n%s", cfg)
	}
}

func TestBindMigratesLegacyPlain(t *testing.T) {
	ctx := context.Background()
	t.Run("legacy plain helper, only aws", func(t *testing.T) {
		sb := gitxtest.New(t)
		repo := sb.InitRepo(t, "acme_design", map[string]string{"origin": ccURL})
		appendConfig(t, repo, "[credential]\n\thelper = \n\thelper = !aws codecommit credential-helper $@ --profile acme\n\tUseHttpPath = true\n")
		es, _ := sb.Git.CredentialEntries(ctx, repo)
		eff := gitx.Resolve(es, mustCred(t, ccURL))
		if c := gitx.Classify(eff, gitx.LocalMetaFrom(es), api.RemoteHTTPS, "", func(string) bool { return true }); !c.LegacyPlain || c.Profile != "acme" {
			t.Fatalf("precondition: %+v", c)
		}
		if err := sb.Git.Bind(ctx, spec(repo, "/opt/homebrew/bin/aws", "acme")); err != nil {
			t.Fatal(err)
		}
		cfg := readConfig(t, repo)
		if strings.Contains(cfg, "[credential]") || strings.Contains(cfg, "UseHttpPath = true\n[") {
			t.Errorf("legacy plain section should be gone:\n%s", cfg)
		}
		es, _ = sb.Git.CredentialEntries(ctx, repo)
		eff = gitx.Resolve(es, mustCred(t, ccURL))
		c := gitx.Classify(eff, gitx.LocalMetaFrom(es), api.RemoteHTTPS, "", func(string) bool { return true })
		if c.Source != api.BindingLocal || c.LegacyPlain || c.NeedsRepair || c.Profile != "acme" {
			t.Errorf("after bind: %+v", c)
		}
	})
	t.Run("keeps unrelated plain helper", func(t *testing.T) {
		sb := gitxtest.New(t)
		repo := sb.InitRepo(t, "r", map[string]string{"origin": ccURL})
		appendConfig(t, repo, "[credential]\n\thelper = \n\thelper = !aws codecommit credential-helper $@ --profile acme\n\thelper = store\n\tUseHttpPath = true\n")
		if err := sb.Git.Bind(ctx, spec(repo, "/opt/homebrew/bin/aws", "acme")); err != nil {
			t.Fatal(err)
		}
		cfg := readConfig(t, repo)
		if !strings.Contains(cfg, "[credential]\n\thelper = \n\thelper = store\n\tUseHttpPath = true") {
			t.Errorf("unrelated plain helper not preserved:\n%s", cfg)
		}
		if strings.Contains(cfg, "!aws codecommit credential-helper $@ --profile acme") {
			t.Errorf("legacy aws helper still there:\n%s", cfg)
		}
	})
}

func TestUnbind(t *testing.T) {
	ctx := context.Background()
	sb := gitxtest.New(t)
	repo := sb.InitRepo(t, "r", map[string]string{"origin": ccURL})
	before := readConfig(t, repo)
	if err := sb.Git.Bind(ctx, spec(repo, "/opt/homebrew/bin/aws", "acme")); err != nil {
		t.Fatal(err)
	}
	if err := sb.Git.Unbind(ctx, repo); err != nil {
		t.Fatal(err)
	}
	es, err := sb.Git.CredentialEntries(ctx, repo)
	if err != nil || len(es) != 0 {
		t.Fatalf("entries after unbind: %v %v", es, err)
	}
	if after := readConfig(t, repo); strings.Contains(after, "credential") || strings.Contains(after, "awsssohub") {
		t.Errorf("unbind left sections:\n%s\n(before)\n%s", after, before)
	}
	// Unbinding an unbound repo is a no-op, not an error.
	if err := sb.Git.Unbind(ctx, repo); err != nil {
		t.Errorf("second unbind: %v", err)
	}
	// Legacy plain.
	legacy := sb.InitRepo(t, "legacy", map[string]string{"origin": ccURL})
	appendConfig(t, legacy, "[credential]\n\thelper = \n\thelper = !aws codecommit credential-helper $@ --profile acme\n\tUseHttpPath = true\n")
	if err := sb.Git.Unbind(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	if cfg := readConfig(t, legacy); strings.Contains(cfg, "credential") {
		t.Errorf("legacy unbind:\n%s", cfg)
	}
}

func TestHostScopedBindingLeavesGitHubAlone(t *testing.T) {
	ctx := context.Background()
	sb := gitxtest.New(t)
	fakeDir := filepath.Join(sb.Root, "fakebin")
	awsPath := sb.FakeAWS(t, fakeDir, "ok")
	sb.WriteGlobal(t, "[credential \"https://github.com\"]\n\thelper = \n\thelper = "+sb.SpyHelper(t, "GLOBAL-GITHUB")+"\n")
	repo := sb.InitRepo(t, "r", map[string]string{"origin": ccURL, "gh": "https://github.com/org/repo.git"})
	if err := sb.Git.Bind(ctx, spec(repo, awsPath, "acme")); err != nil {
		t.Fatal(err)
	}
	sb.ResetLogs(t)
	_, stderr, _ := sb.CredentialFill(t, repo, "https://github.com/org/repo.git")
	if !strings.Contains(sb.SpyLog(t), "spy[GLOBAL-GITHUB] get") {
		t.Errorf("GitHub spy did not run; spy log %q stderr %q", sb.SpyLog(t), stderr)
	}
	if strings.Contains(stderr, "AWS SSO Hub") || strings.Contains(stderr, "told us to quit") {
		t.Errorf("CodeCommit fallback hijacked GitHub: %s", stderr)
	}
	out, _, exit := sb.CredentialFill(t, repo, fillCC)
	if exit != 0 || !strings.Contains(out, "username=ASIAIOSFODNN7EXAMPLE%FAKESESSIONTOKEN") {
		t.Errorf("CodeCommit fill: exit %d out %q", exit, out)
	}
}

func TestFallbackQuitsWithoutPrompt(t *testing.T) {
	ctx := context.Background()
	sb := gitxtest.New(t)
	awsPath := sb.FakeAWS(t, filepath.Join(sb.Root, "fakebin"), "fail")
	repo := sb.InitRepo(t, "r", map[string]string{"origin": ccURL})
	if err := sb.Git.Bind(ctx, spec(repo, awsPath, "acme")); err != nil {
		t.Fatal(err)
	}
	_, stderr, exit := sb.CredentialFill(t, repo, fillCC)
	if exit != 128 {
		t.Errorf("exit = %d, want 128", exit)
	}
	if !strings.Contains(stderr, "AWS SSO Hub: cannot get CodeCommit credentials for AWS profile 'acme'") {
		t.Errorf("stderr lacks the fallback message: %q", stderr)
	}
	if log := sb.AskpassLog(t); log != "" {
		t.Errorf("git prompted for a password: %q", log)
	}
}

func TestHelperWorksWithFinderPath(t *testing.T) {
	ctx := context.Background()
	sb := gitxtest.New(t)
	awsPath := sb.FakeAWS(t, filepath.Join(sb.Root, "Program Files"), "ok")
	if !strings.Contains(awsPath, " ") {
		t.Fatalf("test needs a path with a space: %s", awsPath)
	}
	repo := sb.InitRepo(t, "r", map[string]string{"origin": ccURL})
	if err := sb.Git.Bind(ctx, spec(repo, awsPath, "acme")); err != nil {
		t.Fatal(err)
	}
	out, stderr, exit := sb.CredentialFillEnv(t, repo, fillCC, "PATH=/usr/bin:/bin:/usr/sbin:/sbin")
	if exit != 0 || !strings.Contains(out, "username=") {
		t.Errorf("exit %d out %q stderr %q", exit, out, stderr)
	}
}

func TestResolveAgainstRealGit(t *testing.T) {
	// Experiments c0/c1/c3/g2: Resolve must predict the helper sequence git really runs.
	ctx := context.Background()
	sb := gitxtest.New(t)
	url := "https://" + host + "/v1/repos/initech_infra"
	other := "https://" + host + "/v1/repos/other"
	spy := func(l string) string { return sb.SpyHelper(t, l) }
	global := "[credential]\n\thelper = \n\thelper = " + spy("GLOBAL-DEFAULT") + "\n\tUseHttpPath = true\n" +
		"[credential \"" + url + "\"]\n\thelper = \n\thelper = " + spy("GLOBAL-URL") + "\n" +
		"[credential \"https://github.com\"]\n\thelper = \n\thelper = " + spy("GLOBAL-GITHUB") + "\n"
	sb.WriteGlobal(t, global)

	layouts := map[string]string{
		"c0": "",
		"c1": "[credential]\n\thelper = \n\thelper = " + spy("LOCAL-PLAIN") + "\n",
		"c3": "[credential]\n\thelper = " + spy("LOCAL-NORESET") + "\n",
		"g2": "[credential \"https://" + host + "\"]\n\thelper = \n\thelper = " + spy("LOCAL-HOST") + "\n\tuseHttpPath = true\n",
	}
	targets := []string{url, other, "https://github.com/org/repo.git", "https://git-codecommit.us-west-2.amazonaws.com/v1/repos/x"}
	for name, local := range layouts {
		repo := sb.InitRepo(t, "repo-"+name, map[string]string{"origin": url})
		if local != "" {
			appendConfig(t, repo, local)
		}
		es, err := sb.Git.CredentialEntries(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		for _, target := range targets {
			sb.ResetLogs(t)
			sb.CredentialFill(t, repo, target)
			var ran []string
			for _, l := range strings.Split(strings.TrimSpace(sb.SpyLog(t)), "\n") {
				if l != "" {
					ran = append(ran, strings.TrimSuffix(strings.TrimPrefix(strings.Fields(l)[0], "spy["), "]"))
				}
			}
			eff := gitx.Resolve(es, mustCred(t, target))
			var predicted []string
			for _, h := range eff.Helpers {
				predicted = append(predicted, h.Value[strings.LastIndex(h.Value, " ")+1:])
			}
			if strings.Join(ran, ",") != strings.Join(predicted, ",") {
				t.Errorf("%s %s: git ran %v, Resolve predicted %v", name, target, ran, predicted)
			}
		}
	}
}

func TestCloneFromFileRemote(t *testing.T) {
	ctx := context.Background()
	sb := gitxtest.New(t)
	bare := sb.InitBare(t, "remote.git")
	dest := filepath.Join(sb.Root, "cloned")
	cs := gitx.CloneSpec{URL: bare, Dest: dest, Bind: spec("", "/opt/homebrew/bin/aws", "acme")}
	args, err := gitx.CloneArgs(cs)
	if err != nil {
		t.Fatal(err)
	}
	if args[0] != "clone" || args[len(args)-1] != dest || args[len(args)-2] != bare || args[len(args)-3] != "--" {
		t.Errorf("args: %v", args)
	}
	var lines []string
	if err := sb.Git.Clone(ctx, cs, func(l string) { lines = append(lines, l) }); err != nil {
		t.Fatal(err)
	}
	if len(lines) == 0 {
		t.Error("no progress lines")
	}
	// git lower-cases keys given with -c, so compare the semantics instead of the bytes.
	cfg := managedBlock(readConfig(t, dest))
	for _, frag := range []string{`[credential "https://` + host + `"]`, "\thelper = \n", `--profile 'acme' codecommit credential-helper $@`,
		"usehttppath = true", "interactive = false", "[awsssohub]", "profile = acme", "accountid = 111122223333", "version = 1"} {
		if !strings.Contains(cfg, frag) {
			t.Errorf("cloned config lacks %q:\n%s", frag, cfg)
		}
	}
	es, err := sb.Git.CredentialEntries(ctx, dest)
	if err != nil {
		t.Fatal(err)
	}
	eff := gitx.Resolve(es, mustCred(t, ccURL))
	cl := gitx.Classify(eff, gitx.LocalMetaFrom(es), api.RemoteHTTPS, "", func(string) bool { return true })
	if cl.Source != api.BindingLocal || cl.Profile != "acme" || cl.NeedsRepair || cl.LegacyPlain || !eff.UseHTTPPath {
		t.Errorf("classification of a cloned repo: %+v %+v", cl, eff)
	}
	// Failure: destination exists and is not empty.
	err = sb.Git.Clone(ctx, cs, nil)
	ae, ok := err.(*api.AppError)
	if !ok || ae.Code != api.ErrCloneFailed || ae.Detail == "" {
		t.Errorf("expected E_CLONE_FAILED with detail, got %v", err)
	}
}

func TestLsRemoteFileRemote(t *testing.T) {
	sb := gitxtest.New(t)
	bare := sb.InitBare(t, "remote.git")
	refs, res, err := sb.Git.LsRemote(context.Background(), bare, nil, 10*time.Second)
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("%v %+v", err, res)
	}
	if sha := refs["refs/heads/main"]; len(sha) != 40 {
		t.Errorf("refs: %v", refs)
	}
	refs, res, err = sb.Git.LsRemote(context.Background(), filepath.Join(sb.Root, "missing.git"), nil, 10*time.Second)
	if err != nil || res.ExitCode == 0 || len(refs) != 0 {
		t.Errorf("missing remote: %v %+v %v", refs, res, err)
	}
}

func TestLsRemoteTreatsOptionLikeTargetAsPath(t *testing.T) {
	sb := gitxtest.New(t)
	marker := filepath.Join(sb.Root, "marker")
	_, res, err := sb.Git.LsRemote(context.Background(), "--upload-pack=touch "+marker, nil, 10*time.Second)
	if err != nil || res.ExitCode == 0 {
		t.Errorf("expected a plain failure: %v %+v", err, res)
	}
	if _, serr := os.Stat(marker); serr == nil {
		t.Error("the target was run as an option")
	}
}

func TestCloneDestinationLikeOptionIsAPath(t *testing.T) {
	ctx := context.Background()
	sb := gitxtest.New(t)
	bare := sb.InitBare(t, "remote.git")
	restore := gitx.SetNeutralParent(t.TempDir())
	t.Cleanup(restore)
	marker := filepath.Join(sb.Root, "marker")
	cs := gitx.CloneSpec{URL: bare, Dest: "--upload-pack=touch " + marker, Bind: spec("", "/opt/homebrew/bin/aws", "acme")}
	if err := sb.Git.Clone(ctx, cs, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(gitx.NeutralDir(), cs.Dest, ".git")); err != nil {
		t.Errorf("the clone should have landed in a folder with that name: %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the destination was run as an option")
	}
}

func TestTestRemoteRefusesNonHTTPSWithoutRunningGit(t *testing.T) {
	sb := gitxtest.New(t)
	marker := filepath.Join(sb.Root, "git-ran")
	fake := filepath.Join(sb.Root, "bin", "fakegit")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\ntouch '"+marker+"'\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	g := &gitx.Git{Bin: fake, Env: sb.Env}
	for _, url := range []string{
		"ssh://" + host + "/v1/repos/acme_cs_portal", "codecommit://acme@acme_cs_portal", filepath.Join(sb.Root, "remote.git"),
		"ext::sh -c touch% " + marker, "https://github.com/acme/x", "origin",
	} {
		st, detail := g.TestRemote(context.Background(), url, "acme", "/opt/homebrew/bin/aws", "darwin", 5*time.Second)
		if st != api.TestError || detail == "" {
			t.Errorf("%s: %s %q", url, st, detail)
		}
	}
	if st, _ := g.TestRemote(context.Background(), ccURL, "acme", "relative/aws", "darwin", 5*time.Second); st != api.TestError {
		t.Errorf("relative aws path: %s", st)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("git must not run for refused remotes")
	}
}

func TestTestRemoteConfig(t *testing.T) {
	info, _ := gitx.ParseRemote(ccURL)
	cfg, err := gitx.TestRemoteConfig(info, "acme", "/opt/homebrew/bin/aws", "darwin")
	if err != nil {
		t.Fatal(err)
	}
	k := "credential.https://" + host
	want := []string{
		"protocol.allow=never", "protocol.https.allow=always", "http.sslVerify=true", "core.askPass=", "credential.interactive=false",
		k + ".helper=", k + `.helper=!"/opt/homebrew/bin/aws" --profile 'acme' codecommit credential-helper $@`, k + ".useHttpPath=true",
	}
	if strings.Join(cfg, "\n") != strings.Join(want, "\n") {
		t.Errorf("config:\n%s\nwant\n%s", strings.Join(cfg, "\n"), strings.Join(want, "\n"))
	}
	if _, err := gitx.TestRemoteConfig(info, "a'b", "/opt/homebrew/bin/aws", "darwin"); err == nil {
		t.Error("unsafe profile must be refused")
	}
}

func TestTestRemoteIgnoresRepositoryConfig(t *testing.T) {
	sb := gitxtest.New(t)
	repo := sb.InitRepo(t, "hostile", map[string]string{"origin": ccURL})
	marker := filepath.Join(sb.Root, "marker")
	cfg := "\n[credential]\n\thelper = !touch " + marker + "-helper\n[core]\n\tsshCommand = touch " + marker + "-ssh\n" +
		"[url \"ext::sh -c touch% " + marker + "-ext\"]\n\tinsteadOf = https://" + host + "/\n[protocol \"ext\"]\n\tallow = always\n"
	f, err := os.OpenFile(filepath.Join(repo, ".git", "config"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(cfg)
	f.Close()
	// Everything stays on this machine: https goes to a closed loopback port.
	g := sb.Git.WithEnv("https_proxy=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1", "no_proxy=", "NO_PROXY=")
	st, _ := g.TestRemote(context.Background(), ccURL, "acme", sb.FakeAWS(t, sb.Root, "ok"), "darwin", 10*time.Second)
	if st != api.TestNetwork {
		t.Errorf("status = %s, want network", st)
	}
	for _, suffix := range []string{"-helper", "-ssh", "-ext"} {
		if _, err := os.Stat(marker + suffix); err == nil {
			t.Errorf("repository config ran code (%s)", suffix)
		}
	}
}

func TestRemoteTrackingRefs(t *testing.T) {
	ctx := context.Background()
	sb := gitxtest.New(t)
	bare := sb.InitBare(t, "remote.git")
	repo := filepath.Join(sb.Root, "clone")
	if res, err := sb.Git.Run(ctx, "", nil, "clone", "-q", bare, repo); err != nil || res.ExitCode != 0 {
		t.Fatalf("clone: %v %s", err, res.Stderr)
	}
	refs, err := sb.Git.RemoteTrackingRefs(ctx, repo, "origin")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs["main"]) != 40 || refs["HEAD"] != "" || len(refs) != 1 {
		t.Errorf("refs: %v", refs)
	}
	remote, _, _ := sb.Git.LsRemote(ctx, bare, nil, 10*time.Second)
	if remote["refs/heads/main"] != refs["main"] {
		t.Errorf("sha mismatch %v vs %v", remote, refs)
	}
}

func TestConfigPathWorktree(t *testing.T) {
	ctx := context.Background()
	sb := gitxtest.New(t)
	repo := sb.InitRepo(t, "r", nil)
	sb.CommitFile(t, repo, "a", "x")
	p, err := sb.Git.ConfigPath(ctx, repo)
	if err != nil || p != filepath.Join(repo, ".git", "config") {
		t.Errorf("ConfigPath = %q %v", p, err)
	}
	wt := filepath.Join(sb.Root, "wt")
	if res, err := sb.Git.Run(ctx, repo, nil, "worktree", "add", "-q", wt, "-b", "feature"); err != nil || res.ExitCode != 0 {
		t.Fatalf("worktree add: %v %s", err, res.Stderr)
	}
	p, err = sb.Git.ConfigPath(ctx, wt)
	if err != nil || p != filepath.Join(repo, ".git", "config") {
		t.Errorf("worktree ConfigPath = %q %v", p, err)
	}
}

func TestSetRemoteURL(t *testing.T) {
	ctx := context.Background()
	sb := gitxtest.New(t)
	repo := sb.InitRepo(t, "r", map[string]string{"origin": "ssh://git-codecommit.us-east-1.amazonaws.com/v1/repos/acme_design"})
	if err := sb.Git.SetRemoteURL(ctx, repo, "origin", "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/acme_design", false); err != nil {
		t.Fatal(err)
	}
	if err := sb.Git.SetRemoteURL(ctx, repo, "origin", "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/acme_design", true); err != nil {
		t.Fatal(err)
	}
	cfg := readConfig(t, repo)
	if strings.Contains(cfg, "ssh://") || !strings.Contains(cfg, "pushurl = https://") {
		t.Errorf("config:\n%s", cfg)
	}
}

func TestBindMultipleHosts(t *testing.T) {
	ctx := context.Background()
	sb := gitxtest.New(t)
	repo := sb.InitRepo(t, "r", map[string]string{"origin": ccURL})
	s := spec(repo, "/opt/homebrew/bin/aws", "p")
	s.Hosts = []string{"git-codecommit.us-east-2.amazonaws.com", host, host}
	if err := sb.Git.Bind(ctx, s); err != nil {
		t.Fatal(err)
	}
	cfg := readConfig(t, repo)
	if strings.Count(cfg, "[credential \"https://") != 2 || strings.Count(cfg, "[awsssohub]") != 1 {
		t.Errorf("config:\n%s", cfg)
	}
	cmds := gitx.DisplayCommands(spec(repo, "/opt/homebrew/bin/aws", "acme"))
	if len(cmds) != 8 || !strings.HasPrefix(cmds[0], "git config --local credential.https://"+host+".helper ''") {
		t.Errorf("display commands: %q", cmds)
	}
	if !strings.Contains(cmds[1], `--add credential.https://`+host+`.helper '!"/opt/homebrew/bin/aws" --profile '\''acme'\'' codecommit credential-helper $@'`) {
		t.Errorf("quoting: %s", cmds[1])
	}
}

func appendConfig(t *testing.T, repo, text string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(repo, ".git", "config"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func mustCred(t *testing.T, raw string) gitx.CredURL {
	t.Helper()
	u, err := gitx.ParseCredURL(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// fakeArgvGit returns a Git whose binary appends its arguments, one per line, to the returned file.
func fakeArgvGit(t *testing.T) (*gitx.Git, func() []string) {
	t.Helper()
	sb := gitxtest.New(t)
	log := filepath.Join(sb.Root, "argv.log")
	fake := filepath.Join(sb.Root, "bin", "fakegit-argv")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> '" + log + "'; done\nprintf -- '---\\n' >> '" + log + "'\nexit 0\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	read := func() []string {
		b, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
		// the last invocation only
		for i := len(lines) - 2; i >= 0; i-- {
			if lines[i] == "---" {
				return lines[i+1 : len(lines)-1]
			}
		}
		return lines[:len(lines)-1]
	}
	return &gitx.Git{Bin: fake, Env: sb.Env}, read
}

func lastTwo(argv []string) []string {
	if len(argv) < 2 {
		return argv
	}
	return argv[len(argv)-2:]
}

func TestLsRemotePutsDoubleDashBeforeTarget(t *testing.T) {
	g, argv := fakeArgvGit(t)
	for _, target := range []string{"--upload-pack=touch /tmp/x", "-oProxyCommand=x", "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/acme_design"} {
		if _, _, err := g.LsRemote(context.Background(), target, []string{"credential.interactive=false"}, 5*time.Second); err != nil {
			t.Fatal(err)
		}
		got := argv()
		if want := []string{"--", target}; strings.Join(lastTwo(got), "\x00") != strings.Join(want, "\x00") {
			t.Errorf("ls-remote argv ends %q, want %q (full: %q)", lastTwo(got), want, got)
		}
		var sawCmd bool
		for _, a := range got {
			sawCmd = sawCmd || a == "ls-remote"
		}
		if !sawCmd {
			t.Errorf("no ls-remote in %q", got)
		}
	}
}

func TestTestRemotePassesURLAfterDoubleDash(t *testing.T) {
	g, argv := fakeArgvGit(t)
	g.TestRemote(context.Background(), ccURL, "acme", "/opt/homebrew/bin/aws", "darwin", 5*time.Second)
	got := argv()
	if len(got) < 2 || got[len(got)-2] != "--" || !strings.HasPrefix(got[len(got)-1], "https://"+host+"/") {
		t.Errorf("argv: %q", got)
	}
}

func TestValidRemoteName(t *testing.T) {
	for _, n := range []string{"origin", "up/stream", "team.fork", "a_b-c", "r1"} {
		if !gitx.ValidRemoteName(n) {
			t.Errorf("%q must be accepted", n)
		}
	}
	for _, n := range []string{"", "-x", "--upload-pack=x", "a b", "a\nb", "a\tb", "a;b", "a$b", "a'b", "é"} {
		if gitx.ValidRemoteName(n) {
			t.Errorf("%q must be rejected", n)
		}
	}
}

func TestSetRemoteURLRejectsOptionLikeRemoteWithoutRunningGit(t *testing.T) {
	g, _ := fakeArgvGit(t)
	for _, name := range []string{"-x", "--upload-pack=x", "a b", "a\nb"} {
		err := g.SetRemoteURL(context.Background(), t.TempDir(), name, "https://"+host+"/v1/repos/acme_design", false)
		ae, ok := err.(*api.AppError)
		if !ok || ae.Code != api.ErrInvalidInput || ae.Params["field"] != "remote" {
			t.Errorf("%q: %v", name, err)
		}
	}
}

func TestSetRemoteURLPassesDoubleDash(t *testing.T) {
	g, argv := fakeArgvGit(t)
	url := "https://" + host + "/v1/repos/acme_design"
	if err := g.SetRemoteURL(context.Background(), t.TempDir(), "origin", url, true); err != nil {
		t.Fatal(err)
	}
	got := argv()
	tail := got[len(got)-3:]
	if strings.Join(tail, "\x00") != strings.Join([]string{"--", "origin", url}, "\x00") {
		t.Errorf("argv: %q", got)
	}
	var push bool
	for _, a := range got {
		push = push || a == "--push"
	}
	if !push {
		t.Errorf("--push missing: %q", got)
	}
}
