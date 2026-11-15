package repos

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/codecommit"
	"github.com/rudian/codepass/internal/gitx"
)

type accountGroup struct {
	key      string // account id, or "profile:<name>" for static profiles of unknown account
	profiles []ProfileInfo
}

func (g accountGroup) names() []string {
	out := make([]string, 0, len(g.profiles))
	for _, p := range g.profiles {
		out = append(out, p.Name)
	}
	sort.Strings(out)
	return out
}

// preferred is the first usable profile alphabetically, else the first profile alphabetically.
func (g accountGroup) preferred() ProfileInfo {
	ps := append([]ProfileInfo(nil), g.profiles...)
	sort.Slice(ps, func(i, j int) bool { return ps[i].Name < ps[j].Name })
	for _, p := range ps {
		if p.Usable {
			return p
		}
	}
	return ps[0]
}

// suggested orders the account's profiles for binding: the profile the repository is bound to already, then SSO
// before static keys, then the profile that already carries the most bound repositories, then the name. An SSO
// profile that is not signed in can still be suggested because binding does not need a login.
func (g accountGroup) suggested(bound string, counts map[string]int) ProfileInfo {
	ps := append([]ProfileInfo(nil), g.profiles...)
	rank := func(p ProfileInfo) int {
		if p.Kind == api.ProfileKindSSO {
			return 0
		}
		return 1
	}
	sort.SliceStable(ps, func(i, j int) bool {
		a, b := ps[i], ps[j]
		if (a.Name == bound) != (b.Name == bound) {
			return a.Name == bound
		}
		if rank(a) != rank(b) {
			return rank(a) < rank(b)
		}
		if counts[a.Name] != counts[b.Name] {
			return counts[a.Name] > counts[b.Name]
		}
		return a.Name < b.Name
	})
	return ps[0]
}

// boundProfile is the profile the repository is bound to (a local binding or a git-remote-codecommit URL);
// following a global default does not count as being bound.
func boundProfile(r *rec) string {
	switch r.cls.Source {
	case api.BindingLocal, api.BindingRemoteURL:
		return r.cls.Profile
	}
	return ""
}

// localBoundCountsLocked counts, per profile, the repositories bound to it. The caller holds e.mu.
func (e *Engine) localBoundCountsLocked() map[string]int {
	out := map[string]int{}
	for _, r := range e.recs {
		if p := boundProfile(r); p != "" {
			out[p]++
		}
	}
	return out
}

func groupProfiles(ps []ProfileInfo) []accountGroup {
	idx := map[string]int{}
	var groups []accountGroup
	for _, p := range ps {
		key := p.AccountID
		if key == "" {
			key = "profile:" + p.Name
		}
		i, ok := idx[key]
		if !ok {
			i = len(groups)
			idx[key] = i
			groups = append(groups, accountGroup{key: key})
		}
		groups[i].profiles = append(groups[i].profiles, p)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].key < groups[j].key })
	return groups
}

func (g accountGroup) toAccount() codecommit.Account {
	pref := g.preferred()
	a := codecommit.Account{AccountID: g.key, Profiles: g.names(), Preferred: pref.Name, Provider: pref.Provider, Usable: pref.Usable}
	for _, p := range g.profiles {
		if !p.Usable && p.NotSignedInPortal != "" {
			a.Portal = p.NotSignedInPortal
			break
		}
	}
	return a
}

// DetectOwners finds out which AWS account owns each repository. ids == nil means every
// repository that needs fixing and has an https or ssh remote.
func (e *Engine) DetectOwners(ctx context.Context, ids []string) (api.ReposState, error) {
	if e.d.CodeCommit == nil {
		return api.ReposState{}, api.NewError(api.ErrInternal, "no CodeCommit client configured")
	}
	pm := profileMap(e.profiles(ctx))
	type target struct {
		rec  *rec
		info gitx.RemoteInfo
	}
	var targets []target
	e.mu.Lock()
	pick := func(r *rec, force bool) {
		_, info, _ := primary(r.sr)
		if info.Kind != api.RemoteHTTPS && info.Kind != api.RemoteSSH {
			return
		}
		if !force {
			// Already fine (managed local binding with a known profile): nothing to detect.
			if _, known := pm[r.cls.Profile]; known && r.cls.Source == api.BindingLocal && !r.cls.LegacyPlain && !r.cls.NeedsRepair {
				return
			}
		}
		targets = append(targets, target{r, info})
	}
	if ids == nil {
		for _, r := range e.recs {
			pick(r, false)
		}
	} else {
		for _, id := range ids {
			if r, ok := e.recs[id]; ok {
				pick(r, true)
			}
		}
	}
	for _, t := range targets {
		t.rec.owner = api.OwnerResult{Status: api.OwnerChecking, Candidates: []api.OwnerCandidate{}, SkippedPortals: []string{}}
	}
	e.detecting = true
	e.mu.Unlock()
	e.emitState(ctx)

	groups := groupProfiles(e.profiles(ctx))
	accounts := make([]codecommit.Account, 0, len(groups))
	byAccount := map[string]accountGroup{}
	for _, g := range groups {
		accounts = append(accounts, g.toAccount())
		byAccount[g.key] = g
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].rec.id < targets[j].rec.id })
	qs := make([]codecommit.Query, 0, len(targets))
	for _, t := range targets {
		qs = append(qs, codecommit.Query{Key: t.rec.id, Region: t.info.Region, Name: t.info.Repo})
	}
	results := codecommit.Detect(ctx, qs, accounts, e.d.CodeCommit)

	checkedAt := e.now()
	e.mu.Lock()
	boundCounts := e.localBoundCountsLocked()
	for _, t := range targets {
		res := results[t.rec.id]
		o := api.OwnerResult{
			Candidates: make([]api.OwnerCandidate, 0, len(res.Candidates)), CheckedWith: res.Checked, Skipped: res.Skipped,
			SkippedPortals: append([]string{}, res.SkippedPortals...), CheckedAt: checkedAt,
		}
		for _, c := range res.Candidates {
			oc := api.OwnerCandidate{AccountID: c.AccountID, Profiles: []string{}, KMSWarning: c.KMSWarning}
			if g, ok := byAccount[c.AccountID]; ok {
				oc.Profiles = g.names()
				sug := g.suggested(boundProfile(t.rec), boundCounts)
				oc.Profile = sug.Name
				oc.ColorSlot, oc.Monogram = sug.ColorSlot, sug.Monogram
			}
			o.Candidates = append(o.Candidates, oc)
		}
		switch {
		case len(o.Candidates) == 1:
			o.Status = api.OwnerSingle
		case len(o.Candidates) >= 2:
			o.Status = api.OwnerAmbiguous
		case res.Skipped > 0:
			o.Status = api.OwnerPartial
		default:
			o.Status = api.OwnerNone
		}
		t.rec.owner = o
	}
	e.detecting = false
	e.mu.Unlock()
	return e.emitState(ctx), nil
}

const compareTimeout = 20 * time.Second

func (e *Engine) remoteURL(region, name string) string {
	if e.d.RemoteURLFor != nil {
		return e.d.RemoteURLFor(region, name)
	}
	return "https://" + hostFor(region) + "/v1/repos/" + name
}

// CompareCommits scores each candidate account by how many local remote-tracking branch tips its repository
// has at the same commit. It returns the repository unchanged when fewer than two candidates exist.
func (e *Engine) CompareCommits(ctx context.Context, id string) (api.Repo, error) {
	r, err := e.rec(id)
	if err != nil {
		return api.Repo{}, err
	}
	pm := profileMap(e.profiles(ctx))
	e.mu.Lock()
	owner := r.owner
	e.mu.Unlock()
	if len(owner.Candidates) < 2 {
		return e.buildRepo(r, pm, e.reg.LastTests()), nil
	}
	aws, err := e.awsPath()
	if err != nil {
		return api.Repo{}, err
	}
	g, err := e.git(ctx)
	if err != nil {
		return api.Repo{}, err
	}
	rname, info, _ := primary(r.sr)
	local, err := g.RemoteTrackingRefs(ctx, r.sr.Path, rname)
	if err != nil {
		return api.Repo{}, api.AsAppError(err)
	}
	scores := make([]int, len(owner.Candidates))
	for i, c := range owner.Candidates {
		url := e.remoteURL(info.Region, info.Repo)
		helper, herr := gitx.AWSHelperValue(aws, c.Profile, e.d.GOOS)
		if herr != nil {
			continue
		}
		refs, res, lerr := g.LsRemote(ctx, url, []string{
			"credential.helper=", "credential.helper=" + helper, "credential.useHttpPath=true", "credential.interactive=false",
		}, compareTimeout)
		if lerr != nil || res.ExitCode != 0 {
			continue
		}
		for ref, sha := range refs {
			if b, ok := strings.CutPrefix(ref, "refs/heads/"); ok && local[b] == sha {
				scores[i]++
			}
		}
	}
	best, bestIdx, tie := 0, -1, false
	for i, s := range scores {
		switch {
		case s > best:
			best, bestIdx, tie = s, i, false
		case s == best && s > 0:
			tie = true
		}
	}
	e.mu.Lock()
	cands := make([]api.OwnerCandidate, len(owner.Candidates))
	copy(cands, owner.Candidates)
	for i := range cands {
		cands[i].MatchScore = scores[i]
		cands[i].HeadMatch = !tie && i == bestIdx && best > 0
	}
	r.owner.Candidates = cands
	e.mu.Unlock()
	st := e.emitState(ctx)
	for _, rp := range st.Repos {
		if rp.ID == id {
			return rp, nil
		}
	}
	return api.Repo{}, api.NewError(api.ErrRepoNotFound, "repository not found").WithParam("path", id)
}
