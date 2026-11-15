package transfer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/awsconfig"
	"github.com/rudian/codepass/internal/backup"
	"github.com/rudian/codepass/internal/paths"
)

// Fake credentials are assembled at run time so the source never contains a key-shaped literal.
var (
	fakeKeyID  = "AK" + "IA" + "TESTTESTTEST0001"
	fakeKeyID2 = "AK" + "IA" + "TESTTESTTEST0002"
	fakeSecret = "not-a-real-" + "secret-value-0000000000"
	credText   = "[acme]\naws_access_key_id = " + fakeKeyID + "\n" + "aws_secret_" + "access_key = " + fakeSecret + "\n" +
		"\n[globex]\naws_access_key_id = " + fakeKeyID2 + "\n" + "aws_secret_" + "access_key = " + fakeSecret + "\n"
)

const (
	idInitech = "777788889999"
	urlAcme   = "https://d-1111111111.awsapps.com/start"
)

type memStore struct {
	mu        sync.Mutex
	colors    map[string]int
	monograms map[string]string
	names     map[string]string
	sessions  map[string]api.SessionRecord
}

func newMemStore() *memStore {
	return &memStore{colors: map[string]int{}, monograms: map[string]string{}, names: map[string]string{}, sessions: map[string]api.SessionRecord{}}
}
func (m *memStore) AccountColors() map[string]int {
	m.mu.Lock()
	defer m.mu.Unlock()
	o := map[string]int{}
	for k, v := range m.colors {
		o[k] = v
	}
	return o
}
func (m *memStore) SetAccountColors(c map[string]int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.colors = map[string]int{}
	for k, v := range c {
		m.colors[k] = v
	}
	return nil
}
func (m *memStore) Monograms() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	o := map[string]string{}
	for k, v := range m.monograms {
		o[k] = v
	}
	return o
}
func (m *memStore) SetMonogram(p, v string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v == "" {
		delete(m.monograms, p)
	} else {
		m.monograms[p] = v
	}
	return nil
}
func (m *memStore) AccountNames() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	o := map[string]string{}
	for k, v := range m.names {
		o[k] = v
	}
	return o
}
func (m *memStore) MergeAccountNames(n map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range n {
		m.names[k] = v
	}
	return nil
}
func (m *memStore) SessionRecord(p string) (api.SessionRecord, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.sessions[p]
	return r, ok
}
func (m *memStore) SetSessionRecord(p string, r api.SessionRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[p] = r
	return nil
}

type fixture struct {
	t     *testing.T
	p     paths.Paths
	store *memStore
	svc   *Service
	bk    *backup.Store
	now   time.Time

	signedIn map[string]bool
	changed  int
	writes   int
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func newFixture(t *testing.T, cfg string) *fixture {
	t.Helper()
	root := t.TempDir()
	p := paths.ForRoot(root)
	if err := p.EnsureAppDirs(); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, p: p, store: newMemStore(), now: time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC), signedIn: map[string]bool{}}
	if cfg != "" {
		f.writeConfig(cfg)
		if err := os.WriteFile(p.AWSCredentialsFile, []byte(credText), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bk, err := backup.Open(p.BackupDir, func() int { return 20 }, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	f.bk = bk
	f.svc = New(Deps{Paths: p, Backup: bk, Store: f.store, State: f.state, Now: func() time.Time { return f.now },
		Changed: func(context.Context) { f.changed++ }, OnWrite: func(string, []byte) { f.writes++ }})
	return f
}

func (f *fixture) writeConfig(s string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.p.AWSConfigFile), 0o700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(f.p.AWSConfigFile, []byte(s), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) config() string {
	b, _ := os.ReadFile(f.p.AWSConfigFile)
	return string(b)
}

// state builds the slice of the accounts state the service reads.
func (f *fixture) state(context.Context) (api.AccountsState, error) {
	snap, err := awsconfig.Load(awsconfig.Files{Config: f.p.AWSConfigFile, Credentials: f.p.AWSCredentialsFile})
	if err != nil {
		return api.AccountsState{}, err
	}
	st := api.AccountsState{}
	colors := f.store.AccountColors()
	names := f.store.AccountNames()
	for _, s := range snap.Sessions() {
		m := 480
		if r, ok := f.store.SessionRecord(s.Name); ok && r.SessionMinutes > 0 {
			m = r.SessionMinutes
		}
		state := api.SessionSignedOut
		if f.signedIn[s.Name] {
			state = api.SessionActive
		}
		st.Portals = append(st.Portals, api.Portal{Name: s.Name, Session: api.SessionStatus{Portal: s.Name, State: state, SessionMinutes: m}})
	}
	for _, p := range snap.Profiles() {
		slot := colors[p.AccountID]
		if slot == 0 && p.AccountID != "" {
			// the engine persists the automatic colours the first time it computes the state
			slot = 1
			colors[p.AccountID] = 1
			f.store.colors[p.AccountID] = 1
		}
		st.Profiles = append(st.Profiles, api.Profile{Name: p.Name, Kind: p.Kind, Portal: p.SSOSession, AccountID: p.AccountID, ColorSlot: slot, AccountName: names[p.AccountID]})
	}
	return st, nil
}

func (f *fixture) exportAll(extras bool) api.ExportResult {
	f.t.Helper()
	res, err := f.svc.BuildExport(context.Background(), api.ExportRequest{Portals: []string{"initech"}, Profiles: []string{"initech-admin", "initech-readonly", "acme", "globex"}, IncludeExtras: extras})
	if err != nil {
		f.t.Fatal(err)
	}
	return res
}

func (f *fixture) preview(req api.ImportRequest) api.ImportPreview {
	f.t.Helper()
	p, err := f.svc.PreviewImport(req)
	if err != nil {
		f.t.Fatalf("preview: %v", err)
	}
	return p
}

func (f *fixture) apply(req api.ImportRequest) (api.ImportResult, error) {
	p, err := f.svc.PreviewImport(req)
	if err != nil {
		return api.ImportResult{}, err
	}
	return f.svc.ApplyImport(context.Background(), req, p.BaseHash)
}

func item(t *testing.T, p api.ImportPreview, key string) api.ImportItem {
	t.Helper()
	for _, it := range p.Items {
		if it.Key == key {
			return it
		}
	}
	t.Fatalf("no item %s in %+v", key, p.Items)
	return api.ImportItem{}
}

func doc(portals, profiles string) string {
	return `{"format":"aws-sso-hub.accounts","version":1,"exportedAt":"2026-11-04T10:00:00Z","exportedBy":"Codepass 0.0.1","portals":[` + portals + `],"profiles":[` + profiles + `]}`
}

const acmePortal = `{"name":"acme","startUrl":"` + urlAcme + `","region":"us-east-1","scopes":["sso:account:access"],"sessionMinutes":600}`
const acmeProfile = `{"name":"acme-admin","portal":"acme","accountId":"111122223333","roleName":"AdministratorAccess","region":"us-east-1","output":"json","accountName":"acme","color":4,"monogram":"AA"}`

// ---- export ---------------------------------------------------------------------------------------------------------

func TestExportOnlySSONoSecrets(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	res := f.exportAll(true)
	keyRE := regexp.MustCompile(`A(KI|SI)A[0-9A-Z]{16}`)
	if keyRE.MatchString(res.JSON) || strings.Contains(res.JSON, fakeSecret) || strings.Contains(strings.ToLower(res.JSON), "secret") {
		t.Fatalf("secret in export:\n%s", res.JSON)
	}
	var d fileDoc
	if err := json.Unmarshal([]byte(res.JSON), &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Portals) != 1 || len(d.Profiles) != 2 {
		t.Fatalf("%+v", d)
	}
	reasons := map[string]string{}
	for _, it := range res.Items {
		if it.Kind == api.ExportKindProfile && !it.Included {
			reasons[it.Name] = it.ReasonKey
		}
	}
	if reasons["acme"] != "export.static" || reasons["globex"] != "export.static" {
		t.Fatalf("static profiles must be listed as skipped: %v", reasons)
	}
	if d.Format != FormatName || d.Version != 1 || d.ExportedAt != "2026-11-04T10:00:00Z" || !strings.HasPrefix(d.ExportedBy, "Codepass ") {
		t.Fatalf("%+v", d)
	}
}

func TestExportAllowListOnly(t *testing.T) {
	cfg := readFixture(t, "config.fixture") + "\n[profile initech-extra]\nsso_session = initech\nsso_account_id = 777788889999\nsso_role_name = ReadOnlyAccess\ncli_pager =\nrole_arn = arn:aws:iam::777788889999:role/x\ncredential_process = /bin/echo\nweb_identity_token_file = /tmp/t\n"
	f := newFixture(t, cfg)
	res, err := f.svc.BuildExport(context.Background(), api.ExportRequest{Portals: []string{"initech"}, Profiles: []string{"initech-extra", "initech-admin"}, IncludeExtras: true})
	if err != nil {
		t.Fatal(err)
	}
	var generic struct {
		Portals  []map[string]any `json:"portals"`
		Profiles []map[string]any `json:"profiles"`
	}
	if err := json.Unmarshal([]byte(res.JSON), &generic); err != nil {
		t.Fatal(err)
	}
	allowedP := map[string]bool{"name": true, "startUrl": true, "region": true, "scopes": true, "sessionMinutes": true}
	allowedQ := map[string]bool{"name": true, "portal": true, "accountId": true, "roleName": true, "region": true, "output": true, "accountName": true, "color": true, "monogram": true}
	for _, m := range generic.Portals {
		for k := range m {
			if !allowedP[k] {
				t.Fatalf("portal key %s", k)
			}
		}
	}
	for _, m := range generic.Profiles {
		for k := range m {
			if !allowedQ[k] {
				t.Fatalf("profile key %s", k)
			}
		}
	}
	for _, bad := range []string{"role_arn", "credential_process", "web_identity", "cli_pager", "/bin/echo"} {
		if strings.Contains(res.JSON, bad) {
			t.Fatalf("%s leaked into the export", bad)
		}
	}
	// without extras only the core keys remain
	res2, _ := f.svc.BuildExport(context.Background(), api.ExportRequest{Portals: []string{"initech"}, Profiles: []string{"initech-admin"}})
	for _, k := range []string{"accountName", "color", "monogram", "sessionMinutes"} {
		if strings.Contains(res2.JSON, `"`+k+`"`) {
			t.Fatalf("%s present without extras", k)
		}
	}
}

func TestExportSelectionAndGuards(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	res, err := f.svc.BuildExport(context.Background(), api.ExportRequest{Profiles: []string{"initech-admin"}})
	if err != nil {
		t.Fatal(err)
	}
	if HasIncluded(res) {
		t.Fatalf("a profile without its portal must not be exported: %+v", res.Items)
	}
	var reason string
	for _, it := range res.Items {
		if it.Name == "initech-admin" {
			reason = it.ReasonKey
		}
	}
	if reason != "export.noPortal" {
		t.Fatalf("reason %q", reason)
	}
	if _, err := f.svc.BuildExport(context.Background(), api.ExportRequest{Portals: []string{"nope"}}); api.AsAppError(err).Code != api.ErrPortalNotFound {
		t.Fatalf("%v", err)
	}
	if _, err := f.svc.BuildExport(context.Background(), api.ExportRequest{Profiles: []string{"nope"}}); api.AsAppError(err).Code != api.ErrProfileNotFound {
		t.Fatalf("%v", err)
	}
}

func TestWriteExportAndReadImportFile(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	res := f.exportAll(true)
	dir := t.TempDir()
	path := filepath.Join(dir, DefaultExportName(f.now))
	if filepath.Base(path) != "codepass-accounts-20261104.json" {
		t.Fatal(path)
	}
	if _, err := f.svc.WriteExport("relative.json", res); err == nil {
		t.Fatal("relative path must be refused")
	}
	out, err := f.svc.WriteExport(path, res)
	if err != nil || out.Path != path {
		t.Fatal(err, out.Path)
	}
	src, err := f.svc.ReadImportFile(path)
	if err != nil || src.Name != filepath.Base(path) || src.JSON != res.JSON {
		t.Fatalf("%v %+v", err, src.Name)
	}
	big := filepath.Join(dir, "big.json")
	os.WriteFile(big, []byte(strings.Repeat(" ", MaxFileBytes+1)), 0o600)
	if _, err := f.svc.ReadImportFile(big); api.AsAppError(err).Code != api.ErrImportFormat {
		t.Fatalf("%v", err)
	}
	if _, err := f.svc.ReadImportFile(dir); api.AsAppError(err).Code != api.ErrFileIO {
		t.Fatalf("a folder is not a file: %v", err)
	}
	if _, err := f.svc.ReadImportFile(filepath.Join(dir, "missing.json")); api.AsAppError(err).Code != api.ErrFileIO {
		t.Fatalf("%v", err)
	}
}

func TestExportImportRoundTripSame(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	res := f.exportAll(true)
	before := f.config()
	req := api.ImportRequest{JSON: res.JSON, ImportExtras: true}
	p := f.preview(req)
	if len(p.Items) != 3 {
		t.Fatalf("%+v", p.Items)
	}
	for _, it := range p.Items {
		if it.Action != api.ImportActionSame {
			t.Fatalf("%s is %s", it.Key, it.Action)
		}
	}
	if p.ToWrite != 0 || p.ExtrasCount != 0 || p.Preview.Added != 0 || p.Preview.Removed != 0 {
		t.Fatalf("%+v", p)
	}
	r, err := f.svc.ApplyImport(context.Background(), req, p.BaseHash)
	if err != nil {
		t.Fatal(err)
	}
	if r.Added+r.Replaced+r.Renamed+r.Skipped+r.Rejected != 0 || r.Write.BackupID != "" || f.writes != 0 || f.config() != before {
		t.Fatalf("%+v writes=%d", r, f.writes)
	}
	if entries, _ := f.bk.List(); len(entries) != 0 {
		t.Fatal("no backup expected when nothing is written")
	}
}

// ---- import --------------------------------------------------------------------------------------------------------

func TestImportNewPortalAndProfileGolden(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	req := api.ImportRequest{JSON: doc(acmePortal, acmeProfile)}
	p := f.preview(req)
	if item(t, p, "portal:acme").Action != api.ImportActionNew || item(t, p, "profile:acme-admin").Action != api.ImportActionNew || p.ToWrite != 2 {
		t.Fatalf("%+v", p.Items)
	}
	r, err := f.svc.ApplyImport(context.Background(), req, p.BaseHash)
	if err != nil {
		t.Fatal(err)
	}
	want := readFixture(t, "config.fixture") + `
[sso-session acme]
sso_start_url           = ` + urlAcme + `
sso_region              = us-east-1
sso_registration_scopes = sso:account:access

[profile acme-admin]
sso_session    = acme
sso_account_id = 111122223333
sso_role_name  = AdministratorAccess
region         = us-east-1
output         = json
`
	if got := f.config(); got != want {
		t.Fatalf("config mismatch:\n--- got\n%s\n--- want\n%s", got, want)
	}
	if r.Added != 2 || r.Write.BackupID == "" || len(r.NeedSignIn) != 1 || r.NeedSignIn[0] != "acme" || f.changed != 1 {
		t.Fatalf("%+v changed=%d", r, f.changed)
	}
	// the original bytes stay untouched: the chinese comment block and the alignment survive
	if !strings.Contains(f.config(), "# ===== 平台基础设施（2026-03-01 添加）=====") {
		t.Fatal("comment block lost")
	}
	entries, _ := f.bk.List()
	if len(entries) != 1 || entries[0].Reason != api.ReasonImport || entries[0].ReasonParams["n"] != "2" {
		t.Fatalf("%+v", entries)
	}
	// importing the same file again is a no-op
	p2 := f.preview(req)
	for _, it := range p2.Items {
		if it.Action != api.ImportActionSame {
			t.Fatalf("second preview: %s is %s", it.Key, it.Action)
		}
	}
}

func TestImportNewProfileUsesExistingPortal(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	prof := `{"name":"initech-ro","portal":"initech","accountId":"777788889999","roleName":"ReadOnlyAccess"}`
	r, err := f.apply(api.ImportRequest{JSON: doc("", prof)})
	if err != nil || r.Added != 1 {
		t.Fatalf("%v %+v", err, r)
	}
	if !strings.HasSuffix(f.config(), "\n[profile initech-ro]\nsso_session    = initech\nsso_account_id = 777788889999\nsso_role_name  = ReadOnlyAccess\nregion         = us-west-2\noutput         = json\n") {
		t.Fatalf("%s", f.config())
	}
	if len(r.NeedSignIn) != 1 || r.NeedSignIn[0] != "initech" {
		t.Fatalf("%v", r.NeedSignIn)
	}
	f.signedIn["initech"] = true
	prof2 := `{"name":"initech-ro2","portal":"initech","accountId":"777788889999","roleName":"ReadOnlyAccess"}`
	r, err = f.apply(api.ImportRequest{JSON: doc("", prof2)})
	if err != nil || len(r.NeedSignIn) != 0 {
		t.Fatalf("%v %+v", err, r)
	}
}

func TestImportConflictChoices(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	conflict := `{"name":"initech-admin","portal":"initech","accountId":"777788889999","roleName":"ReadOnlyAccess","region":"eu-west-1"}`
	req := api.ImportRequest{JSON: doc("", conflict)}
	p := f.preview(req)
	it := item(t, p, "profile:initech-admin")
	if it.Action != api.ImportActionConflict || it.Choice != "skip" || it.Suggested != "initech-admin-2" || !it.CanReplace {
		t.Fatalf("%+v", it)
	}

	// skip (default): nothing changes
	before := f.config()
	r, err := f.apply(req)
	if err != nil || r.Skipped != 1 || f.config() != before {
		t.Fatalf("%v %+v", err, r)
	}

	// rename with the suggestion
	req.Choices = map[string]string{"profile:initech-admin": "rename"}
	r, err = f.apply(req)
	if err != nil || r.Renamed != 1 {
		t.Fatalf("%v %+v", err, r)
	}
	if !strings.Contains(f.config(), "[profile initech-admin-2]\nsso_session    = initech\nsso_account_id = 777788889999\nsso_role_name  = ReadOnlyAccess\nregion         = eu-west-1\n") {
		t.Fatalf("%s", f.config())
	}
	if !strings.Contains(f.config(), "[profile initech-admin]\nsso_session    = initech\nsso_account_id = 777788889999\nsso_role_name  = AdministratorAccess\n") {
		t.Fatal("original profile must stay")
	}

	// rename to a taken or invalid name is refused
	req.Renames = map[string]string{"profile:initech-admin": "initech-readonly"}
	if _, err := f.svc.PreviewImport(req); api.AsAppError(err).Code != api.ErrProfileExists {
		t.Fatalf("%v", err)
	}
	req.Renames = map[string]string{"profile:initech-admin": "bad name"}
	if _, err := f.svc.PreviewImport(req); api.AsAppError(err).Code != api.ErrInvalidName {
		t.Fatalf("%v", err)
	}

	// replace keeps the alignment and the other keys
	req.Choices = map[string]string{"profile:initech-admin": "replace"}
	req.Renames = nil
	r, err = f.apply(req)
	if err != nil || r.Replaced != 1 {
		t.Fatalf("%v %+v", err, r)
	}
	if !strings.Contains(f.config(), "[profile initech-admin]\nsso_session    = initech\nsso_account_id = 777788889999\nsso_role_name  = ReadOnlyAccess\nregion         = eu-west-1\noutput         = json\n") {
		t.Fatalf("%s", f.config())
	}

	// unknown choice
	req.Choices = map[string]string{"profile:initech-admin": "merge"}
	if _, err := f.svc.PreviewImport(req); api.AsAppError(err).Code != api.ErrInvalidInput {
		t.Fatalf("%v", err)
	}
}

func TestImportConflictWithStaticProfile(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	prof := `{"name":"acme","portal":"initech","accountId":"111122223333","roleName":"AdministratorAccess"}`
	req := api.ImportRequest{JSON: doc("", prof)}
	it := item(t, f.preview(req), "profile:acme")
	if it.Action != api.ImportActionConflict || it.CanReplace || it.Suggested != "acme-2" {
		t.Fatalf("%+v", it)
	}
	req.Choices = map[string]string{"profile:acme": "replace"}
	if _, err := f.svc.PreviewImport(req); api.AsAppError(err).Code != api.ErrInvalidInput {
		t.Fatalf("a static profile must not be replaced: %v", err)
	}
}

func TestImportPortalConflict(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	portal := `{"name":"initech","startUrl":"https://d-9999999999.awsapps.com/start","region":"eu-west-1"}`
	prof := `{"name":"initech-x","portal":"initech","accountId":"777788889999","roleName":"ReadOnlyAccess"}`
	req := api.ImportRequest{JSON: doc(portal, prof)}
	p := f.preview(req)
	pi := item(t, p, "portal:initech")
	if pi.Action != api.ImportActionConflict || pi.Suggested != "initech-2" {
		t.Fatalf("%+v", pi)
	}
	xi := item(t, p, "profile:initech-x")
	if len(xi.Warnings) != 1 || xi.Warnings[0] != "portal-differs:initech" || xi.Portal != "initech" {
		t.Fatalf("%+v", xi)
	}
	// rename: the profile follows the renamed portal
	req.Choices = map[string]string{"portal:initech": "rename"}
	if _, err := f.apply(req); err != nil {
		t.Fatal(err)
	}
	cfg := f.config()
	if !strings.Contains(cfg, "[sso-session initech-2]\nsso_start_url           = https://d-9999999999.awsapps.com/start\nsso_region              = eu-west-1\nsso_registration_scopes = sso:account:access\n") ||
		!strings.Contains(cfg, "[profile initech-x]\nsso_session    = initech-2\n") {
		t.Fatalf("%s", cfg)
	}
	// replace changes the three keys in place
	f2 := newFixture(t, readFixture(t, "config.fixture"))
	req.Choices = map[string]string{"portal:initech": "replace"}
	if _, err := f2.apply(req); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f2.config(), "[sso-session initech]\nsso_start_url  = https://d-9999999999.awsapps.com/start\nsso_region     = eu-west-1\nsso_registration_scopes = sso:account:access\n") {
		t.Fatalf("%s", f2.config())
	}
}

func TestImportMapsExistingPortalByURL(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	portal := `{"name":"corp-sso","startUrl":"HTTPS://D-1234567890.awsapps.com/start/","region":"us-west-2"}`
	prof := `{"name":"corp-ro","portal":"corp-sso","accountId":"777788889999","roleName":"ReadOnlyAccess","region":"us-west-2"}`
	req := api.ImportRequest{JSON: doc(portal, prof)}
	p := f.preview(req)
	pi := item(t, p, "portal:corp-sso")
	if pi.Action != api.ImportActionMapped || pi.Target != "initech" {
		t.Fatalf("%+v", pi)
	}
	if got := item(t, p, "profile:corp-ro"); got.Action != api.ImportActionNew || got.Portal != "initech" {
		t.Fatalf("%+v", got)
	}
	r, err := f.svc.ApplyImport(context.Background(), req, p.BaseHash)
	if err != nil || r.Added != 1 {
		t.Fatalf("%v %+v", err, r)
	}
	cfg := f.config()
	if strings.Contains(cfg, "corp-sso") || !strings.Contains(cfg, "[profile corp-ro]\nsso_session    = initech\n") {
		t.Fatalf("no duplicate session expected:\n%s", cfg)
	}
}

func TestImportRejectsSecretsAndCommands(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	bad := []string{
		`{"name":"p1","portal":"initech","accountId":"777788889999","roleName":"R","credential_process":"/bin/true"}`,
		`{"name":"p2","portal":"initech","accountId":"777788889999","roleName":"R","aws_access_key_id":"x"}`,
		`{"name":"p3","portal":"initech","accountId":"777788889999","roleName":"R","aws_secret_access_key":"x"}`,
		`{"name":"p4","portal":"initech","accountId":"777788889999","roleName":"R","aws_session_token":"x"}`,
		`{"name":"p5","portal":"initech","accountId":"777788889999","roleName":"R","role_arn":"arn:aws:iam::1:role/x"}`,
		`{"name":"p6","portal":"initech","accountId":"777788889999","roleName":"R","source_profile":"acme"}`,
		`{"name":"p7","portal":"initech","accountId":"777788889999","roleName":"R","credential_source":"Ec2InstanceMetadata"}`,
		`{"name":"p8","portal":"initech","accountId":"777788889999","roleName":"R","web_identity_token_file":"/x"}`,
		`{"name":"p9","portal":"initech","accountId":"777788889999","roleName":"R","myPassword":"x"}`,
		`{"name":"p10","portal":"initech","accountId":"777788889999","roleName":"R","Run-Command":"x"}`,
		`{"name":"p11","portal":"initech","accountId":"777788889999","roleName":"R","roleArn":"x"}`,
		`{"name":"p12","portal":"initech","accountId":"777788889999","roleName":"R","nested":{"deep":{"credential_process":"x"}}}`,
		`{"name":"p13","portal":"initech","accountId":"777788889999","roleName":"R","accountName":"` + fakeKeyID + `"}`,
	}
	good := `{"name":"good","portal":"initech","accountId":"777788889999","roleName":"ReadOnlyAccess"}`
	req := api.ImportRequest{JSON: doc("", strings.Join(append(bad, good), ","))}
	p := f.preview(req)
	if len(p.Items) != len(bad)+1 {
		t.Fatalf("%d items", len(p.Items))
	}
	for _, it := range p.Items {
		if it.Name == "good" {
			if it.Action != api.ImportActionNew {
				t.Fatalf("the rest of the file must still import: %+v", it)
			}
			continue
		}
		if it.Action != api.ImportActionRejected || it.ReasonKey != "import.rejected.secret" || it.Include {
			t.Fatalf("%s: %+v", it.Name, it)
		}
	}
	r, err := f.svc.ApplyImport(context.Background(), req, p.BaseHash)
	if err != nil || r.Added != 1 || r.Rejected != len(bad) {
		t.Fatalf("%v %+v", err, r)
	}
	cfg := f.config()
	for _, s := range []string{"credential_process", "role_arn", "source_profile", "/bin/true", "p1]"} {
		if strings.Contains(cfg, s) {
			t.Fatalf("%q was written:\n%s", s, cfg)
		}
	}
	// a rejected entry cannot be re-included by the caller
	req.Include = map[string]bool{"profile:p1": true}
	if it := item(t, f.preview(req), "profile:p1"); it.Include {
		t.Fatal("rejected entries stay rejected")
	}
}

func TestImportRejectsNewlineInjection(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	cases := map[string]string{
		"region":  `{"name":"evil","portal":"initech","accountId":"777788889999","roleName":"R","region":"us-east-1\n[profile evil]\ncredential_process = x"}`,
		"role":    `{"name":"evil2","portal":"initech","accountId":"777788889999","roleName":"R\r\n[x]"}`,
		"name":    `{"name":"evil3\n","portal":"initech","accountId":"777788889999","roleName":"R"}`,
		"unknown": `{"name":"evil4","portal":"initech","accountId":"777788889999","roleName":"R","note":"a\u2028b"}`,
		"nul":     `{"name":"evil5","portal":"initech","accountId":"777788889999","roleName":"R","accountName":"a\u0000b"}`,
		"key":     `{"name":"evil6","portal":"initech","accountId":"777788889999","roleName":"R","k\ney":"x"}`,
	}
	portalBad := `{"name":"pp","startUrl":"https://d-1234567899.awsapps.com/start\n[profile evil]","region":"us-east-1"}`
	var all []string
	for _, c := range cases {
		all = append(all, c)
	}
	req := api.ImportRequest{JSON: doc(portalBad, strings.Join(all, ","))}
	p := f.preview(req)
	for _, it := range p.Items {
		if it.Action != api.ImportActionRejected || it.Code != api.ErrUnsafeValue || it.ReasonKey != "import.rejected.invalid" {
			t.Fatalf("%s: %+v", it.Name, it)
		}
		if strings.ContainsAny(it.Name, "\n\r\x00\u2028") {
			t.Fatalf("name not sanitised: %q", it.Name)
		}
	}
	before := f.config()
	r, err := f.svc.ApplyImport(context.Background(), req, p.BaseHash)
	if err != nil || r.Rejected != len(cases)+1 || f.config() != before {
		t.Fatalf("%v %+v", err, r)
	}
	if strings.Contains(f.config(), "evil") {
		t.Fatal("injected text reached the file")
	}
}

func TestImportValidatesFields(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	mk := func(field, val string) string {
		m := map[string]any{"name": "v", "portal": "initech", "accountId": "777788889999", "roleName": "ReadOnlyAccess"}
		var v any
		_ = json.Unmarshal([]byte(val), &v)
		m[field] = v
		b, _ := json.Marshal(m)
		return string(b)
	}
	cases := []struct {
		field, val string
		code       api.ErrorCode
		detail     string
	}{
		{"name", `"bad name"`, api.ErrInvalidName, "name"},
		{"portal", `"bad portal!"`, api.ErrInvalidName, "portal"},
		{"accountId", `"12345"`, api.ErrInvalidInput, "accountId"},
		{"accountId", `7`, api.ErrInvalidInput, "accountId"},
		{"roleName", `"has space"`, api.ErrInvalidInput, "roleName"},
		{"region", `"mars-1"`, api.ErrInvalidRegion, "region"},
		{"output", `"xml"`, api.ErrInvalidInput, "output"},
		{"color", `9`, api.ErrInvalidInput, "color"},
		{"color", `2.5`, api.ErrInvalidInput, "color"},
		{"monogram", `"ABC"`, api.ErrInvalidInput, "monogram"},
	}
	for _, c := range cases {
		p := f.preview(api.ImportRequest{JSON: doc("", mk(c.field, c.val))})
		it := p.Items[0]
		if it.Action != api.ImportActionRejected || it.Code != c.code || it.Detail != c.detail || it.ReasonKey != "import.rejected.invalid" {
			t.Fatalf("%s=%s: %+v", c.field, c.val, it)
		}
	}
	badPortals := []struct {
		raw    string
		code   api.ErrorCode
		detail string
	}{
		{`{"name":"x","startUrl":"http://d-1.awsapps.com/start","region":"us-east-1"}`, api.ErrInvalidStartURL, "startUrl"},
		{`{"name":"x","startUrl":"https://example.com/start","region":"us-east-1"}`, api.ErrInvalidStartURL, "startUrl"},
		{`{"name":"x","startUrl":"https://d-1.awsapps.com/start","region":"nowhere"}`, api.ErrInvalidRegion, "region"},
		{`{"name":"x","startUrl":"https://d-1.awsapps.com/start","region":"us-east-1","sessionMinutes":5}`, api.ErrInvalidInput, "sessionMinutes"},
		{`{"name":"x","startUrl":"https://d-1.awsapps.com/start","region":"us-east-1","scopes":["a b"]}`, api.ErrInvalidInput, "scopes"},
		{`{"name":"x y","startUrl":"https://d-1.awsapps.com/start","region":"us-east-1"}`, api.ErrInvalidName, "name"},
		{`"just a string"`, "", "entry"},
	}
	for _, c := range badPortals {
		p := f.preview(api.ImportRequest{JSON: doc(c.raw, "")})
		it := p.Items[0]
		if it.Action != api.ImportActionRejected || it.Code != c.code || it.Detail != c.detail {
			t.Fatalf("%s: %+v", c.raw, it)
		}
	}
	// warnings for unknown keys, duplicates, missing portal
	p := f.preview(api.ImportRequest{JSON: doc("", `{"name":"w","portal":"initech","accountId":"777788889999","roleName":"R1","comment":"hi"},{"name":"w","portal":"initech","accountId":"777788889999","roleName":"R2"},{"name":"nop","portal":"ghost","accountId":"777788889999","roleName":"R3"}`)})
	if got := p.Items[0]; got.Action != api.ImportActionNew || len(got.Warnings) != 1 || got.Warnings[0] != "unknown-key:comment" {
		t.Fatalf("%+v", got)
	}
	if got := p.Items[1]; got.Action != api.ImportActionRejected || got.Detail != "name" {
		t.Fatalf("duplicate: %+v", got)
	}
	if got := p.Items[2]; got.Action != api.ImportActionRejected || got.ReasonKey != "import.rejected.noPortal" || got.Detail != "ghost" {
		t.Fatalf("%+v", got)
	}
}

func TestImportVersionTooNew(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	cases := map[string]string{
		"version":  `{"format":"aws-sso-hub.accounts","version":2,"portals":[],"profiles":[]}`,
		"format":   `{"format":"something-else","version":1}`,
		"missing":  `{"version":1}`,
		"ver-str":  `{"format":"aws-sso-hub.accounts","version":"1"}`,
		"not-json": `hello`,
		"array":    `[]`,
		"trailing": `{"format":"aws-sso-hub.accounts","version":1} {}`,
		"portals":  `{"format":"aws-sso-hub.accounts","version":1,"portals":{"a":1}}`,
	}
	for name, js := range cases {
		_, err := f.svc.PreviewImport(api.ImportRequest{JSON: js})
		ae := api.AsAppError(err)
		if ae == nil || ae.Code != api.ErrImportFormat {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := f.svc.PreviewImport(api.ImportRequest{JSON: strings.Repeat(" ", MaxFileBytes+1)}); api.AsAppError(err).Params["reason"] != "size" {
		t.Fatalf("%v", err)
	}
	// unknown top-level keys are ignored; empty lists and null are fine
	p := f.preview(api.ImportRequest{JSON: `{"format":"aws-sso-hub.accounts","version":1,"future":{"x":1},"portals":null}`})
	if len(p.Items) != 0 || p.Items == nil {
		t.Fatalf("%+v", p)
	}
	// a BOM in front is accepted
	if _, err := f.svc.PreviewImport(api.ImportRequest{JSON: "\ufeff" + doc("", "")}); err != nil {
		t.Fatal(err)
	}
}

func TestImportBackupAndBaseHash(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	req := api.ImportRequest{JSON: doc(acmePortal, acmeProfile)}
	p := f.preview(req)
	if p.BaseHash == "" || p.BaseHash != p.Preview.BaseHash || p.Preview.Added == 0 || p.Preview.Removed != 0 {
		t.Fatalf("%+v", p.Preview)
	}
	// the file changes between preview and apply
	f.writeConfig(f.config() + "\n[profile other]\nregion = us-east-1\n")
	changed := f.config()
	if _, err := f.svc.ApplyImport(context.Background(), req, p.BaseHash); api.AsAppError(err).Code != api.ErrConfigChanged {
		t.Fatalf("%v", err)
	}
	if f.config() != changed {
		t.Fatal("a stale write must not touch the file")
	}
	if entries, _ := f.bk.List(); len(entries) != 0 {
		t.Fatal("no backup for a refused write")
	}
	// fresh preview works, exactly one backup holding the pre-import content
	p = f.preview(req)
	if _, err := f.svc.ApplyImport(context.Background(), req, p.BaseHash); err != nil {
		t.Fatal(err)
	}
	entries, _ := f.bk.List()
	if len(entries) != 1 || entries[0].Kind != api.BackupAWSConfig {
		t.Fatalf("%+v", entries)
	}
	bak, err := os.ReadFile(filepath.Join(f.p.BackupDir, entries[0].ID+".bak"))
	if err != nil || string(bak) != changed {
		t.Fatalf("backup must hold the previous config: %v", err)
	}
	// permissions of the written file
	if fi, err := os.Stat(f.p.AWSConfigFile); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("%v %v", err, fi.Mode())
	}
}

func TestImportReadOnlyConfig(t *testing.T) {
	f := newFixture(t, "[profile a]\nsso_session = nope\ngarbage line\n")
	if _, err := f.svc.PreviewImport(api.ImportRequest{JSON: doc(acmePortal, "")}); api.AsAppError(err).Code != api.ErrReadOnly {
		t.Fatalf("%v", err)
	}
}

func TestImportMissingConfigFile(t *testing.T) {
	f := newFixture(t, "")
	req := api.ImportRequest{JSON: doc(acmePortal, acmeProfile)}
	p := f.preview(req)
	r, err := f.svc.ApplyImport(context.Background(), req, p.BaseHash)
	if err != nil || r.Added != 2 || r.Write.BackupID != "" {
		t.Fatalf("%v %+v", err, r)
	}
	if !strings.HasPrefix(f.config(), "[sso-session acme]\n") {
		t.Fatalf("%q", f.config())
	}
}

func TestImportExtrasMerge(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	// existing user choices
	f.store.colors[idInitech] = 2
	f.store.names[idInitech] = "initech"
	f.store.monograms["initech-admin"] = "IA"
	f.store.sessions["initech"] = api.SessionRecord{SessionMinutes: 240, LoginAt: "2026-11-04T08:00:00Z"}

	portals := acmePortal + `,{"name":"initech","startUrl":"https://d-1234567890.awsapps.com/start","region":"us-west-2","scopes":["sso:account:access"],"sessionMinutes":720}`
	profiles := acmeProfile + `,{"name":"initech-admin","portal":"initech","accountId":"` + idInitech + `","roleName":"AdministratorAccess","region":"us-west-2","output":"json","accountName":"other-name","color":7,"monogram":"ZZ"}`
	req := api.ImportRequest{JSON: doc(portals, profiles), ImportExtras: true}
	p := f.preview(req)
	// acme: colour, name, monogram, session length; initech: nothing (user choices stay)
	if p.ExtrasCount != 4 {
		t.Fatalf("extras %d", p.ExtrasCount)
	}
	if _, err := f.svc.ApplyImport(context.Background(), req, p.BaseHash); err != nil {
		t.Fatal(err)
	}
	if f.store.colors["111122223333"] != 4 || f.store.names["111122223333"] != "acme" || f.store.monograms["acme-admin"] != "AA" || f.store.sessions["acme"].SessionMinutes != 600 {
		t.Fatalf("%+v %+v %+v %+v", f.store.colors, f.store.names, f.store.monograms, f.store.sessions)
	}
	if f.store.colors[idInitech] != 2 || f.store.names[idInitech] != "initech" || f.store.monograms["initech-admin"] != "IA" || f.store.sessions["initech"].SessionMinutes != 240 {
		t.Fatalf("existing choices were overwritten: %+v %+v %+v %+v", f.store.colors, f.store.names, f.store.monograms, f.store.sessions)
	}
	if f.store.sessions["initech"].LoginAt == "" {
		t.Fatal("session record fields must survive")
	}

	// extras off: nothing is merged
	f2 := newFixture(t, readFixture(t, "config.fixture"))
	req.ImportExtras = false
	p2 := f2.preview(req)
	if p2.ExtrasCount != 0 {
		t.Fatal(p2.ExtrasCount)
	}
	if _, err := f2.svc.ApplyImport(context.Background(), req, p2.BaseHash); err != nil {
		t.Fatal(err)
	}
	if f2.store.colors["111122223333"] == 4 || len(f2.store.names)+len(f2.store.monograms)+len(f2.store.sessions) != 0 {
		t.Fatal("extras merged although switched off")
	}

	// replace overwrites the user's values for that entry
	f3 := newFixture(t, readFixture(t, "config.fixture"))
	f3.store.colors[idInitech] = 2
	f3.store.monograms["initech-admin"] = "IA"
	req3 := api.ImportRequest{JSON: doc("", profiles[len(acmeProfile)+1:]), ImportExtras: true,
		Choices: map[string]string{"profile:initech-admin": "replace"}}
	// make it a real conflict
	req3.JSON = strings.Replace(req3.JSON, `"roleName":"AdministratorAccess"`, `"roleName":"PowerUserAccess"`, 1)
	p3 := f3.preview(req3)
	if _, err := f3.svc.ApplyImport(context.Background(), req3, p3.BaseHash); err != nil {
		t.Fatal(err)
	}
	if f3.store.colors[idInitech] != 7 || f3.store.monograms["initech-admin"] != "ZZ" {
		t.Fatalf("%+v %+v", f3.store.colors, f3.store.monograms)
	}

	// an item that is skipped brings no extras
	f4 := newFixture(t, readFixture(t, "config.fixture"))
	req4 := api.ImportRequest{JSON: req3.JSON, ImportExtras: true}
	if p4 := f4.preview(req4); p4.ExtrasCount != 0 {
		t.Fatalf("skipped conflict must not bring extras: %d", p4.ExtrasCount)
	}
}

func TestImportIncludeFlags(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	req := api.ImportRequest{JSON: doc(acmePortal, acmeProfile), Include: map[string]bool{"portal:acme": false}}
	p := f.preview(req)
	if it := item(t, p, "portal:acme"); it.Include || it.Action != api.ImportActionNew {
		t.Fatalf("%+v", it)
	}
	if it := item(t, p, "profile:acme-admin"); it.Action != api.ImportActionRejected || it.ReasonKey != "import.rejected.noPortal" {
		t.Fatalf("a profile of an unticked new portal cannot be imported: %+v", it)
	}
	r, err := f.svc.ApplyImport(context.Background(), req, p.BaseHash)
	if err != nil || r.Skipped != 1 || r.Rejected != 1 || r.Added != 0 {
		t.Fatalf("%v %+v", err, r)
	}
	if strings.Contains(f.config(), "acme") && !strings.Contains(readFixture(t, "config.fixture"), "acme") {
		t.Fatal("unticked portal was written")
	}
	// unticking a profile only skips that profile
	req = api.ImportRequest{JSON: doc(acmePortal, acmeProfile), Include: map[string]bool{"profile:acme-admin": false}}
	r, err = f.apply(req)
	if err != nil || r.Added != 1 || r.Skipped != 1 {
		t.Fatalf("%v %+v", err, r)
	}
	if !strings.Contains(f.config(), "[sso-session acme]") || strings.Contains(f.config(), "[profile acme-admin]") {
		t.Fatal(f.config())
	}
}

func TestImportNeverTouchesCredentials(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	credBefore, _ := os.ReadFile(f.p.AWSCredentialsFile)
	credStat, _ := os.Stat(f.p.AWSCredentialsFile)
	// even an entry named like a static profile only ever edits the config file
	js := doc(acmePortal, acmeProfile+`,{"name":"globex","portal":"acme","accountId":"444455556666","roleName":"R"}`)
	req := api.ImportRequest{JSON: js, Choices: map[string]string{"profile:globex": "rename"}, ImportExtras: true}
	if _, err := f.apply(req); err != nil {
		t.Fatal(err)
	}
	credAfter, _ := os.ReadFile(f.p.AWSCredentialsFile)
	credStat2, _ := os.Stat(f.p.AWSCredentialsFile)
	if string(credBefore) != string(credAfter) || !credStat.ModTime().Equal(credStat2.ModTime()) {
		t.Fatal("credentials file changed")
	}
	if entries, _ := f.bk.List(); len(entries) != 1 || entries[0].Kind != api.BackupAWSConfig {
		t.Fatalf("%+v", entries)
	}
	// nothing secret is in the app data either
	filepath.Walk(f.p.AppDataDir, func(path string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			b, _ := os.ReadFile(path)
			if strings.Contains(string(b), fakeKeyID) || strings.Contains(string(b), fakeSecret) {
				t.Fatalf("secret in %s", path)
			}
		}
		return nil
	})
}

func TestImportPreviewHasNoSideEffects(t *testing.T) {
	f := newFixture(t, readFixture(t, "config.fixture"))
	before := f.config()
	f.preview(api.ImportRequest{JSON: doc(acmePortal, acmeProfile), ImportExtras: true})
	if f.config() != before || f.writes != 0 || len(f.store.names)+len(f.store.sessions) != 0 {
		t.Fatal("preview must not write")
	}
	if entries, _ := f.bk.List(); len(entries) != 0 {
		t.Fatal("preview must not back up")
	}
}

func TestUniqueName(t *testing.T) {
	taken := map[string]bool{"a": true, "a-2": true}
	if got := uniqueName("a", taken, 64); got != "a-3" {
		t.Fatal(got)
	}
	long := strings.Repeat("x", 64)
	if got := uniqueName(long, map[string]bool{}, 64); len(got) != 64 || !strings.HasSuffix(got, "-2") {
		t.Fatal(got)
	}
}
