package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/awsapi"
	"github.com/rudian/codepass/internal/awsapi/awsapitest"
	"github.com/rudian/codepass/internal/awsconfig"
	"github.com/rudian/codepass/internal/backup"
	"github.com/rudian/codepass/internal/paths"
	"github.com/rudian/codepass/internal/sso"
	"github.com/rudian/codepass/internal/sso/ssotest"
)

const (
	oldKeyID = "AKIAIOSFODNN7EXAMPLE"
	ssoKeyID = "ASIAIOSFODNN7EXAMPLE"
)

var bg = context.Background()

type fakeStore struct {
	mu         sync.Mutex
	pending    []PendingDeactivation
	history    []string
	identities map[string]api.StaticIdentity
}

func (s *fakeStore) AddPendingDeactivation(p PendingDeactivation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = append(s.pending, p)
	s.history = append(s.history, "add:"+p.Profile)
	return nil
}
func (s *fakeStore) RemovePendingDeactivation(profile string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keep []PendingDeactivation
	for _, p := range s.pending {
		if p.Profile != profile {
			keep = append(keep, p)
		}
	}
	s.pending = keep
	s.history = append(s.history, "remove:"+profile)
	return nil
}
func (s *fakeStore) SetStaticIdentity(id api.StaticIdentity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.identities == nil {
		s.identities = map[string]api.StaticIdentity{}
	}
	s.identities[id.Profile] = id
	return nil
}

// fakePortals implements Portals over the config file and an in-memory draft map.
type fakePortals struct {
	files     awsconfig.Files
	mu        sync.Mutex
	drafts    map[string]awsconfig.Session
	migrating map[string]bool
	emits     int
}

func (f *fakePortals) SessionFor(portal string) (sso.Session, bool) {
	f.mu.Lock()
	if d, ok := f.drafts[portal]; ok {
		f.mu.Unlock()
		return sso.Session{Name: d.Name, StartURL: d.StartURL, Region: d.Region, Scopes: d.Scopes, ScopesSet: d.ScopesSet}, true
	}
	f.mu.Unlock()
	snap, _ := awsconfig.Load(f.files)
	if s, ok := snap.Session(portal); ok {
		return sso.Session{Name: s.Name, StartURL: s.StartURL, Region: s.Region, Scopes: s.Scopes, ScopesSet: s.ScopesSet}, true
	}
	return sso.Session{}, false
}
func (f *fakePortals) IsDraft(n string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.drafts[n]
	return ok
}
func (f *fakePortals) DraftSession(n string) (awsconfig.Session, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.drafts[n]
	return s, ok
}
func (f *fakePortals) ConsumeDraft(n string) { f.mu.Lock(); delete(f.drafts, n); f.mu.Unlock() }
func (f *fakePortals) MarkMigrating(p string, on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if on {
		f.migrating[p] = true
	} else {
		delete(f.migrating, p)
	}
}
func (f *fakePortals) EmitState(context.Context) { f.mu.Lock(); f.emits++; f.mu.Unlock() }

type env struct {
	t       *testing.T
	p       paths.Paths
	oidc    *ssotest.OIDC
	por     *ssotest.Portal
	aws     *awsapitest.Fake
	sso     *sso.Manager
	bk      *backup.Store
	portals *fakePortals
	store   *fakeStore
	eng     *Engine

	mu       sync.Mutex
	progress []api.MigrationProgress
	writes   []string
}

func readFix(t *testing.T, name string) []byte {
	b, err := os.ReadFile(filepath.Join("..", "awsconfig", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newEnv(t *testing.T) *env {
	t.Helper()
	p := paths.ForRoot(t.TempDir())
	if err := p.EnsureAppDirs(); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Dir(p.AWSConfigFile), 0o700)
	os.WriteFile(p.AWSConfigFile, readFix(t, "config.fixture"), 0o600)
	os.WriteFile(p.AWSCredentialsFile, readFix(t, "credentials.fixture"), 0o600)
	e := &env{t: t, p: p, oidc: ssotest.NewOIDC(t), por: ssotest.NewPortal(t), aws: awsapitest.New(t), store: &fakeStore{}}
	e.sso = sso.NewManager(sso.Config{Home: p.Home, RegistrationsDir: p.RegistrationsDir,
		Endpoints: sso.Endpoints{OIDC: e.oidc.URL, Portal: e.por.URL}, Lang: func() string { return "en" }})
	bk, err := backup.Open(p.BackupDir, func() int { return 20 }, nil)
	if err != nil {
		t.Fatal(err)
	}
	e.bk = bk
	e.portals = &fakePortals{files: awsconfig.Files{Config: p.AWSConfigFile, Credentials: p.AWSCredentialsFile}, drafts: map[string]awsconfig.Session{}, migrating: map[string]bool{}}
	e.eng = New(Deps{
		Paths: p, Sessions: e.sso, Portals: e.portals, Backup: bk, Endpoints: awsapi.Endpoints{STS: e.aws.URL, IAM: e.aws.URL}, Store: e.store,
		Emit: func(name string, data any) {
			if name == api.EventMigrationProgress {
				e.mu.Lock()
				e.progress = append(e.progress, data.(api.MigrationProgress))
				e.mu.Unlock()
			}
		},
		OnWrite: func(path string, _ []byte) { e.mu.Lock(); e.writes = append(e.writes, path); e.mu.Unlock() },
	})
	// the fake world: acme's key is an IAM user in 111122223333, the portal lists that account
	e.aws.Identities[oldKeyID] = awsapitest.Identity{Account: "111122223333", Arn: "arn:aws:iam::111122223333:user/acme-dev", UserID: "AIDA1"}
	e.aws.Identities[ssoKeyID] = awsapitest.Identity{Account: "111122223333", Arn: "arn:aws:sts::111122223333:assumed-role/AWSReservedSSO_AdministratorAccess_abc/alex", UserID: "AROA1:alex"}
	e.aws.Keys["acme-dev"] = []awsapitest.Key{{ID: oldKeyID, Status: "Active"}}
	e.por.SetAccounts([]ssotest.Account{{ID: "111122223333", Name: "acme", Email: "s@x@example.com"}})
	e.por.SetRoles("111122223333", "AdministratorAccess")
	e.token("initech", "https://d-1234567890.awsapps.com/start")
	return e
}

func (e *env) token(session, startURL string) {
	tok := &sso.CachedToken{StartURL: startURL, Region: "us-east-1", AccessToken: "AT-seed", ExpiresAt: time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second),
		ClientID: "CID", ClientSecret: "CS", RegistrationExpiresAt: time.Now().Add(80 * 24 * time.Hour), RefreshToken: "RT"}
	if err := sso.WriteToken(sso.TokenCacheDir(e.p.Home), sso.TokenCacheKey(session, ""), tok); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) readFile(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		e.t.Fatal(err)
	}
	return string(b)
}

func (e *env) hash(p string) string { return awsconfig.Hash([]byte(e.readFile(p))) }

func wantCode(t *testing.T, err error, code api.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s, got nil", code)
	}
	if got := api.AsAppError(err).Code; got != code {
		t.Fatalf("expected %s, got %s (%v)", code, got, err)
	}
}

// toReview runs Start..ChooseAccount..Preview for acme on initech.
func (e *env) toReview() api.MigrationState {
	e.t.Helper()
	if _, err := e.eng.Start(bg, "acme"); err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.eng.ChoosePortal("acme", "initech"); err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.eng.ChooseAccount("acme", api.MigrationAccountChoice{AccountID: "111122223333", AccountName: "acme", Role: "AdministratorAccess"}); err != nil {
		e.t.Fatal(err)
	}
	st, err := e.eng.Preview("acme")
	if err != nil {
		e.t.Fatal(err)
	}
	return st
}

func goldenConfig(t *testing.T) string {
	cfg := readFix(t, "config.fixture")
	out, err := awsconfig.MigrateProfile(cfg, "acme", nil, "initech", "111122223333", "AdministratorAccess", "")
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestMigrationHappyPath(t *testing.T) {
	e := newEnv(t)
	st, err := e.eng.Start(bg, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if st.Step != api.MigrationIdentify || !st.KeyInMemory || st.Identity.AccountID != "111122223333" || st.Identity.IAMUser != "acme-dev" ||
		st.Identity.MaskedKeyID != "AKIA••••••••••••MPLE" || st.Identity.IsRoot || st.Identity.ErrorCode != "" {
		t.Fatalf("%+v", st)
	}
	if !e.portals.migrating["acme"] || e.store.identities["acme"].AccountID != "111122223333" {
		t.Fatal("MarkMigrating / identity store")
	}
	if got, _ := e.eng.Start(bg, "acme"); got.Step != api.MigrationIdentify {
		t.Fatal("Start resumes")
	}
	if st, err = e.eng.ChoosePortal("acme", "initech"); err != nil || st.Step != api.MigrationAccount || st.Portal != "initech" || st.PortalIsDraft {
		t.Fatalf("%+v %v", st, err)
	}
	if st, err = e.eng.ChooseAccount("acme", api.MigrationAccountChoice{AccountID: "111122223333", AccountName: "acme", Role: "AdministratorAccess"}); err != nil || st.Step != api.MigrationReview {
		t.Fatalf("%+v %v", st, err)
	}
	st, err = e.eng.Preview("acme")
	if err != nil || st.ConfigPreview == nil || st.ConfigPreview.Added != 3 || st.ConfigPreview.Removed != 0 {
		t.Fatalf("%+v %v", st.ConfigPreview, err)
	}
	origCfg := e.readFile(e.p.AWSConfigFile)

	st, err = e.eng.Apply(bg, "acme", st.ConfigPreview.BaseHash)
	if err != nil {
		t.Fatal(err)
	}
	if st.Step != api.MigrationVerify || st.Verify == nil || !st.Verify.OK || st.Verify.AccountID != "111122223333" || !strings.Contains(st.Verify.Arn, "assumed-role/AWSReservedSSO_AdministratorAccess_") || st.ConfigBackupID == "" || st.ConfigPreview != nil {
		t.Fatalf("%+v", st)
	}
	if e.readFile(e.p.AWSConfigFile) != goldenConfig(t) {
		t.Fatalf("config:\n%s", e.readFile(e.p.AWSConfigFile))
	}
	var seq []string
	for _, p := range e.progress {
		seq = append(seq, string(p.Item)+":"+string(p.Status))
	}
	want := "backup:running,backup:done,write:running,write:done,verify:running,verify:done"
	if strings.Join(seq, ",") != want {
		t.Fatalf("progress %v", seq)
	}
	if !strings.Contains(e.progress[5].Detail, "111122223333 • arn:aws:sts::") || e.progress[1].Detail == "" {
		t.Fatalf("details %+v", e.progress)
	}
	ent, bak, err := e.bk.Get(st.ConfigBackupID)
	if err != nil || string(bak) != origCfg || ent.Kind != api.BackupAWSConfig || ent.Reason != api.ReasonMigrate || ent.ReasonParams["profile"] != "acme" {
		t.Fatalf("%+v %v", ent, err)
	}

	st, err = e.eng.PreviewKeyRemoval("acme")
	if err != nil || st.Step != api.MigrationRemoveKey || st.CredentialsPreview == nil || st.CredentialsPreview.Removed != 4 {
		t.Fatalf("%+v %v", st.CredentialsPreview, err)
	}
	for _, l := range st.CredentialsPreview.Lines {
		if strings.Contains(l.Text, "fakeSecret") || strings.Contains(l.Text, "AKIAIOSF") {
			t.Fatalf("secret in preview: %q", l.Text)
		}
	}
	st, err = e.eng.RemoveKey("acme", st.CredentialsPreview.BaseHash)
	if err != nil {
		t.Fatal(err)
	}
	if st.Step != api.MigrationDeactivate || !st.KeyRemoved || st.CredentialsBackupID == "" || !st.KeyInMemory {
		t.Fatalf("%+v", st)
	}
	wantCreds := "[globex]\naws_access_key_id = AKIAI44QH8DHBEXAMPLE\naws_secret_access_key = fakeSecretValueDoNotUse1111111111111111\n"
	if e.readFile(e.p.AWSCredentialsFile) != wantCreds {
		t.Fatalf("credentials:\n%s", e.readFile(e.p.AWSCredentialsFile))
	}
	cent, cbak, err := e.bk.Get(st.CredentialsBackupID)
	if err != nil || !cent.Redacted || cent.Kind != api.BackupAWSCredentials || cent.Reason != api.ReasonRemoveKey ||
		strings.Contains(string(cbak), "fakeSecret") || strings.Contains(string(cbak), "AKIAIOSFODNN7EXAMPLE") || !strings.Contains(string(cbak), backup.Placeholder) {
		t.Fatalf("%+v %q", cent, cbak)
	}
	if list, _ := e.bk.List(); len(list) != 2 {
		t.Fatalf("%d backups", len(list))
	}
	if len(e.store.pending) != 1 || e.store.pending[0].IAMUser != "acme-dev" || e.store.pending[0].MaskedKeyID != "AKIA••••••••••••MPLE" || e.store.pending[0].Since == "" {
		t.Fatalf("%+v", e.store.pending)
	}

	st, err = e.eng.Deactivate(bg, "acme")
	if err != nil || st.Deactivation == nil || st.Deactivation.Status != api.DeactivationDone || st.Deactivation.Method != api.DeactivateViaSSORole {
		t.Fatalf("%+v %v", st.Deactivation, err)
	}
	if !strings.HasPrefix(st.Deactivation.ConsoleURL, "https://d-1234567890.awsapps.com/start/#/console?account_id=111122223333&role_name=AdministratorAccess&destination=") ||
		!strings.Contains(st.Deactivation.ConsoleURL, "acme-dev") {
		t.Fatal(st.Deactivation.ConsoleURL)
	}
	if e.aws.KeyStatus("acme-dev", oldKeyID) != "Inactive" || len(e.store.pending) != 0 {
		t.Fatal("key must be inactive and the pending entry gone")
	}
	if strings.Join(e.store.history, ",") != "add:acme,remove:acme" {
		t.Fatal(e.store.history)
	}

	st, err = e.eng.Finish("acme")
	if err != nil || st.Step != api.MigrationDone || st.KeyInMemory {
		t.Fatalf("%+v %v", st, err)
	}
	if e.portals.migrating["acme"] || len(e.eng.InProgress()) != 0 {
		t.Fatal("wizard must be closed")
	}
	if _, err := e.eng.Get("acme"); err == nil {
		t.Fatal("state must be gone")
	}
	if len(e.writes) != 2 {
		t.Fatalf("OnWrite calls: %v", e.writes)
	}
	// nothing secret ever left in any state we handed out
	if strings.Contains(jsonOf(st), "fakeSecret") {
		t.Fatal("secret in state")
	}
}

func TestVerifyUsesSSOCredsNotStaticKey(t *testing.T) {
	e := newEnv(t)
	st := e.toReview()
	before := len(e.aws.Requests())
	if _, err := e.eng.Apply(bg, "acme", st.ConfigPreview.BaseHash); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.readFile(e.p.AWSCredentialsFile), oldKeyID) {
		t.Fatal("precondition: static keys must still be in the credentials file")
	}
	reqs := e.aws.Requests()[before:]
	if len(reqs) != 1 || reqs[0].Action != "GetCallerIdentity" {
		t.Fatalf("%+v", reqs)
	}
	if !strings.Contains(reqs[0].Authorization, "Credential="+ssoKeyID+"/") || strings.Contains(reqs[0].Authorization, "AKIAIOSFODNN7EXAMPLE") {
		t.Fatalf("verify must sign with the SSO role key: %s", reqs[0].Authorization)
	}
	if e.por.CredCalls() != 1 {
		t.Fatal("role credentials must come from the portal")
	}
}

func TestVerifyWrongAccount(t *testing.T) {
	e := newEnv(t)
	e.aws.Identities[ssoKeyID] = awsapitest.Identity{Account: "999900001111", Arn: "arn:aws:sts::999900001111:assumed-role/AWSReservedSSO_X_1/alex", UserID: "x"}
	st := e.toReview()
	st, err := e.eng.Apply(bg, "acme", st.ConfigPreview.BaseHash)
	if err != nil {
		t.Fatal(err)
	}
	ae := st.Verify.Error
	if st.Verify.OK || ae == nil || ae.Code != api.ErrVerifyWrongAccount || ae.Params["got"] != "999900001111" || ae.Params["want"] != "111122223333" {
		t.Fatalf("%+v", st.Verify)
	}
	if st.Step != api.MigrationVerify {
		t.Fatal(st.Step)
	}
	if _, err := e.eng.PreviewKeyRemoval("acme"); err == nil {
		t.Fatal("key removal must be refused after a failed verification")
	}
	// the user fixes the world and retries
	e.aws.Identities[ssoKeyID] = awsapitest.Identity{Account: "111122223333", Arn: "arn:aws:sts::111122223333:assumed-role/AWSReservedSSO_A_1/alex", UserID: "x"}
	st, err = e.eng.RetryVerify(bg, "acme")
	if err != nil || !st.Verify.OK {
		t.Fatalf("%+v %v", st.Verify, err)
	}
	// role not assigned
	e2 := newEnv(t)
	e2.por.SetUnauthorized(true)
	st2 := e2.toReview()
	st2, _ = e2.eng.Apply(bg, "acme", st2.ConfigPreview.BaseHash)
	if st2.Verify.OK || st2.Verify.Error.Code != api.ErrSessionExpired {
		t.Fatalf("%+v", st2.Verify)
	}
}

func TestConfirmOtherRequired(t *testing.T) {
	e := newEnv(t)
	e.eng.Start(bg, "acme")
	e.eng.ChoosePortal("acme", "initech")
	_, err := e.eng.ChooseAccount("acme", api.MigrationAccountChoice{AccountID: "777788889999", Role: "AdministratorAccess"})
	wantCode(t, err, api.ErrMigrateConfirmOther)
	st, err := e.eng.ChooseAccount("acme", api.MigrationAccountChoice{AccountID: "777788889999", Role: "AdministratorAccess", ConfirmOther: true})
	if err != nil || st.Step != api.MigrationReview || !st.ConfirmOther {
		t.Fatalf("%+v %v", st, err)
	}
	for name, c := range map[string]api.MigrationAccountChoice{
		"short id": {AccountID: "123", Role: "R"}, "letters": {AccountID: "11112222333x", Role: "R"},
		"no role": {AccountID: "111122223333"}, "bad region": {AccountID: "111122223333", Role: "R", Region: "mars"},
	} {
		if _, err := e.eng.ChooseAccount("acme", c); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	// choosing a portal again clears the account and previews
	st, _ = e.eng.ChoosePortal("acme", "initech")
	if st.AccountID != "" || st.Step != api.MigrationAccount || st.ConfigPreview != nil {
		t.Fatalf("%+v", st)
	}
	_, err = e.eng.ChoosePortal("acme", "nope")
	wantCode(t, err, api.ErrPortalNotFound)
}

func TestStepGuards(t *testing.T) {
	e := newEnv(t)
	for _, f := range []func() error{
		func() error { _, err := e.eng.Get("acme"); return err },
		func() error { _, err := e.eng.ChoosePortal("acme", "initech"); return err },
		func() error { _, err := e.eng.Preview("acme"); return err },
		func() error { _, err := e.eng.Finish("acme"); return err },
	} {
		wantCode(t, f(), api.ErrMigrateState)
	}
	if _, err := e.eng.Start(bg, "initech-admin"); api.AsAppError(err).Code != api.ErrNoStaticKey {
		t.Fatalf("%v", err)
	}
	if _, err := e.eng.Start(bg, "zzz"); api.AsAppError(err).Code != api.ErrNoStaticKey {
		t.Fatalf("%v", err)
	}
	e.eng.Start(bg, "acme") // step identify
	for name, f := range map[string]func() error{
		"choose account at identify": func() error {
			_, err := e.eng.ChooseAccount("acme", api.MigrationAccountChoice{AccountID: "111122223333", Role: "R"})
			return err
		},
		"preview at identify": func() error { _, err := e.eng.Preview("acme"); return err },
		"apply at identify":   func() error { _, err := e.eng.Apply(bg, "acme", ""); return err },
		"retry verify":        func() error { _, err := e.eng.RetryVerify(bg, "acme"); return err },
		"restore":             func() error { _, err := e.eng.RestoreConfig("acme"); return err },
		"preview key removal": func() error { _, err := e.eng.PreviewKeyRemoval("acme"); return err },
		"remove key":          func() error { _, err := e.eng.RemoveKey("acme", ""); return err },
		"keep key":            func() error { _, err := e.eng.KeepKey("acme"); return err },
		"deactivate":          func() error { _, err := e.eng.Deactivate(bg, "acme"); return err },
		"finish":              func() error { _, err := e.eng.Finish("acme"); return err },
	} {
		if err := f(); api.AsAppError(err).Code != api.ErrMigrateState {
			t.Errorf("%s: %v", name, err)
		}
	}
	e.eng.ChoosePortal("acme", "initech")
	e.eng.ChooseAccount("acme", api.MigrationAccountChoice{AccountID: "111122223333", Role: "AdministratorAccess"})
	// review without a preview: Apply refused
	_, err := e.eng.Apply(bg, "acme", "x")
	wantCode(t, err, api.ErrMigrateState)
	// stale hash
	st, _ := e.eng.Preview("acme")
	f, _ := os.OpenFile(e.p.AWSConfigFile, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("# edited by hand\n")
	f.Close()
	edited := e.readFile(e.p.AWSConfigFile)
	_, err = e.eng.Apply(bg, "acme", st.ConfigPreview.BaseHash)
	wantCode(t, err, api.ErrConfigChanged)
	if e.readFile(e.p.AWSConfigFile) != edited {
		t.Fatal("a refused apply must not write")
	}
	if list, _ := e.bk.List(); len(list) != 0 {
		t.Fatal("no backup before a refused write")
	}
	if got, _ := e.eng.Get("acme"); got.LastError == nil || got.LastError.Code != api.ErrConfigChanged {
		t.Fatalf("LastError: %+v", got.LastError)
	}
	// Discard drops everything without touching files
	e.eng.Discard("acme")
	if e.portals.migrating["acme"] || len(e.eng.InProgress()) != 0 {
		t.Fatal("discard")
	}
	if e.readFile(e.p.AWSConfigFile) != edited {
		t.Fatal("discard must not touch files")
	}
}

func TestRestoreConfig(t *testing.T) {
	e := newEnv(t)
	orig := e.readFile(e.p.AWSConfigFile)
	st := e.toReview()
	st, err := e.eng.Apply(bg, "acme", st.ConfigPreview.BaseHash)
	if err != nil {
		t.Fatal(err)
	}
	st, err = e.eng.RestoreConfig("acme")
	if err != nil {
		t.Fatal(err)
	}
	if st.Step != api.MigrationReview || st.Verify != nil || st.ConfigPreview != nil {
		t.Fatalf("%+v", st)
	}
	if e.readFile(e.p.AWSConfigFile) != orig {
		t.Fatalf("config not restored:\n%s", e.readFile(e.p.AWSConfigFile))
	}
	list, _ := e.bk.List()
	if len(list) != 2 || list[0].Reason != api.ReasonRestore || list[0].ReasonParams["file"] == "" {
		t.Fatalf("a safety snapshot is expected: %+v", list)
	}
	// the wizard can run again
	if _, err := e.eng.Preview("acme"); err != nil {
		t.Fatal(err)
	}
}

func TestDeactivateFallbackToOldKey(t *testing.T) {
	e := newEnv(t)
	e.aws.Deny[ssoKeyID+"|UpdateAccessKey"] = true // e.g. PowerUserAccess
	st := e.toReview()
	e.eng.Apply(bg, "acme", st.ConfigPreview.BaseHash)
	st, _ = e.eng.PreviewKeyRemoval("acme")
	e.eng.RemoveKey("acme", st.CredentialsPreview.BaseHash)
	st, err := e.eng.Deactivate(bg, "acme")
	if err != nil || st.Deactivation.Status != api.DeactivationDone || st.Deactivation.Method != api.DeactivateViaOldKey {
		t.Fatalf("%+v %v", st.Deactivation, err)
	}
	reqs := e.aws.Requests()
	last := reqs[len(reqs)-1]
	if last.Action != "UpdateAccessKey" || last.Form.Has("UserName") || last.AccessKeyID != oldKeyID {
		t.Fatalf("%+v", last)
	}
	if e.aws.KeyStatus("acme-dev", oldKeyID) != "Inactive" || len(e.store.pending) != 0 {
		t.Fatal("not deactivated")
	}
	// both denied -> denied, with the error
	e2 := newEnv(t)
	e2.aws.Deny["*|UpdateAccessKey"] = true
	s2 := e2.toReview()
	e2.eng.Apply(bg, "acme", s2.ConfigPreview.BaseHash)
	s2, _ = e2.eng.PreviewKeyRemoval("acme")
	e2.eng.RemoveKey("acme", s2.CredentialsPreview.BaseHash)
	s2, _ = e2.eng.Deactivate(bg, "acme")
	if s2.Deactivation.Status != api.DeactivationDenied || s2.Deactivation.Error == nil || s2.Deactivation.Error.Code != api.ErrAWSAccessDenied || s2.Deactivation.ConsoleURL == "" {
		t.Fatalf("%+v", s2.Deactivation)
	}
	if len(e2.store.pending) != 1 {
		t.Fatal("the pending deactivation must stay when denied")
	}
	// key not found in IAM
	e3 := newEnv(t)
	e3.aws.Keys["acme-dev"] = nil
	s3 := e3.toReview()
	e3.eng.Apply(bg, "acme", s3.ConfigPreview.BaseHash)
	s3, _ = e3.eng.PreviewKeyRemoval("acme")
	e3.eng.RemoveKey("acme", s3.CredentialsPreview.BaseHash)
	s3, _ = e3.eng.Deactivate(bg, "acme")
	if s3.Deactivation.Status != api.DeactivationKeyNotFound || s3.Deactivation.Method != api.DeactivateViaSSORole {
		t.Fatalf("%+v", s3.Deactivation)
	}
}

func TestKeepKeyThenDeactivateAndFinish(t *testing.T) {
	e := newEnv(t)
	st := e.toReview()
	e.eng.Apply(bg, "acme", st.ConfigPreview.BaseHash)
	if _, err := e.eng.PreviewKeyRemoval("acme"); err != nil {
		t.Fatal(err)
	}
	st, err := e.eng.KeepKey("acme")
	if err != nil || st.Step != api.MigrationDeactivate || st.KeyRemoved || st.CredentialsPreview != nil {
		t.Fatalf("%+v %v", st, err)
	}
	if !strings.Contains(e.readFile(e.p.AWSCredentialsFile), oldKeyID) || len(e.store.pending) != 0 {
		t.Fatal("kept key must stay on disk and no pending entry is created")
	}
	st, err = e.eng.Deactivate(bg, "acme")
	if err != nil || st.Deactivation.Status != api.DeactivationDone {
		t.Fatalf("%+v %v", st.Deactivation, err)
	}
	if _, err := e.eng.Finish("acme"); err != nil {
		t.Fatal(err)
	}
}

func TestDeactivateRootKey(t *testing.T) {
	e := newEnv(t)
	e.aws.Identities[oldKeyID] = awsapitest.Identity{Account: "111122223333", Arn: "arn:aws:iam::111122223333:root", UserID: "111122223333"}
	st := e.toReview()
	if !e.eng.wizards["acme"].st.Identity.IsRoot {
		t.Fatal("root not detected")
	}
	e.eng.Apply(bg, "acme", st.ConfigPreview.BaseHash)
	st, _ = e.eng.PreviewKeyRemoval("acme")
	st, _ = e.eng.RemoveKey("acme", st.CredentialsPreview.BaseHash)
	if len(e.store.pending) != 0 {
		t.Fatal("root keys cannot be deactivated from here: no pending entry")
	}
	before := len(e.aws.Requests())
	st, err := e.eng.Deactivate(bg, "acme")
	if err != nil || st.Deactivation.Status != api.DeactivationRootKey || st.Deactivation.ConsoleURL == "" {
		t.Fatalf("%+v %v", st.Deactivation, err)
	}
	if len(e.aws.Requests()) != before {
		t.Fatal("no IAM call for root keys")
	}
	// rejected key: unavailable
	e2 := newEnv(t)
	delete(e2.aws.Identities, oldKeyID)
	e2.eng.Start(bg, "acme")
	if st, _ := e2.eng.Get("acme"); st.Identity.ErrorCode != "InvalidClientTokenId" || st.Identity.AccountID != "" {
		t.Fatalf("%+v", st.Identity)
	}
	e2.eng.ChoosePortal("acme", "initech")
	e2.eng.ChooseAccount("acme", api.MigrationAccountChoice{AccountID: "111122223333", Role: "AdministratorAccess"})
	s2, _ := e2.eng.Preview("acme")
	e2.eng.Apply(bg, "acme", s2.ConfigPreview.BaseHash)
	s2, _ = e2.eng.PreviewKeyRemoval("acme")
	e2.eng.RemoveKey("acme", s2.CredentialsPreview.BaseHash)
	s2, _ = e2.eng.Deactivate(bg, "acme")
	if s2.Deactivation.Status != api.DeactivationUnavailable {
		t.Fatalf("%+v", s2.Deactivation)
	}
}

func TestResumeKeysLeft(t *testing.T) {
	e := newEnv(t)
	// config already migrated, credentials still hold the key (migrated-keys-left)
	os.WriteFile(e.p.AWSConfigFile, []byte(goldenConfig(t)), 0o600)
	st, err := e.eng.Start(bg, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if st.Step != api.MigrationRemoveKey || st.Verify == nil || !st.Verify.OK || st.Portal != "initech" || st.AccountID != "111122223333" || st.Role != "AdministratorAccess" || st.Region != "us-east-1" || !st.KeyInMemory {
		t.Fatalf("%+v", st)
	}
	if st.Identity.IAMUser != "acme-dev" {
		t.Fatalf("identity must be filled via STS: %+v", st.Identity)
	}
	st, err = e.eng.PreviewKeyRemoval("acme")
	if err != nil || st.CredentialsPreview == nil {
		t.Fatal(err)
	}
	st, err = e.eng.RemoveKey("acme", st.CredentialsPreview.BaseHash)
	if err != nil || st.Step != api.MigrationDeactivate {
		t.Fatalf("%+v %v", st, err)
	}
	if st, err = e.eng.Deactivate(bg, "acme"); err != nil || st.Deactivation.Status != api.DeactivationDone {
		t.Fatalf("%+v %v", st.Deactivation, err)
	}
}

func TestKeyInConfigSectionIsRemovedToo(t *testing.T) {
	e := newEnv(t)
	cfg := e.readFile(e.p.AWSConfigFile)
	cfg = strings.Replace(cfg, "[profile acme]\n", "[profile acme]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\naws_secret_access_key = fakeSecretValueDoNotUse0000000000000000\n", 1)
	os.WriteFile(e.p.AWSConfigFile, []byte(cfg), 0o600)
	st := e.toReview()
	st, err := e.eng.Apply(bg, "acme", st.ConfigPreview.BaseHash)
	if err != nil {
		t.Fatal(err)
	}
	st, _ = e.eng.PreviewKeyRemoval("acme")
	st, err = e.eng.RemoveKey("acme", st.CredentialsPreview.BaseHash)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(e.readFile(e.p.AWSConfigFile), "aws_secret_access_key") || strings.Contains(e.readFile(e.p.AWSCredentialsFile), "acme") {
		t.Fatal("keys must be gone from both files")
	}
	list, _ := e.bk.List()
	var cfgBackups int
	for _, b := range list {
		if b.Kind == api.BackupAWSConfig && b.Reason == api.ReasonRemoveKey {
			cfgBackups++
			_, content, _ := e.bk.Get(b.ID)
			if strings.Contains(string(content), "fakeSecret") {
				t.Fatal("config backup must be redacted")
			}
		}
	}
	if cfgBackups != 1 {
		t.Fatalf("%d config removal backups", cfgBackups)
	}
}

func TestDraftPortalWritesSession(t *testing.T) {
	e := newEnv(t)
	e.portals.drafts["acme-sso"] = awsconfig.Session{Name: "acme-sso", StartURL: "https://d-0000000000.awsapps.com/start", Region: "us-east-1", Scopes: []string{"sso:account:access"}, ScopesSet: true}
	e.token("acme-sso", "https://d-0000000000.awsapps.com/start")
	e.eng.Start(bg, "acme")
	st, err := e.eng.ChoosePortal("acme", "acme-sso")
	if err != nil || !st.PortalIsDraft {
		t.Fatalf("%+v %v", st, err)
	}
	e.eng.ChooseAccount("acme", api.MigrationAccountChoice{AccountID: "111122223333", Role: "AdministratorAccess"})
	st, err = e.eng.Preview("acme")
	if err != nil {
		t.Fatal(err)
	}
	if st.ConfigPreview.Added != 8 { // 3 profile keys + blank + header + 3 session keys
		t.Fatalf("added %d", st.ConfigPreview.Added)
	}
	st, err = e.eng.Apply(bg, "acme", st.ConfigPreview.BaseHash)
	if err != nil {
		t.Fatal(err)
	}
	want := goldenConfigWithSession(t)
	if e.readFile(e.p.AWSConfigFile) != want {
		t.Fatalf("config:\n%s", e.readFile(e.p.AWSConfigFile))
	}
	if e.portals.IsDraft("acme-sso") {
		t.Fatal("draft must be consumed")
	}
	if !st.Verify.OK {
		t.Fatalf("%+v", st.Verify)
	}
}

func goldenConfigWithSession(t *testing.T) string {
	cfg := readFix(t, "config.fixture")
	ses := &awsconfig.Session{Name: "acme-sso", StartURL: "https://d-0000000000.awsapps.com/start", Region: "us-east-1", Scopes: []string{"sso:account:access"}, ScopesSet: true}
	out, err := awsconfig.MigrateProfile(cfg, "acme", ses, "acme-sso", "111122223333", "AdministratorAccess", "")
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func jsonOf(v any) string {
	return strings.Join([]string{strings.ReplaceAll(strings.TrimSpace(sprint(v)), "\n", " ")}, "")
}

func sprint(v any) string { return fmt.Sprintf("%+v", v) }

// toKeyRemoval applies the migration and opens the key-removal preview for acme.
func (e *env) toKeyRemoval() api.MigrationState {
	e.t.Helper()
	st := e.toReview()
	if _, err := e.eng.Apply(bg, "acme", st.ConfigPreview.BaseHash); err != nil {
		e.t.Fatal(err)
	}
	st, err := e.eng.PreviewKeyRemoval("acme")
	if err != nil {
		e.t.Fatal(err)
	}
	return st
}

func TestRemoveKeyWithProfilePrefixedCredentialsSection(t *testing.T) {
	e := newEnv(t)
	creds := strings.Replace(e.readFile(e.p.AWSCredentialsFile), "[acme]", "[profile acme]", 1)
	if err := os.WriteFile(e.p.AWSCredentialsFile, []byte(creds), 0o600); err != nil {
		t.Fatal(err)
	}
	st := e.toKeyRemoval()
	if st.CredentialsPreview == nil || st.CredentialsPreview.Removed == 0 {
		t.Fatalf("the preview must show the key lines going away: %+v", st.CredentialsPreview)
	}
	st, err := e.eng.RemoveKey("acme", st.CredentialsPreview.BaseHash)
	if err != nil {
		t.Fatal(err)
	}
	if !st.KeyRemoved {
		t.Fatal("KeyRemoved must be set")
	}
	after := e.readFile(e.p.AWSCredentialsFile)
	if strings.Contains(after, "AKIAIOSFODNN7EXAMPLE") || strings.Contains(after, "fakeSecretValueDoNotUse0000") || strings.Contains(after, "acme") {
		t.Fatalf("the acme key must be gone:\n%s", after)
	}
	if !strings.Contains(after, "[globex]") {
		t.Fatalf("other profiles stay:\n%s", after)
	}
	if len(e.store.pending) != 1 {
		t.Fatalf("a pending deactivation is expected: %+v", e.store.pending)
	}
}

func TestRemoveKeyFailsWhenNothingChanges(t *testing.T) {
	e := newEnv(t)
	st := e.toKeyRemoval()
	credBefore := e.readFile(e.p.AWSCredentialsFile)
	orig := removeStaticKeys
	removeStaticKeys = func(content []byte, section string) ([]byte, bool, error) { return content, false, nil }
	defer func() { removeStaticKeys = orig }()

	_, err := e.eng.RemoveKey("acme", st.CredentialsPreview.BaseHash)
	wantCode(t, err, api.ErrNoStaticKey)
	got, _ := e.eng.Get("acme")
	if got.KeyRemoved || got.Step != api.MigrationRemoveKey {
		t.Fatalf("the wizard must not advance: %+v", got)
	}
	if e.readFile(e.p.AWSCredentialsFile) != credBefore {
		t.Fatal("no file may change")
	}
	if len(e.store.pending) != 0 {
		t.Fatalf("no deactivation may be recorded: %+v", e.store.pending)
	}
	if _, err := e.eng.PreviewKeyRemoval("acme"); err == nil || api.AsAppError(err).Code != api.ErrNoStaticKey {
		t.Fatalf("the preview must fail as well: %v", err)
	}
}

func TestRestoreConfigRefusesAfterExternalEdit(t *testing.T) {
	e := newEnv(t)
	st := e.toReview()
	if _, err := e.eng.Apply(bg, "acme", st.ConfigPreview.BaseHash); err != nil {
		t.Fatal(err)
	}
	edited := e.readFile(e.p.AWSConfigFile) + "\n[profile added-by-hand]\nregion = eu-west-1\n"
	if err := os.WriteFile(e.p.AWSConfigFile, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := e.eng.RestoreConfig("acme")
	wantCode(t, err, api.ErrConfigChanged)
	if api.AsAppError(err).Params["file"] == "" {
		t.Fatal("the error must name the file")
	}
	if e.readFile(e.p.AWSConfigFile) != edited {
		t.Fatal("the externally edited file must stay as it is")
	}
	if list, _ := e.bk.List(); len(list) != 1 {
		t.Fatalf("no safety snapshot is expected for a refused restore: %d", len(list))
	}
}

func TestChooseAccountRejectsUnsafeRole(t *testing.T) {
	e := newEnv(t)
	if _, err := e.eng.Start(bg, "acme"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.eng.ChoosePortal("acme", "initech"); err != nil {
		t.Fatal(err)
	}
	before := e.hash(e.p.AWSConfigFile)
	for _, role := range []string{"AdministratorAccess\ncredential_process = /usr/bin/id", "Administrator Access", "", "a\tb", strings.Repeat("a", 65), "AdministratorAccess]"} {
		_, err := e.eng.ChooseAccount("acme", api.MigrationAccountChoice{AccountID: "111122223333", Role: role})
		wantCode(t, err, api.ErrInvalidInput)
		if api.AsAppError(err).Params["field"] != "role" {
			t.Fatalf("role %q: %+v", role, api.AsAppError(err).Params)
		}
	}
	if _, err := e.eng.ChooseAccount("acme", api.MigrationAccountChoice{AccountID: "111122223333", Role: "acme.Admin+ops@x", ConfirmOther: true}); err != nil {
		t.Fatalf("a valid role must pass: %v", err)
	}
	if e.hash(e.p.AWSConfigFile) != before {
		t.Fatal("choosing an account must not write")
	}
}
