package gitglobal_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/codecommit"
	"github.com/rudian/aws_sso_hub/internal/gitglobal"
	"github.com/rudian/aws_sso_hub/internal/gitx"
	"github.com/rudian/aws_sso_hub/internal/gitx/gitxtest"
	"github.com/rudian/aws_sso_hub/internal/paths"
	"github.com/rudian/aws_sso_hub/internal/repos"
)

const (
	hostAP      = "git-codecommit.us-east-1.amazonaws.com"
	acctInitech = "777788889999"
	acctAcme    = "111122223333"
)

func ccURL(name string) string { return "https://" + hostAP + "/v1/repos/" + name }

type store struct {
	roots []string
}

func (s *store) ScanRoots() []string                  { return s.roots }
func (s *store) ExtraRepos() []string                 { return nil }
func (s *store) AddExtraRepo(string) error            { return nil }
func (s *store) RemoveExtraRepo(string) error         { return nil }
func (s *store) AutoBindOnAdd() bool                  { return false }
func (s *store) RepoTests() map[string]api.TestResult { return map[string]api.TestResult{} }
func (s *store) SetRepoTest(api.TestResult) error     { return nil }
func (s *store) Lang() string                         { return "en" }

type snapshot struct {
	Path    string
	Reason  api.BackupReason
	Kind    api.BackupKind
	Params  map[string]string
	Content string
}

type backupLog struct {
	mu    sync.Mutex
	snaps []snapshot
}

func (b *backupLog) Snapshot(path, display string, kind api.BackupKind, reason api.BackupReason, params map[string]string) (api.BackupEntry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	content, _ := os.ReadFile(path)
	b.snaps = append(b.snaps, snapshot{path, reason, kind, params, string(content)})
	return api.BackupEntry{ID: fmt.Sprintf("bk%d", len(b.snaps))}, nil
}

type creds struct {
	supported bool
	listed    []string
	cleared   []string
	items     []api.SavedCredential
}

func (c *creds) Supported() bool { return c.supported }
func (c *creds) List(ctx context.Context, hosts []string) ([]api.SavedCredential, error) {
	c.listed = hosts
	return c.items, nil
}
func (c *creds) Clear(ctx context.Context, ids []string) (int, error) {
	c.cleared = ids
	return len(ids), nil
}

type fx struct {
	t       *testing.T
	sb      *gitxtest.Sandbox
	repos   *repos.Engine
	eng     *gitglobal.Engine
	backup  *backupLog
	creds   *creds
	aws     string
	code    string
	written []string
	events  []string
	profs   []repos.ProfileInfo
}

func prov(k string) aws.CredentialsProvider {
	return credentials.NewStaticCredentialsProvider(k, "fakeSecret", "")
}

func newFx(t *testing.T) *fx {
	t.Helper()
	sb := gitxtest.New(t)
	f := &fx{t: t, sb: sb, backup: &backupLog{}, creds: &creds{supported: true}}
	f.code = filepath.Join(sb.Root, "code")
	if err := os.MkdirAll(f.code, 0o755); err != nil {
		t.Fatal(err)
	}
	f.aws = sb.FakeAWS(t, filepath.Join(sb.Root, "fakebin"), "ok")
	f.profs = []repos.ProfileInfo{
		{Name: "acme", AccountID: acctAcme, Kind: api.ProfileKindStatic, ColorSlot: 1, Monogram: "AC", Usable: true, Provider: prov("ACME")},
		{Name: "initech-admin", AccountID: acctInitech, Portal: "initech", Kind: api.ProfileKindSSO, ColorSlot: 3, Monogram: "IA", Usable: true, Provider: prov("INITECH")},
	}
	pathsV := paths.Paths{Home: sb.Home, BackupDir: filepath.Join(sb.Root, "backups"), GitGlobalFile: sb.GlobalConfig}
	profiles := func(ctx context.Context) ([]repos.ProfileInfo, error) { return f.profs, nil }
	gitFn := func(ctx context.Context) (*gitx.Git, error) { return sb.Git, nil }
	f.repos = repos.New(repos.Deps{
		Paths: pathsV, Git: gitFn, AWSPath: func() (string, error) { return f.aws, nil }, AWSReady: func() bool { return true },
		Profiles: profiles, Store: &store{roots: []string{f.code}}, Backup: f.backup,
		CodeCommit: func(p aws.CredentialsProvider, region string) codecommit.API { return nil },
		Emit:       func(name string, data any) {}, Now: time.Now, GOOS: "darwin",
	})
	f.eng = gitglobal.New(gitglobal.Deps{
		Paths: pathsV, Git: gitFn, AWSPath: func() (string, error) { return f.aws, nil }, Repos: f.repos, Profiles: profiles,
		Backup: f.backup, Creds: f.creds, Lang: func() string { return "en" }, GOOS: "darwin",
		Emit:    func(name string, data any) { f.events = append(f.events, name) },
		OnWrite: func(path string, content []byte) { f.written = append(f.written, path) },
	})
	return f
}

func (f *fx) repo(dir, remote string) string {
	return f.sb.InitRepo(f.t, filepath.Join("code", dir), map[string]string{"origin": remote})
}

func (f *fx) scan() api.ReposState {
	f.t.Helper()
	st, err := f.repos.Scan(context.Background())
	if err != nil {
		f.t.Fatal(err)
	}
	return st
}

func (f *fx) id(st api.ReposState, dir string) string {
	f.t.Helper()
	for _, r := range st.Repos {
		if strings.HasSuffix(r.Path, "/"+dir) {
			return r.ID
		}
	}
	f.t.Fatalf("no repo %s", dir)
	return ""
}

func (f *fx) global() string {
	b, err := os.ReadFile(f.sb.GlobalConfig)
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

func sha(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

const (
	ghHelper    = "!gh auth git-credential"
	defaultOnly = "[credential]\n\thelper = \n\thelper = !aws codecommit credential-helper $@ --profile acme\n\tusehttppath = true\n" +
		"[credential \"https://github.com\"]\n\thelper = \n\thelper = " + ghHelper + "\n"
	urlRule = "[credential \"https://" + hostAP + "/v1/repos/infra-terraform\"]\n\thelper = \n\thelper = !aws --profile initech-admin codecommit credential-helper $@\n"
)

func TestStateParsesRealLayout(t *testing.T) {
	f := newFx(t)
	f.sb.WriteGlobal(t, defaultOnly+urlRule)
	f.repo("a", ccURL("a_repo"))
	f.repo("infra-terraform", ccURL("infra-terraform"))
	st := f.scan()
	gs, err := f.eng.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gs.GlobalPath != f.sb.GlobalConfig || gs.BaseHash != sha(defaultOnly+urlRule) || gs.GlobalFile != "~/.gitconfig" {
		t.Errorf("file info: %+v", gs)
	}
	if gd := gs.GlobalDefault; gd == nil || gd.Profile != "acme" || gd.ProfileKind != api.ProfileKindStatic || gd.ColorSlot != 1 || gd.Monogram != "AC" || gd.AffectedRepos != 1 {
		t.Errorf("global default: %+v", gs.GlobalDefault)
	}
	if len(gs.URLRules) != 1 {
		t.Fatalf("rules: %+v", gs.URLRules)
	}
	r := gs.URLRules[0]
	if r.Pattern != ccURL("infra-terraform") || r.Profile != "initech-admin" || !r.ProfileKnown || r.ColorSlot != 3 || r.Monogram != "IA" || r.ExtraKeys ||
		len(r.MatchedRepoIDs) != 1 || r.MatchedRepoIDs[0] != f.id(st, "infra-terraform") {
		t.Errorf("rule: %+v", r)
	}
	if gs.Guard.Installed || len(gs.Guard.Patterns) != 0 || !gs.SavedCredentialsSupported {
		t.Errorf("guard/creds: %+v", gs)
	}
	if len(gs.OtherHelpers) != 1 || gs.OtherHelpers[0].Value != ghHelper || gs.OtherHelpers[0].Scope != "global" || gs.OtherHelpers[0].Origin != "~/.gitconfig" {
		t.Errorf("other helpers: %+v", gs.OtherHelpers)
	}
	if len(gs.CredentialLines) != 7 {
		t.Errorf("credential lines: %d", len(gs.CredentialLines))
	}
	if gs.BoundRepos != 0 || gs.UnboundRepos != 2 {
		t.Errorf("counts: %d/%d", gs.BoundRepos, gs.UnboundRepos)
	}
	// a missing file is a valid, empty state
	if err := os.Remove(f.sb.GlobalConfig); err != nil {
		t.Fatal(err)
	}
	gs, err = f.eng.State(context.Background())
	if err != nil || gs.BaseHash != "" || gs.GlobalDefault != nil || len(gs.URLRules) != 0 {
		t.Errorf("missing file: %+v %v", gs, err)
	}
}

func TestRemoveDefaultRequiresURLRulesMigrated(t *testing.T) {
	f := newFx(t)
	f.sb.WriteGlobal(t, defaultOnly+urlRule)
	f.repo("a", ccURL("a_repo"))
	f.scan()
	before := f.global()
	req := api.DefaultChangeRequest{Action: api.DefaultRemove}
	_, err := f.eng.PreviewDefault(context.Background(), req)
	if ae := api.AsAppError(err); err == nil || ae.Code != api.ErrURLRulesFirst || ae.Params["n"] != "1" {
		t.Errorf("preview: %v", err)
	}
	_, err = f.eng.ApplyDefault(context.Background(), req, sha(before))
	if ae := api.AsAppError(err); err == nil || ae.Code != api.ErrURLRulesFirst {
		t.Errorf("apply: %v", err)
	}
	if f.global() != before || len(f.backup.snaps) != 0 {
		t.Error("nothing may change")
	}
	// keep is always fine
	pv, err := f.eng.PreviewDefault(context.Background(), api.DefaultChangeRequest{Action: api.DefaultKeep})
	if err != nil || len(pv.Preview.Lines) != 0 || pv.Preview.BaseHash != sha(before) {
		t.Errorf("keep: %+v %v", pv, err)
	}
}

func TestPreviewRemoveSimulates(t *testing.T) {
	f := newFx(t)
	f.sb.WriteGlobal(t, defaultOnly)
	bound := f.repo("bound", ccURL("bound_repo"))
	f.repo("unbound", ccURL("unbound_repo"))
	st := f.scan()
	if _, err := f.repos.Bind(context.Background(), f.id(st, "bound"), "initech-admin"); err != nil {
		t.Fatal(err)
	}
	st = f.scan()
	before := f.global()
	snapsBefore := len(f.backup.snaps)
	pv, err := f.eng.PreviewDefault(context.Background(), api.DefaultChangeRequest{Action: api.DefaultRemove})
	if err != nil {
		t.Fatal(err)
	}
	if pv.BoundUnaffected != 1 || pv.UnboundBlocked != 1 || len(pv.BlockedRepoIDs) != 1 || pv.BlockedRepoIDs[0] != f.id(st, "unbound") {
		t.Errorf("impact: %+v", pv)
	}
	if pv.Preview.Added == 0 || pv.Preview.Removed != 1 || pv.Preview.BaseHash != sha(before) || pv.Preview.File != "~/.gitconfig" || len(pv.Preview.Lines) == 0 {
		t.Errorf("preview: %+v", pv.Preview)
	}
	joined := ""
	for _, l := range pv.Preview.Lines {
		joined += string(l.Kind) + ":" + l.Text + "\n"
	}
	if !strings.Contains(joined, "del:\thelper = !aws codecommit credential-helper $@ --profile acme") || !strings.Contains(joined, "add:[credential \"https://git-codecommit.*.amazonaws.com\"]") {
		t.Errorf("diff:\n%s", joined)
	}
	if f.global() != before {
		t.Error("the real global file must be byte-identical after a preview")
	}
	if len(f.backup.snaps) != snapsBefore || len(f.written) != 0 {
		t.Error("a preview must not back up or write")
	}
	_ = bound
	// the local binding is untouched in the real repo
	cfg, _ := os.ReadFile(filepath.Join(bound, ".git", "config"))
	if !strings.Contains(string(cfg), "[awsssohub]") {
		t.Error("local binding lost")
	}
}

func TestApplyRemoveInstallsGuardAfterBackup(t *testing.T) {
	f := newFx(t)
	f.sb.WriteGlobal(t, defaultOnly)
	f.repo("unbound", ccURL("unbound_repo"))
	f.repo("fipsrepo", "https://git-codecommit-fips.us-east-2.amazonaws.com/v1/repos/fips_repo")
	st := f.scan()
	original := f.global()
	res, err := f.eng.ApplyDefault(context.Background(), api.DefaultChangeRequest{Action: api.DefaultRemove}, sha(original))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.backup.snaps) != 1 {
		t.Fatalf("snapshots: %+v", f.backup.snaps)
	}
	s := f.backup.snaps[0]
	if s.Content != original || s.Reason != api.ReasonGlobalDefault || s.Kind != api.BackupGitGlobal || s.Path != f.sb.GlobalConfig || res.BackupID != "bk1" {
		t.Errorf("the backup must hold the file as it was before the write: %+v", s)
	}
	golden, err := os.ReadFile("testdata/apply_remove.golden")
	if err != nil {
		t.Fatal(err)
	}
	if f.global() != string(golden) {
		t.Errorf("file after apply differs from golden\n--- got\n%s\n--- want\n%s", f.global(), golden)
	}
	if res.State.GlobalDefault != nil || !res.State.Guard.Installed || len(res.State.Guard.Patterns) != 2 {
		t.Errorf("state: %+v", res.State.Guard)
	}
	if len(f.written) != 1 || f.written[0] != f.sb.GlobalConfig {
		t.Errorf("OnWrite: %v", f.written)
	}
	if len(f.events) == 0 || f.events[len(f.events)-1] != api.EventReposChanged {
		t.Errorf("events: %v", f.events)
	}
	// the repo engine saw the change
	for _, r := range f.repos.Snapshot() {
		if r.ID == f.id(st, "unbound") && r.Binding.Source != api.BindingGuard {
			t.Errorf("unbound repo should now be guarded: %+v", r.Binding)
		}
	}
	// stale hash right after
	_, err = f.eng.ApplyDefault(context.Background(), api.DefaultChangeRequest{Action: api.DefaultRemove}, sha(original))
	if ae := api.AsAppError(err); err == nil || ae.Code != api.ErrConfigChanged {
		t.Errorf("second apply with the old hash: %v", err)
	}
}

func TestApplyReplace(t *testing.T) {
	f := newFx(t)
	f.sb.WriteGlobal(t, defaultOnly)
	f.repo("a", ccURL("a_repo"))
	f.scan()
	orig := f.global()
	res, err := f.eng.ApplyDefault(context.Background(), api.DefaultChangeRequest{Action: api.DefaultReplace, Profile: "initech-admin"}, sha(orig))
	if err != nil {
		t.Fatal(err)
	}
	g := f.global()
	if strings.Contains(g, "--profile acme") || !strings.Contains(g, `--profile 'initech-admin' codecommit credential-helper $@`) || strings.Count(g, "usehttppath") != 1 ||
		strings.Count(g, "[credential]") != 1 {
		t.Errorf("replace:\n%s", g)
	}
	if res.State.GlobalDefault == nil || res.State.GlobalDefault.Profile != "initech-admin" || res.BackupID == "" {
		t.Errorf("state: %+v", res.State.GlobalDefault)
	}
	// without any remaining plain helper a reset is added in front
	f2 := newFx(t)
	f2.sb.WriteGlobal(t, "[credential]\n\thelper = !aws codecommit credential-helper $@ --profile acme\n")
	f2.scan()
	if _, err := f2.eng.ApplyDefault(context.Background(), api.DefaultChangeRequest{Action: api.DefaultReplace, Profile: "initech-admin"}, sha(f2.global())); err != nil {
		t.Fatal(err)
	}
	if g := f2.global(); !strings.Contains(g, "helper = \n\thelper = !\\\"") {
		t.Errorf("reset missing:\n%s", g)
	}
	// previews validate the profile
	_, err = f.eng.PreviewDefault(context.Background(), api.DefaultChangeRequest{Action: api.DefaultReplace, Profile: "nope"})
	if ae := api.AsAppError(err); err == nil || ae.Code != api.ErrProfileNotFound {
		t.Errorf("unknown profile: %v", err)
	}
	pv, err := f.eng.PreviewDefault(context.Background(), api.DefaultChangeRequest{Action: api.DefaultReplace, Profile: "acme"})
	if err != nil || pv.Preview.Added != 1 || pv.Preview.Removed != 1 || pv.UnboundBlocked != 0 {
		t.Errorf("replace preview: %+v %v", pv, err)
	}
}

func TestApplyRejectsStaleBaseHash(t *testing.T) {
	f := newFx(t)
	f.sb.WriteGlobal(t, defaultOnly)
	f.repo("a", ccURL("a_repo"))
	f.scan()
	pv, err := f.eng.PreviewDefault(context.Background(), api.DefaultChangeRequest{Action: api.DefaultRemove})
	if err != nil {
		t.Fatal(err)
	}
	// somebody edits the file between preview and apply
	f.sb.WriteGlobal(t, defaultOnly+"[user]\n\tname = Someone\n")
	changed := f.global()
	_, err = f.eng.ApplyDefault(context.Background(), api.DefaultChangeRequest{Action: api.DefaultRemove}, pv.Preview.BaseHash)
	ae := api.AsAppError(err)
	if err == nil || ae.Code != api.ErrConfigChanged || ae.Params["file"] != "~/.gitconfig" {
		t.Errorf("err: %v", err)
	}
	if f.global() != changed || len(f.backup.snaps) != 0 || len(f.written) != 0 {
		t.Error("a stale apply must not touch anything")
	}
}

func dupFixture(t *testing.T) (*fx, api.ReposState) {
	f := newFx(t)
	f.sb.WriteGlobal(t, defaultOnly+urlRule)
	f.repo("infra-terraform-one", ccURL("infra-terraform"))
	f.repo("infra-terraform-two", ccURL("infra-terraform"))
	return f, f.scan()
}

func TestURLRuleMigrationDuplicateNeedsAssignment(t *testing.T) {
	f, st := dupFixture(t)
	pattern := ccURL("infra-terraform")
	plan, err := f.eng.PreviewURLRule(context.Background(), pattern)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.DuplicateURLs || len(plan.Repos) != 2 || plan.Profile != "initech-admin" || plan.Preview.Removed == 0 || plan.Preview.BaseHash == "" {
		t.Errorf("plan: %+v", plan)
	}
	one, two := f.id(st, "infra-terraform-one"), f.id(st, "infra-terraform-two")
	before := f.global()
	hash := sha(before)
	for name, m := range map[string]api.URLRuleMigration{
		"only one":  {Pattern: pattern, Assignments: []api.BindItem{{RepoID: one, Profile: "initech-admin"}}},
		"none":      {Pattern: pattern},
		"duplicate": {Pattern: pattern, Assignments: []api.BindItem{{RepoID: one, Profile: "initech-admin"}, {RepoID: one, Profile: "acme"}}},
		"stranger":  {Pattern: pattern, Assignments: []api.BindItem{{RepoID: one, Profile: "initech-admin"}, {RepoID: "zzz", Profile: "initech-admin"}}},
	} {
		_, err := f.eng.MigrateURLRule(context.Background(), m, hash)
		if ae := api.AsAppError(err); err == nil || ae.Code != api.ErrInvalidInput || ae.Params["field"] != "assignments" {
			t.Errorf("%s: %v", name, err)
		}
	}
	if f.global() != before || len(f.backup.snaps) != 0 {
		t.Error("invalid assignments must not change anything")
	}
	res, err := f.eng.MigrateURLRule(context.Background(), api.URLRuleMigration{Pattern: pattern, Assignments: []api.BindItem{
		{RepoID: one, Profile: "initech-admin"}, {RepoID: two, Profile: "acme"},
	}}, hash)
	if err != nil {
		t.Fatal(err)
	}
	if !res.RuleRemoved || len(res.Outcomes) != 2 || !res.Outcomes[0].OK || !res.Outcomes[1].OK || res.BackupID == "" {
		t.Errorf("result: %+v", res)
	}
	byID := map[string]string{}
	for _, o := range res.Outcomes {
		byID[o.RepoID] = o.Repo.Binding.Profile
	}
	if byID[one] != "initech-admin" || byID[two] != "acme" {
		t.Errorf("each clone keeps its own assignment: %v", byID)
	}
	if strings.Contains(f.global(), "infra-terraform") {
		t.Errorf("rule not removed:\n%s", f.global())
	}
	// the global-file backup was taken with the original content
	var globalSnap *snapshot
	for i := range f.backup.snaps {
		if f.backup.snaps[i].Reason == api.ReasonURLRule {
			globalSnap = &f.backup.snaps[i]
		}
	}
	if globalSnap == nil || globalSnap.Content != before || globalSnap.Params["pattern"] != pattern {
		t.Errorf("url-rule snapshot: %+v", globalSnap)
	}
}

func TestMigrateURLRuleRemovesSection(t *testing.T) {
	f := newFx(t)
	f.sb.WriteGlobal(t, defaultOnly+urlRule+"[credential \""+ccURL("other_rule")+"\"]\n\thelper = \n\thelper = !aws --profile acme codecommit credential-helper $@\n\tinteractive = false\n")
	f.repo("infra-terraform", ccURL("infra-terraform"))
	f.repo("other", ccURL("other_rule"))
	st := f.scan()
	gs, _ := f.eng.State(context.Background())
	if len(gs.URLRules) != 2 || gs.URLRules[1].ExtraKeys != true || gs.URLRules[0].ExtraKeys {
		t.Fatalf("rules: %+v", gs.URLRules)
	}
	res, err := f.eng.MigrateURLRule(context.Background(), api2(ccURL("infra-terraform"), f.id(st, "infra-terraform"), "initech-admin"), gs.BaseHash)
	if err != nil || !res.RuleRemoved {
		t.Fatalf("%+v %v", res, err)
	}
	g := f.global()
	if strings.Contains(g, "infra-terraform") || !strings.Contains(g, ccURL("other_rule")) {
		t.Errorf("only the migrated rule must go:\n%s", g)
	}
	// extra keys: only the helper values go
	gs, _ = f.eng.State(context.Background())
	if _, err := f.eng.MigrateURLRule(context.Background(), api2(ccURL("other_rule"), f.id(st, "other"), "acme"), gs.BaseHash); err != nil {
		t.Fatal(err)
	}
	g = f.global()
	if !strings.Contains(g, "interactive = false") || strings.Contains(g, "--profile acme codecommit") {
		t.Errorf("keepOtherKeys:\n%s", g)
	}
	// a failing bind keeps the rule
	f2 := newFx(t)
	f2.sb.WriteGlobal(t, defaultOnly+urlRule)
	f2.repo("infra-terraform", ccURL("infra-terraform"))
	st2 := f2.scan()
	gs2, _ := f2.eng.State(context.Background())
	res, err = f2.eng.MigrateURLRule(context.Background(), api2(ccURL("infra-terraform"), f2.id(st2, "infra-terraform"), "no-such-profile"), gs2.BaseHash)
	if err != nil || res.RuleRemoved || len(res.Outcomes) != 1 || res.Outcomes[0].OK || res.Outcomes[0].Error == nil {
		t.Errorf("failed bind: %+v %v", res, err)
	}
	if !strings.Contains(f2.global(), "infra-terraform") {
		t.Error("rule must stay when a bind failed")
	}
	// unknown pattern
	if _, err := f2.eng.PreviewURLRule(context.Background(), "https://nope"); err == nil || api.AsAppError(err).Code != api.ErrInvalidInput {
		t.Errorf("unknown pattern: %v", err)
	}
}

func api2(pattern, id, profile string) api.URLRuleMigration {
	return api.URLRuleMigration{Pattern: pattern, Assignments: []api.BindItem{{RepoID: id, Profile: profile}}}
}

func TestGuardPatterns(t *testing.T) {
	f := newFx(t)
	f.repo("a", "https://git-codecommit.cn-north-1.amazonaws.com.cn/v1/repos/a_repo")
	f.repo("b", "https://vpce-0b7d-abc.git-codecommit.us-east-2.vpce.amazonaws.com/v1/repos/b_repo")
	f.scan()
	if _, err := f.eng.ApplyDefault(context.Background(), api.DefaultChangeRequest{Action: api.DefaultRemove}, ""); err != nil {
		t.Fatal(err)
	}
	gs, _ := f.eng.State(context.Background())
	want := "https://*.git-codecommit.*.vpce.amazonaws.com,https://git-codecommit.*.amazonaws.com,https://git-codecommit.*.amazonaws.com.cn"
	if got := strings.Join(sorted(gs.Guard.Patterns), ","); got != want {
		t.Errorf("guard patterns: %s", got)
	}
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func TestSavedCredentialsListAndClear(t *testing.T) {
	f := newFx(t)
	f.repo("a", ccURL("a_repo"))
	f.repo("b", "https://git-codecommit.us-west-2.amazonaws.com/v1/repos/b_repo")
	f.scan()
	f.creds.items = []api.SavedCredential{
		{ID: "keychain:" + hostAP, Host: hostAP, Store: api.StoreKeychain},
		{ID: "keychain:git-codecommit.us-west-2.amazonaws.com", Host: "git-codecommit.us-west-2.amazonaws.com", Store: api.StoreKeychain},
	}
	got, err := f.eng.ListSaved(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatalf("%v %v", got, err)
	}
	if strings.Join(f.creds.listed, ",") != hostAP+",git-codecommit.us-west-2.amazonaws.com" {
		t.Errorf("hosts: %v", f.creds.listed)
	}
	n, err := f.eng.ClearSaved(context.Background(), nil)
	if err != nil || n != 2 || len(f.creds.cleared) != 2 {
		t.Errorf("clear all: %d %v %v", n, err, f.creds.cleared)
	}
	n, _ = f.eng.ClearSaved(context.Background(), []string{"keychain:" + hostAP})
	if n != 1 || len(f.creds.cleared) != 1 {
		t.Errorf("clear one: %d %v", n, f.creds.cleared)
	}
}

func TestClearSavedIgnoresUnlistedIds(t *testing.T) {
	f := newFx(t)
	f.repo("a", ccURL("a_repo"))
	f.scan()
	f.creds.items = []api.SavedCredential{{ID: "keychain:" + hostAP, Host: hostAP, Store: api.StoreKeychain}}
	n, err := f.eng.ClearSaved(context.Background(), []string{"keychain:github.com", "keychain:" + hostAP, "keychain:accounts.example.com", "keychain:" + hostAP})
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(f.creds.cleared) != 1 || f.creds.cleared[0] != "keychain:"+hostAP {
		t.Errorf("cleared: %v", f.creds.cleared)
	}
	f.creds.cleared = nil
	n, err = f.eng.ClearSaved(context.Background(), []string{"keychain:github.com"})
	if err != nil || n != 0 || f.creds.cleared != nil {
		t.Errorf("only unlisted ids: n=%d err=%v cleared=%v", n, err, f.creds.cleared)
	}
}
