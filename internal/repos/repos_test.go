package repos_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/codecommit"
	"github.com/rudian/codepass/internal/execx"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/gitx/gitxtest"
	"github.com/rudian/codepass/internal/paths"
	"github.com/rudian/codepass/internal/projects"
	"github.com/rudian/codepass/internal/repos"
)

const (
	hostAP      = "git-codecommit.us-east-1.amazonaws.com"
	acctInitech = "777788889999"
	acctAcme    = "111122223333"
	acctGlobex  = "444455556666"
)

func ccURL(name string) string { return "https://" + hostAP + "/v1/repos/" + name }

type fakeStore struct {
	mu    sync.Mutex
	roots []string
	extra []string
	tests map[string]api.TestResult
	lang  string
	// noAutoBind turns the "bind on add" setting off.
	noAutoBind bool
}

func (s *fakeStore) ScanRoots() []string { return s.roots }
func (s *fakeStore) ExtraRepos() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.extra...)
}
func (s *fakeStore) AddExtraRepo(p string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.extra = append(s.extra, p)
	return nil
}
func (s *fakeStore) RemoveExtraRepo(p string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.extra[:0:0]
	for _, x := range s.extra {
		if x != p {
			kept = append(kept, x)
		}
	}
	s.extra = kept
	return nil
}
func (s *fakeStore) AutoBindOnAdd() bool { return !s.noAutoBind }
func (s *fakeStore) RepoTests() map[string]api.TestResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]api.TestResult{}
	for k, v := range s.tests {
		out[k] = v
	}
	return out
}
func (s *fakeStore) Lang() string { return s.lang }

type snap struct {
	Path, Display string
	Kind          api.BackupKind
	Reason        api.BackupReason
	Params        map[string]string
}

type fakeBackup struct {
	mu    sync.Mutex
	snaps []snap
}

func (b *fakeBackup) Snapshot(path, display string, kind api.BackupKind, reason api.BackupReason, params map[string]string) (api.BackupEntry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.snaps = append(b.snaps, snap{path, display, kind, reason, params})
	return api.BackupEntry{ID: fmt.Sprintf("bk%d", len(b.snaps)), Path: path, Kind: kind, Reason: reason}, nil
}

type fakeCC struct {
	account string
	repos   map[string]string
}

func (f *fakeCC) BatchGetRepositories(ctx context.Context, names []string) (codecommit.BatchResult, error) {
	res := codecommit.BatchResult{Found: []codecommit.RepoMeta{}, NotFound: []string{}, KMSErrors: []string{}}
	for _, n := range names {
		if id := f.repos[n]; id != "" {
			res.Found = append(res.Found, codecommit.RepoMeta{Name: n, AccountID: f.account, RepositoryID: id})
		} else {
			res.NotFound = append(res.NotFound, n)
		}
	}
	return res, nil
}
func (f *fakeCC) ListRepositories(ctx context.Context) ([]codecommit.RepoMeta, error) {
	var out []codecommit.RepoMeta
	for n, id := range f.repos {
		out = append(out, codecommit.RepoMeta{Name: n, AccountID: f.account, RepositoryID: id})
	}
	return out, nil
}

type fixture struct {
	t       *testing.T
	sb      *gitxtest.Sandbox
	eng     *repos.Engine
	store   *fakeStore
	reg     *projects.Store
	clock   time.Time // the engine's Now
	backup  *fakeBackup
	aws     string
	awsOK   *bool
	events  *eventLog
	profs   *[]repos.ProfileInfo
	ccAPIs  map[string]*fakeCC
	code    string
	remote  *func(region, name string) string
	gitHook func(g *gitx.Git) *gitx.Git
}

type eventLog struct {
	mu   sync.Mutex
	list []string
	last map[string]any
}

func (l *eventLog) emit(name string, data any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.list = append(l.list, name)
	if l.last == nil {
		l.last = map[string]any{}
	}
	l.last[name] = data
}
func (l *eventLog) count(name string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, x := range l.list {
		if x == name {
			n++
		}
	}
	return n
}

func prov(key string) aws.CredentialsProvider {
	return credentials.NewStaticCredentialsProvider(key, "fakeSecret", "")
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	sb := gitxtest.New(t)
	f := &fixture{t: t, sb: sb, store: &fakeStore{tests: map[string]api.TestResult{}, lang: "en"}, backup: &fakeBackup{}, events: &eventLog{}}
	f.code = filepath.Join(sb.Root, "code")
	if err := os.MkdirAll(f.code, 0o755); err != nil {
		t.Fatal(err)
	}
	f.reg = projects.OpenMemory()
	f.clock = time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	f.store.roots = []string{f.code}
	f.aws = sb.FakeAWS(t, filepath.Join(sb.Root, "fakebin"), "ok")
	ok := true
	f.awsOK = &ok
	profs := []repos.ProfileInfo{
		{Name: "acme", AccountID: acctAcme, Kind: api.ProfileKindStatic, ColorSlot: 1, Monogram: "AC", Usable: true, Provider: prov("ACME")},
		{Name: "initech-admin", AccountID: acctInitech, Portal: "initech", Kind: api.ProfileKindSSO, ColorSlot: 3, Monogram: "IA", Usable: true, Provider: prov("INITECH")},
		{Name: "initech-readonly", AccountID: acctInitech, Portal: "initech", Kind: api.ProfileKindSSO, ColorSlot: 3, Monogram: "IR", Usable: true, Provider: prov("INITECH")},
		{Name: "globex", AccountID: acctGlobex, Kind: api.ProfileKindStatic, ColorSlot: 6, Monogram: "GL", Usable: true, Provider: prov("GLOBEX")},
	}
	f.profs = &profs
	f.ccAPIs = map[string]*fakeCC{
		"ACME":    {account: acctAcme, repos: map[string]string{}},
		"INITECH": {account: acctInitech, repos: map[string]string{}},
		"GLOBEX":  {account: acctGlobex, repos: map[string]string{}},
	}
	f.remote = new(func(region, name string) string)
	f.gitHook = func(g *gitx.Git) *gitx.Git { return g }
	f.eng = repos.New(repos.Deps{
		Paths: paths.Paths{Home: sb.Home},
		Git:   func(ctx context.Context) (*gitx.Git, error) { return f.gitHook(sb.Git), nil },
		AWSPath: func() (string, error) {
			if f.aws == "" {
				return "", execx.ErrNotFound
			}
			return f.aws, nil
		},
		AWSReady: func() bool { return *f.awsOK },
		Profiles: func(ctx context.Context) ([]repos.ProfileInfo, error) {
			return append([]repos.ProfileInfo(nil), *f.profs...), nil
		},
		Store: f.store, Registry: f.reg, Backup: f.backup,
		CodeCommit: func(p aws.CredentialsProvider, region string) codecommit.API {
			cr, _ := p.Retrieve(context.Background())
			return f.ccAPIs[cr.AccessKeyID]
		},
		Emit: f.events.emit,
		Now:  func() time.Time { return f.clock },
		GOOS: "darwin",
		RemoteURLFor: func(region, name string) string {
			if fn := *f.remote; fn != nil {
				return fn(region, name)
			}
			return ccURL(name)
		},
	})
	return f
}

// newRepo creates a CodeCommit clone under <root>/code/<dir>.
func (f *fixture) newRepo(dir, remote string) string {
	return f.sb.InitRepo(f.t, filepath.Join("code", dir), map[string]string{"origin": remote})
}

func (f *fixture) state() api.ReposState {
	f.t.Helper()
	st, err := f.eng.State(context.Background())
	if err != nil {
		f.t.Fatal(err)
	}
	return st
}

func (f *fixture) byName(st api.ReposState, dir string) api.Repo {
	f.t.Helper()
	for _, r := range st.Repos {
		if strings.HasSuffix(r.Path, string(filepath.Separator)+dir) {
			return r
		}
	}
	f.t.Fatalf("repo %s not in state: %+v", dir, st.Repos)
	return api.Repo{}
}

func appendFile(t *testing.T, path, text string) {
	t.Helper()
	fh, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	if _, err := fh.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) realLayout() {
	f.sb.WriteGlobal(f.t, "[credential]\n\thelper = \n\thelper = !aws codecommit credential-helper $@ --profile acme\n\tusehttppath = true\n"+
		"[credential \""+ccURL("urlrule_repo")+"\"]\n\thelper = \n\thelper = !aws --profile initech-admin codecommit credential-helper $@\n")
}

func TestScanBuildsRepos(t *testing.T) {
	f := newFixture(t)
	f.realLayout()
	managed := f.newRepo("managed", ccURL("managed_repo"))
	f.newRepo("gdefault", ccURL("gdefault_repo"))
	f.newRepo("urlrule", ccURL("urlrule_repo"))
	f.newRepo("sshrepo", "ssh://"+hostAP+"/v1/repos/ssh_repo")
	f.newRepo("grc", "codecommit::us-east-1://globex@grc_repo")
	f.sb.InitRepo(t, "code/github", map[string]string{"origin": "https://github.com/o/r.git"})
	if err := f.sb.Git.Bind(context.Background(), gitx.BindSpec{RepoDir: managed, Hosts: []string{hostAP}, Profile: "initech-admin", AccountID: acctInitech, AWSPath: f.aws, Lang: "en", GOOS: "darwin"}); err != nil {
		t.Fatal(err)
	}
	st := f.state()
	if len(st.Repos) != 5 || st.Counts.All != 5 || st.Counts.ToFix != 3 || st.Counts.Bound != 2 {
		t.Fatalf("counts %+v repos %d", st.Counts, len(st.Repos))
	}
	m := f.byName(st, "managed")
	if m.Binding.Source != api.BindingLocal || m.Binding.Profile != "initech-admin" || !m.Binding.ProfileKnown || m.Binding.AccountID != acctInitech ||
		m.Binding.ColorSlot != 3 || m.Binding.Monogram != "IA" || m.ToFix || m.Name != "managed_repo" || m.Region != "us-east-1" ||
		m.RemoteKind != api.RemoteHTTPS || m.Remote != "origin" || m.HTTPSURL != ccURL("managed_repo") || m.ID == "" {
		t.Errorf("managed: %+v", m)
	}
	if g := f.byName(st, "gdefault"); g.Binding.Source != api.BindingGlobalDefault || g.Binding.Profile != "acme" || !g.ToFix || !g.Binding.ProfileKnown {
		t.Errorf("gdefault: %+v", g.Binding)
	}
	if u := f.byName(st, "urlrule"); u.Binding.Source != api.BindingGlobalURL || u.Binding.Profile != "initech-admin" || u.Binding.URLRule != ccURL("urlrule_repo") {
		t.Errorf("urlrule: %+v", u.Binding)
	}
	if s := f.byName(st, "sshrepo"); s.Binding.Source != api.BindingNone || s.RemoteKind != api.RemoteSSH || !s.ToFix || s.HTTPSURL != ccURL("ssh_repo") {
		t.Errorf("ssh: %+v", s)
	}
	if g := f.byName(st, "grc"); g.Binding.Source != api.BindingRemoteURL || g.Binding.Profile != "globex" || g.ToFix || g.RemoteKind != api.RemoteGRC {
		t.Errorf("grc: %+v", g)
	}
	if st.GlobalDefault == nil || st.GlobalDefault.Profile != "acme" || st.GlobalDefault.AffectedRepos != 1 || st.GlobalDefault.ProfileKind != api.ProfileKindStatic || st.GlobalDefault.Monogram != "AC" {
		t.Errorf("global default: %+v", st.GlobalDefault)
	}
	if !st.AWSCLIReady || st.Scanning || st.ScannedAt != "2026-11-04T10:00:00Z" || len(st.Roots) != 1 {
		t.Errorf("flags: %+v", st)
	}
	// sorted by name
	for i := 1; i < len(st.Repos); i++ {
		if st.Repos[i-1].Name > st.Repos[i].Name {
			t.Errorf("not sorted: %s > %s", st.Repos[i-1].Name, st.Repos[i].Name)
		}
	}
	if f.events.count(api.EventReposChanged) < 1 || f.events.count(api.EventScanProgress) < 1 {
		t.Errorf("events: %v", f.events.list)
	}
	// slices are never nil
	for _, r := range st.Repos {
		if r.Owner.Candidates == nil || r.Owner.SkippedPortals == nil {
			t.Errorf("nil slices in %s", r.Name)
		}
	}
}

func TestBoundCountsAndGlobalDefault(t *testing.T) {
	f := newFixture(t)
	f.realLayout()
	f.newRepo("gdefault", ccURL("gdefault_repo"))
	f.newRepo("gdefault2", ccURL("gdefault2_repo"))
	f.newRepo("urlrule", ccURL("urlrule_repo"))
	f.newRepo("plain", ccURL("plain_repo"))
	f.state()
	counts := f.eng.BoundCounts()
	if counts["acme"] != 3 || counts["initech-admin"] != 1 || len(counts) != 2 {
		t.Errorf("counts: %v", counts)
	}
	p, n, ok := f.eng.GlobalDefault()
	if !ok || p != "acme" || n != 3 {
		t.Errorf("global default: %q %d %v", p, n, ok)
	}
	hosts := f.eng.Hosts()
	if len(hosts) != 1 || hosts[0] != hostAP {
		t.Errorf("hosts: %v", hosts)
	}
	if path, ok := f.eng.RepoPath(f.state().Repos[0].ID); !ok || !strings.Contains(path, "code") {
		t.Errorf("RepoPath: %q %v", path, ok)
	}
	if _, ok := f.eng.RepoPath("nope"); ok {
		t.Error("unknown id")
	}
	// no global default at all
	f2 := newFixture(t)
	f2.newRepo("a", ccURL("a_repo"))
	f2.state()
	if _, _, ok := f2.eng.GlobalDefault(); ok {
		t.Error("expected no global default")
	}
	if st := f2.state(); st.GlobalDefault != nil {
		t.Errorf("state global default: %+v", st.GlobalDefault)
	}
}

func TestDetectOwnersMapsResults(t *testing.T) {
	f := newFixture(t)
	f.newRepo("one", ccURL("svc_one"))
	f.newRepo("two", ccURL("svc_two"))
	f.newRepo("three", ccURL("svc_three"))
	f.ccAPIs["INITECH"].repos["svc_one"] = "rid-one"
	f.ccAPIs["ACME"].repos["svc_two"] = "rid-two"
	// globex is signed out
	(*f.profs)[3].Usable, (*f.profs)[3].NotSignedInPortal, (*f.profs)[3].Provider = false, "globex-portal", nil
	f.state()
	st, err := f.eng.DetectOwners(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	one := f.byName(st, "one").Owner
	if one.Status != api.OwnerSingle || len(one.Candidates) != 1 || one.Candidates[0].AccountID != acctInitech || one.Candidates[0].Profile != "initech-admin" ||
		one.Candidates[0].ColorSlot != 3 || one.Candidates[0].Monogram != "IA" || len(one.Candidates[0].Profiles) != 2 {
		t.Errorf("one: %+v", one)
	}
	if one.CheckedWith != 2 || one.Skipped != 1 || len(one.SkippedPortals) != 1 || one.SkippedPortals[0] != "globex-portal" || one.CheckedAt == "" {
		t.Errorf("one counters: %+v", one)
	}
	if two := f.byName(st, "two").Owner; two.Status != api.OwnerSingle || two.Candidates[0].Profile != "acme" {
		t.Errorf("two: %+v", two)
	}
	// nothing found, one account skipped -> partial
	if three := f.byName(st, "three").Owner; three.Status != api.OwnerPartial || len(three.Candidates) != 0 {
		t.Errorf("three: %+v", three)
	}
	if st.Detecting {
		t.Error("Detecting must be false afterwards")
	}
	// all accounts usable again: nothing found -> none
	(*f.profs)[3].Usable, (*f.profs)[3].NotSignedInPortal, (*f.profs)[3].Provider = true, "", prov("GLOBEX")
	st, _ = f.eng.DetectOwners(context.Background(), []string{f.byName(st, "three").ID})
	if three := f.byName(st, "three").Owner; three.Status != api.OwnerNone || three.Skipped != 0 {
		t.Errorf("three again: %+v", three)
	}
	// the bound profile is preferred inside its account
	r1 := f.byName(st, "one")
	if _, err := f.eng.Bind(context.Background(), r1.ID, "initech-readonly"); err != nil {
		t.Fatal(err)
	}
	st, _ = f.eng.DetectOwners(context.Background(), []string{r1.ID})
	if c := f.byName(st, "one").Owner.Candidates[0]; c.Profile != "initech-readonly" {
		t.Errorf("bound profile should be preferred: %+v", c)
	}
}

func TestDetectAmbiguousAcrossAccounts(t *testing.T) {
	f := newFixture(t)
	f.newRepo("acme_cs_portal", ccURL("acme_cs_portal"))
	f.newRepo("acme_cs_portal_copy", ccURL("acme_cs_portal"))
	f.ccAPIs["ACME"].repos["acme_cs_portal"] = "rid-acme"
	f.ccAPIs["INITECH"].repos["acme_cs_portal"] = "rid-initech"
	f.state()
	st, err := f.eng.DetectOwners(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"acme_cs_portal", "acme_cs_portal_copy"} {
		o := f.byName(st, d).Owner
		if o.Status != api.OwnerAmbiguous || len(o.Candidates) != 2 {
			t.Errorf("%s: %+v", d, o)
		}
	}
}

func TestBindUpdatesStateAndBacksUp(t *testing.T) {
	f := newFixture(t)
	repo := f.newRepo("r", ccURL("r_repo"))
	st := f.state()
	id := f.byName(st, "r").ID
	before := f.events.count(api.EventReposChanged)
	pv, err := f.eng.PreviewBind(context.Background(), id, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(pv.Commands) != 8 || len(pv.Warnings) != 1 || pv.Warnings[0] != api.BindWarnStatic {
		t.Errorf("preview: %+v", pv)
	}
	o, err := f.eng.Bind(context.Background(), id, "initech-admin")
	if err != nil {
		t.Fatal(err)
	}
	if !o.OK || o.Repo == nil || o.Repo.Binding.Source != api.BindingLocal || o.Repo.Binding.Profile != "initech-admin" || o.Repo.ToFix || o.BackupID != "bk1" {
		t.Errorf("outcome: %+v", o)
	}
	if len(f.backup.snaps) != 1 {
		t.Fatalf("snapshots: %+v", f.backup.snaps)
	}
	s := f.backup.snaps[0]
	if s.Path != filepath.Join(repo, ".git", "config") || s.Kind != api.BackupGitLocal || s.Reason != api.ReasonBind || s.Params["repo"] != "r_repo" {
		t.Errorf("snapshot: %+v", s)
	}
	if f.events.count(api.EventReposChanged) != before+1 {
		t.Errorf("events: %d vs %d", f.events.count(api.EventReposChanged), before)
	}
	cur := f.state()
	if cur.Counts.Bound != 1 || cur.Counts.ToFix != 0 {
		t.Errorf("counts: %+v", cur.Counts)
	}
	// preview of a re-bind warns about replacing
	pv, _ = f.eng.PreviewBind(context.Background(), id, "initech-readonly")
	if len(pv.Warnings) != 1 || pv.Warnings[0] != api.BindWarnReplaces {
		t.Errorf("re-bind warnings: %+v", pv.Warnings)
	}
	// unknown profile
	if _, err := f.eng.Bind(context.Background(), id, "nope"); err == nil || api.AsAppError(err).Code != api.ErrProfileNotFound {
		t.Errorf("unknown profile: %v", err)
	}
	if _, err := f.eng.Bind(context.Background(), "nope", "acme"); err == nil || api.AsAppError(err).Code != api.ErrRepoNotFound {
		t.Errorf("unknown repo: %v", err)
	}
	// unsafe profile names never reach git
	(*f.profs)[0].Name = "a'b"
	if _, err := f.eng.Bind(context.Background(), id, "a'b"); err == nil || api.AsAppError(err).Code != api.ErrUnsafeValue {
		t.Errorf("unsafe profile: %v", err)
	}
}

func TestBindWarnings(t *testing.T) {
	f := newFixture(t)
	f.newRepo("r", ccURL("r_repo"))
	f.ccAPIs["INITECH"].repos["r_repo"] = "rid"
	(*f.profs)[2].Usable, (*f.profs)[2].NotSignedInPortal = false, "initech"
	f.state()
	st, _ := f.eng.DetectOwners(context.Background(), nil)
	id := f.byName(st, "r").ID
	pv, err := f.eng.PreviewBind(context.Background(), id, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(warnStrings(pv.Warnings), ",") != "static-profile,owner-mismatch" {
		t.Errorf("warnings %v", pv.Warnings)
	}
	pv, _ = f.eng.PreviewBind(context.Background(), id, "initech-readonly")
	if strings.Join(warnStrings(pv.Warnings), ",") != "signed-out" {
		t.Errorf("warnings %v", pv.Warnings)
	}
}

func warnStrings(ws []api.BindWarning) []string {
	var out []string
	for _, w := range ws {
		out = append(out, string(w))
	}
	return out
}

func TestBindRequiresAWSReady(t *testing.T) {
	f := newFixture(t)
	f.newRepo("r", ccURL("r_repo"))
	id := f.byName(f.state(), "r").ID
	*f.awsOK = false
	f.aws = ""
	for name, fn := range map[string]func() error{
		"Bind": func() error { _, err := f.eng.Bind(context.Background(), id, "acme"); return err },
		"BindMany": func() error {
			_, err := f.eng.BindMany(context.Background(), []api.BindItem{{RepoID: id, Profile: "acme"}})
			return err
		},
		"Test":     func() error { _, err := f.eng.Test(context.Background(), id); return err },
		"Convert":  func() error { _, err := f.eng.ConvertToHTTPS(context.Background(), id, "acme"); return err },
		"Repair":   func() error { _, err := f.eng.RepairAll(context.Background()); return err },
		"TestMany": func() error { _, err := f.eng.TestMany(context.Background(), []string{id}); return err },
		"Clone": func() error {
			_, err := f.eng.Clone(context.Background(), api.CloneRequest{Profile: "acme"})
			return err
		},
		"Preview":   func() error { _, err := f.eng.PreviewBind(context.Background(), id, "acme"); return err },
		"BindManyX": func() error { _, err := f.eng.BindMany(context.Background(), nil); return err },
	} {
		err := fn()
		if err == nil || api.AsAppError(err).Code != api.ErrAWSCLIMissing {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(f.backup.snaps) != 0 {
		t.Errorf("nothing may be written: %+v", f.backup.snaps)
	}
	// too old: the binary exists but is not ready
	f.aws = filepath.Join(f.sb.Root, "fakebin", "aws")
	if _, err := f.eng.Bind(context.Background(), id, "acme"); err == nil || api.AsAppError(err).Code != api.ErrAWSCLITooOld {
		t.Errorf("too old: %v", err)
	}
}

func TestBindRejectsSSHAndGRC(t *testing.T) {
	f := newFixture(t)
	f.newRepo("ssh", "ssh://"+hostAP+"/v1/repos/ssh_repo")
	f.newRepo("grc", "codecommit://globex@grc_repo")
	st := f.state()
	for dir, code := range map[string]api.ErrorCode{"ssh": api.ErrRemoteSSH, "grc": api.ErrRemoteGRC} {
		id := f.byName(st, dir).ID
		if _, err := f.eng.Bind(context.Background(), id, "acme"); err == nil || api.AsAppError(err).Code != code {
			t.Errorf("%s: %v", dir, err)
		}
		if _, err := f.eng.PreviewBind(context.Background(), id, "acme"); err == nil || api.AsAppError(err).Code != code {
			t.Errorf("%s preview: %v", dir, err)
		}
	}
	// BindMany reports per item
	outs, err := f.eng.BindMany(context.Background(), []api.BindItem{{RepoID: f.byName(st, "ssh").ID, Profile: "acme"}, {RepoID: "nope", Profile: "acme"}})
	if err != nil || len(outs) != 2 || outs[0].OK || outs[0].Error == nil || outs[0].Error.Code != api.ErrRemoteSSH || outs[1].Error == nil || outs[1].Error.Code != api.ErrRepoNotFound {
		t.Errorf("BindMany: %+v %v", outs, err)
	}
}

func TestBindManyContinuesAfterFailure(t *testing.T) {
	f := newFixture(t)
	f.newRepo("a", ccURL("a_repo"))
	f.newRepo("ssh", "ssh://"+hostAP+"/v1/repos/ssh_repo")
	f.newRepo("b", ccURL("b_repo"))
	st := f.state()
	events := f.events.count(api.EventReposChanged)
	outs, err := f.eng.BindMany(context.Background(), []api.BindItem{
		{RepoID: f.byName(st, "a").ID, Profile: "acme"}, {RepoID: f.byName(st, "ssh").ID, Profile: "acme"}, {RepoID: f.byName(st, "b").ID, Profile: "globex"},
	})
	if err != nil || len(outs) != 3 || !outs[0].OK || outs[1].OK || !outs[2].OK {
		t.Fatalf("%+v %v", outs, err)
	}
	if got := f.events.count(api.EventReposChanged) - events; got != 1 {
		t.Errorf("BindMany must emit once, emitted %d", got)
	}
	if len(f.backup.snaps) != 2 {
		t.Errorf("snapshots: %d", len(f.backup.snaps))
	}
	if outs[2].Repo.Binding.Profile != "globex" {
		t.Errorf("%+v", outs[2].Repo.Binding)
	}
}

func TestUnbindFallsBackToGlobal(t *testing.T) {
	f := newFixture(t)
	f.realLayout()
	f.newRepo("r", ccURL("r_repo"))
	id := f.byName(f.state(), "r").ID
	if _, err := f.eng.Bind(context.Background(), id, "initech-admin"); err != nil {
		t.Fatal(err)
	}
	if got := f.byName(f.state(), "r").Binding; got.Source != api.BindingLocal {
		t.Fatalf("bound: %+v", got)
	}
	o, err := f.eng.Unbind(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if o.Repo == nil || o.Repo.Binding.Source != api.BindingGlobalDefault || o.Repo.Binding.Profile != "acme" || !o.Repo.ToFix {
		t.Errorf("after unbind: %+v", o.Repo)
	}
	if len(f.backup.snaps) != 2 || f.backup.snaps[1].Reason != api.ReasonUnbind {
		t.Errorf("snapshots: %+v", f.backup.snaps)
	}
}

func TestTestConnectionRecordsResult(t *testing.T) {
	f := newFixture(t)
	f.newRepo("r", ccURL("r_repo"))
	// Send every HTTPS request to a closed local port so nothing leaves this machine.
	f.gitHook = func(g *gitx.Git) *gitx.Git {
		return g.WithEnv("https_proxy=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1", "no_proxy=", "NO_PROXY=", "all_proxy=", "ALL_PROXY=")
	}
	id := f.byName(f.state(), "r").ID
	if _, err := f.eng.Bind(context.Background(), id, "acme"); err != nil {
		t.Fatal(err)
	}
	res, err := f.eng.Test(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if res.RepoID != id || res.Status != api.TestNetwork || res.At != "2026-11-04T10:00:00Z" || res.Profile != "acme" {
		t.Errorf("result: %+v", res)
	}
	if rec, ok := f.reg.LastTests()[id]; !ok || rec.Status != api.TestNetwork {
		t.Errorf("not recorded: %+v", f.reg.LastTests())
	}
	if r := f.byName(f.state(), "r"); r.LastTest == nil || r.LastTest.Status != api.TestNetwork {
		t.Errorf("LastTest: %+v", r.LastTest)
	}
	many, err := f.eng.TestMany(context.Background(), []string{id, "nope"})
	if err != nil || len(many) != 2 || many[0].Status != api.TestNetwork || many[1].Status != api.TestError || many[1].RepoID != "nope" {
		t.Errorf("TestMany: %+v %v", many, err)
	}
}

func TestConvertToHTTPS(t *testing.T) {
	f := newFixture(t)
	repo := f.newRepo("acme_design", "ssh://"+hostAP+"/v1/repos/acme_design")
	appendFile(t, filepath.Join(repo, ".git", "config"), "[remote \"origin\"]\n\tpushurl = ssh://"+hostAP+"/v1/repos/acme_design\n")
	id := f.byName(f.state(), "acme_design").ID
	o, err := f.eng.ConvertToHTTPS(context.Background(), id, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if o.Repo == nil || o.Repo.RemoteKind != api.RemoteHTTPS || o.Repo.Binding.Source != api.BindingLocal || o.Repo.Binding.Profile != "acme" || o.BackupID != "bk1" {
		t.Errorf("outcome: %+v", o)
	}
	if len(f.backup.snaps) != 1 || f.backup.snaps[0].Reason != api.ReasonHTTPS {
		t.Errorf("exactly one https snapshot expected: %+v", f.backup.snaps)
	}
	cfg, _ := os.ReadFile(filepath.Join(repo, ".git", "config"))
	if strings.Contains(string(cfg), "ssh://") || !strings.Contains(string(cfg), "pushurl = https://") {
		t.Errorf("config:\n%s", cfg)
	}
	// without a profile: convert only
	f2 := newFixture(t)
	f2.newRepo("x", "ssh://"+hostAP+"/v1/repos/x")
	o, err = f2.eng.ConvertToHTTPS(context.Background(), f2.byName(f2.state(), "x").ID, "")
	if err != nil || o.Repo.RemoteKind != api.RemoteHTTPS || o.Repo.Binding.Source != api.BindingNone {
		t.Errorf("convert only: %+v %v", o, err)
	}
	// GRC cannot be converted this way
	f3 := newFixture(t)
	f3.newRepo("g", "codecommit://p@g_repo")
	if _, err := f3.eng.ConvertToHTTPS(context.Background(), f3.byName(f3.state(), "g").ID, ""); err == nil || api.AsAppError(err).Code != api.ErrRemoteGRC {
		t.Errorf("grc: %v", err)
	}
}

func TestNeedsRepairWhenAWSPathMissing(t *testing.T) {
	f := newFixture(t)
	f.newRepo("r", ccURL("r_repo"))
	id := f.byName(f.state(), "r").ID
	if _, err := f.eng.Bind(context.Background(), id, "acme"); err != nil {
		t.Fatal(err)
	}
	if st := f.state(); st.NeedsRepair != 0 || st.Counts.ToFix != 0 {
		t.Fatalf("healthy: %+v", st.Counts)
	}
	if err := os.Remove(f.aws); err != nil {
		t.Fatal(err)
	}
	st, err := f.eng.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r := f.byName(st, "r")
	if !r.Binding.NeedsRepair || !r.ToFix || st.NeedsRepair != 1 || st.Counts.ToFix != 1 {
		t.Errorf("after removing aws: %+v %+v", r.Binding, st.Counts)
	}
}

func TestRepairAll(t *testing.T) {
	f := newFixture(t)
	repo := f.newRepo("legacy", ccURL("legacy_repo"))
	appendFile(t, filepath.Join(repo, ".git", "config"), "[credential]\n\thelper = \n\thelper = !aws codecommit credential-helper $@ --profile acme\n\tUseHttpPath = true\n")
	broken := f.newRepo("broken", ccURL("broken_repo"))
	f.newRepo("other", ccURL("other_repo"))
	unknown := f.newRepo("unknownprofile", ccURL("up_repo"))
	appendFile(t, filepath.Join(unknown, ".git", "config"), "[credential]\n\thelper = \n\thelper = !aws codecommit credential-helper $@ --profile gone\n")
	st := f.state()
	if l := f.byName(st, "legacy"); !l.Binding.LegacyPlain || !l.ToFix {
		t.Fatalf("legacy: %+v", l.Binding)
	}
	// bind "broken" with a path that is then removed
	oldAWS := f.aws
	other := f.sb.FakeAWS(t, filepath.Join(f.sb.Root, "oldbin"), "ok")
	f.aws = other
	if _, err := f.eng.Bind(context.Background(), f.byName(st, "broken").ID, "globex"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(other); err != nil {
		t.Fatal(err)
	}
	f.aws = oldAWS
	st, _ = f.eng.Scan(context.Background())
	if b := f.byName(st, "broken"); !b.Binding.NeedsRepair {
		t.Fatalf("broken: %+v", b.Binding)
	}
	_ = broken
	outs, err := f.eng.RepairAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 2 {
		t.Fatalf("repaired %d: %+v", len(outs), outs)
	}
	for _, o := range outs {
		if !o.OK || o.Repo.Binding.NeedsRepair || o.Repo.Binding.LegacyPlain || o.Repo.Binding.Source != api.BindingLocal {
			t.Errorf("outcome: %+v", o)
		}
	}
	for _, s := range f.backup.snaps {
		if s.Reason == api.ReasonRepair && s.Kind != api.BackupGitLocal {
			t.Errorf("snapshot %+v", s)
		}
	}
	st = f.state()
	if st.NeedsRepair != 0 {
		t.Errorf("still needs repair: %d", st.NeedsRepair)
	}
	if u := f.byName(st, "unknownprofile"); !u.ToFix || u.Binding.ProfileKnown {
		t.Errorf("unknown profile must stay untouched: %+v", u.Binding)
	}
	if again, _ := f.eng.RepairAll(context.Background()); len(again) != 0 {
		t.Errorf("second run repaired %d", len(again))
	}
}

func TestCloneRequiresUsableProfile(t *testing.T) {
	f := newFixture(t)
	bare := f.sb.InitBare(t, "remote_repo.git")
	*f.remote = func(region, name string) string { return bare }
	outside := filepath.Join(f.sb.Root, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	(*f.profs)[1].Usable, (*f.profs)[1].NotSignedInPortal = false, "initech"
	req := api.CloneRequest{Profile: "initech-admin", Region: "us-east-1", RepoName: "remote_repo", ParentDir: outside, FolderName: "remote_repo"}
	_, err := f.eng.Clone(context.Background(), req)
	if err == nil || api.AsAppError(err).Code != api.ErrNotSignedIn || api.AsAppError(err).Params["portal"] != "initech" {
		t.Fatalf("signed out: %v", err)
	}
	if _, serr := os.Lstat(filepath.Join(outside, "remote_repo")); serr == nil {
		t.Error("git ran for a signed-out profile")
	}
}

func TestCloneAddsExtraRepo(t *testing.T) {
	f := newFixture(t)
	bare := f.sb.InitBare(t, "remote_repo.git")
	*f.remote = func(region, name string) string { return bare }
	outside := filepath.Join(f.sb.Root, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	req := api.CloneRequest{Profile: "initech-admin", Region: "us-east-1", RepoName: "remote_repo", ParentDir: outside, FolderName: "remote_repo"}
	pv, err := f.eng.PreviewClone(context.Background(), req)
	if err != nil || pv.Exists || pv.Dest != filepath.Join(outside, "remote_repo") || pv.Command != "git clone "+ccURL("remote_repo") || pv.HTTPSURL != ccURL("remote_repo") {
		t.Fatalf("preview: %+v %v", pv, err)
	}
	f.state()
	o, err := f.eng.Clone(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !o.OK || o.Repo == nil || o.Repo.Binding.Source != api.BindingLocal || o.Repo.Binding.Profile != "initech-admin" || o.Repo.Binding.NeedsRepair || o.Repo.Name != "remote_repo" {
		t.Errorf("outcome: %+v", o.Repo)
	}
	if len(f.store.extra) != 1 || f.store.extra[0] != pv.Dest {
		t.Errorf("extra repos: %v", f.store.extra)
	}
	if f.events.count(api.EventCloneProgress) == 0 {
		t.Error("no clone:progress events")
	}
	if st := f.state(); st.Counts.All != 1 || st.Repos[0].Binding.Source != api.BindingLocal {
		t.Errorf("state: %+v", st.Counts)
	}
	// Dest exists now
	if _, err := f.eng.Clone(context.Background(), req); err == nil || api.AsAppError(err).Code != api.ErrDestExists {
		t.Errorf("second clone: %v", err)
	}
	if pv, _ := f.eng.PreviewClone(context.Background(), req); !pv.Exists {
		t.Error("preview must report the existing folder")
	}
	// A clone inside a scan root is not added to extraRepos
	req2 := api.CloneRequest{Profile: "initech-admin", Region: "us-east-1", RepoName: "remote_repo", ParentDir: f.code, FolderName: "inroot"}
	if _, err := f.eng.Clone(context.Background(), req2); err != nil {
		t.Fatal(err)
	}
	if len(f.store.extra) != 1 {
		t.Errorf("in-root clone was added to extras: %v", f.store.extra)
	}
	// validation
	for name, bad := range map[string]api.CloneRequest{
		"region": {Profile: "initech-admin", Region: "nowhere", RepoName: "x", ParentDir: outside, FolderName: "x"},
		"name":   {Profile: "initech-admin", Region: "us-east-1", RepoName: "a/b", ParentDir: outside, FolderName: "x"},
		"folder": {Profile: "initech-admin", Region: "us-east-1", RepoName: "x", ParentDir: outside, FolderName: ".."},
		"parent": {Profile: "initech-admin", Region: "us-east-1", RepoName: "x", ParentDir: filepath.Join(outside, "missing"), FolderName: "x"},
	} {
		if _, err := f.eng.Clone(context.Background(), bad); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := f.eng.Clone(context.Background(), api.CloneRequest{Profile: "nope", Region: "us-east-1", RepoName: "x", ParentDir: outside, FolderName: "x"}); err == nil || api.AsAppError(err).Code != api.ErrProfileNotFound {
		t.Errorf("unknown profile: %v", err)
	}
}

func TestCompareCommitsHeadMatch(t *testing.T) {
	f := newFixture(t)
	bareA := f.sb.InitBare(t, "acct_a.git")
	bareB := f.sb.InitBare(t, "acct_b.git") // different commit (timestamps/messages differ by content below)
	// make B diverge from A
	work := f.sb.InitRepo(t, "bwork", nil)
	f.sb.CommitFile(t, work, "other.txt", "different\n")
	if res, err := f.sb.Git.Run(context.Background(), work, nil, "push", "-q", "--force", bareB, "HEAD:refs/heads/main"); err != nil || res.ExitCode != 0 {
		t.Fatalf("push: %v %s", err, res.Stderr)
	}
	repo := f.newRepo("acme_cs_portal", ccURL("acme_cs_portal"))
	if res, err := f.sb.Git.Run(context.Background(), repo, nil, "fetch", "-q", bareA, "main:refs/remotes/origin/main"); err != nil || res.ExitCode != 0 {
		t.Fatalf("fetch: %v %s", err, res.Stderr)
	}
	f.ccAPIs["ACME"].repos["acme_cs_portal"] = "rid-acme"       // account 111122223333 sorts first -> bare A
	f.ccAPIs["INITECH"].repos["acme_cs_portal"] = "rid-initech" // 777788889999 -> bare B
	calls := 0
	*f.remote = func(region, name string) string {
		calls++
		return []string{bareA, bareB}[calls-1]
	}
	id := f.byName(f.state(), "acme_cs_portal").ID
	// fewer than two candidates: unchanged
	r0, err := f.eng.CompareCommits(context.Background(), id)
	if err != nil || r0.Owner.Status != api.OwnerUnknown || calls != 0 {
		t.Fatalf("no candidates: %+v %v calls=%d", r0.Owner, err, calls)
	}
	if _, err := f.eng.DetectOwners(context.Background(), []string{id}); err != nil {
		t.Fatal(err)
	}
	r, err := f.eng.CompareCommits(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if r.Owner.Status != api.OwnerAmbiguous || len(r.Owner.Candidates) != 2 {
		t.Fatalf("owner: %+v", r.Owner)
	}
	a, b := r.Owner.Candidates[0], r.Owner.Candidates[1]
	if a.AccountID != acctAcme || !a.HeadMatch || a.MatchScore != 1 || b.HeadMatch || b.MatchScore != 0 {
		t.Errorf("scores: %+v / %+v", a, b)
	}
	// tie: both remotes identical -> no head match
	calls = 0
	*f.remote = func(region, name string) string { return bareA }
	r, err = f.eng.CompareCommits(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if r.Owner.Candidates[0].HeadMatch || r.Owner.Candidates[1].HeadMatch || r.Owner.Candidates[0].MatchScore != 1 || r.Owner.Candidates[1].MatchScore != 1 {
		t.Errorf("tie: %+v", r.Owner.Candidates)
	}
}

func TestListRemote(t *testing.T) {
	f := newFixture(t)
	f.ccAPIs["INITECH"].repos["zeta"] = "r1"
	f.ccAPIs["INITECH"].repos["alpha"] = "r2"
	got, err := f.eng.ListRemote(context.Background(), "initech-admin", "us-east-1")
	if err != nil || len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "zeta" || got[0].CloneURL != ccURL("alpha") || got[0].RepositoryID != "r2" {
		t.Errorf("%+v %v", got, err)
	}
	(*f.profs)[1].Usable, (*f.profs)[1].NotSignedInPortal = false, "initech"
	if _, err := f.eng.ListRemote(context.Background(), "initech-admin", "us-east-1"); err == nil || api.AsAppError(err).Code != api.ErrNotSignedIn || api.AsAppError(err).Params["portal"] != "initech" {
		t.Errorf("signed out: %v", err)
	}
	if _, err := f.eng.ListRemote(context.Background(), "nope", "us-east-1"); err == nil || api.AsAppError(err).Code != api.ErrProfileNotFound {
		t.Errorf("unknown: %v", err)
	}
	if _, err := f.eng.ListRemote(context.Background(), "initech-admin", "mars"); err == nil || api.AsAppError(err).Code != api.ErrInvalidRegion {
		t.Errorf("region: %v", err)
	}
}

func TestInvalidateReinspects(t *testing.T) {
	f := newFixture(t)
	f.realLayout()
	f.newRepo("gdefault", ccURL("gdefault_repo"))
	st := f.state()
	if r := f.byName(st, "gdefault"); r.Binding.Source != api.BindingGlobalDefault {
		t.Fatalf("%+v", r.Binding)
	}
	f.sb.WriteGlobal(t, "")
	f.eng.Invalidate(context.Background())
	st = f.state()
	if r := f.byName(st, "gdefault"); r.Binding.Source != api.BindingNone || st.GlobalDefault != nil {
		t.Errorf("after invalidate: %+v %+v", r.Binding, st.GlobalDefault)
	}
}

func TestRescanKeepsOwner(t *testing.T) {
	f := newFixture(t)
	f.newRepo("r", ccURL("r_repo"))
	f.ccAPIs["INITECH"].repos["r_repo"] = "rid"
	id := f.byName(f.state(), "r").ID
	if _, err := f.eng.DetectOwners(context.Background(), []string{id}); err != nil {
		t.Fatal(err)
	}
	st, err := f.eng.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if o := f.byName(st, "r").Owner; o.Status != api.OwnerSingle {
		t.Errorf("owner lost on rescan: %+v", o)
	}
}
