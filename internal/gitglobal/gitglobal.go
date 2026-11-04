// Package gitglobal manages the user's global git configuration around CodeCommit: the global default
// credential helper, the "not bound" guard, URL-scoped rules and the cached passwords of credential helpers.
// Every change is simulated on a temporary copy first (previews), backed up, hash-checked and then applied.
package gitglobal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/credstore"
	"github.com/rudian/aws_sso_hub/internal/gitx"
	"github.com/rudian/aws_sso_hub/internal/paths"
	"github.com/rudian/aws_sso_hub/internal/redact"
	"github.com/rudian/aws_sso_hub/internal/repos"
	"github.com/rudian/aws_sso_hub/internal/textdiff"
)

// Deps wires the engine.
type Deps struct {
	Paths    paths.Paths
	Git      func(ctx context.Context) (*gitx.Git, error)
	AWSPath  func() (string, error)
	Repos    *repos.Engine
	Profiles repos.ProfileSource
	Backup   repos.BackupPort
	Creds    credstore.Store
	Lang     func() string
	GOOS     string
	Emit     func(name string, data any)
	OnWrite  func(path string, content []byte)
}

// Engine implements the Git global panel.
type Engine struct{ d Deps }

// New creates the engine.
func New(d Deps) *Engine {
	if d.Emit == nil {
		d.Emit = func(string, any) {}
	}
	if d.Lang == nil {
		d.Lang = func() string { return "" }
	}
	return &Engine{d: d}
}

func (e *Engine) lang() string { return e.d.Lang() }

func (e *Engine) git(ctx context.Context) (*gitx.Git, error) {
	if e.d.Git == nil {
		return nil, api.NewError(api.ErrGitMissing, "git is not configured")
	}
	g, err := e.d.Git(ctx)
	if err != nil {
		return nil, api.AsAppError(err)
	}
	return g, nil
}

func (e *Engine) globalPath(g *gitx.Git) string {
	return g.GlobalConfigPath(e.d.Paths.Home, e.d.Paths.GitGlobalFile)
}

func hashOf(b []byte, exists bool) string {
	if !exists {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func readFile(path string) (content []byte, exists bool, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, api.NewError(api.ErrFileIO, "cannot read the global git config").WithParam("file", path).WithDetail(err.Error())
	}
	return b, true, nil
}

func (e *Engine) profileMap(ctx context.Context) map[string]repos.ProfileInfo {
	m := map[string]repos.ProfileInfo{}
	if e.d.Profiles == nil {
		return m
	}
	if ps, err := e.d.Profiles(ctx); err == nil {
		for _, p := range ps {
			m[p.Name] = p
		}
	}
	return m
}

// section groups the entries of one credential subsection.
type section struct {
	sub     string
	entries []gitx.Entry
}

func (s section) helpers() []string {
	var out []string
	for _, en := range s.entries {
		if en.Var == "helper" && !en.NoValue && en.Value != "" {
			out = append(out, en.Value)
		}
	}
	return out
}

func (s section) hasGuard() bool {
	for _, h := range s.helpers() {
		if gitx.IsGuard(h) {
			return true
		}
	}
	return false
}

func (s section) firstAWS() (gitx.AWSHelper, bool) {
	for _, h := range s.helpers() {
		if a := gitx.ParseAWSHelper(h); a.IsAWS {
			return a, true
		}
	}
	return gitx.AWSHelper{}, false
}

func (s section) extraKeys() bool {
	for _, en := range s.entries {
		if en.Var != "helper" && en.Var != "usehttppath" {
			return true
		}
	}
	return false
}

func sectionsOf(entries []gitx.Entry) (plain section, subs []section) {
	idx := map[string]int{}
	for _, en := range entries {
		if !strings.HasPrefix(en.Key, "credential.") {
			continue
		}
		if en.Subsection == "" {
			plain.entries = append(plain.entries, en)
			continue
		}
		i, ok := idx[en.Subsection]
		if !ok {
			i = len(subs)
			idx[en.Subsection] = i
			subs = append(subs, section{sub: en.Subsection})
		}
		subs[i].entries = append(subs[i].entries, en)
	}
	return plain, subs
}

// plainDefault returns the first aws helper among the plain helper entries that survives the last reset.
func plainDefault(plain section) (string, bool) {
	var list []string
	for _, en := range plain.entries {
		if en.Var != "helper" || en.NoValue {
			continue
		}
		if en.Value == "" {
			list = list[:0]
			continue
		}
		list = append(list, en.Value)
	}
	for _, v := range list {
		if gitx.ParseAWSHelper(v).IsAWS {
			return v, true
		}
	}
	return "", false
}

func (e *Engine) helperEntry(en gitx.Entry) api.HelperEntry {
	origin := en.Origin
	if filepath.IsAbs(origin) {
		origin = e.d.Paths.Display(origin)
	}
	return api.HelperEntry{Scope: en.Scope, Origin: origin, Key: en.Key, Value: redact.String(en.Value)}
}

// State summarises the system and global credential configuration.
func (e *Engine) State(ctx context.Context) (api.GitGlobalState, error) {
	g, err := e.git(ctx)
	if err != nil {
		return api.GitGlobalState{}, err
	}
	path := e.globalPath(g)
	content, exists, err := readFile(path)
	if err != nil {
		return api.GitGlobalState{}, err
	}
	entries, err := g.GlobalEntries(ctx)
	if err != nil {
		return api.GitGlobalState{}, err
	}
	pm := e.profileMap(ctx)
	rs := e.d.Repos.Snapshot()

	st := api.GitGlobalState{
		GlobalFile: e.d.Paths.Display(path), GlobalPath: path, BaseHash: hashOf(content, exists),
		URLRules: []api.URLRule{}, Guard: api.GuardInfo{Patterns: []string{}},
		OtherHelpers: []api.HelperEntry{}, CredentialLines: []api.HelperEntry{},
		SavedCredentialsSupported: e.d.Creds != nil && e.d.Creds.Supported(),
	}
	plain, subs := sectionsOf(entries)
	if h, ok := plainDefault(plain); ok {
		a := gitx.ParseAWSHelper(h)
		gd := &api.GlobalDefaultInfo{Profile: a.Profile, Helper: redact.String(h)}
		if p, ok := pm[a.Profile]; ok && a.HasProfile {
			gd.ProfileKind, gd.ColorSlot, gd.Monogram = p.Kind, p.ColorSlot, p.Monogram
		}
		for _, r := range rs {
			if r.Binding.Source == api.BindingGlobalDefault {
				gd.AffectedRepos++
			}
		}
		st.GlobalDefault = gd
	}
	for _, s := range subs {
		if s.hasGuard() {
			st.Guard.Installed = true
			st.Guard.Patterns = append(st.Guard.Patterns, s.sub)
			continue
		}
		a, ok := s.firstAWS()
		if !ok {
			continue
		}
		rule := api.URLRule{Pattern: s.sub, Profile: a.Profile, MatchedRepoIDs: []string{}, ExtraKeys: s.extraKeys()}
		if p, ok := pm[a.Profile]; ok && a.HasProfile {
			rule.ProfileKnown, rule.ColorSlot, rule.Monogram = true, p.ColorSlot, p.Monogram
		}
		for _, r := range rs {
			if r.Binding.Source == api.BindingGlobalURL && r.Binding.URLRule == s.sub {
				rule.MatchedRepoIDs = append(rule.MatchedRepoIDs, r.ID)
			}
		}
		st.URLRules = append(st.URLRules, rule)
	}
	for _, en := range entries {
		st.CredentialLines = append(st.CredentialLines, e.helperEntry(en))
		if en.Var != "helper" || en.NoValue || en.Value == "" {
			continue
		}
		v := en.Value
		if gitx.ParseAWSHelper(v).IsAWS || gitx.IsGuard(v) || gitx.IsFallback(v) {
			continue
		}
		st.OtherHelpers = append(st.OtherHelpers, e.helperEntry(en))
	}
	for _, r := range rs {
		if r.ToFix {
			st.UnboundRepos++
		} else {
			st.BoundRepos++
		}
	}
	return st, nil
}

// ---- default changes ----

func (e *Engine) requireURLRulesMigrated(st api.GitGlobalState) error {
	if n := len(st.URLRules); n > 0 {
		return api.NewError(api.ErrURLRulesFirst, "global URL rules must be migrated first").WithParam("n", itoa(n))
	}
	return nil
}

func itoa(n int) string { return strconv.Itoa(n) }

// applyDefaultOps performs the file operations of a default change with g (real or simulated).
func (e *Engine) applyDefaultOps(ctx context.Context, g *gitx.Git, req api.DefaultChangeRequest, hosts []string) error {
	switch req.Action {
	case api.DefaultKeep:
		return nil
	case api.DefaultRemove:
		if err := g.RemoveGlobalAWSDefault(ctx); err != nil {
			return err
		}
		return g.InstallGuard(ctx, gitx.GuardPatterns(hosts), gitx.GuardValue(e.lang(), e.d.GOOS))
	case api.DefaultReplace:
		aws, err := e.awsHelper(req.Profile)
		if err != nil {
			return err
		}
		if err := g.RemoveGlobalAWSDefault(ctx); err != nil {
			return err
		}
		es, err := g.GlobalEntries(ctx)
		if err != nil {
			return err
		}
		plainRemaining := false
		for _, en := range es {
			if en.Scope == "global" && en.Subsection == "" && en.Var == "helper" {
				plainRemaining = true
			}
		}
		return g.AddGlobalDefault(ctx, aws, !plainRemaining)
	}
	return api.NewError(api.ErrInvalidInput, "unknown action").WithParam("field", "action")
}

func (e *Engine) awsHelper(profile string) (string, error) {
	if e.d.AWSPath == nil {
		return "", api.NewError(api.ErrAWSCLIMissing, "the AWS CLI was not found")
	}
	p, err := e.d.AWSPath()
	if err != nil {
		return "", api.NewError(api.ErrAWSCLIMissing, "the AWS CLI was not found").WithDetail(err.Error())
	}
	return gitx.AWSHelperValue(p, profile, e.d.GOOS)
}

func (e *Engine) validateRequest(ctx context.Context, req api.DefaultChangeRequest, st api.GitGlobalState) error {
	switch req.Action {
	case api.DefaultKeep:
		return nil
	case api.DefaultRemove:
		return e.requireURLRulesMigrated(st)
	case api.DefaultReplace:
		if _, ok := e.profileMap(ctx)[req.Profile]; !ok {
			return api.NewError(api.ErrProfileNotFound, "profile not found").WithParam("name", req.Profile)
		}
		_, err := e.awsHelper(req.Profile)
		return err
	}
	return api.NewError(api.ErrInvalidInput, "unknown action").WithParam("field", "action")
}

// simulate runs ops against a temporary copy of the global file and returns the preview and a Git bound to the copy.
func (e *Engine) simulate(ctx context.Context, g *gitx.Git, ops func(sim *gitx.Git) error) (api.FilePreview, *gitx.Git, func(), error) {
	path := e.globalPath(g)
	before, exists, err := readFile(path)
	if err != nil {
		return api.FilePreview{}, nil, nil, err
	}
	tmp, err := os.MkdirTemp("", "awsssohub-sim-")
	if err != nil {
		return api.FilePreview{}, nil, nil, api.AsAppError(err)
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	simPath := filepath.Join(tmp, "gitconfig")
	if err := os.WriteFile(simPath, before, 0o600); err != nil {
		cleanup()
		return api.FilePreview{}, nil, nil, api.AsAppError(err)
	}
	sim := g.WithEnv("GIT_CONFIG_GLOBAL=" + simPath)
	if err := ops(sim); err != nil {
		cleanup()
		return api.FilePreview{}, nil, nil, err
	}
	after, _, _ := readFile(simPath)
	lines, added, removed := textdiff.Diff([]byte(redact.String(string(before))), []byte(redact.String(string(after))), textdiff.DefaultContext)
	pv := api.FilePreview{
		File: e.d.Paths.Display(path), Path: path, BaseHash: hashOf(before, exists), Lines: lines, Added: added, Removed: removed,
		BackupDir: e.d.Paths.Display(e.d.Paths.BackupDir),
	}
	return pv, sim, cleanup, nil
}

func (e *Engine) emptyPreview(g *gitx.Git) (api.FilePreview, error) {
	path := e.globalPath(g)
	b, exists, err := readFile(path)
	if err != nil {
		return api.FilePreview{}, err
	}
	return api.FilePreview{File: e.d.Paths.Display(path), Path: path, BaseHash: hashOf(b, exists), Lines: []api.DiffLine{}, BackupDir: e.d.Paths.Display(e.d.Paths.BackupDir)}, nil
}

// PreviewDefault shows the file change and its effect on the scanned repositories without touching anything.
func (e *Engine) PreviewDefault(ctx context.Context, req api.DefaultChangeRequest) (api.DefaultChangePreview, error) {
	g, err := e.git(ctx)
	if err != nil {
		return api.DefaultChangePreview{}, err
	}
	st, err := e.State(ctx)
	if err != nil {
		return api.DefaultChangePreview{}, err
	}
	if err := e.validateRequest(ctx, req, st); err != nil {
		return api.DefaultChangePreview{}, err
	}
	out := api.DefaultChangePreview{BlockedRepoIDs: []string{}}
	if req.Action == api.DefaultKeep {
		out.Preview, err = e.emptyPreview(g)
		return out, err
	}
	pv, sim, cleanup, err := e.simulate(ctx, g, func(sim *gitx.Git) error {
		return e.applyDefaultOps(ctx, sim, req, e.d.Repos.Hosts())
	})
	if err != nil {
		return api.DefaultChangePreview{}, err
	}
	defer cleanup()
	out.Preview = pv
	out.BoundUnaffected, out.BlockedRepoIDs = e.impact(ctx, sim)
	out.UnboundBlocked = len(out.BlockedRepoIDs)
	return out, nil
}

// impact re-classifies every https repository with the simulated global file.
func (e *Engine) impact(ctx context.Context, sim *gitx.Git) (boundUnaffected int, blocked []string) {
	blocked = []string{}
	rs := e.d.Repos.Snapshot()
	newSource := make([]api.BindingSource, len(rs))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, r := range rs {
		newSource[i] = r.Binding.Source
		if r.RemoteKind != api.RemoteHTTPS {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, r api.Repo) {
			defer wg.Done()
			defer func() { <-sem }()
			entries, err := sim.CredentialEntries(ctx, r.Path)
			if err != nil {
				return
			}
			u, err := gitx.ParseCredURL(r.HTTPSURL)
			if err != nil {
				return
			}
			c := gitx.Classify(gitx.Resolve(entries, u), gitx.LocalMetaFrom(entries), api.RemoteHTTPS, "", func(p string) bool {
				st, err := os.Stat(p)
				return err == nil && !st.IsDir()
			})
			newSource[i] = c.Source
		}(i, r)
	}
	wg.Wait()
	for i, r := range rs {
		switch {
		case newSource[i] == api.BindingGuard && r.Binding.Source != api.BindingGuard:
			blocked = append(blocked, r.ID)
		case r.Binding.Source == api.BindingLocal && newSource[i] == api.BindingLocal:
			boundUnaffected++
		}
	}
	sort.Strings(blocked)
	return boundUnaffected, blocked
}

func (e *Engine) checkHash(path string, baseHash string) error {
	b, exists, err := readFile(path)
	if err != nil {
		return err
	}
	if hashOf(b, exists) != baseHash {
		return api.NewError(api.ErrConfigChanged, "the global git config changed").WithParam("file", e.d.Paths.Display(path))
	}
	return nil
}

func (e *Engine) snapshot(path string, reason api.BackupReason, params map[string]string) (string, error) {
	if e.d.Backup == nil {
		return "", nil
	}
	entry, err := e.d.Backup.Snapshot(path, e.d.Paths.Display(path), api.BackupGitGlobal, reason, params)
	if err != nil {
		return "", api.AsAppError(err)
	}
	return entry.ID, nil
}

func (e *Engine) afterWrite(ctx context.Context, path string) {
	if e.d.OnWrite != nil {
		if b, _, err := readFile(path); err == nil {
			e.d.OnWrite(path, b)
		}
	}
	e.d.Repos.Invalidate(ctx)
	if rs, err := e.d.Repos.State(ctx); err == nil {
		e.d.Emit(api.EventReposChanged, rs)
	}
}

// ApplyDefault performs a default change after hash check and backup.
func (e *Engine) ApplyDefault(ctx context.Context, req api.DefaultChangeRequest, baseHash string) (api.GlobalChangeResult, error) {
	g, err := e.git(ctx)
	if err != nil {
		return api.GlobalChangeResult{}, err
	}
	st, err := e.State(ctx)
	if err != nil {
		return api.GlobalChangeResult{}, err
	}
	if err := e.validateRequest(ctx, req, st); err != nil {
		return api.GlobalChangeResult{}, err
	}
	path := e.globalPath(g)
	if err := e.checkHash(path, baseHash); err != nil {
		return api.GlobalChangeResult{}, err
	}
	if req.Action == api.DefaultKeep {
		return api.GlobalChangeResult{State: st}, nil
	}
	backupID, err := e.snapshot(path, api.ReasonGlobalDefault, nil)
	if err != nil {
		return api.GlobalChangeResult{}, err
	}
	if err := e.applyDefaultOps(ctx, g, req, e.d.Repos.Hosts()); err != nil {
		return api.GlobalChangeResult{}, api.AsAppError(err)
	}
	e.afterWrite(ctx, path)
	st, err = e.State(ctx)
	if err != nil {
		return api.GlobalChangeResult{}, err
	}
	return api.GlobalChangeResult{State: st, BackupID: backupID}, nil
}

// ---- URL rules ----

func (e *Engine) ruleByPattern(st api.GitGlobalState, pattern string) (api.URLRule, error) {
	for _, r := range st.URLRules {
		if r.Pattern == pattern {
			return r, nil
		}
	}
	return api.URLRule{}, api.NewError(api.ErrInvalidInput, "no such global URL rule").WithParam("field", "pattern")
}

func (e *Engine) matchedRepos(rule api.URLRule) []api.Repo {
	ids := map[string]bool{}
	for _, id := range rule.MatchedRepoIDs {
		ids[id] = true
	}
	out := make([]api.Repo, 0, len(ids))
	for _, r := range e.d.Repos.Snapshot() {
		if ids[r.ID] {
			out = append(out, r)
		}
	}
	return out
}

// PreviewURLRule shows what migrating a global URL rule to local bindings involves.
func (e *Engine) PreviewURLRule(ctx context.Context, pattern string) (api.URLRulePlan, error) {
	g, err := e.git(ctx)
	if err != nil {
		return api.URLRulePlan{}, err
	}
	st, err := e.State(ctx)
	if err != nil {
		return api.URLRulePlan{}, err
	}
	rule, err := e.ruleByPattern(st, pattern)
	if err != nil {
		return api.URLRulePlan{}, err
	}
	plan := api.URLRulePlan{Pattern: pattern, Profile: rule.Profile, Repos: e.matchedRepos(rule)}
	seen := map[string]bool{}
	for _, r := range plan.Repos {
		if seen[r.HTTPSURL] {
			plan.DuplicateURLs = true
		}
		seen[r.HTTPSURL] = true
	}
	pv, _, cleanup, err := e.simulate(ctx, g, func(sim *gitx.Git) error { return sim.RemoveURLRule(ctx, pattern, rule.ExtraKeys) })
	if err != nil {
		return api.URLRulePlan{}, err
	}
	cleanup()
	plan.Preview = pv
	return plan, nil
}

// MigrateURLRule binds every repository matched by the rule as assigned and then removes the global rule.
func (e *Engine) MigrateURLRule(ctx context.Context, m api.URLRuleMigration, baseHash string) (api.URLRuleMigrationResult, error) {
	g, err := e.git(ctx)
	if err != nil {
		return api.URLRuleMigrationResult{}, err
	}
	st, err := e.State(ctx)
	if err != nil {
		return api.URLRuleMigrationResult{}, err
	}
	rule, err := e.ruleByPattern(st, m.Pattern)
	if err != nil {
		return api.URLRuleMigrationResult{}, err
	}
	path := e.globalPath(g)
	if err := e.checkHash(path, baseHash); err != nil {
		return api.URLRuleMigrationResult{}, err
	}
	want := map[string]bool{}
	for _, id := range rule.MatchedRepoIDs {
		want[id] = true
	}
	got := map[string]bool{}
	for _, a := range m.Assignments {
		if !want[a.RepoID] || got[a.RepoID] || a.Profile == "" {
			return api.URLRuleMigrationResult{}, api.NewError(api.ErrInvalidInput, "assignments must cover every matched repository exactly once").WithParam("field", "assignments")
		}
		got[a.RepoID] = true
	}
	if len(got) != len(want) {
		return api.URLRuleMigrationResult{}, api.NewError(api.ErrInvalidInput, "assignments must cover every matched repository exactly once").WithParam("field", "assignments")
	}
	res := api.URLRuleMigrationResult{Outcomes: []api.BindOutcome{}}
	if len(m.Assignments) > 0 {
		res.Outcomes, err = e.d.Repos.BindMany(ctx, m.Assignments)
		if err != nil {
			return api.URLRuleMigrationResult{}, err
		}
	}
	for _, o := range res.Outcomes {
		if !o.OK {
			e.afterWrite(ctx, path)
			return res, nil
		}
	}
	res.BackupID, err = e.snapshot(path, api.ReasonURLRule, map[string]string{"pattern": m.Pattern})
	if err != nil {
		return api.URLRuleMigrationResult{}, err
	}
	if err := g.RemoveURLRule(ctx, m.Pattern, rule.ExtraKeys); err != nil {
		return api.URLRuleMigrationResult{}, api.AsAppError(err)
	}
	res.RuleRemoved = true
	e.afterWrite(ctx, path)
	return res, nil
}

// ---- saved credentials ----

// ListSaved lists cached CodeCommit passwords for every host and region seen.
func (e *Engine) ListSaved(ctx context.Context) ([]api.SavedCredential, error) {
	if e.d.Creds == nil {
		return []api.SavedCredential{}, nil
	}
	hosts := e.d.Repos.Hosts()
	seen := map[string]bool{}
	for _, h := range hosts {
		seen[h] = true
	}
	for _, r := range e.d.Repos.Snapshot() {
		if r.Region == "" {
			continue
		}
		h := "git-codecommit." + r.Region + ".amazonaws.com"
		if strings.HasPrefix(r.Region, "cn-") {
			h += ".cn"
		}
		if !seen[h] {
			seen[h] = true
			hosts = append(hosts, h)
		}
	}
	sort.Strings(hosts)
	return e.d.Creds.List(ctx, hosts)
}

// ClearSaved removes cached passwords; no ids means every listed entry. Requested ids that ListSaved does not
// report right now are ignored, so a caller cannot make the app delete credentials of other services.
func (e *Engine) ClearSaved(ctx context.Context, ids []string) (int, error) {
	if e.d.Creds == nil {
		return 0, nil
	}
	all, err := e.ListSaved(ctx)
	if err != nil {
		return 0, err
	}
	listed := make(map[string]bool, len(all))
	for _, c := range all {
		listed[c.ID] = true
	}
	var allowed []string
	if len(ids) == 0 {
		for _, c := range all {
			allowed = append(allowed, c.ID)
		}
	} else {
		seen := map[string]bool{}
		for _, id := range ids {
			if listed[id] && !seen[id] {
				seen[id] = true
				allowed = append(allowed, id)
			}
		}
	}
	if len(allowed) == 0 {
		return 0, nil
	}
	return e.d.Creds.Clear(ctx, allowed)
}
