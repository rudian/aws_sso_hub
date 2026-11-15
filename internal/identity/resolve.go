package identity

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/projects"
	"github.com/rudian/codepass/internal/redact"
)

// includeInfo says why a file is part of the configuration.
type includeInfo struct {
	rule     string // "" for an unconditional include
	cond     bool
	ruleFile string
}

func cleanPath(p string) string {
	p = filepath.Clean(p)
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

func originFile(origin string) string { return strings.TrimPrefix(origin, "file:") }

// absOrigins makes the file origins absolute: git prints the repository's own config relative to the folder
// it was started in (".git/config").
func absOrigins(entries []gitx.ConfigEntry, dir string) []gitx.ConfigEntry {
	for i, en := range entries {
		if f, ok := strings.CutPrefix(en.Origin, "file:"); ok && f != "" && !filepath.IsAbs(f) && dir != "" {
			entries[i].Origin = "file:" + filepath.Join(dir, f)
		}
	}
	return entries
}

// includeTargets maps every file pulled in by include.path / includeIf.<cond>.path to its rule.
func (e *Engine) includeTargets(entries []gitx.ConfigEntry) map[string]includeInfo {
	out := map[string]includeInfo{}
	for _, en := range entries {
		key := strings.ToLower(en.Key)
		var info includeInfo
		switch {
		case key == "include.path":
		case strings.HasPrefix(key, "includeif.") && strings.HasSuffix(key, ".path"):
			// the condition keeps its case: take it from the original key
			info = includeInfo{cond: true, rule: en.Key[len("includeif.") : len(en.Key)-len(".path")]}
		default:
			continue
		}
		p := en.Value
		from := originFile(en.Origin)
		info.ruleFile = from
		switch {
		case p == "~" || strings.HasPrefix(p, "~/"):
			p = filepath.Join(e.d.Paths.Home, filepath.FromSlash(strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/")))
		case !filepath.IsAbs(p):
			p = filepath.Join(filepath.Dir(from), filepath.FromSlash(p))
		}
		out[cleanPath(p)] = info
	}
	return out
}

// field is one candidate value with its source.
type field struct {
	val string
	src api.IdentitySource
}

// attribution decides, for the author name and email, which source git takes the value from. entries is the
// effective configuration in git's own order (later wins); env is the terminal environment (nil = settings
// only). rec and proj classify local values as the repository's own or the project's.
type attrIn struct {
	entries []gitx.ConfigEntry
	env     map[string]string
	envFile func(name string) (display, abs string)
	rec     *projects.RepoRecord
	proj    *projects.Project
	cfgPath string // the repository's own config file ("" for the global view)
	inc     map[string]includeInfo
}

func (e *Engine) attribute(in attrIn) (name, email field) {
	last := func(keys ...string) (gitx.ConfigEntry, bool) {
		var got gitx.ConfigEntry
		ok := false
		for _, en := range in.entries {
			k := strings.ToLower(en.Key)
			for _, want := range keys {
				if k == want {
					got, ok = en, true
				}
			}
		}
		return got, ok
	}
	type pri struct {
		env    string
		config []string
	}
	pick := func(p pri, ownVal func() (string, bool), projVal func() (string, bool)) field {
		if p.env != "" && in.env != nil {
			if v, ok := in.env[p.env]; ok && v != "" {
				src := api.IdentitySource{Kind: api.IdentitySrcEnv, Var: p.env}
				if in.envFile != nil {
					src.File, src.Path = in.envFile(p.env)
				}
				return field{val: v, src: src}
			}
		}
		// config: author.* is read after user.* by git, so it wins when both are set
		for _, key := range p.config {
			if en, ok := last(key); ok {
				return field{val: en.Value, src: e.configSource(en, in, ownVal, projVal)}
			}
		}
		return field{src: api.IdentitySource{Kind: api.IdentitySrcNone}}
	}
	own := func(get func(*api.Author) string) func() (string, bool) {
		return func() (string, bool) {
			if in.rec != nil && in.rec.IdentityOverride != nil && !followsGlobal(in.rec.IdentityOverride) {
				return get(&api.Author{Name: in.rec.IdentityOverride.Name, Email: in.rec.IdentityOverride.Email}), true
			}
			return "", false
		}
	}
	prj := func(get func(*api.Author) string) func() (string, bool) {
		return func() (string, bool) {
			if in.proj != nil && in.proj.Identity != nil {
				return get(&api.Author{Name: in.proj.Identity.Name, Email: in.proj.Identity.Email}), true
			}
			return "", false
		}
	}
	gName := func(a *api.Author) string { return a.Name }
	gEmail := func(a *api.Author) string { return a.Email }
	name = pick(pri{env: "GIT_AUTHOR_NAME", config: []string{"author.name", "user.name"}}, own(gName), prj(gName))
	email = pick(pri{env: "GIT_AUTHOR_EMAIL", config: []string{"author.email", "user.email"}}, own(gEmail), prj(gEmail))
	if email.val == "" && in.env != nil {
		if v, ok := in.env["EMAIL"]; ok && v != "" {
			src := api.IdentitySource{Kind: api.IdentitySrcEnv, Var: "EMAIL"}
			if in.envFile != nil {
				src.File, src.Path = in.envFile("EMAIL")
			}
			email = field{val: v, src: src}
		}
	}
	return name, email
}

// configSource classifies one config entry.
func (e *Engine) configSource(en gitx.ConfigEntry, in attrIn, ownVal, projVal func() (string, bool)) api.IdentitySource {
	file := originFile(en.Origin)
	src := api.IdentitySource{Scope: en.Scope}
	if strings.HasPrefix(en.Origin, "file:") {
		src.Path = file
		src.File = e.d.Paths.Display(file)
	}
	if info, ok := in.inc[cleanPath(file)]; ok && src.Path != "" {
		if info.cond {
			src.Kind = api.IdentitySrcConditional
			src.Rule = info.rule
		} else {
			src.Kind = api.IdentitySrcInclude
		}
		src.RuleFile = e.d.Paths.Display(info.ruleFile)
		return src
	}
	switch en.Scope {
	case "system":
		src.Kind = api.IdentitySrcSystem
	case "global":
		src.Kind = api.IdentitySrcGlobal
	case "worktree":
		src.Kind = api.IdentitySrcWorktree
		src.External = true
	case "command":
		src.Kind = api.IdentitySrcCommand
	case "local":
		if v, ok := ownVal(); ok && v == en.Value {
			src.Kind = api.IdentitySrcRepo
		} else if v, ok := projVal(); ok && v == en.Value {
			src.Kind = api.IdentitySrcFromProject
		} else {
			src.Kind = api.IdentitySrcRepo
			src.External = true
		}
	default:
		src.Kind = api.IdentitySrcNone
	}
	return src
}

func rank(k api.IdentitySourceKind) int {
	switch k {
	case api.IdentitySrcEnv:
		return 8
	case api.IdentitySrcCommand:
		return 7
	case api.IdentitySrcWorktree:
		return 6
	case api.IdentitySrcRepo, api.IdentitySrcFromProject:
		return 5
	case api.IdentitySrcConditional, api.IdentitySrcInclude:
		return 4
	case api.IdentitySrcProject:
		return 3
	case api.IdentitySrcGlobal:
		return 2
	case api.IdentitySrcSystem:
		return 1
	}
	return 0
}

func effective(name, email field) api.EffectiveIdentity {
	ef := api.EffectiveIdentity{Name: name.val, Email: email.val, NameSource: name.src, EmailSource: email.src}
	ef.Set = name.val != "" && email.val != ""
	ef.Source = email.src
	if rank(name.src.Kind) > rank(email.src.Kind) {
		ef.Source = name.src
	}
	return ef
}

// parseIdent splits `Name <email> 1700000000 +0100`.
func parseIdent(s string) (name, email string, ok bool) {
	s = strings.TrimSpace(s)
	gt := strings.LastIndexByte(s, '>')
	if gt < 0 {
		return "", "", false
	}
	lt := strings.LastIndexByte(s[:gt], '<')
	if lt < 0 {
		return "", "", false
	}
	return strings.TrimSpace(s[:lt]), s[lt+1 : gt], true
}

// repoView is what the reads found out about one repository.
type repoView struct {
	row     api.RepoIdentity
	local   *api.Author // the user.* values in the repository's own config file
	cfgPath string
}

// resolveRepo reads one repository with the terminal's environment.
func (e *Engine) resolveRepo(ctx context.Context, g *gitx.Git, t termEnv, rec projects.RepoRecord, proj *projects.Project, name string) repoView {
	row := api.RepoIdentity{RepoID: rec.ID, Name: name, Path: rec.Path, ProjectID: rec.ProjectID}
	if followsGlobal(rec.IdentityOverride) {
		row.FollowsGlobal = true
	} else if rec.IdentityOverride != nil {
		row.Override = &api.Author{Name: rec.IdentityOverride.Name, Email: rec.IdentityOverride.Email}
	}
	v := repoView{row: row}
	if rec.MissingSince != nil || !isDir(rec.Path) {
		v.row.Missing = true
		return v
	}
	rd := gitx.NewReader(e.termGit(g, t))
	entries, err := rd.ConfigList(ctx, rec.Path)
	if err != nil {
		v.row.ReadError = api.AsAppError(err).WithParam("path", e.d.Paths.Display(rec.Path))
		return v
	}
	entries = absOrigins(entries, rec.Path)
	cfg, _ := e.configPath(ctx, g, rec.Path)
	v.cfgPath = cfg
	inc := e.includeTargets(entries)
	in := attrIn{entries: entries, rec: &rec, proj: proj, cfgPath: cfg, inc: inc}
	n, em := e.attribute(in)
	v.row.Setting = effective(n, em)
	in.env = t.vars
	in.envFile = func(name string) (string, string) { return e.profileFileFor(t, name) }
	n, em = e.attribute(in)
	eff := effective(n, em)
	// git has the last word on the value itself (its own precedence, includeIf, trimming).
	if out, err := rd.Var(ctx, rec.Path, "GIT_AUTHOR_IDENT"); err == nil {
		if gn, ge, ok := parseIdent(out); ok {
			if eff.Name != "" && gn != "" {
				eff.Name = gn
			}
			if eff.Email != "" && ge != "" {
				eff.Email = ge
			}
		}
	}
	v.row.Effective = eff
	v.local = localAuthor(entries, cfg)
	return v
}

// followsGlobal reports the marker of a repository set to follow the global identity: an override whose
// name and email are both empty.
func followsGlobal(i *projects.Identity) bool { return i != nil && i.Name == "" && i.Email == "" }

// configPath returns the absolute path of the repository's own config file (worktree-safe), through the
// read-only layer.
func (e *Engine) configPath(ctx context.Context, g *gitx.Git, dir string) (string, error) {
	res, err := gitx.NewReader(g).Run(ctx, dir, "rev-parse", "--git-path", "config")
	if err != nil {
		if _, ok := err.(*api.AppError); !ok && ctx.Err() == nil {
			err = api.NewError(api.ErrGitFailed, "git could not run").WithDetail(err.Error())
		}
		return "", err
	}
	if res.ExitCode != 0 {
		return "", api.NewError(api.ErrGitFailed, "git rev-parse failed (exit %d)", res.ExitCode).WithDetail(redact.Tail(string(res.Stderr), 3))
	}
	p := strings.TrimSpace(string(res.Stdout))
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	return filepath.Clean(p), nil
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// localAuthor returns the user.* values that the repository's own config file holds (nil = none of them).
func localAuthor(entries []gitx.ConfigEntry, cfgPath string) *api.Author {
	var a api.Author
	has := false
	for _, en := range entries {
		if en.Scope != "local" || cleanPath(originFile(en.Origin)) != cleanPath(cfgPath) {
			continue
		}
		switch strings.ToLower(en.Key) {
		case "user.name":
			a.Name, has = en.Value, true
		case "user.email":
			a.Email, has = en.Value, true
		}
	}
	if !has {
		return nil
	}
	return &a
}

func (e *Engine) globalPath(g *gitx.Git) string {
	return g.GlobalConfigPath(e.d.Paths.Home, e.d.Paths.GitGlobalFile)
}

// globalView reads the global level with the app's own environment.
func (e *Engine) globalView(ctx context.Context, g *gitx.Git) (api.GlobalIdentity, error) {
	path := e.globalPath(g)
	content, exists, err := readFile(path)
	if err != nil {
		return api.GlobalIdentity{}, err
	}
	gi := api.GlobalIdentity{File: e.d.Paths.Display(path), Path: path, BaseHash: hashOf(content, exists), Source: api.IdentitySource{Kind: api.IdentitySrcNone}}
	entries, err := gitx.NewReader(g).ConfigList(ctx, "")
	if err != nil {
		return gi, err
	}
	var gl []gitx.ConfigEntry
	for _, en := range entries {
		if en.Scope == "global" || en.Scope == "system" {
			gl = append(gl, en)
		}
	}
	n, em := e.attribute(attrIn{entries: gl, inc: e.includeTargets(entries)})
	gi.Name, gi.Email, gi.Set = n.val, em.val, n.val != "" && em.val != ""
	ef := effective(n, em)
	gi.Source = ef.Source
	return gi, nil
}

// Overview builds the effective identity table.
func (e *Engine) Overview(ctx context.Context) (api.IdentityOverview, error) {
	g, err := e.git(ctx)
	if err != nil {
		return api.IdentityOverview{}, err
	}
	gl, err := e.globalView(ctx, g)
	if err != nil {
		return api.IdentityOverview{}, err
	}
	t := e.terminalEnv(ctx, false)
	data := e.d.Projects.Snapshot()

	type job struct {
		rec  projects.RepoRecord
		proj *projects.Project
	}
	var jobs []job
	for _, r := range data.Repos {
		if r.Hidden || r.Path == "" {
			continue
		}
		var p *projects.Project
		if r.ProjectID != "" {
			p = data.FindProject(r.ProjectID)
		}
		jobs = append(jobs, job{r, p})
	}
	views := make([]repoView, len(jobs))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			views[i] = e.resolveRepo(ctx, g, t, j.rec, j.proj, filepath.Base(j.rec.Path))
		}()
	}
	wg.Wait()

	out := api.IdentityOverview{Global: gl, Projects: []api.ProjectIdentity{}, Ungrouped: []api.RepoIdentity{}, Env: e.envReport(g, t)}
	byProject := map[string]int{}
	for _, p := range sortedProjects(data.Projects) {
		pi := api.ProjectIdentity{ID: p.ID, Name: p.Name, Repos: []api.RepoIdentity{}}
		if p.Identity != nil {
			pi.Identity = &api.Author{Name: p.Identity.Name, Email: p.Identity.Email}
		}
		byProject[p.ID] = len(out.Projects)
		out.Projects = append(out.Projects, pi)
	}
	for _, v := range views {
		// an environment variable that decides a row's author is in effect, whichever variable it is
		if v.row.Effective.NameSource.Kind == api.IdentitySrcEnv || v.row.Effective.EmailSource.Kind == api.IdentitySrcEnv {
			out.Env.Active = true
		}
		if v.row.Setting.Source.Kind == api.IdentitySrcGlobal && v.row.Setting.Set {
			out.Global.UsedBy++
		}
		if i, ok := byProject[v.row.ProjectID]; ok && v.row.ProjectID != "" {
			out.Projects[i].Repos = append(out.Projects[i].Repos, v.row)
		} else {
			out.Ungrouped = append(out.Ungrouped, v.row)
		}
	}
	return out, nil
}

func sortedProjects(ps []projects.Project) []projects.Project {
	out := append([]projects.Project(nil), ps...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Order < out[j-1].Order; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
