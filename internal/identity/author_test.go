package identity

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/backup"
	"github.com/rudian/codepass/internal/execx"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/paths"
	"github.com/rudian/codepass/internal/projects"
	"github.com/rudian/codepass/internal/textdiff"
)

var bg = context.Background()

// rig is one isolated machine: private HOME, global config, ZDOTDIR and projects.json. Nothing here reads the
// real home, the real ~/.gitconfig, a real shell profile or a real repository.
type rig struct {
	t     *testing.T
	root  string
	p     paths.Paths
	zdot  string
	git   *gitx.Git
	proj  *projects.Store
	bk    *backup.Store
	eng   *Engine
	env   []string
	notes []string
}

func unset(t *testing.T, names ...string) {
	for _, n := range names {
		t.Setenv(n, "")
		os.Unsetenv(n)
	}
}

func newRig(t *testing.T, mut ...func(*Deps)) *rig {
	t.Helper()
	if runtimeWindows() {
		t.Skip("uses POSIX shells")
	}
	execx.Configure(execx.Options{DisableLoginShell: true})
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := paths.ForRoot(root)
	r := &rig{t: t, root: root, p: p, zdot: filepath.Join(root, "zdot")}
	for _, d := range []string{p.Home, r.zdot, p.BackupDir, filepath.Join(root, "work")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", p.Home)
	t.Setenv("USERPROFILE", p.Home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(p.Home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", p.GitGlobalFile)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("ZDOTDIR", r.zdot)
	t.Setenv("SHELL", "/bin/zsh")
	unset(t, "GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "EMAIL",
		"GIT_CONFIG_COUNT", "GIT_CONFIG_SYSTEM", "GIT_DIR", "GIT_WORK_TREE")
	r.env = []string{"HOME=" + p.Home, "XDG_CONFIG_HOME=" + filepath.Join(p.Home, ".config"), "GIT_CONFIG_GLOBAL=" + p.GitGlobalFile, "GIT_CONFIG_NOSYSTEM=1"}
	g, err := gitx.New(bg, "", r.env)
	if err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	r.git = g
	r.proj, err = projects.Open(filepath.Join(root, "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	r.bk, err = backup.Open(p.BackupDir, func() int { return 50 }, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	d := Deps{
		Paths: p, Git: func(context.Context) (*gitx.Git, error) { return r.git, nil }, Projects: r.proj, Backup: r.bk,
		Shell: func() string { return "/bin/zsh" },
		Emit:  func(name string, _ any) { r.notes = append(r.notes, name) },
	}
	for _, m := range mut {
		m(&d)
	}
	r.eng = New(d)
	return r
}

func runtimeWindows() bool { return os.PathSeparator == '\\' }

func (r *rig) write(path, content string) {
	r.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) read(path string) string {
	r.t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		r.t.Fatal(err)
	}
	return string(b)
}

func (r *rig) gitIn(dir string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command(r.git.Bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// repo makes a repository below parent and registers it.
func (r *rig) repo(parent, name string) projects.RepoRecord {
	r.t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.t.Fatal(err)
	}
	r.gitIn(dir, "init", "-q")
	rec, _, err := r.proj.EnsureRepo(dir, projects.ProviderOther)
	if err != nil {
		r.t.Fatal(err)
	}
	return rec
}

func (r *rig) project(name string, repos ...projects.RepoRecord) projects.Project {
	r.t.Helper()
	p, err := r.proj.AddProject(name)
	if err != nil {
		r.t.Fatal(err)
	}
	for _, rr := range repos {
		if err := r.proj.SetProject(rr.ID, p.ID); err != nil {
			r.t.Fatal(err)
		}
	}
	return p
}

func (r *rig) setGlobal(name, email string) {
	r.t.Helper()
	r.gitIn(r.p.Home, "config", "--file", r.p.GitGlobalFile, "user.name", name)
	r.gitIn(r.p.Home, "config", "--file", r.p.GitGlobalFile, "user.email", email)
}

func (r *rig) row(ov api.IdentityOverview, id string) api.RepoIdentity {
	r.t.Helper()
	for _, x := range ov.Ungrouped {
		if x.RepoID == id {
			return x
		}
	}
	for _, p := range ov.Projects {
		for _, x := range p.Repos {
			if x.RepoID == id {
				return x
			}
		}
	}
	r.t.Fatalf("repo %s not in the overview", id)
	return api.RepoIdentity{}
}

func (r *rig) overview() api.IdentityOverview {
	r.t.Helper()
	ov, err := r.eng.Overview(bg)
	if err != nil {
		r.t.Fatal(err)
	}
	return ov
}

func (r *rig) localCfg(rec projects.RepoRecord) string {
	return filepath.Join(rec.Path, ".git", "config")
}

func acme() api.Author   { return api.Author{Name: "Alex", Email: "alex@acme.example.com"} }
func globex() api.Author { return api.Author{Name: "Alex G", Email: "alex@globex.example.com"} }

func TestPrecedenceTable(t *testing.T) {
	r := newRig(t)
	r.setGlobal("Alex Global", "global@example.com")
	work := filepath.Join(r.root, "work")
	loose := r.repo(work, "loose")           // ungrouped, nothing local
	member := r.repo(work, "acme_api")       // follows the project
	own := r.repo(work, "acme_agent_portal") // own override
	ext := r.repo(work, "acme_cs_portal")    // local value set outside the app
	r.gitIn(ext.Path, "config", "user.name", "Ext")
	r.gitIn(ext.Path, "config", "user.email", "ext@example.com")
	p := r.project("acme", member, own, ext)

	// global only
	ov := r.overview()
	if g := ov.Global; !g.Set || g.Name != "Alex Global" || g.Source.Kind != api.IdentitySrcGlobal || g.UsedBy != 3 {
		t.Fatalf("global: %+v", g)
	}
	for _, rec := range []projects.RepoRecord{loose, member, own} {
		row := r.row(ov, rec.ID)
		if row.Effective.Email != "global@example.com" || row.Effective.Source.Kind != api.IdentitySrcGlobal || row.Setting.Source.Kind != api.IdentitySrcGlobal {
			t.Fatalf("%s: %+v", rec.Path, row.Effective)
		}
	}
	if row := r.row(ov, ext.ID); row.Effective.Source.Kind != api.IdentitySrcRepo || !row.Effective.Source.External || row.Effective.Email != "ext@example.com" {
		t.Fatalf("external: %+v", row.Effective)
	}

	// project identity: member follows (from-project), own keeps nothing yet, external is kept
	prev, err := r.eng.PreviewProject(bg, api.ProjectIdentityRequest{ProjectID: p.ID, Identity: ptr(acme())})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.eng.ApplyProject(bg, api.ProjectIdentityRequest{ProjectID: p.ID, Identity: ptr(acme())}, prev.PlanHash); err != nil {
		t.Fatal(err)
	}
	// repository override on `own`
	ro := api.RepoIdentityRequest{RepoID: own.ID, Mode: api.RepoModeOwn, Identity: ptr(globex())}
	rp, err := r.eng.PreviewRepo(bg, ro)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.eng.ApplyRepo(bg, ro, rp.PlanHash); err != nil {
		t.Fatal(err)
	}
	ov = r.overview()
	if row := r.row(ov, member.ID); row.Effective.Source.Kind != api.IdentitySrcFromProject || row.Effective.Email != acme().Email {
		t.Fatalf("member: %+v", row.Effective)
	}
	if row := r.row(ov, own.ID); row.Effective.Source.Kind != api.IdentitySrcRepo || row.Effective.Source.External || row.Effective.Email != globex().Email || row.Override == nil {
		t.Fatalf("own: %+v", row.Effective)
	}
	if row := r.row(ov, ext.ID); !row.Effective.Source.External || row.Effective.Email != "ext@example.com" {
		t.Fatalf("external after project save: %+v", row.Effective)
	}
	if row := r.row(ov, loose.ID); row.Effective.Source.Kind != api.IdentitySrcGlobal {
		t.Fatalf("ungrouped: %+v", row.Effective)
	}
	if ov.Projects[0].Identity == nil || ov.Projects[0].Identity.Email != acme().Email {
		t.Fatalf("project row: %+v", ov.Projects[0])
	}
}

func ptr[T any](v T) *T { return &v }

func TestIncludeIfIsTheSource(t *testing.T) {
	r := newRig(t)
	r.setGlobal("Alex Global", "global@example.com")
	work := filepath.Join(r.root, "work")
	incl := filepath.Join(r.p.Home, ".gitconfig-work")
	r.write(incl, "[user]\n\temail = alex@initech.example.com\n")
	rule := "gitdir:" + work + "/"
	r.gitIn(r.p.Home, "config", "--file", r.p.GitGlobalFile, "includeIf."+rule+".path", incl)
	in := r.repo(work, "initech_api")
	out := r.repo(filepath.Join(r.root, "elsewhere"), "other_api")
	// a relative include and a "~/" include are resolved too
	ov := r.overview()
	row := r.row(ov, in.ID)
	src := row.Effective.EmailSource
	if src.Kind != api.IdentitySrcConditional || src.Rule != rule || src.Path != incl || row.Effective.Email != "alex@initech.example.com" {
		t.Fatalf("includeIf repo: %+v", row.Effective)
	}
	if row.Effective.NameSource.Kind != api.IdentitySrcGlobal || row.Effective.Name != "Alex Global" {
		t.Fatalf("name must still come from global: %+v", row.Effective.NameSource)
	}
	if row.Effective.Source.Kind != api.IdentitySrcConditional {
		t.Fatalf("the tag is the conditional rule: %+v", row.Effective.Source)
	}
	if row := r.row(ov, out.ID); row.Effective.Source.Kind != api.IdentitySrcGlobal {
		t.Fatalf("outside the rule: %+v", row.Effective)
	}
}

func TestShellProfileEnvironmentIsReported(t *testing.T) {
	if _, err := os.Stat("/bin/zsh"); err != nil {
		t.Skip("no zsh")
	}
	r := newRig(t)
	r.setGlobal("Alex Global", "global@example.com")
	rec := r.repo(filepath.Join(r.root, "work"), "acme_api")
	// profile files: an export in .zshrc (noise included), a plain assignment that never reaches git in .zprofile
	r.write(filepath.Join(r.zdot, ".zshrc"), "echo noise on stdout\nexport GIT_AUTHOR_EMAIL=profile@example.com\n(sleep 2 &) >/dev/null 2>&1\n")
	r.write(filepath.Join(r.zdot, ".zprofile"), "GIT_AUTHOR_NAME=NotExported\n")
	// another file in the home folder that would be read only if ZDOTDIR were ignored
	r.write(filepath.Join(r.p.Home, ".zshrc"), "export GIT_AUTHOR_NAME=WrongFile\n")

	start := time.Now()
	ov := r.overview()
	if time.Since(start) > 4*time.Second {
		t.Fatalf("a profile that starts a background program must not hold the probe: %v", time.Since(start))
	}
	if !ov.Env.Active || !ov.Env.Probed || ov.Env.Shell != "zsh" {
		t.Fatalf("env: %+v", ov.Env)
	}
	if len(ov.Env.Vars) != 1 || ov.Env.Vars[0].Name != "GIT_AUTHOR_EMAIL" || ov.Env.Vars[0].Value != "profile@example.com" {
		t.Fatalf("vars: %+v", ov.Env.Vars)
	}
	foundRC := false
	for _, f := range ov.Env.Files {
		if f.Path == filepath.Join(r.zdot, ".zshrc") {
			foundRC = true
		}
	}
	if !foundRC {
		t.Fatalf("files: %+v", ov.Env.Files)
	}
	row := r.row(ov, rec.ID)
	if row.Effective.Email != "profile@example.com" || row.Effective.EmailSource.Kind != api.IdentitySrcEnv || row.Effective.EmailSource.Var != "GIT_AUTHOR_EMAIL" {
		t.Fatalf("effective: %+v", row.Effective)
	}
	if row.Effective.EmailSource.Path != filepath.Join(r.zdot, ".zshrc") {
		t.Fatalf("the source names the file to open: %+v", row.Effective.EmailSource)
	}
	if row.Effective.Source.Kind != api.IdentitySrcEnv {
		t.Fatalf("tag: %+v", row.Effective.Source)
	}
	// the setting underneath is still global, and the name (only assigned, never exported) is not env
	if row.Setting.Email != "global@example.com" || row.Setting.Source.Kind != api.IdentitySrcGlobal || row.Effective.Name != "Alex Global" {
		t.Fatalf("setting: %+v / %+v", row.Setting, row.Effective)
	}
}

func TestBashProfileIsProbed(t *testing.T) {
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("no bash")
	}
	r := newRig(t, func(d *Deps) { d.Shell = func() string { return "/bin/bash" } })
	r.setGlobal("Alex Global", "global@example.com")
	rec := r.repo(filepath.Join(r.root, "work"), "acme_api")
	r.write(filepath.Join(r.p.Home, ".bash_profile"), "export GIT_AUTHOR_EMAIL=bash@example.com\nexport GIT_AUTHOR_NAME='Bash Alex'\n")
	row := r.row(r.overview(), rec.ID)
	if row.Effective.Email != "bash@example.com" || row.Effective.Name != "Bash Alex" || row.Effective.Source.Kind != api.IdentitySrcEnv {
		t.Fatalf("%+v", row.Effective)
	}
	env := r.eng.TerminalEnv(bg, true)
	if len(env.Files) == 0 || env.Files[0].Path != filepath.Join(r.p.Home, ".bash_profile") {
		t.Fatalf("%+v", env)
	}
}

func TestNoShellFallsBackToProcessEnvironment(t *testing.T) {
	r := newRig(t, func(d *Deps) { d.Shell = func() string { return "/nonexistent/shell" } })
	r.setGlobal("Alex Global", "global@example.com")
	t.Setenv("GIT_AUTHOR_EMAIL", "proc@example.com")
	env := r.eng.TerminalEnv(bg, true)
	if env.Probed || env.ProbeError == "" || !env.Active {
		t.Fatalf("%+v", env)
	}
}

func TestWindowsReadsTheProcessEnvironment(t *testing.T) {
	r := newRig(t, func(d *Deps) {
		d.GOOS = "windows"
		d.Env = func() []string { return []string{"GIT_AUTHOR_EMAIL=win@example.com", "PATH=x"} }
	})
	env := r.eng.TerminalEnv(bg, true)
	if !env.Probed || !env.Active || env.Vars[0].Value != "win@example.com" {
		t.Fatalf("%+v", env)
	}
}

func TestNothingSetIsNotSet(t *testing.T) {
	r := newRig(t)
	rec := r.repo(filepath.Join(r.root, "work"), "acme_api")
	ov := r.overview()
	if ov.Global.Set {
		t.Fatal("no global identity expected")
	}
	if row := r.row(ov, rec.ID); row.Effective.Set || row.Effective.Source.Kind != api.IdentitySrcNone {
		t.Fatalf("%+v", row.Effective)
	}
}

func TestValidate(t *testing.T) {
	for _, c := range []struct {
		a     api.Author
		field string
	}{
		{api.Author{Name: "", Email: "a@example.com"}, "name"},
		{api.Author{Name: "A <b>", Email: "a@example.com"}, "name"},
		{api.Author{Name: "A\nB", Email: "a@example.com"}, "name"},
		{api.Author{Name: "A", Email: "nope"}, "email"},
		{api.Author{Name: "A", Email: "a b@example.com"}, "email"},
		{api.Author{Name: "A", Email: "<a@example.com>"}, "email"},
	} {
		_, err := Validate(c.a)
		ae := api.AsAppError(err)
		if ae == nil || ae.Code != api.ErrInvalidInput || ae.Params["field"] != c.field {
			t.Errorf("%+v: %v", c.a, err)
		}
	}
	if a, err := Validate(api.Author{Name: "  Alex ", Email: " alex@acme.example.com "}); err != nil || a.Name != "Alex" || a.Email != "alex@acme.example.com" {
		t.Fatalf("%+v %v", a, err)
	}
}

func TestGlobalWriteBackupAndUndo(t *testing.T) {
	r := newRig(t)
	orig := "# keep me\n[core]\n\teditor = vim\n[credential]\n\thelper = osxkeychain\n[user]\n\tname = Old\n\temail = old@example.com\n"
	r.write(r.p.GitGlobalFile, orig)
	pv, err := r.eng.PreviewGlobal(bg, api.Author{Name: "Alex", Email: "alex@acme.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pv.Preview.Path, r.root) {
		t.Fatalf("the global file must come from internal/paths, got %s", pv.Preview.Path)
	}
	if pv.Unchanged || pv.Before == nil || pv.Before.Name != "Old" || pv.Preview.Path != r.p.GitGlobalFile || pv.Preview.BaseHash == "" {
		t.Fatalf("%+v", pv)
	}
	// a stale base hash is refused and nothing is written
	if _, err := r.eng.ApplyGlobal(bg, acme(), "deadbeef"); api.AsAppError(err).Code != api.ErrConfigChanged {
		t.Fatalf("%v", err)
	}
	if r.read(r.p.GitGlobalFile) != orig {
		t.Fatal("file touched by a refused write")
	}
	res, err := r.eng.ApplyGlobal(bg, acme(), pv.Preview.BaseHash)
	if err != nil {
		t.Fatal(err)
	}
	got := r.read(r.p.GitGlobalFile)
	if !strings.Contains(got, "name = Alex") || !strings.Contains(got, "# keep me") || !strings.Contains(got, "osxkeychain") {
		t.Fatalf("%s", got)
	}
	// the preview shows exactly the change
	lines, _, _ := textdiff.Diff([]byte(orig), []byte(got), textdiff.DefaultContext)
	if !reflect.DeepEqual(lines, pv.Preview.Lines) {
		t.Fatalf("preview != write\n%v\n%v", lines, pv.Preview.Lines)
	}
	if len(res.BackupIDs) != 1 || len(r.undoFiles(res)) != 1 {
		t.Fatalf("%+v", res)
	}
	ents, _ := r.bk.List()
	if len(ents) != 1 || ents[0].Kind != api.BackupGitGlobal || ents[0].Reason != api.ReasonIdentity || ents[0].Path != r.p.GitGlobalFile {
		t.Fatalf("%+v", ents)
	}
	if r.notes[len(r.notes)-1] != api.EventIdentityChanged {
		t.Fatal("event")
	}

	// an edit made elsewhere in the meantime blocks the undo
	r.write(r.p.GitGlobalFile, got+"[alias]\n\tco = checkout\n")
	if _, err := r.eng.Undo(bg, res.UndoID); api.AsAppError(err).Code != api.ErrConfigChanged {
		t.Fatalf("%v", err)
	}
	r.write(r.p.GitGlobalFile, got)
	if _, err := r.eng.Undo(bg, res.UndoID); err != nil {
		t.Fatal(err)
	}
	if r.read(r.p.GitGlobalFile) != orig {
		t.Fatalf("undo did not restore the file:\n%s", r.read(r.p.GitGlobalFile))
	}
}

func TestGlobalCreatedFileIsRemovedByUndo(t *testing.T) {
	r := newRig(t)
	if _, err := os.Stat(r.p.GitGlobalFile); err == nil {
		t.Fatal("fixture")
	}
	pv, err := r.eng.PreviewGlobal(bg, acme())
	if err != nil {
		t.Fatal(err)
	}
	if pv.Preview.BaseHash != "" {
		t.Fatal("a missing file has an empty hash")
	}
	res, err := r.eng.ApplyGlobal(bg, acme(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.read(r.p.GitGlobalFile), "alex@acme.example.com") {
		t.Fatal("not written")
	}
	if _, err := r.eng.Undo(bg, res.UndoID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.p.GitGlobalFile); !os.IsNotExist(err) {
		t.Fatal("the file the app created must be gone again")
	}
}

func TestValuesAreArgumentsNotShellText(t *testing.T) {
	r := newRig(t)
	canary := filepath.Join(r.root, "pwned")
	a := api.Author{Name: `-x "; touch ` + canary + `; #`, Email: "a$(touch" + canary + ")@acme.example.com"}
	pv, err := r.eng.PreviewGlobal(bg, a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.eng.ApplyGlobal(bg, a, pv.Preview.BaseHash); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(canary); err == nil {
		t.Fatal("a value ran as a command")
	}
	if got := r.gitIn(r.p.Home, "config", "--file", r.p.GitGlobalFile, "user.name"); got != a.Name {
		t.Fatalf("stored %q", got)
	}
}

func removeAll(p string) error { return os.RemoveAll(p) }

func (r *rig) snap() *projects.Data {
	d := r.proj.Snapshot()
	return &d
}
