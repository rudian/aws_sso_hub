package accounts

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/awsapi"
	"github.com/rudian/aws_sso_hub/internal/awsconfig"
	"github.com/rudian/aws_sso_hub/internal/identity"
	"github.com/rudian/aws_sso_hub/internal/redact"
	"github.com/rudian/aws_sso_hub/internal/sso"
)

func invalid(field string) *api.AppError {
	return api.NewError(api.ErrInvalidInput, "invalid input").WithParam("field", field)
}

// CheckPortalInput validates portal input without side effects.
func (e *Engine) CheckPortalInput(in api.PortalInput) api.PortalInputCheck {
	out := api.PortalInputCheck{Name: strings.TrimSpace(in.Name), Region: strings.TrimSpace(in.Region), Errors: map[string]api.ErrorCode{}}
	norm, guessed, nerr := awsconfig.NormalizeStartURL(in.StartURL)
	if nerr != nil {
		out.StartURL = strings.TrimSpace(in.StartURL)
		out.Errors["startUrl"] = nerr.Code
	} else {
		out.StartURL = norm
	}
	if out.Region == "" {
		out.Region = guessed
	}
	if !awsconfig.RegionRE.MatchString(out.Region) {
		out.Errors["region"] = api.ErrInvalidRegion
	}
	if out.Name == "" && nerr == nil {
		out.Name = awsconfig.SuggestSessionName(norm)
		if out.Name == "" {
			out.Errors["name"] = api.ErrInvalidName
		}
	}
	if out.Name != "" {
		if !awsconfig.SessionNameRE.MatchString(out.Name) {
			out.Errors["name"] = api.ErrInvalidName
		} else if e.portalNameTaken(out.Name) {
			out.Errors["name"] = api.ErrPortalExists
		}
	}
	return out
}

func (e *Engine) portalNameTaken(name string) bool {
	if e.IsDraft(name) {
		return true
	}
	if snap, err := e.Snapshot(); err == nil {
		_, ok := snap.Session(name)
		return ok
	}
	return false
}

func firstPortalError(c api.PortalInputCheck) *api.AppError {
	for _, k := range []string{"startUrl", "region", "name"} {
		if code, ok := c.Errors[k]; ok {
			ae := api.NewError(code, "invalid portal input")
			if code == api.ErrPortalExists {
				ae.WithParam("portal", c.Name)
			}
			return ae
		}
	}
	return nil
}

// AddDraft keeps a portal in memory so the user can sign in before anything is written to ~/.aws/config.
func (e *Engine) AddDraft(in api.PortalInput) (api.Portal, error) {
	c := e.CheckPortalInput(in)
	if ae := firstPortalError(c); ae != nil {
		return api.Portal{}, ae
	}
	s := awsconfig.Session{Name: c.Name, StartURL: c.StartURL, Region: c.Region, Scopes: []string{"sso:account:access"}, ScopesSet: true}
	e.mu.Lock()
	e.drafts[s.Name] = s
	e.mu.Unlock()
	pd := portalDef{name: s.Name, sess: toSSO(s), draft: true}
	p := api.Portal{Name: s.Name, StartURL: s.StartURL, StartURLHost: startURLHost(s.StartURL), Region: s.Region, Draft: true,
		Profiles: make([]string, 0), Session: e.sessionStatusFor(pd, e.d.Now())}
	e.EmitState(context.Background())
	return p, nil
}

func (e *Engine) DiscardDraft(name string) {
	e.mu.Lock()
	_, had := e.drafts[name]
	delete(e.drafts, name)
	e.mu.Unlock()
	if had {
		e.EmitState(context.Background())
	}
}

// PortalURL returns the access portal start URL (config portal, draft or legacy).
func (e *Engine) PortalURL(portal string) (string, error) {
	s, ok := e.SessionFor(portal)
	if !ok {
		return "", api.NewError(api.ErrPortalNotFound, "portal not found").WithParam("portal", portal)
	}
	norm, _, nerr := awsconfig.NormalizeStartURL(s.StartURL)
	if nerr != nil {
		return "", nerr
	}
	return norm, nil
}

// consoleProfile returns an SSO profile that can open the console, with its normalized start URL.
func consoleProfile(snap *awsconfig.Snapshot, profile string) (awsconfig.Profile, string, error) {
	p, ok := snap.Profile(profile)
	if !ok {
		return p, "", api.NewError(api.ErrProfileNotFound, "profile not found").WithParam("name", profile)
	}
	if (p.Kind != api.ProfileKindSSO && p.Kind != api.ProfileKindLegacySSO) || p.AccountID == "" {
		return p, "", invalid("profile")
	}
	start, _, nerr := awsconfig.NormalizeStartURL(p.SSOStartURL)
	if nerr != nil {
		return p, "", nerr
	}
	if !awsconfig.AccountIDRE.MatchString(p.AccountID) {
		return p, "", invalid("accountId")
	}
	if !awsconfig.RoleNameRE.MatchString(p.Role) {
		return p, "", invalid("role")
	}
	return p, start, nil
}

// ConsoleURL builds the portal shortcut link for an SSO profile.
func (e *Engine) ConsoleURL(profile, destination string) (string, error) {
	snap, err := e.Snapshot()
	if err != nil {
		return "", err
	}
	p, start, err := consoleProfile(snap, profile)
	if err != nil {
		return "", err
	}
	return sso.ShortcutURL(start, p.AccountID, p.Role, destination), nil
}

// ConsoleSignInURL returns a link that signs the browser in to the AWS console as the role of an SSO profile,
// made from this app's own Identity Center session (role credentials, then a federation sign-in token), so the
// browser's access-portal session, which may belong to another user, plays no part. The link is a secret:
// open it, never log it or return it to the frontend. Partitions without a known federation endpoint get the
// portal shortcut link (ConsoleURL) instead.
func (e *Engine) ConsoleSignInURL(ctx context.Context, profile string) (string, error) {
	snap, err := e.Snapshot()
	if err != nil {
		return "", err
	}
	p, start, err := consoleProfile(snap, profile)
	if err != nil {
		return "", err
	}
	var pd portalDef
	for _, d := range e.portals(snap) {
		if slices.Contains(d.profiles, p.Name) {
			pd = d
			break
		}
	}
	if pd.name == "" {
		return "", api.NewError(api.ErrPortalNotFound, "portal not found").WithParam("portal", p.SSOSession)
	}
	creds, cerr := e.d.Sessions.RoleCredentials(ctx, pd.sess, p.AccountID, p.Role)
	if cerr != nil {
		return "", sso.ToAppError(cerr, pd.name)
	}
	in := awsapi.ConsoleSignIn{Credentials: creds, SSORegion: pd.sess.Region, Region: p.Region, Issuer: start}
	u, err := awsapi.ConsoleSignInURL(ctx, in, e.d.Endpoints)
	if errors.Is(err, awsapi.ErrConsoleUnsupported) {
		return sso.ShortcutURL(start, p.AccountID, p.Role, ""), nil
	}
	return u, err
}

// IdentifyStatic asks STS who owns a static key. A rejected key is a result, not an error.
func (e *Engine) IdentifyStatic(ctx context.Context, profile string) (api.StaticIdentity, error) {
	snap, err := e.Snapshot()
	if err != nil {
		return api.StaticIdentity{}, err
	}
	key, ok := snap.StaticKey(profile)
	if !ok {
		return api.StaticIdentity{}, api.NewError(api.ErrNoStaticKey, "no static key").WithParam("name", profile)
	}
	id := api.StaticIdentity{Profile: profile, MaskedKeyID: redact.MaskKeyID(key.AccessKeyID)}
	ci, cerr := awsapi.GetCallerIdentity(ctx, awsapi.StaticProvider(key), e.d.Endpoints)
	if cerr != nil {
		code := awsapi.ErrorCode(cerr)
		if !awsapi.IsInvalidKeyCode(code) {
			return api.StaticIdentity{}, awsapi.ToAppError(cerr)
		}
		id.ErrorCode = code
	} else {
		id.AccountID, id.Arn, id.IAMUser, id.IsRoot = ci.AccountID, ci.Arn, ci.IAMUser, ci.IsRoot
	}
	if err := e.d.Store.SetStaticIdentity(id); err != nil {
		return api.StaticIdentity{}, api.AsAppError(err)
	}
	e.mu.Lock()
	e.identified[profile] = true
	e.mu.Unlock()
	e.EmitState(ctx)
	return id, nil
}

// Discover lists accounts and roles of a signed-in portal (max 4 concurrent role lookups).
func (e *Engine) Discover(ctx context.Context, portal string) ([]api.DiscoveredAccount, error) {
	snap, err := e.Snapshot()
	if err != nil {
		return nil, err
	}
	pd, ok := e.portalDef(snap, portal)
	if !ok {
		return nil, api.NewError(api.ErrPortalNotFound, "portal not found").WithParam("portal", portal)
	}
	accts, lerr := e.d.Sessions.ListAccounts(ctx, pd.sess)
	if lerr != nil {
		return nil, sso.ToAppError(lerr, portal)
	}
	sort.SliceStable(accts, func(i, j int) bool {
		if accts[i].Name != accts[j].Name {
			return accts[i].Name < accts[j].Name
		}
		return accts[i].ID < accts[j].ID
	})
	rolesOf := make([][]string, len(accts))
	errsOf := make([]*api.AppError, len(accts))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, a := range accts {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r, rerr := e.d.Sessions.ListAccountRoles(ctx, pd.sess, id)
			if rerr != nil {
				errsOf[i] = sso.ToAppError(rerr, portal)
				return
			}
			sort.Strings(r)
			rolesOf[i] = r
		}(i, a.ID)
	}
	wg.Wait()
	if ctx.Err() != nil {
		return nil, sso.ToAppError(ctx.Err(), portal)
	}

	names := map[string]string{}
	taken := map[string]bool{}
	existing := map[[2]string]string{}
	for _, p := range snap.Profiles() {
		taken[p.Name] = true
		if p.SSOSession == pd.sess.Name && pd.sess.Name != "" {
			existing[[2]string{p.AccountID, p.Role}] = p.Name
		}
	}
	var extra []identity.Input
	known := e.d.Store.AccountColors()
	for _, a := range accts {
		names[a.ID] = a.Name
		if _, has := known[a.ID]; !has {
			extra = append(extra, identity.Input{Profile: "\x00new-" + a.ID, AccountID: a.ID})
		}
	}
	ids := e.assignIdentities(snap, extra...)

	out := make([]api.DiscoveredAccount, 0, len(accts))
	for i, a := range accts {
		da := api.DiscoveredAccount{AccountID: a.ID, AccountName: a.Name, Email: a.Email, Roles: make([]api.DiscoveredRole, 0), RolesError: errsOf[i]}
		da.ColorSlot = ids.newColors[a.ID]
		for _, role := range rolesOf[i] {
			dr := api.DiscoveredRole{Name: role, ExistingProfile: existing[[2]string{a.ID, role}]}
			dr.SuggestedProfile = awsconfig.SuggestProfileName(a.Name, role, taken)
			taken[dr.SuggestedProfile] = true
			da.Roles = append(da.Roles, dr)
		}
		out = append(out, da)
	}
	_ = e.d.Store.MergeAccountNames(names)
	return out, nil
}

// LoadRoles lists the roles of one account.
func (e *Engine) LoadRoles(ctx context.Context, portal, accountID string) (api.DiscoveredAccount, error) {
	snap, err := e.Snapshot()
	if err != nil {
		return api.DiscoveredAccount{}, err
	}
	pd, ok := e.portalDef(snap, portal)
	if !ok {
		return api.DiscoveredAccount{}, api.NewError(api.ErrPortalNotFound, "portal not found").WithParam("portal", portal)
	}
	roles, rerr := e.d.Sessions.ListAccountRoles(ctx, pd.sess, accountID)
	if rerr != nil {
		return api.DiscoveredAccount{}, sso.ToAppError(rerr, portal)
	}
	sort.Strings(roles)
	name := e.d.Store.AccountNames()[accountID]
	taken := map[string]bool{}
	existing := map[string]string{}
	for _, p := range snap.Profiles() {
		taken[p.Name] = true
		if p.SSOSession == pd.sess.Name && pd.sess.Name != "" && p.AccountID == accountID {
			existing[p.Role] = p.Name
		}
	}
	da := api.DiscoveredAccount{AccountID: accountID, AccountName: name, Roles: make([]api.DiscoveredRole, 0)}
	for _, r := range roles {
		dr := api.DiscoveredRole{Name: r, ExistingProfile: existing[r], SuggestedProfile: awsconfig.SuggestProfileName(name, r, taken)}
		taken[dr.SuggestedProfile] = true
		da.Roles = append(da.Roles, dr)
	}
	ids := e.assignIdentities(snap, identity.Input{Profile: "\x00new", AccountID: accountID})
	da.ColorSlot = ids.newColors[accountID]
	return da, nil
}

type createPlan struct {
	snap     *awsconfig.Snapshot
	after    []byte
	names    []string
	draft    string
	newCount int
}

func (e *Engine) planCreate(req api.CreateProfilesRequest) (*createPlan, error) {
	snap, err := e.Snapshot()
	if err != nil {
		return nil, err
	}
	if len(req.Profiles) == 0 {
		return nil, invalid("profiles")
	}
	var newSession *awsconfig.Session
	var sessName, sessRegion, draft string
	if s, ok := snap.Session(req.Portal); ok {
		sessName, sessRegion = s.Name, s.Region
	} else if ds, ok := e.DraftSession(req.Portal); ok {
		sessName, sessRegion, draft = ds.Name, ds.Region, ds.Name
		cp := ds
		newSession = &cp
	} else {
		return nil, api.NewError(api.ErrPortalNotFound, "portal not found").WithParam("portal", req.Portal)
	}
	taken := map[string]bool{}
	for _, p := range snap.Profiles() {
		taken[p.Name] = true
	}
	var nps []awsconfig.NewProfile
	var names []string
	for _, sp := range req.Profiles {
		if !awsconfig.ProfileNameRE.MatchString(sp.Name) {
			return nil, api.NewError(api.ErrInvalidName, "invalid profile name").WithParam("name", sp.Name)
		}
		if taken[sp.Name] {
			return nil, api.NewError(api.ErrProfileExists, "profile exists").WithParam("name", sp.Name)
		}
		taken[sp.Name] = true
		if !awsconfig.AccountIDRE.MatchString(sp.AccountID) {
			return nil, invalid("accountId")
		}
		if !awsconfig.RoleNameRE.MatchString(sp.Role) {
			return nil, invalid("role")
		}
		region := sp.Region
		if region == "" {
			region = sessRegion
		}
		if !awsconfig.RegionRE.MatchString(region) {
			return nil, api.NewError(api.ErrInvalidRegion, "invalid region")
		}
		nps = append(nps, awsconfig.NewProfile{Name: sp.Name, Session: sessName, AccountID: sp.AccountID, Role: sp.Role, Region: region})
		names = append(names, sp.Name)
	}
	after, aerr := awsconfig.AddProfiles(snap.ConfigBytes(), newSession, nps)
	if aerr != nil {
		return nil, aerr
	}
	return &createPlan{snap: snap, after: after, names: names, draft: draft, newCount: len(nps)}, nil
}

func (e *Engine) PreviewCreate(req api.CreateProfilesRequest) (api.FilePreview, error) {
	plan, err := e.planCreate(req)
	if err != nil {
		return api.FilePreview{}, err
	}
	return e.preview(e.d.Paths.AWSConfigFile, plan.snap.ConfigBytes(), plan.after), nil
}

func (e *Engine) Create(req api.CreateProfilesRequest, baseHash string) (api.CreateProfilesResult, error) {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	plan, err := e.planCreate(req)
	if err != nil {
		return api.CreateProfilesResult{}, err
	}
	id, werr := e.writeConfig(plan.snap, baseHash, plan.after, api.ReasonAddProfiles, map[string]string{"n": itoa(plan.newCount)})
	if werr != nil {
		return api.CreateProfilesResult{}, werr
	}
	if plan.draft != "" {
		e.ConsumeDraft(plan.draft)
	}
	e.EmitState(context.Background())
	return api.CreateProfilesResult{BackupID: id, Profiles: plan.names}, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func (e *Engine) PreviewRemove(name string) (api.FilePreview, error) {
	snap, err := e.Snapshot()
	if err != nil {
		return api.FilePreview{}, err
	}
	after, rerr := awsconfig.RemoveProfile(snap.ConfigBytes(), name)
	if rerr != nil {
		return api.FilePreview{}, rerr
	}
	return e.preview(e.d.Paths.AWSConfigFile, snap.ConfigBytes(), after), nil
}

// Remove deletes [profile name] from the config file only (credentials stay untouched).
func (e *Engine) Remove(name, baseHash string) (api.WriteResult, error) {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	snap, err := e.Snapshot()
	if err != nil {
		return api.WriteResult{}, err
	}
	after, rerr := awsconfig.RemoveProfile(snap.ConfigBytes(), name)
	if rerr != nil {
		return api.WriteResult{}, rerr
	}
	id, werr := e.writeConfig(snap, baseHash, after, api.ReasonRemoveProfile, map[string]string{"name": name})
	if werr != nil {
		return api.WriteResult{}, werr
	}
	e.EmitState(context.Background())
	return api.WriteResult{File: e.display(e.d.Paths.AWSConfigFile), BackupID: id}, nil
}

// SetIdentity stores the colour slot (per account) and the monogram override (per profile).
func (e *Engine) SetIdentity(u api.IdentityUpdate) (api.AccountsState, error) {
	if u.ColorSlot < 1 || u.ColorSlot > 8 {
		return api.AccountsState{}, invalid("colorSlot")
	}
	if u.Monogram != "" && !identity.ValidMonogram(u.Monogram) {
		return api.AccountsState{}, invalid("monogram")
	}
	snap, err := e.Snapshot()
	if err != nil {
		return api.AccountsState{}, err
	}
	p, ok := snap.Profile(u.Profile)
	if !ok {
		return api.AccountsState{}, api.NewError(api.ErrProfileNotFound, "profile not found").WithParam("name", u.Profile)
	}
	key := identity.ProfileKeyPrefix + p.Name
	acct := p.AccountID
	if acct == "" && p.Kind == api.ProfileKindStatic {
		if si, has := e.staticIdentityFor(snap, p.Name); has {
			acct = si.AccountID
		}
	}
	if acct != "" {
		key = acct
	}
	colors := e.d.Store.AccountColors()
	if colors == nil {
		colors = map[string]int{}
	}
	colors[key] = u.ColorSlot
	if err := e.d.Store.SetAccountColors(colors); err != nil {
		return api.AccountsState{}, api.AsAppError(err)
	}
	if err := e.d.Store.SetMonogram(u.Profile, strings.ToUpper(u.Monogram)); err != nil {
		return api.AccountsState{}, api.AsAppError(err)
	}
	st := e.stateFrom(snap)
	e.emit(api.EventAccountsChanged, st)
	return st, nil
}
