// Package migrate runs the static-key to SSO migration wizard. All wizard state, including the
// old static key, lives in memory only and is dropped on Finish, Discard or app exit.
package migrate

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/awsapi"
	"github.com/rudian/aws_sso_hub/internal/awsconfig"
	"github.com/rudian/aws_sso_hub/internal/backup"
	"github.com/rudian/aws_sso_hub/internal/paths"
	"github.com/rudian/aws_sso_hub/internal/redact"
	"github.com/rudian/aws_sso_hub/internal/sso"
)

// PendingDeactivation is a removed key whose IAM deactivation is still open.
type PendingDeactivation struct {
	Profile     string `json:"profile"`
	AccountID   string `json:"accountId"`
	IAMUser     string `json:"iamUser"`
	MaskedKeyID string `json:"maskedKeyId"`
	Since       string `json:"since"`
}

type Store interface {
	AddPendingDeactivation(p PendingDeactivation) error
	RemovePendingDeactivation(profile string) error
	SetStaticIdentity(id api.StaticIdentity) error
}

// Portals is implemented by *accounts.Engine.
type Portals interface {
	SessionFor(portal string) (sso.Session, bool)
	IsDraft(name string) bool
	DraftSession(name string) (awsconfig.Session, bool)
	ConsumeDraft(name string)
	MarkMigrating(profile string, on bool)
	EmitState(ctx context.Context)
}

type Deps struct {
	Paths     paths.Paths
	Sessions  *sso.Manager
	Portals   Portals
	Backup    *backup.Store
	Endpoints awsapi.Endpoints
	Store     Store
	Emit      func(name string, data any)
	Now       func() time.Time
	OnWrite   func(path string, content []byte)
}

type wizard struct {
	mu       sync.Mutex
	st       api.MigrationState
	key      awsconfig.StaticKey // SECRET, memory only
	hasKey   bool
	draftCpy *awsconfig.Session // draft portal session kept so a restore can re-create it
	// appliedHash is the hash of the config as Apply wrote it; RestoreConfig refuses when the file has
	// changed since.
	appliedHash string
}

// Engine holds the per-profile wizards.
type Engine struct {
	d Deps

	mu      sync.Mutex
	wizards map[string]*wizard
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
	return &Engine{d: d, wizards: map[string]*wizard{}}
}

func stateErr() *api.AppError { return api.NewError(api.ErrMigrateState, "step not available") }

func (e *Engine) files() awsconfig.Files {
	return awsconfig.Files{Config: e.d.Paths.AWSConfigFile, Credentials: e.d.Paths.AWSCredentialsFile}
}

func (e *Engine) display(p string) string { return e.d.Paths.Display(p) }

func (e *Engine) get(profile string) (*wizard, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	w, ok := e.wizards[profile]
	if !ok {
		return nil, stateErr()
	}
	return w, nil
}

// copyState returns a copy that shares no pointers with the wizard.
func copyState(s api.MigrationState) api.MigrationState {
	if s.ConfigPreview != nil {
		c := *s.ConfigPreview
		s.ConfigPreview = &c
	}
	if s.CredentialsPreview != nil {
		c := *s.CredentialsPreview
		s.CredentialsPreview = &c
	}
	if s.Verify != nil {
		c := *s.Verify
		s.Verify = &c
	}
	if s.Deactivation != nil {
		c := *s.Deactivation
		s.Deactivation = &c
	}
	if s.LastError != nil {
		c := *s.LastError
		s.LastError = &c
	}
	return s
}

func (w *wizard) out() api.MigrationState {
	s := copyState(w.st)
	s.KeyInMemory = w.hasKey
	return s
}

func (e *Engine) progress(profile string, item api.MigrationItem, status api.ProgressStatus, detail string) {
	e.d.Emit(api.EventMigrationProgress, api.MigrationProgress{Profile: profile, Item: item, Status: status, Detail: detail})
}

func (e *Engine) fail(w *wizard, err error) error {
	ae := api.AsAppError(err)
	w.st.LastError = ae
	return ae
}

// InProgress returns the profiles with a wizard open.
func (e *Engine) InProgress() map[string]bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[string]bool, len(e.wizards))
	for k := range e.wizards {
		out[k] = true
	}
	return out
}

// Start opens (or resumes) the wizard for a profile that has a static key.
func (e *Engine) Start(ctx context.Context, profile string) (api.MigrationState, error) {
	e.mu.Lock()
	if w, ok := e.wizards[profile]; ok {
		e.mu.Unlock()
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.out(), nil
	}
	e.mu.Unlock()

	snap, err := awsconfig.Load(e.files())
	if err != nil {
		return api.MigrationState{}, err
	}
	p, ok := snap.Profile(profile)
	key, hasKey := snap.StaticKey(profile)
	if !ok || !hasKey || (p.Kind != api.ProfileKindStatic && p.Kind != api.ProfileKindSSO) {
		return api.MigrationState{}, api.NewError(api.ErrNoStaticKey, "no static key").WithParam("name", profile)
	}
	w := &wizard{key: key, hasKey: true}
	w.st = api.MigrationState{Profile: profile, Step: api.MigrationIdentify,
		Identity: api.StaticIdentity{Profile: profile, MaskedKeyID: redact.MaskKeyID(key.AccessKeyID)}}

	ci, cerr := awsapi.GetCallerIdentity(ctx, awsapi.StaticProvider(key), e.d.Endpoints)
	switch {
	case cerr == nil:
		w.st.Identity.AccountID, w.st.Identity.Arn, w.st.Identity.IAMUser, w.st.Identity.IsRoot = ci.AccountID, ci.Arn, ci.IAMUser, ci.IsRoot
	case awsapi.IsInvalidKeyCode(awsapi.ErrorCode(cerr)):
		w.st.Identity.ErrorCode = awsapi.ErrorCode(cerr)
	default:
		w.st.LastError = awsapi.ToAppError(cerr)
	}
	if cerr == nil || w.st.Identity.ErrorCode != "" {
		_ = e.d.Store.SetStaticIdentity(w.st.Identity)
	}
	if p.Kind == api.ProfileKindSSO {
		w.st.Portal, w.st.AccountID, w.st.Role, w.st.Region = p.SSOSession, p.AccountID, p.Role, p.Region
		w.st.AccountName = ""
		w.st.Verify = &api.VerifyResult{OK: true}
		w.st.Step = api.MigrationRemoveKey
	}

	e.mu.Lock()
	if existing, dup := e.wizards[profile]; dup { // lost a race with a concurrent Start
		e.mu.Unlock()
		existing.mu.Lock()
		defer existing.mu.Unlock()
		return existing.out(), nil
	}
	e.wizards[profile] = w
	e.mu.Unlock()
	e.d.Portals.MarkMigrating(profile, true)
	e.d.Portals.EmitState(ctx)
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.out(), nil
}

func (e *Engine) Get(profile string) (api.MigrationState, error) {
	w, err := e.get(profile)
	if err != nil {
		return api.MigrationState{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.out(), nil
}

func stepIn(s api.MigrationStep, allowed ...api.MigrationStep) bool {
	for _, a := range allowed {
		if s == a {
			return true
		}
	}
	return false
}

func (w *wizard) clearAccount() {
	w.st.AccountID, w.st.AccountName, w.st.Role, w.st.Region, w.st.ConfirmOther = "", "", "", "", false
	w.st.ConfigPreview, w.st.Verify = nil, nil
}

func (e *Engine) ChoosePortal(profile, portal string) (api.MigrationState, error) {
	w, err := e.get(profile)
	if err != nil {
		return api.MigrationState{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !stepIn(w.st.Step, api.MigrationIdentify, api.MigrationPortal, api.MigrationAccount, api.MigrationReview) {
		return api.MigrationState{}, stateErr()
	}
	if _, ok := e.d.Portals.SessionFor(portal); !ok {
		return api.MigrationState{}, api.NewError(api.ErrPortalNotFound, "portal not found").WithParam("portal", portal)
	}
	w.st.Portal, w.st.PortalIsDraft = portal, e.d.Portals.IsDraft(portal)
	if w.st.PortalIsDraft {
		if ds, ok := e.d.Portals.DraftSession(portal); ok {
			w.draftCpy = &ds
		}
	} else {
		w.draftCpy = nil
	}
	w.clearAccount()
	w.st.LastError = nil
	w.st.Step = api.MigrationAccount
	return w.out(), nil
}

func (e *Engine) ChooseAccount(profile string, c api.MigrationAccountChoice) (api.MigrationState, error) {
	w, err := e.get(profile)
	if err != nil {
		return api.MigrationState{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !stepIn(w.st.Step, api.MigrationAccount, api.MigrationReview) {
		return api.MigrationState{}, stateErr()
	}
	if !awsconfig.AccountIDRE.MatchString(c.AccountID) {
		return api.MigrationState{}, api.NewError(api.ErrInvalidInput, "invalid account id").WithParam("field", "accountId")
	}
	if !awsconfig.RoleNameRE.MatchString(c.Role) {
		return api.MigrationState{}, api.NewError(api.ErrInvalidInput, "invalid role").WithParam("field", "role")
	}
	if c.Region != "" && !awsconfig.RegionRE.MatchString(c.Region) {
		return api.MigrationState{}, api.NewError(api.ErrInvalidRegion, "invalid region")
	}
	if w.st.Identity.AccountID != "" && w.st.Identity.AccountID != c.AccountID && !c.ConfirmOther {
		return api.MigrationState{}, api.NewError(api.ErrMigrateConfirmOther, "confirm the different account")
	}
	w.st.AccountID, w.st.AccountName, w.st.Role, w.st.Region, w.st.ConfirmOther = c.AccountID, c.AccountName, c.Role, c.Region, c.ConfirmOther
	w.st.ConfigPreview, w.st.Verify, w.st.LastError = nil, nil, nil
	w.st.Step = api.MigrationReview
	return w.out(), nil
}

// plan computes the new config bytes for the current choices on the given snapshot.
func (e *Engine) plan(w *wizard, snap *awsconfig.Snapshot) ([]byte, error) {
	var newSession *awsconfig.Session
	if _, exists := snap.Session(w.st.Portal); !exists {
		if ds, ok := e.d.Portals.DraftSession(w.st.Portal); ok {
			newSession = &ds
		} else if w.draftCpy != nil && w.draftCpy.Name == w.st.Portal {
			cp := *w.draftCpy
			newSession = &cp
		} else {
			return nil, api.NewError(api.ErrPortalNotFound, "portal not found").WithParam("portal", w.st.Portal)
		}
	}
	region := ""
	if cur, ok := snap.Profile(w.st.Profile); w.st.Region != "" && (!ok || cur.Region != w.st.Region) {
		region = w.st.Region
	}
	return awsconfig.MigrateProfile(snap.ConfigBytes(), w.st.Profile, newSession, w.st.Portal, w.st.AccountID, w.st.Role, region)
}

func (e *Engine) Preview(profile string) (api.MigrationState, error) {
	w, err := e.get(profile)
	if err != nil {
		return api.MigrationState{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.st.Step != api.MigrationReview {
		return api.MigrationState{}, stateErr()
	}
	snap, lerr := awsconfig.Load(e.files())
	if lerr != nil {
		return api.MigrationState{}, e.fail(w, lerr)
	}
	after, perr := e.plan(w, snap)
	if perr != nil {
		return api.MigrationState{}, e.fail(w, perr)
	}
	pv := awsconfig.Preview(e.d.Paths.AWSConfigFile, e.display(e.d.Paths.AWSConfigFile), e.display(e.d.Backup.Dir()), snap.ConfigBytes(), after)
	w.st.ConfigPreview, w.st.LastError = &pv, nil
	return w.out(), nil
}

// Apply backs up the config, writes the migrated profile and verifies it with SSO role credentials.
func (e *Engine) Apply(ctx context.Context, profile, baseHash string) (api.MigrationState, error) {
	w, err := e.get(profile)
	if err != nil {
		return api.MigrationState{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.st.Step != api.MigrationReview || w.st.ConfigPreview == nil {
		return api.MigrationState{}, stateErr()
	}
	snap, lerr := awsconfig.Load(e.files())
	if lerr != nil {
		return api.MigrationState{}, e.fail(w, lerr)
	}
	if snap.ReadOnly() {
		return api.MigrationState{}, e.fail(w, api.NewError(api.ErrReadOnly, "the config file has a parse error"))
	}
	if snap.ConfigHash() != baseHash {
		return api.MigrationState{}, e.fail(w, api.NewError(api.ErrConfigChanged, "config changed on disk").WithParam("file", e.display(e.d.Paths.AWSConfigFile)))
	}
	after, perr := e.plan(w, snap)
	if perr != nil {
		return api.MigrationState{}, e.fail(w, perr)
	}

	e.progress(profile, api.MigrationItemBackup, api.ProgressRunning, "")
	ent, berr := e.d.Backup.Snapshot(e.d.Paths.AWSConfigFile, e.display(e.d.Paths.AWSConfigFile), api.BackupAWSConfig, api.ReasonMigrate, map[string]string{"profile": profile})
	if berr != nil {
		e.progress(profile, api.MigrationItemBackup, api.ProgressFailed, "")
		return api.MigrationState{}, e.fail(w, berr)
	}
	e.progress(profile, api.MigrationItemBackup, api.ProgressDone, e.display(e.d.Backup.Dir()))

	e.progress(profile, api.MigrationItemWrite, api.ProgressRunning, "")
	if werr := backup.WriteFileAtomic(e.d.Paths.AWSConfigFile, after, 0o600); werr != nil {
		e.progress(profile, api.MigrationItemWrite, api.ProgressFailed, "")
		return api.MigrationState{}, e.fail(w, api.NewError(api.ErrFileIO, "cannot write the config file").WithParam("file", e.display(e.d.Paths.AWSConfigFile)).WithDetail(werr.Error()))
	}
	e.d.OnWrite(e.d.Paths.AWSConfigFile, after)
	w.appliedHash = awsconfig.Hash(after)
	if w.st.PortalIsDraft || e.d.Portals.IsDraft(w.st.Portal) {
		e.d.Portals.ConsumeDraft(w.st.Portal)
	}
	e.progress(profile, api.MigrationItemWrite, api.ProgressDone, "")
	w.st.ConfigBackupID, w.st.ConfigPreview, w.st.LastError = ent.ID, nil, nil
	w.st.Step = api.MigrationVerify

	e.progress(profile, api.MigrationItemVerify, api.ProgressRunning, "")
	w.st.Verify = e.verify(ctx, w)
	if w.st.Verify.OK {
		e.progress(profile, api.MigrationItemVerify, api.ProgressDone, w.st.Verify.AccountID+" • "+w.st.Verify.Arn)
	} else {
		e.progress(profile, api.MigrationItemVerify, api.ProgressFailed, "")
	}
	e.d.Portals.EmitState(ctx)
	return w.out(), nil
}

// verify signs STS with the SSO role credentials explicitly; it never resolves the profile through the SDK
// chain, which would pick up the static key that is still in ~/.aws/credentials.
func (e *Engine) verify(ctx context.Context, w *wizard) *api.VerifyResult {
	sess, ok := e.d.Portals.SessionFor(w.st.Portal)
	if !ok {
		return &api.VerifyResult{Error: api.NewError(api.ErrPortalNotFound, "portal not found").WithParam("portal", w.st.Portal)}
	}
	creds, err := e.d.Sessions.RoleCredentials(ctx, sess, w.st.AccountID, w.st.Role)
	if err != nil {
		return &api.VerifyResult{Error: sso.ToAppError(err, w.st.Portal)}
	}
	prov := aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) { return creds, nil })
	ci, err := awsapi.GetCallerIdentity(ctx, prov, e.d.Endpoints)
	if err != nil {
		return &api.VerifyResult{Error: awsapi.ToAppError(err)}
	}
	if ci.AccountID != w.st.AccountID {
		return &api.VerifyResult{AccountID: ci.AccountID, Arn: ci.Arn,
			Error: api.NewError(api.ErrVerifyWrongAccount, "signed in to a different account").WithParam("got", ci.AccountID).WithParam("want", w.st.AccountID)}
	}
	return &api.VerifyResult{OK: true, AccountID: ci.AccountID, Arn: ci.Arn}
}

func (e *Engine) RetryVerify(ctx context.Context, profile string) (api.MigrationState, error) {
	w, err := e.get(profile)
	if err != nil {
		return api.MigrationState{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.st.Step != api.MigrationVerify {
		return api.MigrationState{}, stateErr()
	}
	w.st.Verify = e.verify(ctx, w)
	return w.out(), nil
}

// RestoreConfig puts the config file back to the pre-migration backup.
func (e *Engine) RestoreConfig(profile string) (api.MigrationState, error) {
	w, err := e.get(profile)
	if err != nil {
		return api.MigrationState{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.st.Step != api.MigrationVerify || w.st.ConfigBackupID == "" {
		return api.MigrationState{}, stateErr()
	}
	_, content, gerr := e.d.Backup.Get(w.st.ConfigBackupID)
	if gerr != nil {
		return api.MigrationState{}, e.fail(w, gerr)
	}
	snap, lerr := awsconfig.Load(e.files())
	if lerr != nil {
		return api.MigrationState{}, e.fail(w, lerr)
	}
	if w.appliedHash == "" || snap.ConfigHash() != w.appliedHash {
		return api.MigrationState{}, e.fail(w, api.NewError(api.ErrConfigChanged, "config changed on disk").WithParam("file", e.display(e.d.Paths.AWSConfigFile)))
	}
	merged, _, merr := backup.MergeRedacted(content, snap.ConfigBytes())
	if merr != nil {
		return api.MigrationState{}, e.fail(w, merr)
	}
	disp := e.display(e.d.Paths.AWSConfigFile)
	if _, serr := e.d.Backup.Snapshot(e.d.Paths.AWSConfigFile, disp, api.BackupAWSConfig, api.ReasonRestore, map[string]string{"file": disp}); serr != nil {
		return api.MigrationState{}, e.fail(w, serr)
	}
	if werr := backup.WriteFileAtomic(e.d.Paths.AWSConfigFile, merged, 0o600); werr != nil {
		return api.MigrationState{}, e.fail(w, api.NewError(api.ErrFileIO, "cannot write the config file").WithParam("file", disp).WithDetail(werr.Error()))
	}
	e.d.OnWrite(e.d.Paths.AWSConfigFile, merged)
	w.st.Step, w.st.Verify, w.st.ConfigPreview, w.st.ConfigBackupID, w.st.LastError = api.MigrationReview, nil, nil, "", nil
	w.appliedHash = ""
	e.d.Portals.EmitState(context.Background())
	return w.out(), nil
}

// keyFiles finds where the profile's static keys are: the credentials-file sections that hold them ([name]
// or [profile name]) and whether the config-file section holds some too.
func keyFiles(snap *awsconfig.Snapshot, profile string) (credSections []string, cfgHas bool) {
	if _, ok := snap.Profile(profile); !ok {
		return nil, false
	}
	credSections = snap.CredentialsKeySections(profile)
	// a config section can hold keys as well, even when the credentials file has some too
	if _, changed, err := awsconfig.RemoveStaticKeys(snap.ConfigBytes(), awsconfig.ConfigSection(profile)); err == nil {
		cfgHas = changed
	}
	return credSections, cfgHas
}

// removeStaticKeys is the editor removeKeys uses; tests replace it to exercise the "nothing changed" guard.
var removeStaticKeys = awsconfig.RemoveStaticKeys

// removeKeys deletes the key lines from every listed section and reports whether anything changed.
func removeKeys(content []byte, sections ...string) ([]byte, bool, error) {
	out, changed := content, false
	for _, sec := range sections {
		next, c, err := removeStaticKeys(out, sec)
		if err != nil {
			return nil, false, err
		}
		if c {
			out, changed = next, true
		}
	}
	return out, changed, nil
}

func noKeyRemoved(profile string) *api.AppError {
	return api.NewError(api.ErrNoStaticKey, "the key lines could not be removed").WithParam("name", profile)
}

func (e *Engine) PreviewKeyRemoval(profile string) (api.MigrationState, error) {
	w, err := e.get(profile)
	if err != nil {
		return api.MigrationState{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !(w.st.Step == api.MigrationRemoveKey || (w.st.Step == api.MigrationVerify && w.st.Verify != nil && w.st.Verify.OK)) {
		return api.MigrationState{}, stateErr()
	}
	snap, lerr := awsconfig.Load(e.files())
	if lerr != nil {
		return api.MigrationState{}, e.fail(w, lerr)
	}
	credSecs, cfgHas := keyFiles(snap, profile)
	var path string
	var before []byte
	var after []byte
	var changed bool
	switch {
	case len(credSecs) > 0:
		path, before = e.d.Paths.AWSCredentialsFile, snap.CredentialsBytes()
		after, changed, err = removeKeys(before, credSecs...)
	case cfgHas:
		path, before = e.d.Paths.AWSConfigFile, snap.ConfigBytes()
		after, changed, err = removeKeys(before, awsconfig.ConfigSection(profile))
	default:
		return api.MigrationState{}, e.fail(w, api.NewError(api.ErrNoStaticKey, "no static key").WithParam("name", profile))
	}
	if err != nil {
		return api.MigrationState{}, e.fail(w, err)
	}
	if !changed {
		return api.MigrationState{}, e.fail(w, noKeyRemoved(profile))
	}
	pv := awsconfig.Preview(path, e.display(path), e.display(e.d.Backup.Dir()), before, after)
	w.st.CredentialsPreview, w.st.LastError = &pv, nil
	w.st.Step = api.MigrationRemoveKey
	return w.out(), nil
}

// RemoveKey deletes the static key from disk (backup first, redacted) and records the open deactivation.
// It fails, without touching any file, when a file that holds the key would not change.
func (e *Engine) RemoveKey(profile, baseHash string) (api.MigrationState, error) {
	w, err := e.get(profile)
	if err != nil {
		return api.MigrationState{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.st.Step != api.MigrationRemoveKey || w.st.CredentialsPreview == nil {
		return api.MigrationState{}, stateErr()
	}
	snap, lerr := awsconfig.Load(e.files())
	if lerr != nil {
		return api.MigrationState{}, e.fail(w, lerr)
	}
	if snap.ReadOnly() {
		return api.MigrationState{}, e.fail(w, api.NewError(api.ErrReadOnly, "an AWS file has a parse error"))
	}
	credSecs, cfgHas := keyFiles(snap, profile)
	credHas := len(credSecs) > 0
	if !credHas && !cfgHas {
		return api.MigrationState{}, e.fail(w, api.NewError(api.ErrNoStaticKey, "no static key").WithParam("name", profile))
	}
	previewPath := w.st.CredentialsPreview.Path
	cur := snap.CredentialsHash()
	if previewPath == e.d.Paths.AWSConfigFile {
		cur = snap.ConfigHash()
	}
	if cur != baseHash {
		return api.MigrationState{}, e.fail(w, api.NewError(api.ErrConfigChanged, "file changed on disk").WithParam("file", e.display(previewPath)))
	}

	// compute every result first so a no-op fails before any file is written
	var credOut, cfgOut []byte
	if credHas {
		out, changed, rerr := removeKeys(snap.CredentialsBytes(), credSecs...)
		if rerr != nil {
			return api.MigrationState{}, e.fail(w, rerr)
		}
		if !changed {
			return api.MigrationState{}, e.fail(w, noKeyRemoved(profile))
		}
		credOut = out
	}
	if cfgHas {
		out, changed, rerr := removeKeys(snap.ConfigBytes(), awsconfig.ConfigSection(profile))
		if rerr != nil {
			return api.MigrationState{}, e.fail(w, rerr)
		}
		if !changed {
			return api.MigrationState{}, e.fail(w, noKeyRemoved(profile))
		}
		cfgOut = out
	}

	write := func(path string, kind api.BackupKind, out []byte) (string, error) {
		ent, serr := e.d.Backup.Snapshot(path, e.display(path), kind, api.ReasonRemoveKey, map[string]string{"profile": profile})
		if serr != nil {
			return "", serr
		}
		if werr := backup.WriteFileAtomic(path, out, 0o600); werr != nil {
			return "", api.NewError(api.ErrFileIO, "cannot write file").WithParam("file", e.display(path)).WithDetail(werr.Error())
		}
		e.d.OnWrite(path, out)
		return ent.ID, nil
	}
	if credHas {
		id, rerr := write(e.d.Paths.AWSCredentialsFile, api.BackupAWSCredentials, credOut)
		if rerr != nil {
			return api.MigrationState{}, e.fail(w, rerr)
		}
		w.st.CredentialsBackupID = id
	}
	if cfgHas {
		id, rerr := write(e.d.Paths.AWSConfigFile, api.BackupAWSConfig, cfgOut)
		if rerr != nil {
			return api.MigrationState{}, e.fail(w, rerr)
		}
		if !credHas {
			w.st.CredentialsBackupID = id
		}
	}
	w.st.KeyRemoved, w.st.LastError, w.st.CredentialsPreview = true, nil, nil
	id := w.st.Identity
	if id.AccountID != "" && id.ErrorCode == "" && !id.IsRoot {
		_ = e.d.Store.AddPendingDeactivation(PendingDeactivation{Profile: profile, AccountID: id.AccountID, IAMUser: id.IAMUser,
			MaskedKeyID: id.MaskedKeyID, Since: e.d.Now().UTC().Format(time.RFC3339)})
	}
	w.st.Step = api.MigrationDeactivate
	e.d.Portals.EmitState(context.Background())
	return w.out(), nil
}

func (e *Engine) KeepKey(profile string) (api.MigrationState, error) {
	w, err := e.get(profile)
	if err != nil {
		return api.MigrationState{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.st.Step != api.MigrationRemoveKey {
		return api.MigrationState{}, stateErr()
	}
	w.st.KeyRemoved, w.st.CredentialsPreview, w.st.LastError = false, nil, nil
	w.st.Step = api.MigrationDeactivate
	return w.out(), nil
}

// Deactivate tries to set the old key Inactive: SSO role, then the old key itself, then console guidance.
func (e *Engine) Deactivate(ctx context.Context, profile string) (api.MigrationState, error) {
	w, err := e.get(profile)
	if err != nil {
		return api.MigrationState{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.st.Step != api.MigrationDeactivate {
		return api.MigrationState{}, stateErr()
	}
	res := &api.DeactivationResult{}
	id := w.st.Identity
	startURL := ""
	if s, ok := e.d.Portals.SessionFor(w.st.Portal); ok {
		startURL = s.StartURL
	}
	dest := "https://console.aws.amazon.com/iam/home#/users"
	if id.IAMUser != "" {
		dest = awsapi.IAMUserDestination(id.IAMUser)
	}
	if startURL != "" {
		if norm, _, nerr := awsconfig.NormalizeStartURL(startURL); nerr == nil {
			res.ConsoleURL = sso.ShortcutURL(norm, w.st.AccountID, w.st.Role, dest)
		}
	}
	w.st.LastError = nil
	switch {
	case id.IsRoot:
		res.Status = api.DeactivationRootKey
	case id.ErrorCode != "" || id.IAMUser == "" || !w.hasKey:
		res.Status = api.DeactivationUnavailable
	default:
		e.deactivate(ctx, w, res)
	}
	if res.Status == api.DeactivationDone || res.Status == api.DeactivationAlready {
		_ = e.d.Store.RemovePendingDeactivation(profile)
	}
	w.st.Deactivation = res
	return w.out(), nil
}

func (e *Engine) deactivate(ctx context.Context, w *wizard, res *api.DeactivationResult) {
	id := w.st.Identity
	sess, ok := e.d.Portals.SessionFor(w.st.Portal)
	if !ok {
		res.Status, res.Error = api.DeactivationUnavailable, api.NewError(api.ErrPortalNotFound, "portal not found").WithParam("portal", w.st.Portal)
		return
	}
	prov := awsapi.SSOProvider(e.d.Sessions, sess, w.st.AccountID, w.st.Role)
	st, derr := awsapi.DeactivateWithRole(ctx, prov, id.IAMUser, w.key.AccessKeyID, e.d.Endpoints)
	if derr == nil {
		res.Status, res.Method = st, api.DeactivateViaSSORole
		return
	}
	ae := awsapi.ToAppError(derr)
	if ae.Code != api.ErrAWSAccessDenied {
		var sae *api.AppError
		if errors.As(derr, &sae) {
			ae = sae
		}
		// the role could not even be used (signed out, network...): nothing else will work either
		if ae.Code == api.ErrNotSignedIn || ae.Code == api.ErrSessionExpired || ae.Code == api.ErrNetwork || ae.Code == api.ErrRoleNotAssigned {
			res.Status, res.Error = api.DeactivationUnavailable, ae
			return
		}
		res.Status, res.Error = api.DeactivationDenied, ae
		return
	}
	if !w.hasKey {
		res.Status, res.Error = api.DeactivationDenied, ae
		return
	}
	st, serr := awsapi.DeactivateSelf(ctx, w.key, e.d.Endpoints)
	if serr == nil {
		res.Status, res.Method = st, api.DeactivateViaOldKey
		return
	}
	res.Status, res.Error = api.DeactivationDenied, awsapi.ToAppError(serr)
}

// Finish closes the wizard and forgets the in-memory key.
func (e *Engine) Finish(profile string) (api.MigrationState, error) {
	w, err := e.get(profile)
	if err != nil {
		return api.MigrationState{}, err
	}
	w.mu.Lock()
	if !stepIn(w.st.Step, api.MigrationDeactivate, api.MigrationRemoveKey) {
		w.mu.Unlock()
		return api.MigrationState{}, stateErr()
	}
	w.st.Step = api.MigrationDone
	w.key, w.hasKey = awsconfig.StaticKey{}, false
	out := w.out()
	w.mu.Unlock()
	e.drop(profile)
	e.d.Portals.EmitState(context.Background())
	return out, nil
}

func (e *Engine) drop(profile string) {
	e.mu.Lock()
	delete(e.wizards, profile)
	e.mu.Unlock()
	e.d.Portals.MarkMigrating(profile, false)
}

// Discard drops the wizard and the key without touching any file.
func (e *Engine) Discard(profile string) {
	e.mu.Lock()
	w, ok := e.wizards[profile]
	e.mu.Unlock()
	if ok {
		w.mu.Lock()
		w.key, w.hasKey = awsconfig.StaticKey{}, false
		w.mu.Unlock()
	}
	e.drop(profile)
	e.d.Portals.EmitState(context.Background())
}
