package repos

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/redact"
)

const testTimeout = 20 * time.Second

// awsPath returns the aws binary or the error that explains why binding is unavailable.
func (e *Engine) awsPath() (string, error) {
	if e.d.AWSReady == nil || !e.d.AWSReady() {
		if e.d.AWSPath != nil {
			if _, err := e.d.AWSPath(); err != nil {
				if ae := api.AsAppError(err); ae.Code != api.ErrInternal {
					return "", ae
				}
				return "", api.NewError(api.ErrAWSCLIMissing, "the AWS CLI was not found")
			}
		}
		return "", api.NewError(api.ErrAWSCLITooOld, "the AWS CLI is too old").WithParam("version", "").WithParam("min", "2.9.0")
	}
	if e.d.AWSPath == nil {
		return "", api.NewError(api.ErrAWSCLIMissing, "the AWS CLI was not found")
	}
	p, err := e.d.AWSPath()
	if err != nil {
		return "", api.NewError(api.ErrAWSCLIMissing, "the AWS CLI was not found").WithDetail(err.Error())
	}
	return p, nil
}

// bindHosts collects the distinct CodeCommit HTTPS hosts of a repository's fetch and push URLs.
func bindHosts(sr gitx.ScannedRepo) []string {
	set := map[string]bool{}
	for _, r := range sr.Remotes {
		if r.Info.Kind == api.RemoteHTTPS {
			set[r.Info.Host] = true
		}
		if r.PushInfo != nil && r.PushInfo.Kind == api.RemoteHTTPS {
			set[r.PushInfo.Host] = true
		}
	}
	out := make([]string, 0, len(set))
	for h := range set {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

func (e *Engine) profileFor(ctx context.Context, name string) (ProfileInfo, error) {
	for _, p := range e.profiles(ctx) {
		if p.Name == name {
			return p, nil
		}
	}
	return ProfileInfo{}, api.NewError(api.ErrProfileNotFound, "profile not found").WithParam("name", name)
}

func accountOf(p ProfileInfo) string {
	if len(p.AccountID) == 12 {
		return p.AccountID
	}
	return ""
}

func (e *Engine) bindSpec(r *rec, p ProfileInfo, aws string) gitx.BindSpec {
	return gitx.BindSpec{
		RepoDir: r.sr.Path, Hosts: bindHosts(r.sr), Profile: p.Name, AccountID: accountOf(p), AWSPath: aws,
		Lang: e.d.Store.Lang(), GOOS: e.d.GOOS,
	}
}

func (e *Engine) checkBindable(r *rec) error {
	_, info, _ := primary(r.sr)
	switch info.Kind {
	case api.RemoteSSH:
		return api.NewError(api.ErrRemoteSSH, "SSH remotes cannot be bound; convert to HTTPS first")
	case api.RemoteGRC:
		return api.NewError(api.ErrRemoteGRC, "the remote URL already names the profile")
	}
	if len(bindHosts(r.sr)) == 0 {
		return api.NewError(api.ErrNotCodeCommit, "no CodeCommit HTTPS remote")
	}
	return nil
}

// PreviewBind describes what Bind would write and warns about side effects.
func (e *Engine) PreviewBind(ctx context.Context, id, profile string) (api.BindPreview, error) {
	r, err := e.rec(id)
	if err != nil {
		return api.BindPreview{}, err
	}
	if err := e.checkBindable(r); err != nil {
		return api.BindPreview{}, err
	}
	p, err := e.profileFor(ctx, profile)
	if err != nil {
		return api.BindPreview{}, err
	}
	aws, err := e.awsPath()
	if err != nil {
		return api.BindPreview{}, err
	}
	spec := e.bindSpec(r, p, aws)
	if _, err := gitx.BindArgs(spec, false); err != nil {
		return api.BindPreview{}, err
	}
	pv := api.BindPreview{RepoID: id, Profile: profile, Commands: gitx.DisplayCommands(spec), Warnings: []api.BindWarning{}}
	hasLocalAWS := false
	for _, en := range r.sr.Entries {
		if en.Scope == "local" && en.Var == "helper" && gitx.ParseAWSHelper(en.Value).IsAWS {
			hasLocalAWS = true
		}
	}
	if hasLocalAWS || r.cls.LegacyPlain {
		pv.Warnings = append(pv.Warnings, api.BindWarnReplaces)
	}
	if p.Kind == api.ProfileKindStatic {
		pv.Warnings = append(pv.Warnings, api.BindWarnStatic)
	}
	if p.NotSignedInPortal != "" {
		pv.Warnings = append(pv.Warnings, api.BindWarnSignedOut)
	}
	e.mu.Lock()
	owner := r.owner
	e.mu.Unlock()
	if owner.Status == api.OwnerSingle && len(owner.Candidates) == 1 && p.AccountID != "" && owner.Candidates[0].AccountID != p.AccountID {
		pv.Warnings = append(pv.Warnings, api.BindWarnOwnerMismatch)
	}
	return pv, nil
}

func (e *Engine) snapshotConfig(ctx context.Context, g *gitx.Git, r *rec, reason api.BackupReason) (string, error) {
	if e.d.Backup == nil {
		return "", nil
	}
	cfg, err := g.ConfigPath(ctx, r.sr.Path)
	if err != nil {
		return "", err
	}
	_, info, _ := primary(r.sr)
	entry, err := e.d.Backup.Snapshot(cfg, e.d.Paths.Display(cfg), api.BackupGitLocal, reason, map[string]string{"repo": info.Repo})
	if err != nil {
		return "", api.AsAppError(err)
	}
	return entry.ID, nil
}

func (e *Engine) outcome(ctx context.Context, id, backupID string) api.BindOutcome {
	pm := profileMap(e.profiles(ctx))
	e.mu.Lock()
	r, ok := e.recs[id]
	e.mu.Unlock()
	o := api.BindOutcome{RepoID: id, OK: true, BackupID: backupID}
	if ok {
		repo := e.buildRepo(r, pm, e.reg.LastTests())
		o.Repo = &repo
	}
	return o
}

func failed(id string, err error) api.BindOutcome {
	return api.BindOutcome{RepoID: id, Error: api.AsAppError(err)}
}

// bindOne performs a bind without emitting events.
func (e *Engine) bindOne(ctx context.Context, g *gitx.Git, id, profile string, backup bool, reason api.BackupReason) (api.BindOutcome, error) {
	aws, err := e.awsPath()
	if err != nil {
		return api.BindOutcome{}, err
	}
	r, err := e.rec(id)
	if err != nil {
		return api.BindOutcome{}, err
	}
	if err := e.checkBindable(r); err != nil {
		return api.BindOutcome{}, err
	}
	p, err := e.profileFor(ctx, profile)
	if err != nil {
		return api.BindOutcome{}, err
	}
	spec := e.bindSpec(r, p, aws)
	if _, err := gitx.BindArgs(spec, false); err != nil {
		return api.BindOutcome{}, err
	}
	backupID := ""
	if backup {
		if backupID, err = e.snapshotConfig(ctx, g, r, reason); err != nil {
			return api.BindOutcome{}, err
		}
	}
	if err := g.Bind(ctx, spec); err != nil {
		return api.BindOutcome{}, err
	}
	if err := e.refresh(ctx, g, id); err != nil {
		return api.BindOutcome{}, err
	}
	return e.outcome(ctx, id, backupID), nil
}

// Bind binds a repository to a profile.
func (e *Engine) Bind(ctx context.Context, id, profile string) (api.BindOutcome, error) {
	g, err := e.git(ctx)
	if err != nil {
		return api.BindOutcome{}, err
	}
	o, err := e.bindOne(ctx, g, id, profile, true, api.ReasonBind)
	if err != nil {
		return api.BindOutcome{}, api.AsAppError(err)
	}
	e.emitState(ctx)
	return o, nil
}

// BindMany binds several repositories; one failure does not stop the others.
func (e *Engine) BindMany(ctx context.Context, items []api.BindItem) ([]api.BindOutcome, error) {
	g, err := e.git(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := e.awsPath(); err != nil {
		return nil, err
	}
	out := make([]api.BindOutcome, 0, len(items))
	for _, it := range items {
		if ctx.Err() != nil {
			out = append(out, failed(it.RepoID, ctx.Err()))
			continue
		}
		o, err := e.bindOne(ctx, g, it.RepoID, it.Profile, true, api.ReasonBind)
		if err != nil {
			out = append(out, failed(it.RepoID, err))
			continue
		}
		out = append(out, o)
	}
	e.emitState(ctx)
	return out, nil
}

// Unbind removes the repository's local managed binding; it then follows whatever global configuration applies.
func (e *Engine) Unbind(ctx context.Context, id string) (api.BindOutcome, error) {
	g, err := e.git(ctx)
	if err != nil {
		return api.BindOutcome{}, err
	}
	r, err := e.rec(id)
	if err != nil {
		return api.BindOutcome{}, err
	}
	backupID, err := e.snapshotConfig(ctx, g, r, api.ReasonUnbind)
	if err != nil {
		return api.BindOutcome{}, err
	}
	if err := g.Unbind(ctx, r.sr.Path); err != nil {
		return api.BindOutcome{}, api.AsAppError(err)
	}
	if err := e.refresh(ctx, g, id); err != nil {
		return api.BindOutcome{}, api.AsAppError(err)
	}
	o := e.outcome(ctx, id, backupID)
	e.emitState(ctx)
	return o, nil
}

// Test checks the connection with `git ls-remote` and records the result.
func (e *Engine) Test(ctx context.Context, id string) (api.TestResult, error) {
	res, err := e.test(ctx, id)
	if err != nil {
		return api.TestResult{}, err
	}
	e.emitState(ctx)
	return res, nil
}

func (e *Engine) test(ctx context.Context, id string) (api.TestResult, error) {
	aws, err := e.awsPath()
	if err != nil {
		return api.TestResult{}, err
	}
	g, err := e.git(ctx)
	if err != nil {
		return api.TestResult{}, err
	}
	r, err := e.rec(id)
	if err != nil {
		return api.TestResult{}, err
	}
	_, info, _ := primary(r.sr)
	res := api.TestResult{RepoID: id, Status: api.TestError, At: e.now(), Profile: r.cls.Profile}
	switch {
	case info.Kind != api.RemoteHTTPS:
		// Only the canonical CodeCommit HTTPS URL is contacted; SSH and other remotes never reach git.
		res.Detail = "Only HTTPS CodeCommit remotes can be tested."
	case r.cls.Profile == "":
		res.Detail = "This repository is not bound to an AWS profile."
	default:
		// The test does not run inside the repository, so its local config cannot affect it.
		res.Status, res.Detail = g.TestRemote(ctx, info.HTTPSURL, r.cls.Profile, aws, e.d.GOOS, testTimeout)
	}
	if res.Status == api.TestSignedOut {
		for _, p := range e.profiles(ctx) {
			if p.Name == r.cls.Profile {
				res.Portal = p.Portal
				if p.NotSignedInPortal != "" {
					res.Portal = p.NotSignedInPortal
				}
			}
		}
	}
	_ = e.reg.SetLastTest(res.RepoID, res)
	return res, nil
}

// TestMany tests several repositories (at most four at a time); results keep the input order.
func (e *Engine) TestMany(ctx context.Context, ids []string) ([]api.TestResult, error) {
	if _, err := e.awsPath(); err != nil {
		return nil, err
	}
	out := make([]api.TestResult, len(ids))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, id string) {
			defer wg.Done()
			defer func() { <-sem }()
			res, err := e.test(ctx, id)
			if err != nil {
				ae := api.AsAppError(err)
				res = api.TestResult{RepoID: id, Status: api.TestError, At: e.now(), Detail: redact.String(ae.Message)}
			}
			out[i] = res
		}(i, id)
	}
	wg.Wait()
	e.emitState(ctx)
	return out, nil
}

// ConvertToHTTPS rewrites SSH remotes (and push URLs) to HTTPS and, when profile is given, binds the repository.
func (e *Engine) ConvertToHTTPS(ctx context.Context, id, profile string) (api.BindOutcome, error) {
	if profile != "" {
		if _, err := e.awsPath(); err != nil {
			return api.BindOutcome{}, err
		}
	}
	g, err := e.git(ctx)
	if err != nil {
		return api.BindOutcome{}, err
	}
	r, err := e.rec(id)
	if err != nil {
		return api.BindOutcome{}, err
	}
	rname, info, _ := primary(r.sr)
	if info.Kind == api.RemoteGRC {
		return api.BindOutcome{}, api.NewError(api.ErrRemoteGRC, "the remote URL already names the profile")
	}
	if profile != "" {
		if _, err := e.profileFor(ctx, profile); err != nil {
			return api.BindOutcome{}, err
		}
	}
	backupID, err := e.snapshotConfig(ctx, g, r, api.ReasonHTTPS)
	if err != nil {
		return api.BindOutcome{}, err
	}
	rm := r.sr.Remotes[r.sr.Primary]
	if info.Kind == api.RemoteSSH {
		if err := g.SetRemoteURL(ctx, r.sr.Path, rname, info.HTTPSURL, false); err != nil {
			return api.BindOutcome{}, api.AsAppError(err)
		}
	}
	if rm.PushInfo != nil && rm.PushInfo.Kind == api.RemoteSSH {
		if err := g.SetRemoteURL(ctx, r.sr.Path, rname, rm.PushInfo.HTTPSURL, true); err != nil {
			return api.BindOutcome{}, api.AsAppError(err)
		}
	}
	if err := e.refresh(ctx, g, id); err != nil {
		return api.BindOutcome{}, api.AsAppError(err)
	}
	if profile != "" {
		o, err := e.bindOne(ctx, g, id, profile, false, api.ReasonHTTPS)
		if err != nil {
			return api.BindOutcome{}, api.AsAppError(err)
		}
		o.BackupID = backupID
		e.emitState(ctx)
		return o, nil
	}
	o := e.outcome(ctx, id, backupID)
	e.emitState(ctx)
	return o, nil
}

// RepairAll re-binds every managed or legacy binding whose helper needs repair and whose profile is known.
func (e *Engine) RepairAll(ctx context.Context) ([]api.BindOutcome, error) {
	if _, err := e.awsPath(); err != nil {
		return nil, err
	}
	g, err := e.git(ctx)
	if err != nil {
		return nil, err
	}
	pm := profileMap(e.profiles(ctx))
	type job struct{ id, profile string }
	var jobs []job
	e.mu.Lock()
	for _, r := range e.recs {
		if r.cls.Source == api.BindingLocal && (r.cls.NeedsRepair || r.cls.LegacyPlain) {
			if _, ok := pm[r.cls.Profile]; ok && r.cls.Profile != "" {
				jobs = append(jobs, job{r.id, r.cls.Profile})
			}
		}
	}
	e.mu.Unlock()
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].id < jobs[j].id })
	out := make([]api.BindOutcome, 0, len(jobs))
	for _, j := range jobs {
		o, err := e.bindOne(ctx, g, j.id, j.profile, true, api.ReasonRepair)
		if err != nil {
			out = append(out, failed(j.id, err))
			continue
		}
		out = append(out, o)
	}
	e.emitState(ctx)
	return out, nil
}
