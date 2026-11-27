package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rudian/aws_sso_hub/internal/accounts"
	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/migrate"
	"github.com/rudian/aws_sso_hub/internal/repos"
)

// Compile-time proof that Store implements every port it is wired to.
var (
	_ accounts.Store = (*Store)(nil)
	_ migrate.Store  = (*Store)(nil)
	_ repos.Store    = (*Store)(nil)
)

func TestImplementsPorts(t *testing.T) {
	s, _, _ := open(t)
	var (
		_ accounts.Store = s
		_ migrate.Store  = s
		_ repos.Store    = s
	)
}

func fixedNow() time.Time { return time.Date(2026, 11, 4, 2, 18, 0, 0, time.UTC) }

func open(t *testing.T) (*Store, string, bool) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	s, first, err := Open(path, filepath.Join(dir, "code"), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	return s, path, first
}

func TestDefaultsFirstRun(t *testing.T) {
	s, path, first := open(t)
	if !first {
		t.Fatal("first run expected")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("nothing may be written before the first update")
	}
	a := s.API()
	if a.Language != api.LanguageSystem || a.Theme != api.ThemeSystem || a.Motion != api.MotionSystem {
		t.Fatalf("enums: %+v", a)
	}
	if len(a.ScanRoots) != 1 || !strings.HasSuffix(a.ScanRoots[0], "code") {
		t.Fatalf("scanRoots %v", a.ScanRoots)
	}
	if !a.Reminders.AutoSignIn || !a.Reminders.AutoRenew || a.Reminders.LeadMinutes != 10 {
		t.Fatalf("reminders %+v", a.Reminders)
	}
	if a.Tray.Enabled || a.BackupRetention != 20 || a.TerminalApp != api.TerminalAuto {
		t.Fatalf("defaults %+v", a)
	}
	if a.TutorialDone == nil {
		t.Fatal("slices must not be nil")
	}
	if s.UILanguage() != "zh-CN" {
		t.Fatal(s.UILanguage())
	}
	if err := s.CompleteFirstRun(); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode())
	}
	_, first, err = Open(path, "/x", fixedNow)
	if err != nil || first {
		t.Fatalf("first=%v err=%v", first, err)
	}
}

func TestCorruptFileRenamed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, first, err := Open(path, filepath.Join(dir, "code"), fixedNow)
	if err != nil || !first {
		t.Fatalf("first=%v err=%v", first, err)
	}
	moved := fmt.Sprintf("%s.corrupt-%d", path, fixedNow().Unix())
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("corrupt file not moved aside: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("original must be gone")
	}
	if s.API().Theme != api.ThemeSystem {
		t.Fatal("defaults expected")
	}
}

func TestInvalidValuesReplacedOnLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	body := `{"theme":"neon","language":"fr","scanRoots":[],"reminders":{"leadMinutes":45,"expiryNotice":false,"autoRenew":true},
	 "backupRetention":30,"unknownField":1,"terminalApp":"xterm",
	 "sessions":{"a":{"loginAt":"2026-11-04T01:00:00Z","sessionMinutes":5},"b":{"sessionMinutes":999999},"c":{"sessionMinutes":480}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _, err := Open(path, "/default/root", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	a := s.API()
	if a.Theme != api.ThemeSystem || a.Language != api.LanguageSystem || a.TerminalApp != api.TerminalAuto {
		t.Fatalf("%+v", a)
	}
	if a.Reminders.LeadMinutes != 10 || a.Reminders.AutoSignIn || a.BackupRetention != 20 {
		t.Fatalf("%+v", a)
	}
	if len(a.ScanRoots) != 1 || a.ScanRoots[0] != "/default/root" {
		t.Fatalf("roots %v", a.ScanRoots)
	}
	for k, want := range map[string]int{"a": 0, "b": 0, "c": 480} {
		r, _ := s.SessionRecord(k)
		if r.SessionMinutes != want {
			t.Fatalf("%s: %d", k, r.SessionMinutes)
		}
	}
}

func wantInvalid(t *testing.T, err error, field string) {
	t.Helper()
	ae := api.AsAppError(err)
	if ae == nil || ae.Code != api.ErrInvalidInput || ae.Params["field"] != field {
		t.Fatalf("want E_INVALID_INPUT(%s), got %v", field, err)
	}
}

func TestUpdateValidates(t *testing.T) {
	s, _, _ := open(t)
	before := s.API()
	_, err := s.Update(api.SettingsPatch{ScanRoots: []string{}})
	wantInvalid(t, err, "scanRoots")
	_, err = s.Update(api.SettingsPatch{ScanRoots: []string{"relative/path"}})
	wantInvalid(t, err, "scanRoots")
	_, err = s.Update(api.SettingsPatch{Reminders: &api.ReminderSettings{LeadMinutes: 45}})
	wantInvalid(t, err, "reminders.leadMinutes")
	r := 30
	_, err = s.Update(api.SettingsPatch{BackupRetention: &r})
	wantInvalid(t, err, "backupRetention")
	bad := api.Theme("neon")
	_, err = s.Update(api.SettingsPatch{Theme: &bad})
	wantInvalid(t, err, "theme")
	nl := "/usr/bin/aws\n--evil"
	_, err = s.Update(api.SettingsPatch{AWSCLIPath: &nl})
	wantInvalid(t, err, "awsCliPath")
	// a rejected patch changes nothing, even when other fields in it are valid
	dark := api.ThemeDark
	_, err = s.Update(api.SettingsPatch{Theme: &dark, BackupRetention: &r})
	wantInvalid(t, err, "backupRetention")
	if got := s.API(); got.Theme != before.Theme {
		t.Fatal("rejected patch must not apply")
	}
	// valid patch
	ret := 50
	root := t.TempDir()
	out, err := s.Update(api.SettingsPatch{Theme: &dark, BackupRetention: &ret, ScanRoots: []string{root, root}, TutorialDone: []string{"why", "why", "bind"}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Theme != api.ThemeDark || out.BackupRetention != 50 || len(out.ScanRoots) != 1 || len(out.TutorialDone) != 2 {
		t.Fatalf("%+v", out)
	}
	cleared, err := s.Update(api.SettingsPatch{TutorialDone: []string{}})
	if err != nil || len(cleared.TutorialDone) != 0 || cleared.TutorialDone == nil {
		t.Fatalf("%+v %v", cleared, err)
	}
}

func TestSessionsRecordRoundTrip(t *testing.T) {
	s, path, _ := open(t)
	rec := api.SessionRecord{LoginAt: "2026-11-04T01:30:00Z", ClientIDHash: "3f9a0c7be21d4e58", TokenIssuedAt: "2026-11-04T02:18:00Z", SessionMinutes: 480}
	if err := s.SetSessionRecord("initech", rec); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionRecord("bad", api.SessionRecord{SessionMinutes: 5}); err == nil {
		t.Fatal("minutes below 15 must be rejected")
	}
	s2, _, err := Open(path, "/x", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.SessionRecord("initech")
	if !ok || got != rec {
		t.Fatalf("got %+v", got)
	}
	if err := s2.DeleteSessionRecord("initech"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s2.SessionRecord("initech"); ok {
		t.Fatal("record must be deleted")
	}
	if err := s2.DeleteSessionRecord("never-existed"); err != nil {
		t.Fatal(err)
	}
}

func TestSetUILanguageValidates(t *testing.T) {
	s, _, _ := open(t)
	wantInvalid(t, s.SetUILanguage("fr"), "lang")
	if err := s.SetUILanguage("en"); err != nil || s.Lang() != "en" {
		t.Fatal(err)
	}
}

func TestAccountColorsNoopDoesNotWrite(t *testing.T) {
	s, path, _ := open(t)
	if err := s.SetAccountColors(map[string]int{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("unchanged colours must not create the file")
	}
	if err := s.SetAccountColors(map[string]int{"777788889999": 3}); err != nil {
		t.Fatal(err)
	}
	if s.AccountColors()["777788889999"] != 3 {
		t.Fatal("colour lost")
	}
	if err := s.MergeAccountNames(map[string]string{"777788889999": "initech-platform"}); err != nil {
		t.Fatal(err)
	}
	if s.AccountNames()["777788889999"] != "initech-platform" {
		t.Fatal("name lost")
	}
}

func TestExtraReposAndRepoTests(t *testing.T) {
	s, _, _ := open(t)
	p := t.TempDir()
	if err := s.AddExtraRepo(p); err != nil || s.AddExtraRepo(p) != nil {
		t.Fatal("add")
	}
	if got := s.ExtraRepos(); len(got) != 1 || got[0] != p {
		t.Fatalf("%v", got)
	}
	if err := s.SetRepoTest(api.TestResult{RepoID: "r1", Status: api.TestOK, At: "2026-11-04T09:58:00Z", Profile: "initech-admin", Detail: "stderr text"}); err != nil {
		t.Fatal(err)
	}
	got := s.RepoTests()["r1"]
	if got.Status != api.TestOK || got.Detail != "" {
		t.Fatalf("%+v", got)
	}
}

func TestPendingDeactivations(t *testing.T) {
	s, _, _ := open(t)
	pd := migrate.PendingDeactivation{Profile: "acme", AccountID: "111122223333", IAMUser: "acme-dev", MaskedKeyID: "AKIA••••••••••••MPLE", Since: "2026-11-04T10:12:00Z"}
	if err := s.AddPendingDeactivation(pd); err != nil || s.AddPendingDeactivation(pd) != nil {
		t.Fatal("add")
	}
	if got := s.PendingDeactivations(); len(got) != 1 || got[0] != pd {
		t.Fatalf("%v", got)
	}
	if err := s.RemovePendingDeactivation("acme"); err != nil || len(s.PendingDeactivations()) != 0 {
		t.Fatal("remove")
	}
}

func TestNoSecretPatterns(t *testing.T) {
	s, path, _ := open(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	dark := api.ThemeDark
	ret := 50
	_, err := s.Update(api.SettingsPatch{Theme: &dark, BackupRetention: &ret})
	must(err)
	must(s.CompleteFirstRun())
	must(s.SetUILanguage("en"))
	must(s.SetAccountColors(map[string]int{"profile:acme": 2}))
	must(s.SetMonogram("acme", "AC"))
	must(s.MergeAccountNames(map[string]string{"111122223333": "acme-prod"}))
	// A raw key id and a secret smuggled into fields that are persisted must be masked / never appear.
	must(s.SetStaticIdentity(api.StaticIdentity{Profile: "acme", AccountID: "111122223333", Arn: "arn:aws:iam::111122223333:user/acme-dev",
		IAMUser: "acme-dev", MaskedKeyID: "AKIAIOSFODNN7EXAMPLE", ErrorCode: "aws_secret_access_key=fakeSecretValueDoNotUse"}))
	must(s.SetSessionRecord("initech", api.SessionRecord{LoginAt: "2026-11-04T01:30:00Z", ClientIDHash: "3f9a0c7be21d4e58", SessionMinutes: 480}))
	must(s.AddExtraRepo(t.TempDir()))
	must(s.SetRepoTest(api.TestResult{RepoID: "r", Status: api.TestError, At: "2026-11-04T09:58:00Z", Detail: "fakeSecretValueDoNotUse"}))
	must(s.AddPendingDeactivation(migrate.PendingDeactivation{Profile: "acme", MaskedKeyID: "AKIAIOSFODNN7EXAMPLE", Since: "2026-11-04T10:12:00Z"}))
	b, err := os.ReadFile(path)
	must(err)
	if regexp.MustCompile(`A(KI|SI)A[0-9A-Z]{16}`).Match(b) || strings.Contains(string(b), "fakeSecret") {
		t.Fatalf("secret pattern in settings.json:\n%s", b)
	}
}

func TestAutoBindOnAddRoundTrip(t *testing.T) {
	s, path, _ := open(t)
	if !s.API().AutoBindOnAdd || !s.AutoBindOnAdd() {
		t.Fatal("autoBindOnAdd defaults to true")
	}
	off := false
	got, err := s.Update(api.SettingsPatch{AutoBindOnAdd: &off})
	if err != nil || got.AutoBindOnAdd {
		t.Fatalf("update: %+v %v", got, err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"autoBindOnAdd": false`) {
		t.Fatalf("file: %s", raw)
	}
	// An unrelated patch keeps the value, and a reload sees it.
	theme := api.ThemeDark
	if _, err := s.Update(api.SettingsPatch{Theme: &theme}); err != nil {
		t.Fatal(err)
	}
	re, _, err := Open(path, filepath.Join(filepath.Dir(path), "code"), fixedNow)
	if err != nil || re.API().AutoBindOnAdd || re.API().Theme != api.ThemeDark {
		t.Fatalf("reload: %+v %v", re.API(), err)
	}
	// A file written before the setting existed gets the default.
	old := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(old, []byte(`{"schemaVersion":1,"firstRunDone":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	o, _, err := Open(old, "/tmp/code", fixedNow)
	if err != nil || !o.AutoBindOnAdd() {
		t.Fatalf("old file: %v", err)
	}
}

func TestRemoveExtraRepo(t *testing.T) {
	s, _, _ := open(t)
	a, b := t.TempDir(), t.TempDir()
	for _, p := range []string{a, b} {
		if err := s.AddExtraRepo(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RemoveExtraRepo(a); err != nil || s.RemoveExtraRepo(a) != nil {
		t.Fatal("remove")
	}
	if got := s.ExtraRepos(); len(got) != 1 || got[0] != b {
		t.Fatalf("%v", got)
	}
}

func writeTool(t *testing.T, dirMode os.FileMode, rel string, mode os.FileMode) string {
	t.Helper()
	d := filepath.Join(t.TempDir(), filepath.Dir(rel))
	if err := os.MkdirAll(d, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(d, dirMode); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(d, filepath.Base(rel))
	if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestUpdateChecksToolPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}
	s, _, _ := open(t)
	good := writeTool(t, 0o700, "ok/aws", 0o755)
	goodGit := writeTool(t, 0o700, "ok/git", 0o755)
	out, err := s.Update(api.SettingsPatch{AWSCLIPath: &good, GitPath: &goodGit})
	if err != nil || out.AWSCLIPath != good || out.GitPath != goodGit {
		t.Fatalf("valid paths: %+v %v", out, err)
	}
	build := writeTool(t, 0o700, "bad/build.sh", 0o755)
	noExec := writeTool(t, 0o700, "noexec/aws", 0o644)
	openDir := writeTool(t, 0o777, "open/aws", 0o755)
	quoted := writeTool(t, 0o700, `q"x/aws`, 0o755)
	dirNamedAws := filepath.Join(t.TempDir(), "aws")
	if err := os.Mkdir(dirNamedAws, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]string{"relative": "bin/aws", "other program": build, "not executable": noExec,
		"writable folder": openDir, "quote": quoted, "directory": dirNamedAws, "git as aws": goodGit} {
		p := p
		_, err := s.Update(api.SettingsPatch{AWSCLIPath: &p})
		if err == nil {
			t.Errorf("%s: %q must be rejected", name, p)
			continue
		}
		wantInvalid(t, err, "awsCliPath")
	}
	rel := "bin/git"
	_, err = s.Update(api.SettingsPatch{GitPath: &rel})
	wantInvalid(t, err, "gitPath")
	if got := s.API(); got.AWSCLIPath != good || got.GitPath != goodGit {
		t.Errorf("a rejected patch must change nothing: %+v", got)
	}
	empty := ""
	out, err = s.Update(api.SettingsPatch{AWSCLIPath: &empty})
	if err != nil || out.AWSCLIPath != "" {
		t.Errorf("empty means automatic: %+v %v", out, err)
	}
}

func TestLoadDropsInvalidToolPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	good := writeTool(t, 0o700, "ok/git", 0o755)
	body := `{"firstRunDone": true, "awsCliPath": "scripts/build.sh", "gitPath": ` + strconv.Quote(good) + `}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _, err := Open(path, filepath.Join(dir, "code"), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.AWSCLIPath(); got != "" {
		t.Errorf("invalid stored awsCliPath survived: %q", got)
	}
	if got := s.GitPath(); got != good {
		t.Errorf("valid stored gitPath dropped: %q", got)
	}
	if d := s.DroppedToolPaths(); len(d) != 1 || d[0] != "awsCliPath" {
		t.Errorf("dropped: %v", d)
	}
}

func TestCloneAndPortalSettingsDefaults(t *testing.T) {
	s, _, _ := open(t)
	a := s.API()
	if a.CollapsedPortals == nil || len(a.CollapsedPortals) != 0 || a.LastCloneProfile != "" || a.LastCloneRegion != "" {
		t.Fatalf("%+v", a)
	}
}

func TestCloneAndPortalSettingsRoundTripAndPatch(t *testing.T) {
	s, path, _ := open(t)
	prof, region := "dev-admin", "eu-west-1"
	out, err := s.Update(api.SettingsPatch{CollapsedPortals: []string{" acme ", "initech", "acme"}, LastCloneProfile: &prof, LastCloneRegion: &region})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(out.CollapsedPortals) != "[acme initech]" || out.LastCloneProfile != prof || out.LastCloneRegion != region {
		t.Fatalf("%+v", out)
	}
	// no aliasing: mutating the returned slice must not change the store
	out.CollapsedPortals[0] = "x"
	if s.API().CollapsedPortals[0] != "acme" {
		t.Fatal("slice aliased")
	}
	s2, _, err := Open(path, filepath.Join(t.TempDir(), "code"), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if a := s2.API(); fmt.Sprint(a.CollapsedPortals) != "[acme initech]" || a.LastCloneProfile != prof || a.LastCloneRegion != region {
		t.Fatalf("reload: %+v", a)
	}
	// nil leaves everything unchanged
	if out, err = s.Update(api.SettingsPatch{}); err != nil || len(out.CollapsedPortals) != 2 || out.LastCloneProfile != prof || out.LastCloneRegion != region {
		t.Fatalf("%+v %v", out, err)
	}
	// empty slice / empty strings clear
	empty := ""
	out, err = s.Update(api.SettingsPatch{CollapsedPortals: []string{}, LastCloneProfile: &empty, LastCloneRegion: &empty})
	if err != nil || out.CollapsedPortals == nil || len(out.CollapsedPortals) != 0 || out.LastCloneProfile != "" || out.LastCloneRegion != "" {
		t.Fatalf("%+v %v", out, err)
	}
}

func TestCloneAndPortalSettingsValidation(t *testing.T) {
	s, _, _ := open(t)
	bad := func(name string, p api.SettingsPatch, field string) {
		t.Helper()
		_, err := s.Update(p)
		var ae *api.AppError
		if !errors.As(err, &ae) || ae.Code != api.ErrInvalidInput || ae.Params["field"] != field {
			t.Fatalf("%s: %v", name, err)
		}
	}
	str := func(v string) *string { return &v }
	many := make([]string, 501)
	for i := range many {
		many[i] = "p" + strconv.Itoa(i)
	}
	bad("region", api.SettingsPatch{LastCloneRegion: str("mars")}, "lastCloneRegion")
	bad("profile long", api.SettingsPatch{LastCloneProfile: str(strings.Repeat("a", 201))}, "lastCloneProfile")
	bad("profile ctrl", api.SettingsPatch{LastCloneProfile: str("a\nb")}, "lastCloneProfile")
	bad("portal empty", api.SettingsPatch{CollapsedPortals: []string{"ok", "  "}}, "collapsedPortals")
	bad("portal long", api.SettingsPatch{CollapsedPortals: []string{strings.Repeat("a", 201)}}, "collapsedPortals")
	bad("portal ctrl", api.SettingsPatch{CollapsedPortals: []string{"a\x00b"}}, "collapsedPortals")
	bad("too many", api.SettingsPatch{CollapsedPortals: many}, "collapsedPortals")
	if _, err := s.Update(api.SettingsPatch{CollapsedPortals: many[:500]}); err != nil {
		t.Fatalf("500 entries must be accepted: %v", err)
	}
}

func TestCloneAndPortalSettingsNormalizeOnLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	raw := `{"collapsedPortals":[" a ","a","","b"],"lastCloneProfile":"bad\u0001","lastCloneRegion":"mars"}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _, err := Open(path, filepath.Join(dir, "code"), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	a := s.API()
	if fmt.Sprint(a.CollapsedPortals) != "[a b]" || a.LastCloneProfile != "" || a.LastCloneRegion != "" {
		t.Fatalf("%+v", a)
	}
}

func TestPrivateSignInDefaultAndPatch(t *testing.T) {
	s, path, _ := open(t)
	if !s.PrivateSignIn() || !s.API().PrivateSignIn {
		t.Fatal("privateSignIn defaults to true")
	}
	off, on := false, true
	got, err := s.Update(api.SettingsPatch{PrivateSignIn: &off})
	if err != nil || got.PrivateSignIn || s.PrivateSignIn() {
		t.Fatalf("patch false: %+v %v", got, err)
	}
	// an unrelated patch keeps it; a reload sees false
	theme := api.ThemeDark
	if _, err := s.Update(api.SettingsPatch{Theme: &theme}); err != nil {
		t.Fatal(err)
	}
	re, _, err := Open(path, filepath.Join(filepath.Dir(path), "code"), fixedNow)
	if err != nil || re.PrivateSignIn() || re.API().PrivateSignIn {
		t.Fatalf("reload: %+v %v", re.API(), err)
	}
	got, err = re.Update(api.SettingsPatch{PrivateSignIn: &on})
	if err != nil || !got.PrivateSignIn || !re.PrivateSignIn() {
		t.Fatalf("patch true: %+v %v", got, err)
	}
	re2, _, err := Open(path, filepath.Join(filepath.Dir(path), "code"), fixedNow)
	if err != nil || !re2.PrivateSignIn() {
		t.Fatalf("reload true: %v", err)
	}
}

func TestPrivateSignInOldFileIsOn(t *testing.T) {
	old := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(old, []byte(`{"schemaVersion":1,"firstRunDone":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	o, _, err := Open(old, "/tmp/code", fixedNow)
	if err != nil || !o.PrivateSignIn() || !o.API().PrivateSignIn {
		t.Fatalf("old file: %v", err)
	}
}

func TestConsoleProxiesDefaultPatchAndReload(t *testing.T) {
	s, path, _ := open(t)
	if a := s.API(); a.ConsoleProxies == nil || len(a.ConsoleProxies) != 0 || s.ConsoleProxy("corp") != "" {
		t.Fatalf("default: %#v", a.ConsoleProxies)
	}
	const legacy = "legacy:d-1234567890.awsapps.com/start"
	out, err := s.Update(api.SettingsPatch{ConsoleProxies: map[string]string{" corp ": " 192.0.2.10:03128 ", legacy: "Proxy.Example.com:8080"}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"corp": "192.0.2.10:3128", legacy: "proxy.example.com:8080"}
	if !reflect.DeepEqual(out.ConsoleProxies, want) || s.ConsoleProxy("corp") != "192.0.2.10:3128" || s.ConsoleProxy(legacy) != "proxy.example.com:8080" {
		t.Fatalf("stored in canonical form: %#v", out.ConsoleProxies)
	}
	// no aliasing: changing the returned map changes nothing
	out.ConsoleProxies["corp"] = "x"
	if s.ConsoleProxy("corp") != "192.0.2.10:3128" {
		t.Fatal("map aliased")
	}
	// a patch changes only the portals it names; "" (or spaces) turns a proxy off
	out, err = s.Update(api.SettingsPatch{ConsoleProxies: map[string]string{"corp": " ", "other": "198.51.100.7:3128", "never-set": ""}})
	want = map[string]string{legacy: "proxy.example.com:8080", "other": "198.51.100.7:3128"}
	if err != nil || !reflect.DeepEqual(out.ConsoleProxies, want) || s.ConsoleProxy("corp") != "" {
		t.Fatalf("%#v %v", out.ConsoleProxies, err)
	}
	// nil and unrelated patches keep them; a reload sees them
	theme := api.ThemeDark
	if out, err = s.Update(api.SettingsPatch{Theme: &theme}); err != nil || !reflect.DeepEqual(out.ConsoleProxies, want) {
		t.Fatalf("%#v %v", out.ConsoleProxies, err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"consoleProxies": {`) || !strings.Contains(string(raw), `"other": "198.51.100.7:3128"`) {
		t.Fatalf("file: %s", raw)
	}
	re, _, err := Open(path, filepath.Join(filepath.Dir(path), "code"), fixedNow)
	if err != nil || !reflect.DeepEqual(re.API().ConsoleProxies, want) || re.ConsoleProxy("other") != "198.51.100.7:3128" {
		t.Fatalf("reload: %#v %v", re.API().ConsoleProxies, err)
	}
	// turning every proxy off leaves an empty map, never null
	if out, err = re.Update(api.SettingsPatch{ConsoleProxies: map[string]string{legacy: "", "other": ""}}); err != nil || out.ConsoleProxies == nil || len(out.ConsoleProxies) != 0 {
		t.Fatalf("%#v %v", out.ConsoleProxies, err)
	}
}

func TestConsoleProxiesValidation(t *testing.T) {
	s, _, _ := open(t)
	if _, err := s.Update(api.SettingsPatch{ConsoleProxies: map[string]string{"corp": "192.0.2.10:3128"}}); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]map[string]string{
		"scheme":         {"corp": "http://192.0.2.10:3128"},
		"path":           {"corp": "192.0.2.10:3128/x"},
		"no port":        {"corp": "192.0.2.10"},
		"port 0":         {"corp": "192.0.2.10:0"},
		"port too big":   {"corp": "192.0.2.10:65536"},
		"space inside":   {"corp": "192.0.2.10 :3128"},
		"user info":      {"corp": "user@proxy.example.com:3128"},
		"ipv6":           {"corp": "[2001:db8::1]:3128"},
		"not an address": {"corp": "300.0.2.10:3128"},
		"empty portal":   {" ": "192.0.2.10:3128"},
		"long portal":    {strings.Repeat("a", 201): "192.0.2.10:3128"},
		"control char":   {"a\nb": "192.0.2.10:3128"},
		"one bad entry":  {"new": "198.51.100.7:3128", "corp": "nope"},
		"bad entry, off": {"corp": "", "x": "http://198.51.100.7"},
	} {
		_, err := s.Update(api.SettingsPatch{ConsoleProxies: p})
		if ae := api.AsAppError(err); ae == nil || ae.Code != api.ErrInvalidInput || ae.Params["field"] != "consoleProxies" {
			t.Errorf("%s: %v", name, err)
		}
	}
	if got := s.API().ConsoleProxies; !reflect.DeepEqual(got, map[string]string{"corp": "192.0.2.10:3128"}) {
		t.Fatalf("a rejected patch must change nothing: %#v", got)
	}
	many := map[string]string{}
	for i := 0; i < maxConsoleProxies; i++ {
		many["p"+strconv.Itoa(i)] = "203.0.113.5:3128"
	}
	_, err := s.Update(api.SettingsPatch{ConsoleProxies: many})
	wantInvalid(t, err, "consoleProxies")
	delete(many, "p0")
	if out, err := s.Update(api.SettingsPatch{ConsoleProxies: many}); err != nil || len(out.ConsoleProxies) != maxConsoleProxies {
		t.Fatalf("%d proxies must fit: %d %v", maxConsoleProxies, len(out.ConsoleProxies), err)
	}
}

func TestConsoleProxiesNormalizeOnLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	raw := `{"consoleProxies":{" corp ":" 192.0.2.10:3128 ","scheme":"http://192.0.2.10:3128","":"192.0.2.10:3128",` +
		`"lan":"PROXY.example.com:0080","ctl\u0001":"192.0.2.10:3128","noport":"192.0.2.10"}}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _, err := Open(path, filepath.Join(dir, "code"), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"corp": "192.0.2.10:3128", "lan": "proxy.example.com:80"}
	if got := s.API().ConsoleProxies; !reflect.DeepEqual(got, want) {
		t.Fatalf("%#v", got)
	}
	// a file from an older version (no key) and a null map both read as "no proxies"
	for _, body := range []string{`{"schemaVersion":1,"firstRunDone":true}`, `{"consoleProxies":null}`} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		o, _, err := Open(path, filepath.Join(dir, "code"), fixedNow)
		if err != nil || o.API().ConsoleProxies == nil || len(o.API().ConsoleProxies) != 0 || o.ConsoleProxy("corp") != "" {
			t.Fatalf("%s: %#v %v", body, o.API().ConsoleProxies, err)
		}
	}
}

func TestReminderSwitchBecomesAutoSignIn(t *testing.T) {
	for _, c := range []struct {
		name, reminders string
		on              bool
		lead            int
	}{
		{"old switch on", `{"expiryNotice":true,"leadMinutes":30,"autoRenew":true}`, true, 10},
		{"old switch off", `{"expiryNotice":false,"leadMinutes":60,"autoRenew":true}`, false, 10},
		{"new settings stay", `{"autoSignIn":false,"leadMinutes":30,"autoRenew":true}`, false, 30},
		{"new settings with a stray old key", `{"autoSignIn":true,"expiryNotice":false,"leadMinutes":15,"autoRenew":true}`, true, 15},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(path, []byte(`{"schemaVersion":1,"reminders":`+c.reminders+`}`), 0o600); err != nil {
				t.Fatal(err)
			}
			s, _, err := Open(path, "/default/root", fixedNow)
			if err != nil {
				t.Fatal(err)
			}
			if r := s.API().Reminders; r.AutoSignIn != c.on || r.LeadMinutes != c.lead || !r.AutoRenew || s.AutoSignIn() != c.on ||
				s.SignInLead() != time.Duration(c.lead)*time.Minute {
				t.Fatalf("%+v", r)
			}
		})
	}
}

func TestSignInLeadValues(t *testing.T) {
	s, _, err := Open(filepath.Join(t.TempDir(), "settings.json"), "/default/root", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{10, 15, 30, 60} {
		if _, err := s.Update(api.SettingsPatch{Reminders: &api.ReminderSettings{AutoSignIn: true, LeadMinutes: n, AutoRenew: true}}); err != nil {
			t.Fatalf("%d: %v", n, err)
		}
	}
	for _, n := range []int{0, 5, 20, 45, 90} {
		_, err := s.Update(api.SettingsPatch{Reminders: &api.ReminderSettings{AutoSignIn: true, LeadMinutes: n, AutoRenew: true}})
		wantInvalid(t, err, "reminders.leadMinutes")
	}
}
