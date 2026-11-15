package repos_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/projects"
	"github.com/rudian/codepass/internal/repos"
)

// ---- clone folder names ----

func TestFolderNameValidationTable(t *testing.T) {
	long := strings.Repeat("a", 256)
	cases := []struct {
		name, reason string // reason "" = valid
	}{
		{"api-main", ""}, {"acme_api", ""}, {"a.b", ""}, {".hidden", ""}, {"con-sole", ""}, {"console", ""}, {"nulled", ""},
		{"COM0", ""}, {"COM10", ""}, {"LPT0", ""}, {"com", ""}, {"lpt", ""}, {"données", ""}, {"仓库", ""}, {"a b", ""},
		{"", repos.FolderEmpty}, {".", repos.FolderDots}, {"..", repos.FolderDots},
		{"a/b", repos.FolderSeparator}, {`a\b`, repos.FolderSeparator}, {"/abs", repos.FolderSeparator}, {"../x", repos.FolderSeparator},
		{"CON", repos.FolderReserved}, {"con", repos.FolderReserved}, {"Prn", repos.FolderReserved}, {"AUX", repos.FolderReserved},
		{"NUL", repos.FolderReserved}, {"COM1", repos.FolderReserved}, {"com9", repos.FolderReserved}, {"LPT1", repos.FolderReserved},
		{"lpt9", repos.FolderReserved}, {"CON.txt", repos.FolderReserved}, {"nul.tar.gz", repos.FolderReserved},
		{"com1.old", repos.FolderReserved}, {"aux.", repos.FolderTrailing}, {"COM¹", repos.FolderReserved}, {"LPT²", repos.FolderReserved},
		{"CONIN$", repos.FolderReserved},
		{"api.", repos.FolderTrailing}, {"api ", repos.FolderTrailing}, {"api. ", repos.FolderTrailing}, {"x..", repos.FolderTrailing},
		{"a:b", repos.FolderBadChar}, {"a*b", repos.FolderBadChar}, {"a?b", repos.FolderBadChar}, {"a|b", repos.FolderBadChar},
		{"a<b", repos.FolderBadChar}, {"a\"b", repos.FolderBadChar}, {"a\x00b", repos.FolderBadChar}, {"a\nb", repos.FolderBadChar},
		{long, repos.FolderTooLong},
	}
	for _, c := range cases {
		err := repos.ValidateFolderName(c.name)
		switch {
		case c.reason == "" && err != nil:
			t.Errorf("%q refused: %v %v", c.name, err, err.Params)
		case c.reason != "" && err == nil:
			t.Errorf("%q accepted", c.name)
		case c.reason != "" && (err.Code != api.ErrInvalidInput || err.Params["field"] != "folderName" || err.Params["reason"] != c.reason):
			t.Errorf("%q: %+v, want reason %s", c.name, err, c.reason)
		}
	}
}

func TestCloneUnderAnotherNameKeepsBothNames(t *testing.T) {
	f := newFixture(t)
	bare := f.sb.InitBare(t, "acme_api.git")
	*f.remote = func(region, name string) string { return bare }
	outside := filepath.Join(f.sb.Root, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	req := api.CloneRequest{Profile: "initech-admin", Region: "us-east-1", RepoName: "acme_api", ParentDir: outside, FolderName: "api-main"}
	pv, err := f.eng.PreviewClone(ctx, req)
	if err != nil || pv.Dest != filepath.Join(outside, "api-main") || pv.FolderName != "api-main" || pv.Exists {
		t.Fatalf("preview: %+v %v", pv, err)
	}
	f.state()
	o, err := f.eng.Clone(ctx, req)
	if err != nil || !o.OK || o.Repo == nil {
		t.Fatalf("clone: %v %+v", err, o)
	}
	if o.Repo.LocalName != "api-main" || o.Repo.RemoteName != "acme_api" || o.Repo.Name != "acme_api" || o.Repo.Provider != api.ProviderCodeCommit {
		t.Errorf("names: local %q remote %q name %q", o.Repo.LocalName, o.Repo.RemoteName, o.Repo.Name)
	}
	// The same remote under a second name is a second repository with its own ID.
	req2 := req
	req2.FolderName = "api-review"
	o2, err := f.eng.Clone(ctx, req2)
	if err != nil || !o2.OK {
		t.Fatalf("second clone: %v", err)
	}
	st := f.state()
	if len(st.Repos) != 2 || o.Repo.ID == o2.Repo.ID {
		t.Fatalf("two clones of one remote: %+v", st.Repos)
	}
	if r := f.byName(st, "api-review"); r.LocalName != "api-review" || r.RemoteName != "acme_api" {
		t.Errorf("second: %+v", r)
	}
	// An empty folder name means the remote's name; a name that exists is refused; bad names are refused with a reason.
	req3 := req
	req3.FolderName = ""
	pv3, err := f.eng.PreviewClone(ctx, req3)
	if err != nil || pv3.FolderName != "acme_api" || pv3.Dest != filepath.Join(outside, "acme_api") {
		t.Fatalf("default name: %+v %v", pv3, err)
	}
	if _, err := f.eng.Clone(ctx, req); err == nil || api.AsAppError(err).Code != api.ErrDestExists {
		t.Errorf("existing destination: %v", err)
	}
	for name, reason := range map[string]string{"CON": repos.FolderReserved, "nul.txt": repos.FolderReserved, "x.": repos.FolderTrailing, "a/b": repos.FolderSeparator} {
		bad := req
		bad.FolderName = name
		_, err := f.eng.Clone(ctx, bad)
		if ae := api.AsAppError(err); err == nil || ae.Code != api.ErrInvalidInput || ae.Params["reason"] != reason {
			t.Errorf("%q: %v", name, err)
		}
		if _, err := f.eng.PreviewClone(ctx, bad); err == nil {
			t.Errorf("preview accepted %q", name)
		}
		if _, serr := os.Lstat(filepath.Join(outside, name)); serr == nil {
			t.Errorf("git ran for %q", name)
		}
	}
}

// ---- the moved-folder question reaches the row ----

func TestMovedQuestionIsOnTheRowAndAnswersMapToErrorCodes(t *testing.T) {
	f := newFixture(t)
	dir := f.newRepo("acme_api", ccURL("acme_api"))
	f.sb.CommitFile(t, dir, "README.md", "hi\n")
	ctx := context.Background()
	st := f.state()
	oldID := st.Repos[0].ID
	if st.Repos[0].MovedFrom != nil {
		t.Fatal("question without a move")
	}
	moved := filepath.Join(f.code, "api-main")
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	st = f.scan()
	if len(st.Repos) != 1 {
		t.Fatalf("%+v", st.Repos)
	}
	row := st.Repos[0]
	if row.MovedFrom == nil || row.MovedFrom.OldID != oldID || row.MovedFrom.Name != "acme_api" || row.LocalName != "api-main" || row.RemoteName != "acme_api" {
		t.Fatalf("row: %+v", row)
	}
	// FU-18a: the person puts the found folder into a project before answering. The question goes away.
	if err := f.reg.Mutate(func(d *projects.Data) error {
		d.Projects = append(d.Projects, projects.Project{ID: "p_1", Name: "acme"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	st, err := f.eng.MoveToProject(ctx, []string{row.ID}, "p_1")
	if err != nil {
		t.Fatal(err)
	}
	if st.Repos[0].MovedFrom != nil || len(f.eng.MovedOffers()) != 0 || st.Repos[0].ProjectID != "p_1" {
		t.Fatalf("question stays after the found folder got a project: %+v", st.Repos[0])
	}
	if _, err := f.eng.ReattachMoved(ctx, row.ID); err == nil {
		t.Fatal("answer accepted")
	}
	// Even a stale answer (the data arrived between the scan and the click) maps to its own code, not E_INTERNAL.
	f2 := newFixture(t)
	d2 := f2.newRepo("acme_api", ccURL("acme_api"))
	f2.sb.CommitFile(t, d2, "README.md", "hi\n")
	f2.state()
	if err := os.Rename(d2, filepath.Join(f2.code, "api-main")); err != nil {
		t.Fatal(err)
	}
	st2 := f2.scan()
	newID := st2.Repos[0].ID
	if err := f2.reg.SetHidden(newID, true); err != nil { // data set behind the engine's back
		t.Fatal(err)
	}
	if _, err := f2.eng.ReattachMoved(ctx, newID); err == nil || api.AsAppError(err).Code != api.ErrMovedHasData {
		t.Fatalf("has data: %v", err)
	}
	if _, err := f2.eng.ReattachMoved(ctx, newID); err == nil || api.AsAppError(err).Code != api.ErrMovedGone {
		t.Fatalf("answered twice: %v", err)
	}
}

func TestMovedQuestionOldFolderBackIsGone(t *testing.T) {
	f := newFixture(t)
	dir := f.newRepo("acme_api", ccURL("acme_api"))
	f.sb.CommitFile(t, dir, "README.md", "hi\n")
	f.state()
	moved := filepath.Join(f.code, "api-main")
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	st := f.scan()
	newID := st.Repos[0].ID
	// The old folder comes back (a copy) between the question and the answer.
	f.git(f.code, "clone", "-q", moved, dir)
	f.git(dir, "remote", "set-url", "origin", ccURL("acme_api"))
	if _, _, err := f.reg.Reconcile(projects.ProbePath, f.clock); err != nil {
		t.Fatal(err)
	}
	_, err := f.eng.ReattachMoved(context.Background(), newID)
	if ae := api.AsAppError(err); err == nil || ae.Code != api.ErrMovedGone {
		t.Fatalf("%v", err)
	}
}

// A rescan between the question and the answer drops the offer: both answers are "out of date", an unknown ID
// stays "not found".
func TestMovedAnswerAfterRescanIsGone(t *testing.T) {
	f := newFixture(t)
	dir := f.newRepo("acme_api", ccURL("acme_api"))
	f.sb.CommitFile(t, dir, "README.md", "hi\n")
	f.state()
	moved := filepath.Join(f.code, "api-main")
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	st := f.scan()
	newID := st.Repos[0].ID
	f.git(f.code, "clone", "-q", moved, dir)
	f.git(dir, "remote", "set-url", "origin", ccURL("acme_api"))
	f.scan()
	ctx := context.Background()
	if _, err := f.eng.ReattachMoved(ctx, newID); api.AsAppError(err).Code != api.ErrMovedGone {
		t.Fatalf("yes: %v", err)
	}
	if _, err := f.eng.DeclineMoved(ctx, newID); api.AsAppError(err).Code != api.ErrMovedGone {
		t.Fatalf("no: %v", err)
	}
	if _, err := f.eng.ReattachMoved(ctx, "r_nope"); api.AsAppError(err).Code != api.ErrRepoNotFound {
		t.Fatalf("unknown: %v", err)
	}
}

// ---- FU-18b, FU-18c ----

func TestRegistryNotWritableGivesWarningAndFileError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not apply")
	}
	f := newFixture(t)
	f.newRepo("acme_api", ccURL("acme_api"))
	data := t.TempDir()
	eng, _, _ := engineWith(t, f, data)
	ctx := context.Background()
	if st, err := eng.Scan(ctx); err != nil || len(st.Repos) != 1 || len(st.Warnings) != 0 {
		t.Fatalf("%v %+v", err, st.Warnings)
	}
	// Two new folders, one of each kind, while projects.json cannot be written.
	newCC := f.newRepo("acme_web", ccURL("acme_web"))
	f.sb.InitRepo(t, "code/gh", map[string]string{"origin": ghURL})
	if err := os.Chmod(data, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(data, 0o755)
	st, err := eng.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Repos) != 1 || len(st.Warnings) != 1 || st.Warnings[0].Code != api.WarnProjectsNotWritable ||
		st.Warnings[0].Count != 2 || st.Warnings[0].File != "projects.json" {
		t.Fatalf("repos %d warnings %+v", len(st.Repos), st.Warnings)
	}
	// FU-18b: adding the new CodeCommit folder by hand names the file instead of "repository not found".
	_, err = eng.AddExisting(ctx, newCC)
	if ae := api.AsAppError(err); err == nil || ae.Code != api.ErrFileIO || ae.Params["file"] != "projects.json" {
		t.Fatalf("AddExisting: %v", err)
	}
	_, err = eng.AddFolder(ctx, filepath.Join(f.code, "gh"))
	if ae := api.AsAppError(err); err == nil || ae.Code != api.ErrFileIO || ae.Params["file"] != "projects.json" {
		t.Fatalf("AddFolder: %v", err)
	}
	// Writable again: the warning goes away with the next scan.
	_ = os.Chmod(data, 0o755)
	if st, err = eng.Scan(ctx); err != nil || len(st.Warnings) != 0 || len(st.Repos) != 2 || len(st.Found) != 1 {
		t.Fatalf("%v warnings %+v repos %d found %d", err, st.Warnings, len(st.Repos), len(st.Found))
	}
}

// ---- projects and suggestions ----

func TestProjectsSuggestionsAndMoves(t *testing.T) {
	f := newFixture(t)
	for _, n := range []string{"initech_api", "initech_web", "initech_docs", "acme-one", "acme-two"} {
		f.newRepo(n, ccURL(n))
	}
	f.sb.InitRepo(t, "code/team/x1", map[string]string{"origin": ghURL})
	f.sb.InitRepo(t, "code/team/x2", map[string]string{"origin": ghURL})
	f.sb.InitRepo(t, "code/team/x3", map[string]string{"origin": ghURL})
	f.state()
	st := f.reviewAll()
	if len(st.Suggestions) != 2 {
		t.Fatalf("suggestions: %+v", st.Suggestions)
	}
	s0 := st.Suggestions[0]
	if s0.Kind != api.SuggestPrefix || s0.Label != "initech" || len(s0.RepoIDs) != 3 || s0.Key != "prefix:initech" {
		t.Errorf("prefix suggestion: %+v", s0)
	}
	if s1 := st.Suggestions[1]; s1.Kind != api.SuggestFolder || s1.Label != "team" || len(s1.RepoIDs) != 3 {
		t.Errorf("folder suggestion: %+v", s1)
	}
	ctx := context.Background()
	// Dismissed suggestions do not return, also after a restart.
	if st, err := f.eng.DismissSuggestion(ctx, s0.Key); err != nil || len(st.Suggestions) != 1 {
		t.Fatalf("dismiss: %v %+v", err, st.Suggestions)
	}
	if st := f.scan(); len(st.Suggestions) != 1 {
		t.Errorf("dismissed suggestion came back: %+v", st.Suggestions)
	}
	if st := f.engineOn(f.reg, nil); st != nil {
		s, _ := st.Scan(ctx)
		if len(s.Suggestions) != 1 {
			t.Errorf("after restart: %+v", s.Suggestions)
		}
	}
	// Accepting the folder suggestion: a project holds the three.
	st, err := f.eng.CreateProjectWith(ctx, " team ", st.Suggestions[1].RepoIDs)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Projects) != 1 || st.Projects[0].Name != "team" || st.Projects[0].RepoCount != 3 || len(st.Suggestions) != 0 {
		t.Fatalf("projects: %+v suggestions %+v", st.Projects, st.Suggestions)
	}
	pid := st.Projects[0].ID
	x1 := f.byName(st, "x1")
	if x1.ProjectID != pid {
		t.Errorf("x1: %+v", x1)
	}
	// Move one out to a second project, then to none; rename; collapse; remove.
	st, err = f.eng.CreateProject(ctx, "later")
	if err != nil || len(st.Projects) != 2 {
		t.Fatal(err, st.Projects)
	}
	later := st.Projects[1].ID
	if st, err = f.eng.MoveToProject(ctx, []string{x1.ID}, later); err != nil || f.byName(st, "x1").ProjectID != later {
		t.Fatalf("move: %v", err)
	}
	if st, err = f.eng.MoveToProject(ctx, []string{x1.ID}, ""); err != nil || f.byName(st, "x1").ProjectID != "" {
		t.Fatalf("ungroup: %v", err)
	}
	if st, err = f.eng.RenameProject(ctx, later, "Later"); err != nil || st.Projects[1].Name != "Later" {
		t.Fatalf("rename: %v %+v", err, st.Projects)
	}
	if st, err = f.eng.SetProjectCollapsed(ctx, later, true); err != nil || !st.Projects[1].Collapsed {
		t.Fatalf("collapse: %v", err)
	}
	if st, err = f.eng.RemoveProject(ctx, pid); err != nil || len(st.Projects) != 1 || f.byName(st, "x2").ProjectID != "" {
		t.Fatalf("remove: %v", err)
	}
	for _, e := range []error{
		func() error { _, err := f.eng.MoveToProject(ctx, []string{x1.ID}, "p_nope"); return err }(),
		func() error { _, err := f.eng.RenameProject(ctx, "p_nope", "x"); return err }(),
		func() error { _, err := f.eng.RemoveProject(ctx, "p_nope"); return err }(),
	} {
		if e == nil || api.AsAppError(e).Code != api.ErrProjectNotFound {
			t.Errorf("unknown project: %v", e)
		}
	}
	if _, err := f.eng.MoveToProject(ctx, []string{"r_nope"}, later); err == nil || api.AsAppError(err).Code != api.ErrRepoNotFound {
		t.Errorf("unknown repository: %v", err)
	}
	if _, err := f.eng.CreateProject(ctx, "  "); err == nil || api.AsAppError(err).Code != api.ErrInvalidInput {
		t.Errorf("empty name: %v", err)
	}
	// A project's repositories leave the count when hidden.
	if st, err = f.eng.MoveToProject(ctx, []string{x1.ID}, later); err != nil || st.Projects[0].RepoCount != 1 {
		t.Fatal(err)
	}
	if st, err = f.eng.SetHidden(ctx, x1.ID, true); err != nil || st.Projects[0].RepoCount != 0 {
		t.Fatalf("hidden member counted: %+v", st.Projects)
	}
}

// ---- add by hand ----

func TestAddFolderListsAnyRepository(t *testing.T) {
	f := newFixture(t)
	f.newRepo("acme_api", ccURL("acme_api"))
	outside := f.sb.InitRepo(t, "elsewhere/tool", map[string]string{"origin": "https://git.example.com/team/tool.git"})
	ctx := context.Background()
	f.state()
	res, err := f.eng.AddFolder(ctx, outside)
	if err != nil || !res.Added || res.RepoID == "" || len(res.State.Repos) != 2 || len(res.State.Found) != 0 {
		t.Fatalf("%v %+v", err, res)
	}
	var row api.Repo
	for _, r := range res.State.Repos {
		if r.ID == res.RepoID {
			row = r
		}
	}
	if row.LocalName != "tool" || row.RemoteName != "tool" || row.Provider != api.ProviderOther || !row.Extra || row.Binding.Source != api.BindingMachine {
		t.Errorf("row: %+v", row)
	}
	if again, err := f.eng.AddFolder(ctx, outside); err != nil || again.Added || again.RepoID != res.RepoID {
		t.Errorf("second add: %v %+v", err, again)
	}
	// A hidden repository comes back when it is added by hand.
	if _, err := f.eng.SetHidden(ctx, res.RepoID, true); err != nil {
		t.Fatal(err)
	}
	if back, err := f.eng.AddFolder(ctx, outside); err != nil || len(back.State.Repos) != 2 {
		t.Fatalf("%v %+v", err, back.State.Counts)
	}
	// A CodeCommit folder goes through the old path (account detection, binding).
	cc := f.sb.InitRepo(t, "elsewhere/ccrepo", map[string]string{"origin": ccURL("cc_outside")})
	if r, err := f.eng.AddFolder(ctx, cc); err != nil || !r.Added {
		t.Fatalf("%v %+v", err, r)
	}
	if _, err := f.eng.AddFolder(ctx, filepath.Join(f.sb.Root, "nothing")); err == nil || api.AsAppError(err).Code != api.ErrRepoNotFound {
		t.Errorf("not a repository: %v", err)
	}
}

// ---- status ----

func TestStatusFieldsAndStates(t *testing.T) {
	f := newFixture(t)
	dirty := f.newRepo("dirty", ccURL("dirty_repo"))
	f.sb.CommitFile(t, dirty, "a.txt", "one\n")
	appendFile(t, filepath.Join(dirty, "a.txt"), "two\n")
	if err := os.WriteFile(filepath.Join(dirty, "new.txt"), []byte("n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.newRepo("unborn", ccURL("unborn_repo"))
	ctx := context.Background()
	f.state()
	st, err := f.eng.RefreshStatus(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	d := f.byName(st, "dirty").Status
	if d == nil || d.Branch != "main" || d.Unborn || d.Detached || d.Unstaged != 1 || d.Untracked != 1 || d.Changes != 2 || d.SyncKnown || d.RenamesSkipped || d.At == "" {
		t.Errorf("dirty: %+v", d)
	}
	if u := f.byName(st, "unborn").Status; u == nil || !u.Unborn || u.Changes != 0 {
		t.Errorf("unborn: %+v", u)
	}
	// Only the listed IDs are read; unknown IDs are ignored.
	appendFile(t, filepath.Join(dirty, "a.txt"), "three\n")
	if err := os.WriteFile(filepath.Join(dirty, "new2.txt"), []byte("n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if st, err = f.eng.RefreshStatus(ctx, []string{"r_nope"}); err != nil || f.byName(st, "dirty").Status.Untracked != 1 {
		t.Fatalf("%v", err)
	}
	if st, err = f.eng.RefreshStatus(ctx, []string{f.byName(st, "dirty").ID}); err != nil || f.byName(st, "dirty").Status.Untracked != 2 {
		t.Fatalf("%v", err)
	}
}
