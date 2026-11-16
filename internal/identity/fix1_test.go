package identity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/projects"
)

// envWithout is a terminal environment that exports none of IdentityEnvNames (no XDG_CONFIG_HOME either),
// which is the usual state of a Mac.
func (r *rig) bareEnv() []string {
	return []string{"HOME=" + r.p.Home, "PATH=" + os.Getenv("PATH"), "ZDOTDIR=" + r.zdot}
}

// The probe script's last variable is not exported on a default machine; the shell's exit status is then 1.
// That must not read as a failed probe.
func TestProbeSucceedsWhenNothingIsExported(t *testing.T) {
	for _, sh := range []string{"/bin/zsh", "/bin/bash", "/bin/sh"} {
		if _, err := os.Stat(sh); err != nil {
			continue
		}
		t.Run(filepath.Base(sh), func(t *testing.T) {
			r := newRig(t, func(d *Deps) { d.Shell = func() string { return sh } })
			r.eng.d.Env = func() []string { return r.bareEnv() }
			vars, err := ProbeShellEnv(bg, ProbeOptions{Shell: sh, Env: r.bareEnv(), Dir: r.p.Home})
			if err != nil || len(vars) != 0 {
				t.Fatalf("%v %v", vars, err)
			}
			env := r.eng.TerminalEnv(bg, true)
			if !env.Probed || env.ProbeError != "" || env.Active || len(env.Vars) != 0 {
				t.Fatalf("%+v", env)
			}
			// the hint scan lists files only when something is exported
			r.write(filepath.Join(r.zdot, ".zshrc"), "GIT_AUTHOR_NAME=plain\n")
			r.write(filepath.Join(r.p.Home, ".bash_profile"), "GIT_AUTHOR_NAME=plain\n")
			if env := r.eng.TerminalEnv(bg, true); len(env.Files) != 0 || !env.Probed {
				t.Fatalf("%+v", env)
			}
		})
	}
}

func TestProbeFailureKinds(t *testing.T) {
	r := newRig(t)
	bad := filepath.Join(r.root, "badshell")
	r.write(bad, "#!/bin/sh\nexit 7\n")
	if err := os.Chmod(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := ProbeShellEnv(bg, ProbeOptions{Shell: bad, Env: r.bareEnv()})
	pf, ok := err.(*ProbeFailure)
	if !ok || pf.Kind != api.ProbeFailed || pf.Exit != 7 {
		t.Fatalf("%v", err)
	}
	slow := filepath.Join(r.root, "slowshell")
	r.write(slow, "#!/bin/sh\nsleep 5\n")
	if err := os.Chmod(slow, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = ProbeShellEnv(bg, ProbeOptions{Shell: slow, Env: r.bareEnv(), Timeout: 200 * time.Millisecond})
	if pf, ok := err.(*ProbeFailure); !ok || pf.Kind != api.ProbeTimeout {
		t.Fatalf("%v", err)
	}
	e2 := newRig(t, func(d *Deps) { d.Shell = func() string { return bad } })
	env := e2.eng.TerminalEnv(bg, true)
	if env.Probed || env.ProbeError != api.ProbeFailed || env.ProbeExitCode != 7 {
		t.Fatalf("%+v", env)
	}
	e3 := newRig(t, func(d *Deps) { d.Shell = func() string { return "/nonexistent/shell" } })
	if env := e3.eng.TerminalEnv(bg, true); env.ProbeError != api.ProbeNoShell {
		t.Fatalf("%+v", env)
	}
}

// A probe that failed only because the caller gave up is not remembered.
func TestCancelledProbeIsNotCached(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	r := newRig(t, func(d *Deps) {
		d.Shell = func() string { mu.Lock(); calls++; mu.Unlock(); return "/bin/zsh" }
	})
	r.eng.d.Env = func() []string { return r.bareEnv() }
	ctx, cancel := context.WithCancel(bg)
	cancel()
	if env := r.eng.TerminalEnv(ctx, false); env.Probed {
		t.Fatalf("%+v", env)
	}
	if env := r.eng.TerminalEnv(bg, false); !env.Probed || env.ProbeError != "" {
		t.Fatalf("a later call must probe again: %+v", env)
	}
	if calls != 2 {
		t.Fatalf("probes: %d", calls)
	}
}

func TestTerminalEnvBuilder(t *testing.T) {
	base := []string{
		"AWSSSOHUB_SECRET=x", "WAILS_DEBUG=1", "TERM=dumb", "LANG=C", "HTTP_PROXY=http://old", "https_proxy=http://old2",
		"HOME=/real/home", "GIT_AUTHOR_EMAIL=a@example.com", "AWS_PROFILE=keep", "PATH=/nowhere",
	}
	env := terminalEnvFrom(base, "/tmp/home", []string{"HTTP_PROXY=http://new"})
	m := map[string][]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = append(m[k], v)
	}
	one := func(k, want string) {
		t.Helper()
		if len(m[k]) != 1 || m[k][0] != want {
			t.Errorf("%s = %v, want %s", k, m[k], want)
		}
	}
	one("TERM", "xterm-256color")
	one("COLORTERM", "truecolor")
	one("LANG", "en_US.UTF-8")
	one("HOME", "/tmp/home")
	one("HTTP_PROXY", "http://new")
	one("GIT_AUTHOR_EMAIL", "a@example.com") // inherited, as in a real terminal
	one("AWS_PROFILE", "keep")
	for _, k := range []string{"AWSSSOHUB_SECRET", "WAILS_DEBUG", "https_proxy", "GIT_TERMINAL_PROMPT", "GCM_INTERACTIVE", "AWS_PAGER"} {
		if _, ok := m[k]; ok {
			t.Errorf("%s must not be in the terminal environment", k)
		}
	}
	// an existing UTF-8 locale is kept
	env = terminalEnvFrom([]string{"LANG=zh_CN.UTF-8"}, "/h", nil)
	found := false
	for _, kv := range env {
		found = found || kv == "LANG=zh_CN.UTF-8"
	}
	if !found {
		t.Errorf("%v", env)
	}
}

// Only GIT_AUTHOR_* decides the author. EMAIL is git's last fallback and GIT_COMMITTER_* never changes the author.
func TestEnvActiveMeansAuthorVariables(t *testing.T) {
	r := newRig(t)
	r.setGlobal("Alex Global", "global@example.com")
	rec := r.repo(filepath.Join(r.root, "work"), "acme_api")
	r.write(filepath.Join(r.zdot, ".zshrc"), "export EMAIL=fallback@example.com\nexport GIT_COMMITTER_NAME=Committer\nexport GIT_COMMITTER_EMAIL=c@example.com\n")
	ov := r.overview()
	if ov.Env.Active || !ov.Env.FallbackEmail || !ov.Env.CommitterVars || !ov.Env.Probed {
		t.Fatalf("%+v", ov.Env)
	}
	if row := r.row(ov, rec.ID); row.Effective.EmailSource.Kind != api.IdentitySrcGlobal || row.Effective.Email != "global@example.com" {
		t.Fatalf("%+v", row.Effective)
	}
	r.write(filepath.Join(r.zdot, ".zshrc"), "export GIT_AUTHOR_NAME=Env\n")
	r.eng.TerminalEnv(bg, true) // a new probe
	if ov := r.overview(); !ov.Env.Active || ov.Env.CommitterVars || ov.Env.FallbackEmail {
		t.Fatalf("%+v", ov.Env)
	}
}

// With no global identity EMAIL is the source of the email, and then the environment is in effect.
func TestEnvActiveWhenAnEnvVarIsASource(t *testing.T) {
	r := newRig(t)
	rec := r.repo(filepath.Join(r.root, "work"), "acme_api")
	r.write(filepath.Join(r.zdot, ".zshrc"), "export EMAIL=fallback@example.com\n")
	ov := r.overview()
	if row := r.row(ov, rec.ID); row.Effective.EmailSource.Kind != api.IdentitySrcEnv || row.Effective.EmailSource.Var != "EMAIL" {
		t.Fatalf("%+v", row.Effective)
	}
	if !ov.Env.Active {
		t.Fatalf("an effective env source must raise the warning: %+v", ov.Env)
	}
}

func TestNoShellReportsNoShellName(t *testing.T) {
	r := newRig(t, func(d *Deps) { d.GOOS = "windows"; d.Shell = func() string { return "" } })
	if env := r.eng.TerminalEnv(bg, true); env.Shell != "" {
		t.Fatalf("shell %q", env.Shell)
	}
}

// OnWrite after an undo that removed the file gets nil content.
func TestUndoOfCreatedGlobalTellsTheWatcherItIsGone(t *testing.T) {
	var mu sync.Mutex
	var got []struct {
		path string
		nil_ bool
	}
	r := newRig(t, func(d *Deps) {
		d.OnWrite = func(p string, c []byte) {
			mu.Lock()
			got = append(got, struct {
				path string
				nil_ bool
			}{p, c == nil})
			mu.Unlock()
		}
	})
	res, err := r.eng.ApplyGlobal(bg, acme(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.eng.Undo(bg, res.UndoID); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].nil_ || !got[1].nil_ || got[1].path != r.p.GitGlobalFile {
		t.Fatalf("%+v", got)
	}
}

// The repository config path is read through the read-only layer.
func TestOverviewReadsConfigPathReadOnly(t *testing.T) {
	r := newRig(t)
	log := filepath.Join(r.root, "git.log")
	wrapper := filepath.Join(r.root, "gitwrap")
	r.write(wrapper, "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '"+log+"'\nexec '"+r.git.Bin+"' \"$@\"\n")
	if err := os.Chmod(wrapper, 0o755); err != nil {
		t.Fatal(err)
	}
	wg, err := gitx.New(bg, wrapper, r.env)
	if err != nil {
		t.Fatal(err)
	}
	r.eng.d.Git = func(context.Context) (*gitx.Git, error) { return wg, nil }
	r.repo(filepath.Join(r.root, "work"), "acme_api")
	_ = os.Remove(log)
	r.overview()
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if strings.Contains(line, "--git-path") {
			n++
			if !strings.Contains(line, "--no-optional-locks") || !strings.Contains(line, "core.fsmonitor=false") {
				t.Errorf("rev-parse without the read-only flags: %s", line)
			}
		}
	}
	if n == 0 {
		t.Fatalf("no rev-parse seen:\n%s", b)
	}
}

// worktree makes a linked worktree of rec next to it and registers it.
func (r *rig) worktree(rec projects.RepoRecord, name string) projects.RepoRecord {
	r.t.Helper()
	r.gitIn(rec.Path, "-c", "user.name=T", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "x")
	dir := filepath.Join(filepath.Dir(rec.Path), name)
	r.gitIn(rec.Path, "worktree", "add", "-q", "-b", "wt-"+name, dir)
	wt, _, err := r.proj.EnsureRepo(dir, projects.ProviderOther)
	if err != nil {
		r.t.Fatal(err)
	}
	return wt
}

// A repository and its linked worktree share one config file; one project save writes it once.
func TestProjectApplyWithLinkedWorktree(t *testing.T) {
	r := newRig(t)
	r.setGlobal("Alex Global", "global@example.com")
	work := filepath.Join(r.root, "work")
	api1 := r.repo(work, "acme_api")
	wt := r.worktree(api1, "acme_api-wt")
	p := r.project("acme", api1, wt)
	req := api.ProjectIdentityRequest{ProjectID: p.ID, Identity: ptr(acme())}
	pv, err := r.eng.PreviewProject(bg, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(pv.Repos) != 2 || pv.Repos[0].Action != api.RepoActionSet || pv.Repos[1].Action != api.RepoActionSet {
		t.Fatalf("%+v", actions(pv.Repos))
	}
	res, err := r.eng.ApplyProject(bg, req, pv.PlanHash)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range res.Outcomes {
		if !o.OK {
			t.Fatalf("%+v", res.Outcomes)
		}
	}
	if !res.RecordUpdated || r.snap().FindProject(p.ID).Identity == nil {
		t.Fatal("the project record must be saved")
	}
	if n := len(r.undoFiles(res)); n != 1 || len(res.BackupIDs) != 1 {
		t.Fatalf("one write expected, got %d files %d backups", n, len(res.BackupIDs))
	}
	if got := r.gitIn(wt.Path, "config", "user.email"); got != acme().Email {
		t.Fatal(got)
	}
	if _, err := r.eng.Undo(bg, res.UndoID); err != nil {
		t.Fatal(err)
	}
}

// A member whose config cannot be read is reported, not skipped as missing, and the project record is kept.
func TestUnreadableRepoConfigIsAnErrorNotMissing(t *testing.T) {
	r := newRig(t)
	r.setGlobal("Alex Global", "global@example.com")
	work := filepath.Join(r.root, "work")
	good := r.repo(work, "acme_api")
	bad := r.repo(work, "acme_bad")
	p := r.project("acme", good, bad)
	r.write(r.localCfg(bad), "[user\n\tname = broken\n")
	req := api.ProjectIdentityRequest{ProjectID: p.ID, Identity: ptr(acme())}
	pv, err := r.eng.PreviewProject(bg, req)
	if err != nil {
		t.Fatal(err)
	}
	var plan *api.RepoChangePlan
	for i := range pv.Repos {
		if pv.Repos[i].Name == "acme_bad" {
			plan = &pv.Repos[i]
		}
	}
	if plan == nil || plan.Action != api.RepoActionError || plan.Error == nil || plan.Error.Params["file"] == "" ||
		(plan.Error.Code != api.ErrGitFailed && plan.Error.Code != api.ErrFileIO) {
		t.Fatalf("%+v", pv.Repos)
	}
	res, err := r.eng.ApplyProject(bg, req, pv.PlanHash)
	if err != nil {
		t.Fatal(err)
	}
	var failed *api.RepoChangeOutcome
	for i, o := range res.Outcomes {
		if !o.OK {
			failed = &res.Outcomes[i]
		}
	}
	if failed == nil || failed.Name != "acme_bad" || failed.Error == nil || res.RecordUpdated || r.snap().FindProject(p.ID).Identity != nil {
		t.Fatalf("%+v", res)
	}
	// the Overview row carries a translatable error
	row := r.row(r.overview(), bad.ID)
	if row.ReadError == nil || row.ReadError.Code == "" {
		t.Fatalf("%+v", row)
	}
}

// A write that fails before git changed anything leaves the file alone, including its mode.
func TestFailedFirstWriteDoesNotRestoreTheFile(t *testing.T) {
	r := newRig(t)
	r.setGlobal("Alex Global", "global@example.com")
	rec := r.repo(filepath.Join(r.root, "work"), "acme_api")
	cfg := r.localCfg(rec)
	if err := os.Chmod(cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	r.write(cfg+".lock", "")
	req := api.RepoIdentityRequest{RepoID: rec.ID, Mode: api.RepoModeOwn, Identity: ptr(acme())}
	pv, err := r.eng.PreviewRepo(bg, req)
	if err != nil {
		t.Fatal(err)
	}
	before := r.read(cfg)
	if _, err := r.eng.ApplyRepo(bg, req, pv.PlanHash); api.AsAppError(err).Code != api.ErrFileIO {
		t.Fatalf("%v", err)
	}
	st, _ := os.Stat(cfg)
	if r.read(cfg) != before || st.Mode().Perm() != 0o644 {
		t.Fatalf("file touched: %v", st.Mode())
	}
}

// Relative and "~/" includes resolve to the file they name (relative: next to the file that has the rule).
func TestRelativeAndTildeIncludesAreSources(t *testing.T) {
	r := newRig(t)
	r.setGlobal("Alex Global", "global@example.com")
	rel := filepath.Join(r.p.Home, "rel.inc")
	tilde := filepath.Join(r.p.Home, ".tilde.inc")
	r.write(rel, "[user]\n\tname = Rel Name\n")
	r.write(tilde, "[user]\n\temail = tilde@example.com\n")
	r.gitIn(r.p.Home, "config", "--file", r.p.GitGlobalFile, "include.path", "rel.inc")
	r.gitIn(r.p.Home, "config", "--file", r.p.GitGlobalFile, "--add", "include.path", "~/.tilde.inc")
	rec := r.repo(filepath.Join(r.root, "work"), "acme_api")
	row := r.row(r.overview(), rec.ID)
	if row.Effective.NameSource.Kind != api.IdentitySrcInclude || row.Effective.NameSource.Path != rel || row.Effective.Name != "Rel Name" {
		t.Fatalf("relative: %+v", row.Effective.NameSource)
	}
	if row.Effective.EmailSource.Kind != api.IdentitySrcInclude || row.Effective.EmailSource.Path != tilde || row.Effective.Email != "tilde@example.com" {
		t.Fatalf("tilde: %+v", row.Effective.EmailSource)
	}
}

// Rows of the precedence table that the first test does not cover.
func TestPrecedenceMoreRows(t *testing.T) {
	r := newRig(t)
	r.setGlobal("Alex Global", "global@example.com")
	work := filepath.Join(r.root, "work")

	// includeIf (read at its place in the global file) loses to a local value
	incl := filepath.Join(r.p.Home, ".gitconfig-work")
	r.write(incl, "[user]\n\temail = cond@example.com\n\tname = Cond\n")
	r.gitIn(r.p.Home, "config", "--file", r.p.GitGlobalFile, "includeIf.gitdir:"+work+"/.path", incl)
	cond := r.repo(work, "cond_only")
	local := r.repo(work, "cond_and_local")
	r.gitIn(local.Path, "config", "user.email", "local@example.com")
	ov := r.overview()
	if row := r.row(ov, cond.ID); row.Effective.EmailSource.Kind != api.IdentitySrcConditional || row.Effective.Email != "cond@example.com" {
		t.Fatalf("includeIf alone: %+v", row.Effective)
	}
	if row := r.row(ov, local.ID); row.Effective.EmailSource.Kind != api.IdentitySrcRepo || row.Effective.Email != "local@example.com" ||
		row.Effective.NameSource.Kind != api.IdentitySrcConditional {
		t.Fatalf("includeIf vs local: %+v", row.Effective)
	}

	// author.* beats user.* whatever the order
	r.gitIn(local.Path, "config", "author.email", "author@example.com")
	if row := r.row(r.overview(), local.ID); row.Effective.Email != "author@example.com" || row.Effective.EmailSource.Kind != api.IdentitySrcRepo {
		t.Fatalf("author.*: %+v", row.Effective)
	}

	// the environment beats a local value, and the setting view ignores it
	r.write(filepath.Join(r.zdot, ".zshrc"), "export GIT_AUTHOR_EMAIL=env@example.com\n")
	r.eng.TerminalEnv(bg, true)
	row := r.row(r.overview(), local.ID)
	if row.Effective.Email != "env@example.com" || row.Effective.EmailSource.Kind != api.IdentitySrcEnv {
		t.Fatalf("env vs local: %+v", row.Effective)
	}
	if row.Setting.Email != "author@example.com" || row.Setting.EmailSource.Kind == api.IdentitySrcEnv {
		t.Fatalf("setting ignores env: %+v", row.Setting)
	}
}

// FU-16a: a per-repository change must not write a config that a linked worktree with its own identity reads.
func TestRepoChangeRefusesAConfigSharedWithAnotherOwnIdentity(t *testing.T) {
	r := newRig(t)
	r.setGlobal("Alex Global", "global@example.com")
	work := filepath.Join(r.root, "work")
	api1 := r.repo(work, "acme_api")
	wt := r.worktree(api1, "acme_api-wt")
	other := r.repo(work, "acme_web") // its own config file
	// acme_api sits in a project that has an identity, so "follow the project" has something to write
	prj := r.project("acme", api1)
	preq := api.ProjectIdentityRequest{ProjectID: prj.ID, Identity: ptr(acme())}
	ppv, err := r.eng.PreviewProject(bg, preq)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.eng.ApplyProject(bg, preq, ppv.PlanHash); err != nil {
		t.Fatal(err)
	}
	// the worktree gets an identity of its own first: nobody else has one, so that is allowed
	own := api.RepoIdentityRequest{RepoID: wt.ID, Mode: api.RepoModeOwn, Identity: ptr(globex())}
	pv, err := r.eng.PreviewRepo(bg, own)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.eng.ApplyRepo(bg, own, pv.PlanHash); err != nil {
		t.Fatal(err)
	}
	before := r.read(r.localCfg(api1))
	for _, mode := range []api.RepoIdentityMode{api.RepoModeOwn, api.RepoModeGlobal, api.RepoModeProject} {
		req := api.RepoIdentityRequest{RepoID: api1.ID, Mode: mode}
		if mode == api.RepoModeOwn {
			req.Identity = ptr(acme())
		}
		_, err := r.eng.PreviewRepo(bg, req)
		var ae *api.AppError
		if !errors.As(err, &ae) || ae.Code != api.ErrConfigShared || ae.Params["file"] == "" {
			t.Fatalf("mode %s: %v", mode, err)
		}
		if _, err := r.eng.ApplyRepo(bg, req, "any"); !errors.As(err, &ae) || ae.Code != api.ErrConfigShared {
			t.Fatalf("apply, mode %s: %v", mode, err)
		}
	}
	if r.read(r.localCfg(api1)) != before {
		t.Fatal("nothing may be written")
	}
	// another repository with its own config is not affected
	if _, err := r.eng.PreviewRepo(bg, api.RepoIdentityRequest{RepoID: other.ID, Mode: api.RepoModeOwn, Identity: ptr(acme())}); err != nil {
		t.Fatal(err)
	}
}

// FU-13a: a worktree that shares its repository's config file reports the owner's outcome: when the write of
// that one file fails, both report the failure and the project record stays as it was.
func TestLinkedWorktreeReportsTheOwnersFailedWrite(t *testing.T) {
	r := newRig(t)
	r.setGlobal("Alex Global", "global@example.com")
	work := filepath.Join(r.root, "work")
	api1 := r.repo(work, "acme_api")
	wt := r.worktree(api1, "acme_api-wt")
	p := r.project("acme", api1, wt)
	r.write(r.localCfg(api1)+".lock", "") // git cannot take the shared config
	req := api.ProjectIdentityRequest{ProjectID: p.ID, Identity: ptr(acme())}
	pv, err := r.eng.PreviewProject(bg, req)
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.eng.ApplyProject(bg, req, pv.PlanHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Outcomes) != 2 {
		t.Fatalf("%+v", res.Outcomes)
	}
	for _, o := range res.Outcomes {
		if o.OK || o.Error == nil || o.Error.Code != api.ErrFileIO {
			t.Errorf("%s reports success for a file that was not written: %+v", o.Name, o)
		}
	}
	if res.RecordUpdated || r.snap().FindProject(p.ID).Identity != nil {
		t.Fatal("the project record must not change while a write failed")
	}
}
