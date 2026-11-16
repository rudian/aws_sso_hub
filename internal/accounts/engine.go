// Package accounts is the AWS-side engine behind AccountService: it builds the accounts state from the
// config files and settings, runs sign-in/refresh/logout with the session countdown, discovers accounts
// and writes profiles.
package accounts

import (
	"context"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/awsapi"
	"github.com/rudian/codepass/internal/awsconfig"
	"github.com/rudian/codepass/internal/backup"
	"github.com/rudian/codepass/internal/identity"
	"github.com/rudian/codepass/internal/paths"
	"github.com/rudian/codepass/internal/redact"
	"github.com/rudian/codepass/internal/sso"
)

// Store is the persistence the engine needs (implemented by internal/settings).
type Store interface {
	AccountColors() map[string]int
	SetAccountColors(colors map[string]int) error
	Monograms() map[string]string
	SetMonogram(profile, monogram string) error // "" deletes the override
	AccountNames() map[string]string
	MergeAccountNames(names map[string]string) error
	StaticIdentity(profile string) (api.StaticIdentity, bool)
	SetStaticIdentity(id api.StaticIdentity) error
	SessionRecord(portal string) (api.SessionRecord, bool)
	SetSessionRecord(portal string, r api.SessionRecord) error
	DeleteSessionRecord(portal string) error
	AutoRenew() bool
	ReminderLead() time.Duration
	ExpiryNotice() bool
	EndedNotice() bool // reminders.endedNoticeOff == false
}

// NoticeLog remembers which sign-in notices were sent (internal/notify keeps it in notifications.json) and is
// told when a portal was signed in or out. A portal's record holds, per kind ("headsup", "ended"), the key of
// the last notice sent for it: the sign-in time. It never fails; a log that cannot write keeps its records in
// memory.
type NoticeLog interface {
	Last(portal, kind string) string
	Mark(portal, kind, key string)
	// SignInStarting is called when a sign-in of the portal begins, before the browser opens.
	SignInStarting(portal string)
	// SignedIn is called after a successful sign-in of the portal: the record starts over and the portal's
	// delivered notice is taken back.
	SignedIn(portal string)
	// SignedOut is called after the person signed the portal out: nothing is sent for it, a delivered heads-up is
	// taken back.
	SignedOut(portal string)
}

// RepoStats is implemented by the repo side (wired in internal/app).
type RepoStats interface {
	BoundCounts() map[string]int
	GlobalDefault() (profile string, affected int, ok bool)
}

// ProfileCreds describes whether a profile can sign API calls right now.
type ProfileCreds struct {
	Name, AccountID, Portal string
	Kind                    api.ProfileKind
	ColorSlot               int
	Monogram                string
	Usable                  bool
	NotSignedInPortal       string
	Provider                aws.CredentialsProvider // nil when !Usable
}

// Deps wires the engine.
type Deps struct {
	Paths       paths.Paths
	Sessions    *sso.Manager
	Backup      *backup.Store
	Store       Store
	Endpoints   awsapi.Endpoints
	OpenBrowser func(url string) error
	Emit        func(name string, data any)
	Now         func() time.Time
	OnWrite     func(path string, content []byte) // informs the file watcher of the app's own writes
	Notices     NoticeLog                         // nil: the records live in memory
}

const (
	defaultSessionMinutes = 480
	minSessionMinutes     = 15
	maxSessionMinutes     = 129600
	expiringWindow        = 15 * time.Minute
)

// Engine implements the AWS-side account logic. Safe for concurrent use.
type Engine struct {
	d Deps

	writeMu sync.Mutex // serialises config writes

	mu          sync.Mutex
	drafts      map[string]awsconfig.Session
	migrating   map[string]bool
	identified  map[string]bool // static profiles identified in this app session
	stats       RepoStats
	lastSig     map[string]string
	lastRenewal map[string]time.Time
	watch       map[string]*endWatch // portal -> what the ticker saw of its sign-in (see watchEnd)
	probes      map[string]string    // portal -> LoginAt of the sign-in whose check renewal is pending (see probeDelay)
}

func New(d Deps) *Engine {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Emit == nil {
		d.Emit = func(string, any) {}
	}
	if d.OnWrite == nil {
		d.OnWrite = func(string, []byte) {}
	}
	if d.Notices == nil {
		d.Notices = newMemLog()
	}
	return &Engine{d: d, drafts: map[string]awsconfig.Session{}, migrating: map[string]bool{}, identified: map[string]bool{},
		lastSig: map[string]string{}, lastRenewal: map[string]time.Time{}, watch: map[string]*endWatch{}, probes: map[string]string{}}
}

// LockWrites runs fn while no other config write of this engine is in progress.
func (e *Engine) LockWrites(fn func()) {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	fn()
}

func (e *Engine) SetRepoStats(r RepoStats) { e.mu.Lock(); e.stats = r; e.mu.Unlock() }

func (e *Engine) files() awsconfig.Files {
	return awsconfig.Files{Config: e.d.Paths.AWSConfigFile, Credentials: e.d.Paths.AWSCredentialsFile}
}

func (e *Engine) display(p string) string { return e.d.Paths.Display(p) }

// Snapshot reads both AWS files.
func (e *Engine) Snapshot() (*awsconfig.Snapshot, error) { return awsconfig.Load(e.files()) }

func (e *Engine) emit(name string, data any) { e.d.Emit(name, data) }

// EmitState emits accounts:changed with the freshly computed state.
func (e *Engine) EmitState(ctx context.Context) {
	if st, err := e.State(ctx); err == nil {
		e.emit(api.EventAccountsChanged, st)
	}
}

func (e *Engine) MarkMigrating(profile string, on bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if on {
		e.migrating[profile] = true
	} else {
		delete(e.migrating, profile)
	}
}

func startURLHost(u string) string {
	return strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
}

func legacyName(startURL string) string { return "legacy:" + startURLHost(startURL) }

// portalDef is a portal as the engine sees it.
type portalDef struct {
	name     string
	sess     sso.Session
	draft    bool
	legacy   bool
	profiles []string
}

func toSSO(s awsconfig.Session) sso.Session {
	return sso.Session{Name: s.Name, StartURL: s.StartURL, Region: s.Region, Scopes: s.Scopes, ScopesSet: s.ScopesSet}
}

// portals lists config sessions (by name), drafts, then legacy pseudo-portals.
func (e *Engine) portals(snap *awsconfig.Snapshot) []portalDef {
	profs := snap.Profiles()
	var out []portalDef
	for _, s := range snap.Sessions() {
		pd := portalDef{name: s.Name, sess: toSSO(s)}
		for _, p := range profs {
			if p.SSOSession == s.Name {
				pd.profiles = append(pd.profiles, p.Name)
			}
		}
		out = append(out, pd)
	}
	e.mu.Lock()
	var dn []string
	for n := range e.drafts {
		dn = append(dn, n)
	}
	sort.Strings(dn)
	for _, n := range dn {
		dup := false
		for _, o := range out {
			if o.name == n {
				dup = true
			}
		}
		if !dup {
			out = append(out, portalDef{name: n, sess: toSSO(e.drafts[n]), draft: true})
		}
	}
	e.mu.Unlock()
	legacy := map[string]*portalDef{}
	var lnames []string
	for _, p := range profs {
		if p.Kind != api.ProfileKindLegacySSO {
			continue
		}
		n := legacyName(p.SSOStartURL)
		pd, ok := legacy[n]
		if !ok {
			pd = &portalDef{name: n, legacy: true, sess: sso.Session{StartURL: p.SSOStartURL, Region: p.SSORegion}}
			legacy[n] = pd
			lnames = append(lnames, n)
		}
		pd.profiles = append(pd.profiles, p.Name)
	}
	sort.Strings(lnames)
	for _, n := range lnames {
		out = append(out, *legacy[n])
	}
	return out
}

func (e *Engine) portalDef(snap *awsconfig.Snapshot, name string) (portalDef, bool) {
	for _, p := range e.portals(snap) {
		if p.name == name {
			return p, true
		}
	}
	return portalDef{}, false
}

// SessionFor returns the sso.Session of a config portal, a draft or a legacy pseudo-portal.
func (e *Engine) SessionFor(portal string) (sso.Session, bool) {
	snap, err := e.Snapshot()
	if err != nil {
		return sso.Session{}, false
	}
	pd, ok := e.portalDef(snap, portal)
	return pd.sess, ok
}

func (e *Engine) IsDraft(name string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.drafts[name]
	return ok
}

func (e *Engine) DraftSession(name string) (awsconfig.Session, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.drafts[name]
	return s, ok
}

func (e *Engine) ConsumeDraft(name string) {
	e.mu.Lock()
	delete(e.drafts, name)
	e.mu.Unlock()
}

// staticIdentityFor returns the stored identity of a static profile when it belongs to the key in the files.
func (e *Engine) staticIdentityFor(snap *awsconfig.Snapshot, name string) (api.StaticIdentity, bool) {
	si, ok := e.d.Store.StaticIdentity(name)
	if !ok {
		return api.StaticIdentity{}, false
	}
	if k, has := snap.StaticKey(name); has && si.MaskedKeyID != "" && si.MaskedKeyID != redact.MaskKeyID(k.AccessKeyID) {
		return api.StaticIdentity{}, false
	}
	return si, true
}

type profileIdentity struct {
	res       map[string]identity.Result
	newColors map[string]int
}

func (e *Engine) assignIdentities(snap *awsconfig.Snapshot, extra ...identity.Input) profileIdentity {
	profs := snap.Profiles()
	sort.SliceStable(profs, func(i, j int) bool { return profs[i].Order < profs[j].Order })
	inputs := make([]identity.Input, 0, len(profs)+len(extra))
	for _, p := range profs {
		acct := p.AccountID
		if acct == "" && p.Kind == api.ProfileKindStatic {
			if si, ok := e.staticIdentityFor(snap, p.Name); ok {
				acct = si.AccountID
			}
		}
		inputs = append(inputs, identity.Input{Profile: p.Name, AccountID: acct})
	}
	inputs = append(inputs, extra...)
	res, nc := identity.Assign(inputs, e.d.Store.AccountColors(), e.d.Store.Monograms())
	return profileIdentity{res: res, newColors: nc}
}

func sameColors(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// State computes the full accounts state from disk and settings (no network).
func (e *Engine) State(ctx context.Context) (api.AccountsState, error) {
	snap, err := e.Snapshot()
	if err != nil {
		return api.AccountsState{}, err
	}
	return e.stateFrom(snap), nil
}

func (e *Engine) Reload(ctx context.Context) (api.AccountsState, error) { return e.State(ctx) }

func (e *Engine) stateFrom(snap *awsconfig.Snapshot) api.AccountsState {
	now := e.d.Now()
	ids := e.assignIdentities(snap)
	if old := e.d.Store.AccountColors(); !sameColors(old, ids.newColors) {
		_ = e.d.Store.SetAccountColors(ids.newColors)
	}
	e.mu.Lock()
	stats, migrating, identified := e.stats, copyBool(e.migrating), copyBool(e.identified)
	e.mu.Unlock()
	var counts map[string]int
	gdProfile, gdAffected, gdOK := "", 0, false
	if stats != nil {
		counts = stats.BoundCounts()
		gdProfile, gdAffected, gdOK = stats.GlobalDefault()
	}
	names := e.d.Store.AccountNames()

	portals := e.portals(snap)
	portalOf := map[string]string{}
	for _, pd := range portals {
		for _, n := range pd.profiles {
			portalOf[n] = pd.name
		}
	}

	st := api.AccountsState{
		ConfigFile: e.display(e.d.Paths.AWSConfigFile), CredentialsFile: e.display(e.d.Paths.AWSCredentialsFile),
		ReadOnly: snap.ReadOnly(), Issues: snap.Issues(e.display),
		Portals: make([]api.Portal, 0, len(portals)), Profiles: make([]api.Profile, 0),
	}
	for _, p := range snap.Profiles() {
		ap := api.Profile{
			Name: p.Name, Kind: p.Kind, Portal: portalOf[p.Name], AccountID: p.AccountID, Role: p.Role, Region: p.Region,
			HasStaticKeys: p.HasStaticKeys, BoundRepos: counts[p.Name], MigrationInProgress: migrating[p.Name],
		}
		if r, ok := ids.res[p.Name]; ok {
			ap.ColorSlot, ap.Monogram = r.Slot, r.Monogram
		}
		if p.Kind == api.ProfileKindSSO || p.Kind == api.ProfileKindLegacySSO {
			ap.AccountName = names[p.AccountID]
		}
		if p.HasStaticKeys {
			if k, ok := snap.StaticKey(p.Name); ok {
				ap.MaskedKeyID = redact.MaskKeyID(k.AccessKeyID)
			}
			if si, ok := e.staticIdentityFor(snap, p.Name); ok {
				ap.IAMUser, ap.IsRootKey = si.IAMUser, si.IsRoot
				if p.Kind == api.ProfileKindStatic {
					ap.AccountID, ap.AccountName = si.AccountID, names[si.AccountID]
				}
				if identified[p.Name] {
					ap.StaticError = si.ErrorCode
				}
			}
		}
		switch {
		case p.Kind == api.ProfileKindStatic:
			ap.StaticState = api.StaticUnverified
			if identified[p.Name] {
				if ap.StaticError != "" {
					ap.StaticState = api.StaticInvalid
				} else {
					ap.StaticState = api.StaticOK
				}
			}
		case p.Kind == api.ProfileKindSSO && p.HasStaticKeys:
			ap.StaticState = api.StaticKeysLeft
		}
		if p.HasStaticKeys && p.Kind != api.ProfileKindOther {
			st.StaticCount++
		}
		if gdOK && gdProfile == p.Name {
			ap.IsGlobalDefault, ap.GlobalDefaultRepos = true, gdAffected
		}
		st.Profiles = append(st.Profiles, ap)
	}
	st.StaticCount = 0
	for _, ap := range st.Profiles {
		if ap.Kind == api.ProfileKindStatic {
			st.StaticCount++
		}
	}
	for _, pd := range portals {
		sess := e.sessionStatusFor(pd, now)
		e.seedSig(pd.name, sess)
		st.Portals = append(st.Portals, api.Portal{
			Name: pd.name, StartURL: pd.sess.StartURL, StartURLHost: startURLHost(pd.sess.StartURL), Region: pd.sess.Region,
			LegacyFormat: pd.legacy, Draft: pd.draft, Profiles: nonNil(pd.profiles), Session: sess,
		})
	}
	return st
}

func copyBool(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return make([]string, 0)
	}
	return s
}

func (e *Engine) writeConfig(snap *awsconfig.Snapshot, baseHash string, content []byte, kind api.BackupReason, params map[string]string) (string, error) {
	if snap.ReadOnly() {
		return "", api.NewError(api.ErrReadOnly, "the config file has a parse error")
	}
	if snap.ConfigHash() != baseHash {
		return "", api.NewError(api.ErrConfigChanged, "config changed on disk").WithParam("file", e.display(e.d.Paths.AWSConfigFile))
	}
	ent, err := e.d.Backup.Snapshot(e.d.Paths.AWSConfigFile, e.display(e.d.Paths.AWSConfigFile), api.BackupAWSConfig, kind, params)
	if err != nil {
		return "", err
	}
	if err := backup.WriteFileAtomic(e.d.Paths.AWSConfigFile, content, 0o600); err != nil {
		return "", api.NewError(api.ErrFileIO, "cannot write the config file").WithParam("file", e.display(e.d.Paths.AWSConfigFile)).WithDetail(err.Error())
	}
	e.d.OnWrite(e.d.Paths.AWSConfigFile, content)
	return ent.ID, nil
}

func (e *Engine) preview(path string, before, after []byte) api.FilePreview {
	return awsconfig.Preview(path, e.display(path), e.display(e.d.Backup.Dir()), before, after)
}

// ProfileCreds lists every profile with a credentials provider when it can sign requests right now.
func (e *Engine) ProfileCreds(ctx context.Context) ([]ProfileCreds, error) {
	snap, err := e.Snapshot()
	if err != nil {
		return nil, err
	}
	ids := e.assignIdentities(snap)
	portals := e.portals(snap)
	byProfile := map[string]portalDef{}
	for _, pd := range portals {
		for _, n := range pd.profiles {
			byProfile[n] = pd
		}
	}
	var out []ProfileCreds
	for _, p := range snap.Profiles() {
		pc := ProfileCreds{Name: p.Name, AccountID: p.AccountID, Kind: p.Kind}
		if r, ok := ids.res[p.Name]; ok {
			pc.ColorSlot, pc.Monogram = r.Slot, r.Monogram
		}
		switch p.Kind {
		case api.ProfileKindSSO, api.ProfileKindLegacySSO:
			pd := byProfile[p.Name]
			pc.Portal = pd.name
			switch e.d.Sessions.Status(pd.sess).State {
			case api.SessionActive, api.SessionExpiring, api.SessionRefreshable:
				pc.Usable = true
				pc.Provider = awsapi.SSOProvider(e.d.Sessions, pd.sess, p.AccountID, p.Role)
			default:
				pc.NotSignedInPortal = pd.name
			}
		case api.ProfileKindStatic:
			if k, ok := snap.StaticKey(p.Name); ok {
				if si, has := e.staticIdentityFor(snap, p.Name); has && pc.AccountID == "" {
					pc.AccountID = si.AccountID
				}
				pc.Usable, pc.Provider = true, awsapi.StaticProvider(k)
			}
		default:
			// Profiles that rely on credential_process, role chaining, environment keys or instance metadata are
			// never resolved: doing so would run programs and contact network endpoints named in the config.
		}
		out = append(out, pc)
	}
	return out, nil
}

// deleteRoleCaches removes CLI role cache files of the session that carry "ProviderType": "sso".
func (e *Engine) deleteRoleCaches(sessionName string, roles []sso.AccountRole) int {
	n := 0
	for _, r := range roles {
		p := sso.CLICacheDir(e.d.Paths.Home) + string(os.PathSeparator) + sso.RoleCacheKey(r.AccountID, r.Role, sessionName, "") + ".json"
		if isSSORoleCache(p) && os.Remove(p) == nil {
			n++
		}
	}
	return n
}

func (e *Engine) sessionRoles(snap *awsconfig.Snapshot, sessionName string) []sso.AccountRole {
	var roles []sso.AccountRole
	for _, p := range snap.Profiles() {
		if p.SSOSession == sessionName && p.AccountID != "" && p.Role != "" {
			roles = append(roles, sso.AccountRole{AccountID: p.AccountID, Role: p.Role})
		}
	}
	return roles
}
