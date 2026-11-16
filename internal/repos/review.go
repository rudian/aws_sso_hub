package repos

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/projects"
)

// This file: the one-time review of newly found repositories, hiding, projects, suggestions, and the answer to
// the moved-folder question. Every change is a write to projects.json (never to settings.json, never to a
// repository folder) followed by a fresh state.

// minGroup is how many ungrouped repositories must share a prefix or a parent folder before the engine suggests
// a project.
const minGroup = 3

// regErr maps a registry error to the api error the person sees. moved is true for the moved-folder answers.
func regErr(err error, moved bool) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, projects.ErrHasData):
		return api.NewError(api.ErrMovedHasData, "the repository found at the new place already has data of its own")
	case errors.Is(err, projects.ErrNotFound):
		if moved {
			return api.NewError(api.ErrMovedGone, "the moved-folder question is out of date")
		}
		return api.NewError(api.ErrRepoNotFound, "repository not found")
	}
	return fileIOProjects(err)
}

// knownRepo reports whether the engine lists (or holds for review) a repository with this ID.
func (e *Engine) knownRepo(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.recs[id]; ok {
		return true
	}
	_, ok := e.others[id]
	return ok
}

func (e *Engine) requireRepos(ids []string) error {
	for _, id := range ids {
		if !e.knownRepo(id) {
			return api.NewError(api.ErrRepoNotFound, "repository not found").WithParam("path", id)
		}
	}
	return nil
}

// forgetOffers drops the moved-folder questions of repositories that now carry data of their own: answering
// "yes" would delete it (FU-18a).
func (e *Engine) forgetOffers(ids ...string) {
	e.mu.Lock()
	for _, id := range ids {
		delete(e.moved, id)
	}
	e.mu.Unlock()
}

// ForgetMovedOffers is forgetOffers for a caller outside the engine that wrote projects.json itself (the identity
// engine's move into a project, a repository's own identity): the question about those repositories is out of
// date.
func (e *Engine) ForgetMovedOffers(ids ...string) { e.forgetOffers(ids...) }

func (e *Engine) changed(ctx context.Context) api.ReposState {
	st := e.emitState(ctx)
	e.syncWatches()
	return st
}

// ResolveFound answers the one-time review: show joins the list, hide hides (they can be unhidden later).
// "Hide them all" is every ID of State.Found in hide. IDs that are not waiting are ignored.
func (e *Engine) ResolveFound(ctx context.Context, show, hide []string) (api.ReposState, error) {
	if err := e.requireRepos(append(append([]string{}, show...), hide...)); err != nil {
		return api.ReposState{}, err
	}
	if err := e.reg.ResolvePending(show, hide); err != nil {
		return api.ReposState{}, regErr(err, false)
	}
	e.forgetOffers(hide...)
	e.fillLastCommitsFor(ctx)
	return e.changed(ctx), nil
}

// SetHidden hides a repository or brings it back. A hidden repository leaves the list, the counts and all
// background work (status reads, watching, fetch); nothing on disk changes.
func (e *Engine) SetHidden(ctx context.Context, id string, hidden bool) (api.ReposState, error) {
	if err := e.requireRepos([]string{id}); err != nil {
		return api.ReposState{}, err
	}
	// A row that waits for the review leaves it when the person hides it, and joins the list when they bring it
	// back: one registry write for the hide, both flags cleared for the unhide.
	if hidden {
		if err := e.reg.ResolvePending(nil, []string{id}); err != nil {
			return api.ReposState{}, regErr(err, false)
		}
	} else {
		if err := e.reg.ResolvePending([]string{id}, nil); err != nil {
			return api.ReposState{}, regErr(err, false)
		}
		if err := e.reg.SetHidden(id, false); err != nil {
			return api.ReposState{}, regErr(err, false)
		}
	}
	if hidden {
		e.forgetOffers(id)
		e.mu.Lock()
		delete(e.status, id)
		delete(e.opened, id)
		e.mu.Unlock()
		e.fillLastCommitsFor(ctx)
	}
	return e.changed(ctx), nil
}

// MoveToProject puts repositories into a project ("" takes them out of any project). Moving is a plain change
// of the record: the folders stay where they are.
func (e *Engine) MoveToProject(ctx context.Context, repoIDs []string, projectID string) (api.ReposState, error) {
	if err := e.requireRepos(repoIDs); err != nil {
		return api.ReposState{}, err
	}
	if projectID != "" && !e.hasProject(projectID) {
		return api.ReposState{}, api.NewError(api.ErrProjectNotFound, "project not found").WithParam("id", projectID)
	}
	for _, id := range repoIDs {
		if err := e.reg.SetProject(id, projectID); err != nil {
			return api.ReposState{}, regErr(err, false)
		}
	}
	if projectID != "" {
		e.forgetOffers(repoIDs...)
	}
	return e.changed(ctx), nil
}

func (e *Engine) hasProject(id string) bool {
	d := e.reg.Snapshot()
	return d.FindProject(id) != nil
}

func trimName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return "", api.NewError(api.ErrInvalidInput, "invalid project name").WithParam("field", "name")
	}
	return name, nil
}

// CreateProject makes an empty project and returns the new state.
func (e *Engine) CreateProject(ctx context.Context, name string) (api.ReposState, error) {
	name, err := trimName(name)
	if err != nil {
		return api.ReposState{}, err
	}
	if _, err := e.reg.AddProject(name); err != nil {
		return api.ReposState{}, regErr(err, false)
	}
	return e.changed(ctx), nil
}

// CreateProjectWith makes a project and puts the repositories into it in one step (the suggestion banner's
// "Create project initech").
func (e *Engine) CreateProjectWith(ctx context.Context, name string, repoIDs []string) (api.ReposState, error) {
	name, err := trimName(name)
	if err != nil {
		return api.ReposState{}, err
	}
	if err := e.requireRepos(repoIDs); err != nil {
		return api.ReposState{}, err
	}
	p, err := e.reg.AddProject(name)
	if err != nil {
		return api.ReposState{}, regErr(err, false)
	}
	return e.MoveToProject(ctx, repoIDs, p.ID)
}

func (e *Engine) RenameProject(ctx context.Context, id, name string) (api.ReposState, error) {
	name, err := trimName(name)
	if err != nil {
		return api.ReposState{}, err
	}
	if !e.hasProject(id) {
		return api.ReposState{}, api.NewError(api.ErrProjectNotFound, "project not found").WithParam("id", id)
	}
	if err := e.reg.RenameProject(id, name); err != nil {
		return api.ReposState{}, regErr(err, false)
	}
	return e.changed(ctx), nil
}

// RemoveProject deletes a project; its repositories become ungrouped. Nothing on disk is touched.
func (e *Engine) RemoveProject(ctx context.Context, id string) (api.ReposState, error) {
	if !e.hasProject(id) {
		return api.ReposState{}, api.NewError(api.ErrProjectNotFound, "project not found").WithParam("id", id)
	}
	if err := e.reg.RemoveProject(id); err != nil {
		return api.ReposState{}, regErr(err, false)
	}
	return e.changed(ctx), nil
}

func (e *Engine) SetProjectCollapsed(ctx context.Context, id string, collapsed bool) (api.ReposState, error) {
	if !e.hasProject(id) {
		return api.ReposState{}, api.NewError(api.ErrProjectNotFound, "project not found").WithParam("id", id)
	}
	if err := e.reg.SetProjectCollapsed(id, collapsed); err != nil {
		return api.ReposState{}, regErr(err, false)
	}
	return e.changed(ctx), nil
}

// DismissSuggestion remembers "no thanks" for a suggestion (State.Suggestions[].Key); it does not come back.
func (e *Engine) DismissSuggestion(ctx context.Context, key string) (api.ReposState, error) {
	if strings.TrimSpace(key) == "" {
		return api.ReposState{}, api.NewError(api.ErrInvalidInput, "invalid suggestion").WithParam("field", "key")
	}
	if err := e.reg.DismissSuggestion(key); err != nil {
		return api.ReposState{}, regErr(err, false)
	}
	return e.changed(ctx), nil
}

// fillLastCommitsFor reads the commit dates of rows that just became found or hidden.
func (e *Engine) fillLastCommitsFor(ctx context.Context) {
	if g, err := e.git(ctx); err == nil {
		e.fillLastCommits(ctx, g)
	}
}

// projectInfos lists every project in order with the number of listed repositories in it.
func projectInfos(d projects.Data, listed []api.Repo) []api.ProjectInfo {
	count := map[string]int{}
	for _, r := range listed {
		if r.ProjectID != "" {
			count[r.ProjectID]++
		}
	}
	out := make([]api.ProjectInfo, 0, len(d.Projects))
	for _, p := range d.Projects {
		out = append(out, api.ProjectInfo{ID: p.ID, Name: p.Name, Collapsed: p.Collapsed, Order: p.Order, RepoCount: count[p.ID]})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// namePrefix is the part of a folder name before its first "_", "-" or "."; "" when there is none or it is
// shorter than two characters.
func namePrefix(name string) string {
	i := strings.IndexAny(name, "_-.")
	if i < 2 {
		return ""
	}
	return name[:i]
}

// suggestionsFor proposes projects for listed repositories that are in none: three or more that share a name
// prefix, or three or more that share a parent folder (other than a scan folder itself, which every repository
// shares). Dismissed suggestions are left out. Prefix suggestions come first, larger groups first.
func suggestionsFor(d projects.Data, listed []api.Repo, roots []string) []api.ProjectSuggestion {
	dismissed := map[string]bool{}
	for _, k := range d.DismissedSuggestions {
		dismissed[k] = true
	}
	isRoot := map[string]bool{}
	for _, r := range roots {
		isRoot[filepath.Clean(r)] = true
	}
	type group struct {
		label string
		ids   []string
	}
	pre := map[string]*group{}
	dir := map[string]*group{}
	for _, r := range listed {
		if r.ProjectID != "" {
			continue
		}
		if p := namePrefix(r.LocalName); p != "" {
			k := strings.ToLower(p)
			if pre[k] == nil {
				pre[k] = &group{label: p}
			}
			pre[k].ids = append(pre[k].ids, r.ID)
		}
		parent := filepath.Dir(r.Path)
		if !isRoot[parent] && parent != filepath.Dir(parent) {
			if dir[parent] == nil {
				dir[parent] = &group{label: filepath.Base(parent)}
			}
			dir[parent].ids = append(dir[parent].ids, r.ID)
		}
	}
	collect := func(m map[string]*group, kind api.SuggestionKind) []api.ProjectSuggestion {
		var out []api.ProjectSuggestion
		for k, g := range m {
			key := string(kind) + ":" + k
			if len(g.ids) < minGroup || dismissed[key] {
				continue
			}
			sort.Strings(g.ids)
			out = append(out, api.ProjectSuggestion{Key: key, Kind: kind, Label: g.label, RepoIDs: g.ids})
		}
		sort.Slice(out, func(i, j int) bool {
			if len(out[i].RepoIDs) != len(out[j].RepoIDs) {
				return len(out[i].RepoIDs) > len(out[j].RepoIDs)
			}
			return out[i].Key < out[j].Key
		})
		return out
	}
	out := append(collect(pre, api.SuggestPrefix), collect(dir, api.SuggestFolder)...)
	if out == nil {
		out = []api.ProjectSuggestion{}
	}
	return out
}

// fillRecord adds what the registry knows about a repository (project, hidden flag, review state, moved-folder
// question) to a row that was built outside buildState (a bind outcome, a comparison result).
func (e *Engine) fillRecord(repo *api.Repo) {
	d := e.reg.Snapshot()
	if rr := d.FindRepo(repo.ID); rr != nil {
		if d.FindProject(rr.ProjectID) != nil {
			repo.ProjectID = rr.ProjectID
		}
		repo.Hidden, repo.Pending = rr.Hidden, rr.Pending
	}
	e.mu.Lock()
	mo, ok := e.moved[repo.ID]
	e.mu.Unlock()
	if ok {
		repo.MovedFrom = &api.MovedFrom{OldID: mo.OldID, Name: mo.Name}
	}
}
