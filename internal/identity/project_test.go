package identity

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/projects"
	"github.com/rudian/codepass/internal/textdiff"
)

type acmeSet struct {
	p                     projects.Project
	plain, own, ext, same projects.RepoRecord // no local value / own override / set outside the app / already equal
	gone                  projects.RepoRecord
}

func newAcme(t *testing.T) (*rig, acmeSet) {
	r := newRig(t)
	r.setGlobal("Alex Global", "global@example.com")
	work := filepath.Join(r.root, "work")
	s := acmeSet{
		plain: r.repo(work, "acme_api"), own: r.repo(work, "acme_agent_portal"), ext: r.repo(work, "acme_cs_portal"),
		same: r.repo(work, "acme_merchant_portal_v4"), gone: r.repo(work, "acme_gone"),
	}
	r.gitIn(s.ext.Path, "config", "user.name", "Ext")
	r.gitIn(s.ext.Path, "config", "user.email", "ext@example.com")
	r.gitIn(s.same.Path, "config", "user.name", acme().Name)
	r.gitIn(s.same.Path, "config", "user.email", acme().Email)
	s.p = r.project("acme", s.plain, s.own, s.ext, s.same, s.gone)
	// the override of `own`
	req := api.RepoIdentityRequest{RepoID: s.own.ID, Mode: api.RepoModeOwn, Identity: ptr(globex())}
	pv, err := r.eng.PreviewRepo(bg, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.eng.ApplyRepo(bg, req, pv.PlanHash); err != nil {
		t.Fatal(err)
	}
	if err := removeAll(s.gone.Path); err != nil {
		t.Fatal(err)
	}
	return r, s
}

func actions(pl []api.RepoChangePlan) map[string]api.RepoAction {
	m := map[string]api.RepoAction{}
	for _, p := range pl {
		m[p.Name] = p.Action
	}
	return m
}

func TestProjectPreviewEqualsApply(t *testing.T) {
	r, s := newAcme(t)
	req := api.ProjectIdentityRequest{ProjectID: s.p.ID, Identity: ptr(acme())}
	snap := map[string]string{}
	for _, rec := range []projects.RepoRecord{s.plain, s.own, s.ext, s.same} {
		snap[rec.Path] = r.read(r.localCfg(rec))
	}
	pv, err := r.eng.PreviewProject(bg, req)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]api.RepoAction{
		"acme_api": api.RepoActionSet, "acme_agent_portal": api.RepoActionKeepOwn, "acme_cs_portal": api.RepoActionKeepExternal,
		"acme_merchant_portal_v4": api.RepoActionUnchanged, "acme_gone": api.RepoActionSkipMissing,
	}
	if got := actions(pv.Repos); !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	if pv.Writes != 1 || pv.KeepsOwn != 2 || pv.Identity == nil || pv.Previous != nil {
		t.Fatalf("%+v", pv)
	}
	// the repositories that keep their own setting come last
	n := len(pv.Repos)
	if a := pv.Repos[n-1].Action; a != api.RepoActionKeepOwn && a != api.RepoActionKeepExternal && a != api.RepoActionSkipMissing {
		t.Fatalf("order: %v", actions(pv.Repos))
	}
	// preview changes nothing
	for _, rec := range []projects.RepoRecord{s.plain, s.own, s.ext, s.same} {
		if r.read(r.localCfg(rec)) != snap[rec.Path] {
			t.Fatalf("preview wrote %s", rec.Path)
		}
	}
	// a stale hash is refused
	if _, err := r.eng.ApplyProject(bg, req, "stale"); api.AsAppError(err).Code != api.ErrConfigChanged {
		t.Fatalf("%v", err)
	}
	res, err := r.eng.ApplyProject(bg, req, pv.PlanHash)
	if err != nil || !res.RecordUpdated {
		t.Fatalf("%v %+v", err, res)
	}
	for _, o := range res.Outcomes {
		if !o.OK {
			t.Fatalf("%+v", o)
		}
	}
	// what apply wrote is the preview, line by line, for every repository that changed
	for _, pl := range pv.Repos {
		switch pl.Action {
		case api.RepoActionSet, api.RepoActionRemove:
			if pl.Preview == nil {
				t.Fatalf("%s: no preview", pl.Name)
			}
			after := r.read(filepath.Join(pl.Path, ".git", "config"))
			lines, add, rem := textdiff.Diff([]byte(snap[pl.Path]), []byte(after), textdiff.DefaultContext)
			if !reflect.DeepEqual(lines, pl.Preview.Lines) || add != pl.Preview.Added || rem != pl.Preview.Removed {
				t.Fatalf("%s: preview != write\n%v\n%v", pl.Name, lines, pl.Preview.Lines)
			}
			if pl.Preview.BaseHash != sha([]byte(snap[pl.Path])) {
				t.Fatalf("%s: base hash", pl.Name)
			}
		default:
			if pl.Preview != nil {
				t.Fatalf("%s: unexpected preview", pl.Name)
			}
			if pl.Action == api.RepoActionSkipMissing {
				continue
			}
			if got := r.read(filepath.Join(pl.Path, ".git", "config")); got != snap[pl.Path] {
				t.Fatalf("%s (%s) was written", pl.Name, pl.Action)
			}
		}
	}
	if got := r.gitIn(s.plain.Path, "config", "--local", "user.email"); got != acme().Email {
		t.Fatal(got)
	}
	if r.snap().FindProject(s.p.ID).Identity == nil {
		t.Fatal("the project record must hold the identity")
	}
	if len(r.undoFiles(res)) != 1 || len(res.BackupIDs) != 1 {
		t.Fatalf("exactly one config changed: %+v", res)
	}
	ents, _ := r.bk.List()
	found := false
	for _, e := range ents {
		if e.Kind == api.BackupGitLocal && e.Reason == api.ReasonIdentity && e.ReasonParams["repo"] == "acme_api" {
			found = true
		}
	}
	if !found {
		t.Fatalf("%+v", ents)
	}

	// a second apply of the same request is a no-op
	pv2, _ := r.eng.PreviewProject(bg, req)
	if pv2.Writes != 0 {
		t.Fatalf("%+v", actions(pv2.Repos))
	}

	// undo: configs and the record are back
	if _, err := r.eng.Undo(bg, res.UndoID); err != nil {
		t.Fatal(err)
	}
	for _, rec := range []projects.RepoRecord{s.plain, s.own, s.ext, s.same} {
		if r.read(r.localCfg(rec)) != snap[rec.Path] {
			t.Fatalf("undo did not restore %s", rec.Path)
		}
	}
	if r.snap().FindProject(s.p.ID).Identity != nil {
		t.Fatal("the project record must be back")
	}
}

func TestProjectChangeAndClear(t *testing.T) {
	r, s := newAcme(t)
	apply := func(id *api.Author) api.IdentityChangeResult {
		t.Helper()
		req := api.ProjectIdentityRequest{ProjectID: s.p.ID, Identity: id}
		pv, err := r.eng.PreviewProject(bg, req)
		if err != nil {
			t.Fatal(err)
		}
		res, err := r.eng.ApplyProject(bg, req, pv.PlanHash)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	apply(ptr(acme()))
	// change: the repository the app wrote follows, the equal one follows, the external one stays
	apply(ptr(globex()))
	for _, rec := range []projects.RepoRecord{s.plain, s.same} {
		if got := r.gitIn(rec.Path, "config", "--local", "user.email"); got != globex().Email {
			t.Fatalf("%s: %s", rec.Path, got)
		}
	}
	if got := r.gitIn(s.ext.Path, "config", "--local", "user.email"); got != "ext@example.com" {
		t.Fatal(got)
	}
	// clear: the repositories that held the project's value lose it, others stay
	apply(nil)
	for _, rec := range []projects.RepoRecord{s.plain, s.same} {
		cfg := r.read(r.localCfg(rec))
		if strings.Contains(cfg, "user") || strings.Contains(cfg, "email") {
			t.Fatalf("%s still has an identity:\n%s", rec.Path, cfg)
		}
	}
	if got := r.gitIn(s.ext.Path, "config", "--local", "user.email"); got != "ext@example.com" {
		t.Fatal(got)
	}
	if row := r.row(r.overview(), s.plain.ID); row.Effective.Source.Kind != api.IdentitySrcGlobal {
		t.Fatalf("%+v", row.Effective)
	}
	// make the external one follow: a repository change
	req := api.RepoIdentityRequest{RepoID: s.ext.ID, Mode: api.RepoModeProject}
	pv, err := r.eng.PreviewRepo(bg, req)
	if err != nil || pv.Plan.Action != api.RepoActionRemove {
		t.Fatalf("%v %+v", err, pv.Plan)
	}
	if _, err := r.eng.ApplyRepo(bg, req, pv.PlanHash); err != nil {
		t.Fatal(err)
	}
	if row := r.row(r.overview(), s.ext.ID); row.Effective.Source.Kind != api.IdentitySrcGlobal {
		t.Fatalf("%+v", row.Effective)
	}
}

func TestRepositoryOverrideLifecycle(t *testing.T) {
	r, s := newAcme(t)
	// own: Alex G; now follow global is refused while the project has an identity
	proj := api.ProjectIdentityRequest{ProjectID: s.p.ID, Identity: ptr(acme())}
	ppv, _ := r.eng.PreviewProject(bg, proj)
	if _, err := r.eng.ApplyProject(bg, proj, ppv.PlanHash); err != nil {
		t.Fatal(err)
	}
	// follow global is offered even while the project has an identity (design 5.12): the local values go and a
	// marker keeps project saves away from the repository; undo puts the own identity back
	gl := api.RepoIdentityRequest{RepoID: s.own.ID, Mode: api.RepoModeGlobal}
	gpv, err := r.eng.PreviewRepo(bg, gl)
	if err != nil || gpv.Plan.Action != api.RepoActionRemove || gpv.Plan.After != nil {
		t.Fatalf("%v %+v", err, gpv.Plan)
	}
	ownCfg := r.read(r.localCfg(s.own))
	gres, err := r.eng.ApplyRepo(bg, gl, gpv.PlanHash)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.read(r.localCfg(s.own)), "email") {
		t.Fatal("local values must be removed")
	}
	row := r.row(r.overview(), s.own.ID)
	if !row.FollowsGlobal || row.Override != nil || row.Effective.Source.Kind != api.IdentitySrcGlobal {
		t.Fatalf("%+v", row)
	}
	again, err := r.eng.PreviewProject(bg, api.ProjectIdentityRequest{ProjectID: s.p.ID, Identity: ptr(globex())})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range again.Repos {
		if p.RepoID == s.own.ID && p.Action != api.RepoActionKeepOwn {
			t.Fatalf("a repository that follows global is left alone by project saves: %+v", p)
		}
	}
	if _, err := r.eng.Undo(bg, gres.UndoID); err != nil {
		t.Fatal(err)
	}
	if r.read(r.localCfg(s.own)) != ownCfg || r.snap().FindRepo(s.own.ID).IdentityOverride == nil || r.snap().FindRepo(s.own.ID).IdentityOverride.Email != globex().Email {
		t.Fatal("undo must bring the own identity back")
	}
	if _, err := r.eng.PreviewRepo(bg, api.RepoIdentityRequest{RepoID: s.own.ID, Mode: api.RepoModeOwn}); api.AsAppError(err).Params["field"] != "identity" {
		t.Fatalf("%v", err)
	}
	// follow the project: the override goes, the project's value is written
	req := api.RepoIdentityRequest{RepoID: s.own.ID, Mode: api.RepoModeProject}
	pv, err := r.eng.PreviewRepo(bg, req)
	if err != nil || pv.Plan.Action != api.RepoActionSet || pv.Plan.After.Email != acme().Email || pv.Plan.Before.Email != globex().Email {
		t.Fatalf("%v %+v", err, pv.Plan)
	}
	before := r.read(r.localCfg(s.own))
	res, err := r.eng.ApplyRepo(bg, req, pv.PlanHash)
	if err != nil {
		t.Fatal(err)
	}
	if r.snap().FindRepo(s.own.ID).IdentityOverride != nil {
		t.Fatal("override must be cleared")
	}
	if row := r.row(r.overview(), s.own.ID); row.Effective.Source.Kind != api.IdentitySrcFromProject {
		t.Fatalf("%+v", row.Effective)
	}
	// a stale hash after an outside edit
	if _, err := r.eng.ApplyRepo(bg, req, "stale"); api.AsAppError(err).Code != api.ErrConfigChanged {
		t.Fatal(err)
	}
	if _, err := r.eng.Undo(bg, res.UndoID); err != nil {
		t.Fatal(err)
	}
	if r.read(r.localCfg(s.own)) != before || r.snap().FindRepo(s.own.ID).IdentityOverride == nil {
		t.Fatal("undo must restore the config and the override")
	}
}

func TestRepoConfigWriteFailureIsReportedPerRepo(t *testing.T) {
	r, s := newAcme(t)
	// a stale lock file makes git refuse to write this repository's config
	r.write(r.localCfg(s.plain)+".lock", "")
	req := api.ProjectIdentityRequest{ProjectID: s.p.ID, Identity: ptr(acme())}
	pv, err := r.eng.PreviewProject(bg, req)
	if err != nil {
		t.Fatal(err)
	}
	before := r.read(r.localCfg(s.plain))
	res, err := r.eng.ApplyProject(bg, req, pv.PlanHash)
	if err != nil {
		t.Fatal(err)
	}
	var bad *api.RepoChangeOutcome
	for i, o := range res.Outcomes {
		if !o.OK {
			bad = &res.Outcomes[i]
		}
	}
	if bad == nil || bad.Name != "acme_api" || bad.Error == nil || bad.Error.Code != api.ErrFileIO || !strings.Contains(bad.Error.Params["file"], "config") {
		t.Fatalf("%+v", res.Outcomes)
	}
	if res.RecordUpdated || r.snap().FindProject(s.p.ID).Identity != nil {
		t.Fatal("the project's own record must not change while a write failed")
	}
	if r.read(r.localCfg(s.plain)) != before {
		t.Fatal("a failed write must leave the file as it was")
	}
}

func TestMoveInAndOut(t *testing.T) {
	r := newRig(t)
	r.setGlobal("Alex Global", "global@example.com")
	work := filepath.Join(r.root, "work")
	repo := r.repo(work, "acme_api")
	pa := r.project("acme")
	pg := r.project("globex")
	for _, c := range []struct {
		p  projects.Project
		id api.Author
	}{{pa, acme()}, {pg, globex()}} {
		req := api.ProjectIdentityRequest{ProjectID: c.p.ID, Identity: ptr(c.id)}
		pv, _ := r.eng.PreviewProject(bg, req)
		if _, err := r.eng.ApplyProject(bg, req, pv.PlanHash); err != nil {
			t.Fatal(err)
		}
	}
	move := func(to string) api.IdentityChangeResult {
		t.Helper()
		pv, err := r.eng.PreviewMove(bg, repo.ID, to)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.eng.ApplyMove(bg, repo.ID, to, "stale"); api.AsAppError(err).Code != api.ErrConfigChanged {
			t.Fatalf("%v", err)
		}
		res, err := r.eng.ApplyMove(bg, repo.ID, to, pv.PlanHash)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	local := func() string {
		out := r.read(r.localCfg(repo))
		return out
	}
	// in: takes the project's identity
	move(pa.ID)
	if !strings.Contains(local(), acme().Email) || r.snap().FindRepo(repo.ID).ProjectID != pa.ID {
		t.Fatalf("move in:\n%s", local())
	}
	// between projects: follows the new one
	res := move(pg.ID)
	if !strings.Contains(local(), globex().Email) || strings.Contains(local(), acme().Email) {
		t.Fatalf("move between:\n%s", local())
	}
	// undo restores the membership and the file
	if _, err := r.eng.Undo(bg, res.UndoID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(local(), acme().Email) || r.snap().FindRepo(repo.ID).ProjectID != pa.ID {
		t.Fatalf("undo:\n%s", local())
	}
	// out: loses the project's value
	move("")
	if strings.Contains(local(), "email") || r.snap().FindRepo(repo.ID).ProjectID != "" {
		t.Fatalf("move out:\n%s", local())
	}
	// a repository with its own identity keeps it through every move
	req := api.RepoIdentityRequest{RepoID: repo.ID, Mode: api.RepoModeOwn, Identity: ptr(api.Author{Name: "Own", Email: "own@example.com"})}
	pv, _ := r.eng.PreviewRepo(bg, req)
	if _, err := r.eng.ApplyRepo(bg, req, pv.PlanHash); err != nil {
		t.Fatal(err)
	}
	mp, err := r.eng.PreviewMove(bg, repo.ID, pa.ID)
	if err != nil || mp.Plan.Action != api.RepoActionKeepOwn {
		t.Fatalf("%v %+v", err, mp.Plan)
	}
	if _, err := r.eng.ApplyMove(bg, repo.ID, pa.ID, mp.PlanHash); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(local(), "own@example.com") {
		t.Fatal("own identity lost")
	}
	// a value set outside the app stays too
	r.gitIn(repo.Path, "config", "user.email", "hand@example.com")
	r.gitIn(repo.Path, "config", "user.name", "Hand")
	reqP := api.RepoIdentityRequest{RepoID: repo.ID, Mode: api.RepoModeProject}
	pp, _ := r.eng.PreviewRepo(bg, reqP)
	if _, err := r.eng.ApplyRepo(bg, reqP, pp.PlanHash); err != nil {
		t.Fatal(err)
	}
	mp, _ = r.eng.PreviewMove(bg, repo.ID, pg.ID)
	// after "follow project" the repository holds the acme project's value, which is not globex's: it follows
	if mp.Plan.Action != api.RepoActionSet || mp.Plan.After == nil || mp.Plan.After.Email != globex().Email {
		t.Fatalf("%+v", mp.Plan)
	}
	r.gitIn(repo.Path, "config", "user.email", "hand2@example.com")
	mp, _ = r.eng.PreviewMove(bg, repo.ID, pg.ID)
	if mp.Plan.Action != api.RepoActionKeepExternal {
		t.Fatalf("%+v", mp.Plan)
	}
	if _, err := r.eng.PreviewMove(bg, repo.ID, "p_nope"); api.AsAppError(err).Params["field"] != "projectId" {
		t.Fatal(err)
	}
}

func TestFollowAction(t *testing.T) {
	a, b, c := ptr(acme()), ptr(globex()), ptr(api.Author{Name: "X", Email: "x@example.com"})
	for _, tc := range []struct {
		name              string
		local, prev, next *api.Author
		want              api.RepoAction
	}{
		{"nothing to do", nil, nil, nil, api.RepoActionUnchanged},
		{"no local, project set", nil, nil, a, api.RepoActionSet},
		{"already equal", a, nil, a, api.RepoActionUnchanged},
		{"was project, changes", a, a, b, api.RepoActionSet},
		{"was project, cleared", a, a, nil, api.RepoActionRemove},
		{"outside the app", c, a, b, api.RepoActionKeepExternal},
		{"outside the app, cleared", c, a, nil, api.RepoActionKeepExternal},
		{"outside the app, no project before", c, nil, a, api.RepoActionKeepExternal},
		{"partial local", &api.Author{Name: "Alex"}, a, b, api.RepoActionKeepExternal},
	} {
		if got, _ := followAction(tc.local, tc.prev, tc.next); got != tc.want {
			t.Errorf("%s: got %s want %s", tc.name, got, tc.want)
		}
	}
}

// FU-14h: a move into a project and an identity of its own tell the caller which repository now carries data, so
// the repositories engine drops a pending "moved here?" question about it.
func TestMoveAndOwnIdentityTellWhoCarriesData(t *testing.T) {
	var told []string
	r := newRig(t, func(d *Deps) { d.OnRecordData = func(ids ...string) { told = append(told, ids...) } })
	r.setGlobal("Alex Global", "global@example.com")
	repo := r.repo(filepath.Join(r.root, "work"), "acme_api")
	pa := r.project("acme")
	pv, err := r.eng.PreviewMove(bg, repo.ID, pa.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.eng.ApplyMove(bg, repo.ID, pa.ID, pv.PlanHash); err != nil {
		t.Fatal(err)
	}
	if len(told) != 1 || told[0] != repo.ID {
		t.Fatalf("told %v after a move into a project", told)
	}
	// out of every project: it carries no project data, nobody is told
	told = nil
	pv, _ = r.eng.PreviewMove(bg, repo.ID, "")
	if _, err := r.eng.ApplyMove(bg, repo.ID, "", pv.PlanHash); err != nil || len(told) != 0 {
		t.Fatalf("%v told %v", err, told)
	}
	req := api.RepoIdentityRequest{RepoID: repo.ID, Mode: api.RepoModeOwn, Identity: ptr(globex())}
	rv, err := r.eng.PreviewRepo(bg, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.eng.ApplyRepo(bg, req, rv.PlanHash); err != nil || len(told) != 1 || told[0] != repo.ID {
		t.Fatalf("%v told %v after an identity of its own", err, told)
	}
}
