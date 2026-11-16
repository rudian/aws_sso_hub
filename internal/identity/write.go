package identity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/backup"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/projects"
	"github.com/rudian/codepass/internal/redact"
	"github.com/rudian/codepass/internal/textdiff"
)

var emailRE = regexp.MustCompile(`^[^\s@<>]+@[^\s@<>]+$`)

// Validate checks and trims an author. Errors are E_INVALID_INPUT with field "name" or "email".
func Validate(a api.Author) (api.Author, error) {
	a.Name, a.Email = strings.TrimSpace(a.Name), strings.TrimSpace(a.Email)
	bad := func(field string) error {
		return api.NewError(api.ErrInvalidInput, "invalid author %s", field).WithParam("field", field)
	}
	if a.Name == "" || len(a.Name) > 200 || strings.ContainsAny(a.Name, "<>") || strings.IndexFunc(a.Name, unicode.IsControl) >= 0 {
		return a, bad("name")
	}
	if len(a.Email) > 254 || !emailRE.MatchString(a.Email) || strings.IndexFunc(a.Email, unicode.IsControl) >= 0 {
		return a, bad("email")
	}
	return a, nil
}

func sameAuthor(a, b *api.Author) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Name == b.Name && a.Email == b.Email
}

func fromProjects(i *projects.Identity) *api.Author {
	if i == nil {
		return nil
	}
	return &api.Author{Name: i.Name, Email: i.Email}
}

func copyAuthor(a *api.Author) *api.Author {
	if a == nil {
		return nil
	}
	c := *a
	return &c
}

// op is one change of a config file: set user.name and user.email, or remove them.
type op struct{ set *api.Author }

// runOp applies o to file with `git config --file <file>` (argument lists only, no shell). wrote reports
// whether at least one git call went through, so a caller knows whether the file may have changed.
func runOp(ctx context.Context, g *gitx.Git, file string, o op) (wrote bool, err error) {
	run := func(args ...string) error {
		res, err := g.Run(ctx, "", nil, append([]string{"config", "--file", file}, args...)...)
		if err != nil {
			return err
		}
		if res.ExitCode != 0 && !(res.ExitCode == 5 && len(args) > 0 && args[0] == "--unset-all") {
			return api.NewError(api.ErrGitFailed, "git config failed (exit %d)", res.ExitCode).WithDetail(redact.Tail(string(res.Stderr), 3))
		}
		wrote = true
		return nil
	}
	if o.set != nil {
		if err := run("--replace-all", "user.name", o.set.Name); err != nil {
			return wrote, err
		}
		return wrote, run("--replace-all", "user.email", o.set.Email)
	}
	if err := run("--unset-all", "user.name"); err != nil {
		return wrote, err
	}
	return wrote, run("--unset-all", "user.email")
}

// simulate applies o to a temporary copy of file and returns the file before and after. The same runOp that
// the real write uses runs on the copy, so a preview is what a write produces.
func (e *Engine) simulate(ctx context.Context, g *gitx.Git, file string, o op) (before []byte, existed bool, after []byte, err error) {
	before, existed, err = readFile(file)
	if err != nil {
		return nil, false, nil, err
	}
	tmp, err := os.MkdirTemp("", "codepass-ident-")
	if err != nil {
		return nil, false, nil, api.AsAppError(err)
	}
	defer os.RemoveAll(tmp)
	sim := filepath.Join(tmp, "config")
	if err := os.WriteFile(sim, before, 0o600); err != nil {
		return nil, false, nil, api.AsAppError(err)
	}
	if _, err := runOp(ctx, g, sim, o); err != nil {
		return nil, false, nil, api.AsAppError(err)
	}
	after, _, err = readFile(sim)
	return before, existed, after, err
}

func (e *Engine) filePreview(path string, before []byte, existed bool, after []byte) api.FilePreview {
	lines, added, removed := textdiff.Diff([]byte(redact.String(string(before))), []byte(redact.String(string(after))), textdiff.DefaultContext)
	return api.FilePreview{
		File: e.d.Paths.Display(path), Path: path, BaseHash: hashOf(before, existed), Lines: lines, Added: added, Removed: removed,
		BackupDir: e.d.Paths.Display(e.d.Paths.BackupDir),
	}
}

// item is one file write of a plan.
type item struct {
	plan     api.RepoChangePlan
	rec      projects.RepoRecord
	path     string // config file
	kind     api.BackupKind
	op       *op // nil = nothing to write
	before   []byte
	existed  bool
	after    []byte
	baseHash string
	err      *api.AppError // set with RepoActionError: nothing is written for this item
}

func (it *item) afterHash() string { return hashOf(it.after, true) }

// planFile fills the item's simulation. o nil = no write.
func (e *Engine) planFile(ctx context.Context, g *gitx.Git, it *item, o *op) error {
	b, ex, err := readFile(it.path)
	if err != nil {
		return err
	}
	it.before, it.existed, it.baseHash = b, ex, hashOf(b, ex)
	it.after = b
	if o == nil {
		return nil
	}
	_, _, after, err := e.simulate(ctx, g, it.path, *o)
	if err != nil {
		return err
	}
	if string(after) == string(b) && ex {
		return nil // already as wanted
	}
	it.op, it.after = o, after
	pv := e.filePreview(it.path, b, ex, after)
	it.plan.Preview = &pv
	return nil
}

// writeItem applies one planned write: hash check, backup, git config --file. On failure the file is put
// back from the backup.
func (e *Engine) writeItem(ctx context.Context, g *gitx.Git, it *item, params map[string]string) (undoFile, string, error) {
	display := e.d.Paths.Display(it.path)
	cur, ex, err := readFile(it.path)
	if err != nil {
		return undoFile{}, "", err
	}
	if hashOf(cur, ex) != it.baseHash {
		return undoFile{}, "", api.NewError(api.ErrConfigChanged, "a git config file changed since the preview").WithParam("file", display)
	}
	var entry api.BackupEntry
	if e.d.Backup != nil {
		entry, err = e.d.Backup.Snapshot(it.path, display, it.kind, api.ReasonIdentity, params)
		if err != nil {
			return undoFile{}, "", api.AsAppError(err)
		}
	}
	if wrote, err := runOp(ctx, g, it.path, *it.op); err != nil {
		// Put the file back only after a partial write. When the first call failed (a lock held by another
		// git, say), the file is not ours to touch: restoring would revert that writer's change.
		if wrote {
			if entry.ID != "" {
				_ = e.restoreFrom(entry.ID, it.path)
			} else if !ex {
				_ = os.Remove(it.path)
			}
		}
		return undoFile{}, "", api.NewError(api.ErrFileIO, "cannot write a git config file").WithParam("file", display).WithDetail(api.AsAppError(err).Message)
	}
	after, _, _ := readFile(it.path)
	if it.kind == api.BackupGitGlobal && e.d.OnWrite != nil {
		e.d.OnWrite(it.path, after)
	}
	return undoFile{path: it.path, file: display, backupID: entry.ID, existed: ex, afterHash: sha(after)}, entry.ID, nil
}

// restoreFrom puts the backup's content back, keeping the secret values that the backup holds as
// placeholders from the current file.
func (e *Engine) restoreFrom(backupID, path string) error {
	_, content, err := e.d.Backup.Get(backupID)
	if err != nil {
		return err
	}
	cur, _, _ := readFile(path)
	merged, _, err := backup.MergeRedacted(content, cur)
	if err != nil {
		return err
	}
	if err := backup.WriteFileAtomic(path, merged, 0o600); err != nil {
		return api.NewError(api.ErrFileIO, "cannot write a git config file").WithParam("file", e.d.Paths.Display(path)).WithDetail(err.Error())
	}
	return nil
}

func planHash(parts ...string) string {
	return sha([]byte(strings.Join(parts, "\x00")))
}

func authorKey(a *api.Author) string {
	if a == nil {
		return "-"
	}
	return a.Name + "\x01" + a.Email
}

func (it *item) key() string {
	code := ""
	if it.err != nil {
		code = string(it.err.Code)
	}
	return strings.Join([]string{code, it.plan.RepoID, string(it.plan.Action), it.path, it.baseHash, hashOf(it.after, it.op != nil)}, "|")
}

// ---- global ----

// PreviewGlobal previews saving the global identity.
func (e *Engine) PreviewGlobal(ctx context.Context, a api.Author) (api.GlobalIdentityPreview, error) {
	a, err := Validate(a)
	if err != nil {
		return api.GlobalIdentityPreview{}, err
	}
	g, err := e.git(ctx)
	if err != nil {
		return api.GlobalIdentityPreview{}, err
	}
	gl, err := e.globalView(ctx, g)
	if err != nil {
		return api.GlobalIdentityPreview{}, err
	}
	before, existed, after, err := e.simulate(ctx, g, gl.Path, op{set: &a})
	if err != nil {
		return api.GlobalIdentityPreview{}, err
	}
	out := api.GlobalIdentityPreview{Preview: e.filePreview(gl.Path, before, existed, after), After: a, Unchanged: string(before) == string(after) && existed}
	if gl.Set {
		out.Before = &api.Author{Name: gl.Name, Email: gl.Email}
	}
	return out, nil
}

// ApplyGlobal writes the global identity (backup first). baseHash is the preview's.
func (e *Engine) ApplyGlobal(ctx context.Context, a api.Author, baseHash string) (api.IdentityChangeResult, error) {
	a, err := Validate(a)
	if err != nil {
		return api.IdentityChangeResult{}, err
	}
	g, err := e.git(ctx)
	if err != nil {
		return api.IdentityChangeResult{}, err
	}
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	path := e.globalPath(g)
	cur, ex, err := readFile(path)
	if err != nil {
		return api.IdentityChangeResult{}, err
	}
	if hashOf(cur, ex) != baseHash {
		return api.IdentityChangeResult{}, api.NewError(api.ErrConfigChanged, "the global git config changed").WithParam("file", e.d.Paths.Display(path))
	}
	it := &item{path: path, kind: api.BackupGitGlobal, op: &op{set: &a}, baseHash: baseHash}
	res := api.IdentityChangeResult{Outcomes: []api.RepoChangeOutcome{}, BackupIDs: []string{}}
	var undo undoState
	uf, bid, err := e.writeItem(ctx, g, it, map[string]string{})
	if err != nil {
		return api.IdentityChangeResult{}, err
	}
	undo.files = append(undo.files, uf)
	if bid != "" {
		res.BackupIDs = append(res.BackupIDs, bid)
	}
	res.RecordUpdated = true
	res.UndoID = e.remember(undo)
	e.emit()
	return res, nil
}

// ---- repositories ----

// followAction decides what a repository whose local config holds local does when the identity it follows
// changes from prev to next. A local value that is neither prev nor next was set outside the app and stays.
func followAction(local, prev, next *api.Author) (api.RepoAction, *api.Author) {
	switch {
	case local == nil && next == nil:
		return api.RepoActionUnchanged, nil
	case local == nil:
		return api.RepoActionSet, next
	case next != nil && sameAuthor(local, next):
		return api.RepoActionUnchanged, local
	case prev != nil && sameAuthor(local, prev):
		if next != nil {
			return api.RepoActionSet, next
		}
		return api.RepoActionRemove, nil
	}
	return api.RepoActionKeepExternal, local
}

// repoInfo is the repository's current local state.
type repoInfo struct {
	rec     projects.RepoRecord
	name    string
	cfgPath string
	local   *api.Author
	missing bool
}

func (e *Engine) repoInfo(ctx context.Context, g *gitx.Git, rec projects.RepoRecord) (repoInfo, error) {
	ri := repoInfo{rec: rec, name: filepath.Base(rec.Path)}
	if rec.MissingSince != nil || !isDir(rec.Path) {
		ri.missing = true
		return ri, nil
	}
	cfg, err := e.configPath(ctx, g, rec.Path)
	if err != nil {
		return ri, e.repoReadError(rec, "", err)
	}
	entries, err := gitx.NewReader(g).ConfigList(ctx, rec.Path)
	if err != nil {
		return ri, e.repoReadError(rec, cfg, err)
	}
	ri.cfgPath, ri.local = cfg, localAuthor(absOrigins(entries, rec.Path), cfg)
	return ri, nil
}

// repoReadError classifies a failed read of a repository's config. E_REPO_NOT_FOUND is only for "not a git
// repository"; every other failure is E_GIT_FAILED (or E_FILE_IO when the error already says so) with the
// config's display path, so the popover can say "cannot write this repository's config" (design 5.12).
// Cancellation passes through unchanged.
func (e *Engine) repoReadError(rec projects.RepoRecord, cfg string, cause error) error {
	var ae *api.AppError
	if !errors.As(cause, &ae) {
		return cause // context cancellation and the like
	}
	if strings.Contains(strings.ToLower(ae.Detail+" "+ae.Message), "not a git repository") {
		return api.NewError(api.ErrRepoNotFound, "not a git repository").WithParam("path", e.d.Paths.Display(rec.Path)).WithDetail(ae.Message)
	}
	file := e.failedFile(rec, cfg, ae.Detail+"\n"+ae.Message)
	if ae.Code == api.ErrFileIO {
		if f := ae.Params["file"]; f != "" {
			file = e.d.Paths.Display(f)
		}
		out := api.NewError(api.ErrFileIO, "cannot read this repository's config")
		if file != "" {
			out = out.WithParam("file", file)
		}
		return out.WithDetail(ae.Message)
	}
	out := api.NewError(api.ErrGitFailed, "cannot read this repository's config")
	if file != "" {
		out = out.WithParam("file", file)
	}
	if ae.Detail != "" {
		return out.WithDetail(ae.Detail)
	}
	return out.WithDetail(ae.Message)
}

// gitFileInMessage matches the file git names in a config error: "bad config line 1 in file /x/.gitconfig" and
// "bad config variable 'a.b' in file '/x/config' at line 3".
var gitFileInMessage = regexp.MustCompile(`(?m)\bin file '?([^'\n]+?)'?(?: at line \d+)?\s*$`)

// failedFile is the display path of the file a failed config read is about: the file git names in its message
// (it may be ~/.gitconfig, not the repository's own config), else the repository's config. It is "" when that
// config is unknown and .git is a file (a linked worktree or a separate git dir).
func (e *Engine) failedFile(rec projects.RepoRecord, cfg, msg string) string {
	if m := gitFileInMessage.FindStringSubmatch(msg); m != nil {
		f := strings.TrimSpace(m[1])
		if !filepath.IsAbs(f) {
			f = filepath.Join(rec.Path, f) // git names a repository's own config relative to where it ran
		}
		return e.d.Paths.Display(f)
	}
	if cfg != "" {
		return e.d.Paths.Display(cfg)
	}
	if st, err := os.Stat(filepath.Join(rec.Path, ".git")); err == nil && st.IsDir() {
		return e.d.Paths.Display(filepath.Join(rec.Path, ".git", "config"))
	}
	return ""
}

// errorItem is the plan item of a repository whose folder exists but whose config cannot be read: it is
// listed, nothing is written for it, and an apply reports it as failed.
func (e *Engine) errorItem(rec projects.RepoRecord, cause error) *item {
	file := e.d.Paths.Display(filepath.Join(rec.Path, ".git", "config"))
	ae := api.AsAppError(cause)
	code := api.ErrGitFailed
	if ae != nil && ae.Code == api.ErrFileIO {
		code = api.ErrFileIO
		if f := ae.Params["file"]; f != "" {
			file = e.d.Paths.Display(f)
		}
	}
	perr := api.NewError(code, "cannot read this repository's config").WithParam("file", file)
	if ae != nil {
		perr = perr.WithDetail(ae.Message)
	}
	it := &item{rec: rec, kind: api.BackupGitLocal, err: perr}
	it.plan = api.RepoChangePlan{RepoID: rec.ID, Name: filepath.Base(rec.Path), Path: rec.Path, Action: api.RepoActionError, Error: perr}
	return it
}

// newItem builds the plan item of one repository for a wanted action.
func (e *Engine) newItem(ctx context.Context, g *gitx.Git, ri repoInfo, action api.RepoAction, after *api.Author) (*item, error) {
	it := &item{rec: ri.rec, path: ri.cfgPath, kind: api.BackupGitLocal}
	it.plan = api.RepoChangePlan{RepoID: ri.rec.ID, Name: ri.name, Path: ri.rec.Path, Action: action, Before: copyAuthor(ri.local), After: copyAuthor(after)}
	if ri.missing {
		it.plan.Action, it.plan.After = api.RepoActionSkipMissing, nil
		return it, nil
	}
	var o *op
	switch action {
	case api.RepoActionSet:
		o = &op{set: after}
	case api.RepoActionRemove:
		o = &op{}
	}
	if err := e.planFile(ctx, g, it, o); err != nil {
		return nil, err
	}
	if o != nil && it.op == nil { // the file already says it
		it.plan.Action = api.RepoActionUnchanged
	}
	if it.plan.Action == api.RepoActionKeepOwn || it.plan.Action == api.RepoActionKeepExternal || it.plan.Action == api.RepoActionUnchanged {
		it.plan.After = copyAuthor(ri.local)
	}
	return it, nil
}

func (e *Engine) repoRecord(rec projects.Data, id string) (projects.RepoRecord, *projects.Project, error) {
	r := rec.FindRepo(id)
	if r == nil {
		return projects.RepoRecord{}, nil, api.NewError(api.ErrInvalidInput, "unknown repository").WithParam("field", "repoId")
	}
	var p *projects.Project
	if r.ProjectID != "" {
		p = rec.FindProject(r.ProjectID)
	}
	return *r, p, nil
}

type repoPlan struct {
	it       *item
	override *api.Author // the record's override afterwards
	mode     api.RepoIdentityMode
	hash     string
}

func (e *Engine) planRepo(ctx context.Context, g *gitx.Git, req api.RepoIdentityRequest) (*repoPlan, error) {
	data := e.d.Projects.Snapshot()
	rec, proj, err := e.repoRecord(data, req.RepoID)
	if err != nil {
		return nil, err
	}
	ri, err := e.repoInfo(ctx, g, rec)
	if err != nil {
		return nil, err
	}
	rp := &repoPlan{mode: req.Mode}
	var action api.RepoAction
	var after *api.Author
	switch req.Mode {
	case api.RepoModeOwn:
		if req.Identity == nil {
			return nil, api.NewError(api.ErrInvalidInput, "an identity is required").WithParam("field", "identity")
		}
		a, err := Validate(*req.Identity)
		if err != nil {
			return nil, err
		}
		rp.override, after, action = &a, &a, api.RepoActionSet
	case api.RepoModeProject:
		var want *api.Author
		if proj != nil {
			want = fromProjects(proj.Identity)
		}
		switch {
		case want != nil:
			action, after = api.RepoActionSet, want
		case ri.local != nil:
			action = api.RepoActionRemove
		default:
			action = api.RepoActionUnchanged
		}
	case api.RepoModeGlobal:
		// No local values, and an empty override that marks the choice: project changes leave it alone.
		rp.override = &api.Author{}
		if ri.local != nil {
			action = api.RepoActionRemove
		} else {
			action = api.RepoActionUnchanged
		}
	default:
		return nil, api.NewError(api.ErrInvalidInput, "unknown mode").WithParam("field", "mode")
	}
	rp.it, err = e.newItem(ctx, g, ri, action, after)
	if err != nil {
		return nil, err
	}
	rp.hash = planHash("repo", string(req.Mode), rec.ID, authorKey(fromProjects(rec.IdentityOverride)), authorKey(rp.override), rp.it.key())
	return rp, nil
}

// PreviewRepo previews a repository change.
func (e *Engine) PreviewRepo(ctx context.Context, req api.RepoIdentityRequest) (api.RepoIdentityPreview, error) {
	g, err := e.git(ctx)
	if err != nil {
		return api.RepoIdentityPreview{}, err
	}
	rp, err := e.planRepo(ctx, g, req)
	if err != nil {
		return api.RepoIdentityPreview{}, err
	}
	return api.RepoIdentityPreview{RepoID: req.RepoID, Mode: req.Mode, Plan: rp.it.plan, PlanHash: rp.hash}, nil
}

// ApplyRepo applies a repository change. planHash is the preview's.
func (e *Engine) ApplyRepo(ctx context.Context, req api.RepoIdentityRequest, planHash string) (api.IdentityChangeResult, error) {
	g, err := e.git(ctx)
	if err != nil {
		return api.IdentityChangeResult{}, err
	}
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	rp, err := e.planRepo(ctx, g, req)
	if err != nil {
		return api.IdentityChangeResult{}, err
	}
	if rp.hash != planHash {
		return api.IdentityChangeResult{}, api.NewError(api.ErrConfigChanged, "the repository changed since the preview").WithParam("file", e.d.Paths.Display(rp.it.path))
	}
	res := api.IdentityChangeResult{Outcomes: []api.RepoChangeOutcome{}, BackupIDs: []string{}}
	var undo undoState
	oc := api.RepoChangeOutcome{RepoID: rp.it.plan.RepoID, Name: rp.it.plan.Name, Action: rp.it.plan.Action, OK: true}
	if rp.it.op != nil {
		uf, bid, err := e.writeItem(ctx, g, rp.it, map[string]string{"repo": rp.it.plan.Name})
		if err != nil {
			return api.IdentityChangeResult{}, err
		}
		undo.files = append(undo.files, uf)
		if bid != "" {
			res.BackupIDs = append(res.BackupIDs, bid)
		}
	}
	prev := rp.it.rec.IdentityOverride
	err = e.d.Projects.Mutate(func(d *projects.Data) error {
		r := d.FindRepo(req.RepoID)
		if r == nil {
			return projects.ErrNotFound
		}
		if rp.override == nil {
			r.IdentityOverride = nil
		} else {
			r.IdentityOverride = &projects.Identity{Name: rp.override.Name, Email: rp.override.Email}
		}
		return nil
	})
	if err != nil {
		for _, f := range undo.files {
			_ = e.restoreFile(f)
		}
		return api.IdentityChangeResult{}, api.NewError(api.ErrFileIO, "cannot write projects.json").WithParam("file", "projects.json").WithDetail(err.Error())
	}
	undo.records = append(undo.records, identityRecord("repo-override", req.RepoID, prev))
	res.Outcomes = append(res.Outcomes, oc)
	res.RecordUpdated = true
	res.UndoID = e.remember(undo)
	e.emit()
	return res, nil
}

func identityRecord(kind, id string, prev *projects.Identity) undoRecord {
	r := undoRecord{kind: kind, id: id}
	if prev != nil {
		r.has, r.name, r.email = true, prev.Name, prev.Email
	}
	return r
}

// ---- projects ----

type projectPlan struct {
	proj  projects.Project
	prev  *api.Author
	next  *api.Author
	items []*item
	hash  string
}

func (e *Engine) planProject(ctx context.Context, g *gitx.Git, req api.ProjectIdentityRequest) (*projectPlan, error) {
	data := e.d.Projects.Snapshot()
	p := data.FindProject(req.ProjectID)
	if p == nil {
		return nil, api.NewError(api.ErrInvalidInput, "unknown project").WithParam("field", "projectId")
	}
	pp := &projectPlan{proj: *p, prev: fromProjects(p.Identity)}
	if req.Identity != nil {
		a, err := Validate(*req.Identity)
		if err != nil {
			return nil, err
		}
		pp.next = &a
	}
	var members []projects.RepoRecord
	for _, r := range data.Repos {
		if r.ProjectID == p.ID {
			members = append(members, r)
		}
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Path < members[j].Path })
	type member struct {
		rec  projects.RepoRecord
		info repoInfo
		err  error
	}
	var ms []member
	ownPaths := map[string]bool{} // config files a member with its own setting reads from
	for _, r := range members {
		ri, err := e.repoInfo(ctx, g, r)
		if err != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err == nil && !ri.missing && r.IdentityOverride != nil {
			ownPaths[cleanPath(ri.cfgPath)] = true
		}
		ms = append(ms, member{r, ri, err})
	}
	claimed := map[string]bool{} // config files one earlier member already writes
	for _, m := range ms {
		r, ri := m.rec, m.info
		var it *item
		var err error
		switch {
		case m.err != nil:
			it = e.errorItem(r, m.err)
		case ri.missing:
			it, err = e.newItem(ctx, g, ri, api.RepoActionSkipMissing, nil)
		case r.IdentityOverride != nil:
			it, err = e.newItem(ctx, g, ri, api.RepoActionKeepOwn, nil)
		case ownPaths[cleanPath(ri.cfgPath)]:
			// the file is shared with a repository that has its own setting (a linked worktree): not ours to change
			it, err = e.newItem(ctx, g, ri, api.RepoActionKeepExternal, ri.local)
		default:
			action, after := followAction(ri.local, pp.prev, pp.next)
			it, err = e.newItem(ctx, g, ri, action, after)
		}
		if err != nil {
			return nil, err
		}
		if it.op != nil {
			// A repository and its linked worktree share one config file: the first member writes it, the
			// others report the same action and outcome without a second write.
			key := cleanPath(it.path)
			if claimed[key] {
				it.op = nil
			}
			claimed[key] = true
		}
		pp.items = append(pp.items, it)
	}
	order := func(a api.RepoAction) int {
		switch a {
		case api.RepoActionError:
			return -1
		case api.RepoActionSet, api.RepoActionRemove:
			return 0
		case api.RepoActionUnchanged:
			return 1
		case api.RepoActionSkipMissing:
			return 2
		}
		return 3 // keeps last
	}
	sort.SliceStable(pp.items, func(i, j int) bool { return order(pp.items[i].plan.Action) < order(pp.items[j].plan.Action) })
	parts := []string{"project", p.ID, authorKey(pp.prev), authorKey(pp.next)}
	for _, it := range pp.items {
		parts = append(parts, it.key())
	}
	pp.hash = planHash(parts...)
	return pp, nil
}

// PreviewProject lists the member repositories and what saving does to each.
func (e *Engine) PreviewProject(ctx context.Context, req api.ProjectIdentityRequest) (api.ProjectIdentityPreview, error) {
	g, err := e.git(ctx)
	if err != nil {
		return api.ProjectIdentityPreview{}, err
	}
	pp, err := e.planProject(ctx, g, req)
	if err != nil {
		return api.ProjectIdentityPreview{}, err
	}
	out := api.ProjectIdentityPreview{ProjectID: pp.proj.ID, Name: pp.proj.Name, Identity: pp.next, Previous: pp.prev, Repos: []api.RepoChangePlan{}, PlanHash: pp.hash}
	for _, it := range pp.items {
		out.Repos = append(out.Repos, it.plan)
		switch it.plan.Action {
		case api.RepoActionSet, api.RepoActionRemove:
			out.Writes++
		case api.RepoActionKeepOwn, api.RepoActionKeepExternal:
			out.KeepsOwn++
		}
	}
	return out, nil
}

// ApplyProject writes the plan. The project's own record changes only when every write succeeded, so a
// retry plans the remaining repositories from the same starting point.
func (e *Engine) ApplyProject(ctx context.Context, req api.ProjectIdentityRequest, planHash string) (api.IdentityChangeResult, error) {
	g, err := e.git(ctx)
	if err != nil {
		return api.IdentityChangeResult{}, err
	}
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	pp, err := e.planProject(ctx, g, req)
	if err != nil {
		return api.IdentityChangeResult{}, err
	}
	if pp.hash != planHash {
		return api.IdentityChangeResult{}, api.NewError(api.ErrConfigChanged, "the repositories changed since the preview").WithParam("file", pp.proj.Name)
	}
	res := api.IdentityChangeResult{Outcomes: []api.RepoChangeOutcome{}, BackupIDs: []string{}}
	var undo undoState
	failed := false
	for _, it := range pp.items {
		oc := api.RepoChangeOutcome{RepoID: it.plan.RepoID, Name: it.plan.Name, Action: it.plan.Action, OK: true}
		if it.err != nil {
			oc.OK, oc.Error, failed = false, it.err, true
		} else if it.op != nil {
			uf, bid, err := e.writeItem(ctx, g, it, map[string]string{"repo": it.plan.Name})
			if err != nil {
				oc.OK, oc.Error, failed = false, api.AsAppError(err), true
			} else {
				undo.files = append(undo.files, uf)
				if bid != "" {
					res.BackupIDs = append(res.BackupIDs, bid)
				}
			}
		}
		res.Outcomes = append(res.Outcomes, oc)
	}
	if !failed {
		prev := pp.proj.Identity
		err := e.d.Projects.Mutate(func(d *projects.Data) error {
			p := d.FindProject(pp.proj.ID)
			if p == nil {
				return projects.ErrNotFound
			}
			if pp.next == nil {
				p.Identity = nil
			} else {
				p.Identity = &projects.Identity{Name: pp.next.Name, Email: pp.next.Email}
			}
			return nil
		})
		if err != nil {
			for _, f := range undo.files {
				_ = e.restoreFile(f)
			}
			return api.IdentityChangeResult{}, api.NewError(api.ErrFileIO, "cannot write projects.json").WithParam("file", "projects.json").WithDetail(err.Error())
		}
		undo.records = append(undo.records, identityRecord("project-identity", pp.proj.ID, prev))
		res.RecordUpdated = true
	}
	res.UndoID = e.remember(undo)
	e.emit()
	return res, nil
}

// ---- moves ----

type movePlan struct {
	it   *item
	from *projects.Project
	to   *projects.Project
	hash string
	rec  projects.RepoRecord
}

func (e *Engine) planMove(ctx context.Context, g *gitx.Git, repoID, toProjectID string) (*movePlan, error) {
	data := e.d.Projects.Snapshot()
	rec, from, err := e.repoRecord(data, repoID)
	if err != nil {
		return nil, err
	}
	var to *projects.Project
	if toProjectID != "" {
		if to = data.FindProject(toProjectID); to == nil {
			return nil, api.NewError(api.ErrInvalidInput, "unknown project").WithParam("field", "projectId")
		}
	}
	ri, err := e.repoInfo(ctx, g, rec)
	if err != nil {
		return nil, err
	}
	var prev, next *api.Author
	if from != nil {
		prev = fromProjects(from.Identity)
	}
	if to != nil {
		next = fromProjects(to.Identity)
	}
	var action api.RepoAction
	var after *api.Author
	switch {
	case ri.missing:
		action = api.RepoActionSkipMissing
	case rec.IdentityOverride != nil:
		action = api.RepoActionKeepOwn
	case rec.ProjectID == toProjectID:
		action, after = api.RepoActionUnchanged, ri.local
	default:
		action, after = followAction(ri.local, prev, next)
	}
	it, err := e.newItem(ctx, g, ri, action, after)
	if err != nil {
		return nil, err
	}
	return &movePlan{it: it, from: from, to: to, rec: rec, hash: planHash("move", rec.ID, rec.ProjectID, toProjectID, authorKey(prev), authorKey(next), it.key())}, nil
}

// PreviewMove previews the identity side of moving a repository into toProjectID ("" = out of every
// project): the repository follows the new project's identity unless it has its own or holds a value that
// was set outside the app.
func (e *Engine) PreviewMove(ctx context.Context, repoID, toProjectID string) (api.MoveIdentityPreview, error) {
	g, err := e.git(ctx)
	if err != nil {
		return api.MoveIdentityPreview{}, err
	}
	mp, err := e.planMove(ctx, g, repoID, toProjectID)
	if err != nil {
		return api.MoveIdentityPreview{}, err
	}
	return api.MoveIdentityPreview{RepoID: repoID, FromProjectID: mp.rec.ProjectID, ToProjectID: toProjectID, Plan: mp.it.plan, PlanHash: mp.hash}, nil
}

// ApplyMove writes the identity change and moves the repository (projects.json) in one step. When the
// membership cannot be saved, the config write is put back.
func (e *Engine) ApplyMove(ctx context.Context, repoID, toProjectID, planHash string) (api.IdentityChangeResult, error) {
	g, err := e.git(ctx)
	if err != nil {
		return api.IdentityChangeResult{}, err
	}
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	mp, err := e.planMove(ctx, g, repoID, toProjectID)
	if err != nil {
		return api.IdentityChangeResult{}, err
	}
	if mp.hash != planHash {
		return api.IdentityChangeResult{}, api.NewError(api.ErrConfigChanged, "the repository changed since the preview").WithParam("file", e.d.Paths.Display(mp.it.path))
	}
	res := api.IdentityChangeResult{Outcomes: []api.RepoChangeOutcome{}, BackupIDs: []string{}}
	var undo undoState
	if mp.it.op != nil {
		uf, bid, err := e.writeItem(ctx, g, mp.it, map[string]string{"repo": mp.it.plan.Name})
		if err != nil {
			return api.IdentityChangeResult{}, err
		}
		undo.files = append(undo.files, uf)
		if bid != "" {
			res.BackupIDs = append(res.BackupIDs, bid)
		}
	}
	if err := e.d.Projects.SetProject(repoID, toProjectID); err != nil {
		for _, f := range undo.files {
			_ = e.restoreFile(f)
		}
		return api.IdentityChangeResult{}, api.NewError(api.ErrFileIO, "cannot write projects.json").WithParam("file", "projects.json").WithDetail(err.Error())
	}
	undo.records = append(undo.records, undoRecord{kind: "repo-project", id: repoID, value: mp.rec.ProjectID})
	res.Outcomes = append(res.Outcomes, api.RepoChangeOutcome{RepoID: repoID, Name: mp.it.plan.Name, Action: mp.it.plan.Action, OK: true})
	res.RecordUpdated = true
	res.UndoID = e.remember(undo)
	e.emit()
	return res, nil
}
