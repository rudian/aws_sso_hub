package app

import (
	"bytes"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/paths"
	"github.com/rudian/aws_sso_hub/internal/sso"
	"github.com/rudian/aws_sso_hub/internal/version"
)

// TestShutdownKeepsLogin: quitting never signs out. Token file, role cache and the settings
// session record stay byte-identical.
func TestShutdownKeepsLogin(t *testing.T) {
	w := newWorld(t)
	rec := api.SessionRecord{LoginAt: "2026-11-04T01:30:00Z", ClientIDHash: "3f9a0c7be21d4e58", TokenIssuedAt: "2026-11-04T02:18:00Z", SessionMinutes: 480}
	if err := w.c.Settings.SetSessionRecord("other", rec); err != nil {
		t.Fatal(err)
	}
	roleKey := sso.RoleCacheKey(acctAcme, "AdministratorAccess", "other", "")
	if err := os.MkdirAll(w.p.CLICacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	roleFile := filepath.Join(w.p.CLICacheDir, roleKey+".json")
	if err := os.WriteFile(roleFile, []byte(`{"ProviderType":"sso","Credentials":{"AccessKeyId":"x"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tokenFile := filepath.Join(w.p.SSOCacheDir, sso.TokenCacheKey("other", "")+".json")
	snap := func() [3]string {
		sess := w.c.Settings.API() // loaded from memory; re-read the file for the record
		_ = sess
		b, err := os.ReadFile(w.p.SettingsFile)
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Sessions map[string]api.SessionRecord `json:"sessions"`
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			t.Fatal(err)
		}
		recJSON, _ := json.Marshal(doc.Sessions["other"])
		return [3]string{w.read(tokenFile), w.read(roleFile), string(recJSON)}
	}
	before := snap()

	svc := NewAccountService(w.c)
	if err := svc.ServiceStartup(bg, applicationServiceOptions()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond) // ticker, watcher and environment check ran
	if err := svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	w.c.Shutdown() // idempotent
	if after := snap(); after != before {
		t.Fatalf("shutdown changed login state:\nbefore %q\nafter  %q", before, after)
	}
	// a fresh process reads the same session
	st, err := w.acc.GetState(bg)
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := portalOf(st, "other"); p.Session.State != api.SessionActive {
		t.Fatalf("session after restart: %+v", p.Session)
	}
}

func TestWatcherReportsOnlyExternalChanges(t *testing.T) {
	w := newWorld(t)
	w.c.files.prime()
	w.c.pollFiles(bg)
	if n := w.rt.count(api.EventExternalChange); n != 0 {
		t.Fatalf("no change yet, got %d events", n)
	}
	// the app's own write (reported through OnWrite) is not an external change
	own := []byte(configText + "\n# written by the app\n")
	if err := os.WriteFile(w.p.AWSConfigFile, own, 0o600); err != nil {
		t.Fatal(err)
	}
	w.c.files.note(w.p.AWSConfigFile, own)
	w.c.pollFiles(bg)
	if n := w.rt.count(api.EventExternalChange); n != 0 {
		t.Fatalf("own write reported as external: %d", n)
	}
	// an outside edit is
	if err := os.WriteFile(w.p.AWSConfigFile, append(own, []byte("# edited in a terminal\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	before := w.rt.count(api.EventAccountsChanged)
	w.c.pollFiles(bg)
	var got []api.ExternalChange
	for _, ev := range w.rt.snapshot() {
		if ev.Name == api.EventExternalChange {
			got = append(got, ev.Data.(api.ExternalChange))
		}
	}
	if len(got) != 1 || got[0].Kind != "aws-config" || got[0].File != "~/.aws/config" {
		t.Fatalf("events: %+v", got)
	}
	if w.rt.count(api.EventAccountsChanged) <= before || w.rt.count(api.EventReposChanged) == 0 {
		t.Fatal("an external change must refresh accounts and repos")
	}
	// the global git config
	if err := os.WriteFile(w.p.GitGlobalFile, []byte("[user]\n\tname = Someone\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w.c.pollFiles(bg)
	if n := w.rt.count(api.EventExternalChange); n != 2 {
		t.Fatalf("git-global change: %d events", n)
	}
	w.c.pollFiles(bg)
	if n := w.rt.count(api.EventExternalChange); n != 2 {
		t.Fatalf("the same change must be reported once, got %d", n)
	}
}

func TestUpdateSettingsHooksAndRescan(t *testing.T) {
	w := newWorld(t)
	var prevT, nextT api.TraySettings
	w.c.OnSettingsChanged(func(prev, next api.Settings) { prevT, nextT = prev.Tray, next.Tray })
	langs := []string{}
	w.c.OnUILanguage(func(l string) { langs = append(langs, l) })

	w.gitRepo("web-app", ccURL("web-app"))
	root := t.TempDir()
	tray := api.TraySettings{Enabled: true, KeepRunning: true}
	got, err := w.sys.UpdateSettings(api.SettingsPatch{Tray: &tray, ScanRoots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Tray.Enabled || len(got.ScanRoots) != 1 || got.ScanRoots[0] != root {
		t.Fatalf("%+v", got)
	}
	if prevT.Enabled || !nextT.Enabled || !nextT.KeepRunning {
		t.Fatalf("hook: %+v -> %+v", prevT, nextT)
	}
	if w.rt.count(api.EventSettingsChanged) != 1 {
		t.Fatal("settings:changed")
	}
	// the rescan runs in the background and ends with a repos:changed carrying the new (empty) root
	deadline := time.Now().Add(15 * time.Second)
	for w.rt.count(api.EventReposChanged) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if w.rt.count(api.EventReposChanged) == 0 {
		t.Fatal("changing the scan roots must rescan")
	}
	if err := w.sys.SetUILanguage("en"); err != nil || len(langs) != 1 || langs[0] != "en" {
		t.Fatalf("%v %v", err, langs)
	}
	info, _ := w.sys.GetAppInfo()
	if info.UILanguage != "en" {
		t.Fatal(info.UILanguage)
	}
	w.c.Shutdown()
}

func TestOpenURLAndRevealPathGuards(t *testing.T) {
	w := newWorld(t)
	if err := w.sys.OpenURL("https://d-1234567890.awsapps.com/start"); err != nil || len(w.rt.urls) != 1 {
		t.Fatalf("%v %v", err, w.rt.urls)
	}
	for _, bad := range []string{"http://example.com", "file:///etc/passwd", "javascript:alert(1)", "https://", "mailto:x@y.z", ""} {
		if err := w.sys.OpenURL(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
	inside := filepath.Join(w.p.Home, ".aws")
	if err := w.sys.RevealPath(inside); err != nil || len(w.rt.revealed) != 1 {
		t.Fatalf("%v %v", err, w.rt.revealed)
	}
	// Health findings carry the display form of a path.
	if err := w.sys.RevealPath("~/.aws"); err != nil || len(w.rt.revealed) != 2 || w.rt.revealed[1] != inside {
		t.Fatalf("display path: %v %v", err, w.rt.revealed)
	}
	for _, bad := range []string{"/etc", "/", filepath.Join(w.p.Home, "..", "elsewhere"), "~/../elsewhere", "~/missing-dir", filepath.Join(w.p.Home, "missing-dir"), "relative/path"} {
		if err := w.sys.RevealPath(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
	if err := w.sys.OpenAWSConfig(); err != nil || len(w.rt.opened) != 1 || w.rt.opened[0] != w.p.AWSConfigFile {
		t.Fatalf("%v %v", err, w.rt.opened)
	}
	w.rt.folder = "/some/where"
	if dir, err := w.sys.ChooseFolder("pick"); err != nil || dir != "/some/where" {
		t.Fatalf("%q %v", dir, err)
	}
	if err := w.sys.OpenBackupFolder(); err != nil || w.rt.revealed[len(w.rt.revealed)-1] != w.p.BackupDir {
		t.Fatalf("%v", err)
	}
}

func TestRestoreBackup(t *testing.T) {
	w := newWorld(t)
	// a config backup (no secrets) and a credentials backup (redacted)
	cfgBefore := w.read(w.p.AWSConfigFile)
	e1, err := w.c.Backup.Snapshot(w.p.AWSConfigFile, "~/.aws/config", api.BackupAWSConfig, api.ReasonAddProfiles, map[string]string{"n": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.p.AWSConfigFile, []byte("[profile changed]\nregion = eu-west-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pv, err := w.sys.PreviewRestore(e1.ID)
	if err != nil || pv.Preview.Added == 0 || pv.Preview.Removed == 0 || len(pv.LostProfiles) != 0 {
		t.Fatalf("%+v %v", pv, err)
	}
	safety, err := w.sys.RestoreBackup(e1.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if safety.Reason != api.ReasonRestore || safety.ID == "" || safety.ID == e1.ID {
		t.Fatalf("safety snapshot: %+v", safety)
	}
	if w.read(w.p.AWSConfigFile) != cfgBefore {
		t.Fatal("config not restored")
	}
	if _, sbytes, _ := w.c.Backup.Get(safety.ID); !strings.Contains(string(sbytes), "[profile changed]") {
		t.Fatal("the safety snapshot must hold the file as it was before the restore")
	}
	if w.rt.count(api.EventAccountsChanged) == 0 {
		t.Fatal("restoring AWS config must emit accounts:changed")
	}

	// credentials: the backup holds placeholders; the keys were since removed from the file
	e2, err := w.c.Backup.Snapshot(w.p.AWSCredentialsFile, "~/.aws/credentials", api.BackupAWSCredentials, api.ReasonRemoveKey, map[string]string{"profile": "acme"})
	if err != nil || !e2.Redacted {
		t.Fatalf("%+v %v", e2, err)
	}
	if err := os.WriteFile(w.p.AWSCredentialsFile, []byte("[unrelated]\naws_access_key_id = AKIAI44QH8DHBEXAMPLE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pv, err = w.sys.PreviewRestore(e2.ID)
	if err != nil || len(pv.LostProfiles) != 2 {
		t.Fatalf("lost profiles: %+v %v", pv.LostProfiles, err)
	}
	for _, l := range pv.Preview.Lines {
		for _, s := range secretStrings {
			if strings.Contains(l.Text, s) {
				t.Fatalf("secret in restore preview: %q", l.Text)
			}
		}
	}
	_, err = w.sys.RestoreBackup(e2.ID, false)
	ae := api.AsAppError(err)
	if ae == nil || ae.Code != api.ErrRestoreNeedsConfirm || !strings.Contains(ae.Params["profiles"], "acme") {
		t.Fatalf("want E_RESTORE_NEEDS_CONFIRM, got %v", err)
	}
	if !strings.Contains(w.read(w.p.AWSCredentialsFile), "[unrelated]") {
		t.Fatal("nothing may change without confirmation")
	}
	if _, err := w.sys.RestoreBackup(e2.ID, true); err != nil {
		t.Fatal(err)
	}
	if got := w.read(w.p.AWSCredentialsFile); strings.Contains(got, "fakeSecret") || !strings.Contains(got, "[acme]") {
		t.Fatalf("restored credentials:\n%s", got)
	}
}

func TestEndpointsFromEnvOnlyInSandbox(t *testing.T) {
	t.Setenv("AWSSSOHUB_OIDC_ENDPOINT", "http://127.0.0.1:1")
	t.Setenv("AWSSSOHUB_CODECOMMIT_ENDPOINT", "http://127.0.0.1:2")
	t.Setenv("AWSSSOHUB_SIGNIN_ENDPOINT", "http://127.0.0.1:3")
	if ep := EndpointsFromEnv(paths.Paths{Sandbox: false}); ep != (Endpoints{}) {
		t.Fatalf("overrides must be ignored outside sandbox mode: %+v", ep)
	}
	ep := EndpointsFromEnv(paths.ForRoot(t.TempDir()))
	if ep.OIDC != "http://127.0.0.1:1" || ep.CodeCommit != "http://127.0.0.1:2" || ep.SignIn != "http://127.0.0.1:3" || ep.STS != "" {
		t.Fatalf("%+v", ep)
	}
}

func TestSelfTestReport(t *testing.T) {
	w := newWorld(t)
	awsPath := w.fake
	if _, err := w.sys.UpdateSettings(api.SettingsPatch{AWSCLIPath: &awsPath}); err != nil {
		t.Fatal(err)
	}
	w.gitRepo("web-app", ccURL("web-app"))
	out := filepath.Join(t.TempDir(), "selftest.json")
	if err := RunSelfTest(w.c, out); err != nil {
		t.Fatal(err)
	}
	var rep struct {
		OK       bool     `json:"ok"`
		Version  string   `json:"version"`
		Sandbox  bool     `json:"sandbox"`
		Profiles int      `json:"profiles"`
		Portals  int      `json:"portals"`
		Repos    int      `json:"repos"`
		Errors   []string `json:"errors"`
		AWSCLI   struct {
			Status string `json:"status"`
			Path   string `json:"path"`
		} `json:"awsCli"`
		Git struct {
			Status string `json:"status"`
		} `json:"git"`
	}
	if err := json.Unmarshal([]byte(w.read(out)), &rep); err != nil {
		t.Fatal(err)
	}
	if !rep.OK || !rep.Sandbox || rep.Version != version.Version || rep.Profiles != 3 || rep.Portals != 2 || rep.Repos != 1 ||
		rep.AWSCLI.Status != "ok" || rep.AWSCLI.Path != awsPath || rep.Git.Status != "ok" || rep.Errors == nil || len(rep.Errors) != 0 {
		t.Fatalf("%+v", rep)
	}
}

func TestNonNilSlices(t *testing.T) {
	type inner struct{ Items []string }
	type outer struct {
		A     []int
		B     *inner
		C     []inner
		Named string
	}
	got := nn(outer{B: &inner{}, C: []inner{{}, {Items: []string{"x"}}}})
	b, _ := json.Marshal(got)
	if strings.Contains(string(b), "null") {
		t.Fatalf("null left in %s", b)
	}
	st := nn(api.AccountsState{})
	b, _ = json.Marshal(st)
	for _, k := range []string{`"issues":[]`, `"portals":[]`, `"profiles":[]`} {
		if !strings.Contains(string(b), k) {
			t.Fatalf("%s missing in %s", k, b)
		}
	}
}

func TestTrayModel(t *testing.T) {
	now := time.Date(2026, 11, 4, 4, 0, 0, 0, time.UTC)
	st := api.AccountsState{
		Portals: []api.Portal{
			{Name: "initech", Profiles: []string{"initech-admin", "initech-readonly"}, Session: api.SessionStatus{
				State: api.SessionActive, SessionKnown: true, SessionEndsAt: now.Add(6*time.Hour + 12*time.Minute).Format(time.RFC3339)}},
			{Name: "acme", Profiles: []string{"acme-admin"}, Session: api.SessionStatus{State: api.SessionExpired}},
			{Name: "globex", Session: api.SessionStatus{State: api.SessionSignedOut}},
			{Name: "draft", Draft: true},
			{Name: "legacy:x", LegacyFormat: true},
		},
		Profiles: []api.Profile{{Name: "acme", Kind: api.ProfileKindStatic}, {Name: "initech-admin", Kind: api.ProfileKindSSO}},
	}
	zhItems := buildTrayModel(st, "zh-CN", now)
	got := labelsOf(zhItems)
	want := []string{
		"AWS SSO Hub", "initech  已登录 6:12", "  initech-admin", "  initech-readonly", "acme  已过期", "  acme-admin", "globex  未登录",
		"acme  访问密钥（待迁移）", "", "登录 acme…", "登录 globex…", "打开 AWS SSO Hub", "退出",
	}
	if got != strings.Join(want, "\n")+"\n" {
		t.Fatalf("zh menu:\n%s", got)
	}
	enItems := buildTrayModel(st, "en", now)
	en := labelsOf(enItems)
	for _, s := range []string{"AWS SSO Hub", "initech  Signed in 6:12", "acme  Expired", "globex  Not signed in", "acme  Access key (to migrate)", "Sign in to acme…", "Open AWS SSO Hub", "Quit"} {
		if !strings.Contains(en, s) {
			t.Errorf("en menu lacks %q:\n%s", s, en)
		}
	}
	var logins []string
	for _, it := range enItems {
		if it.Kind == trayLogin {
			logins = append(logins, it.Arg)
		}
	}
	if strings.Join(logins, ",") != "acme,globex" {
		t.Fatalf("login items %v (an active portal gets none)", logins)
	}
	// under an hour the label switches to minutes; unknown sessions show no time
	st.Portals[0].Session.SessionEndsAt = now.Add(48 * time.Minute).Format(time.RFC3339)
	if l := portalStateLabel(st.Portals[0].Session, "en", now); l != "Signed in 48m" {
		t.Fatal(l)
	}
	st.Portals[0].Session.SessionKnown = false
	if l := portalStateLabel(st.Portals[0].Session, "en", now); l != "Signed in" {
		t.Fatal(l)
	}
}

func TestTrayIcons(t *testing.T) {
	for _, tc := range []struct {
		template bool
		size     int
	}{{true, 44}, {false, 32}} {
		img, err := png.Decode(bytes.NewReader(trayIconPNG(tc.template)))
		if err != nil {
			t.Fatal(err)
		}
		if b := img.Bounds(); b.Dx() != tc.size || b.Dy() != tc.size {
			t.Fatalf("template=%v size %v", tc.template, b)
		}
		opaque, transparent := 0, 0
		for y := 0; y < tc.size; y++ {
			for x := 0; x < tc.size; x++ {
				_, _, _, a := img.At(x, y).RGBA()
				if a > 0 {
					opaque++
				} else {
					transparent++
				}
			}
		}
		if opaque < 30 || transparent < 30 {
			t.Fatalf("template=%v: icon looks empty or solid (opaque %d, transparent %d)", tc.template, opaque, transparent)
		}
		if tc.template { // template icons are pure black + alpha
			r, g, b, _ := img.At(tc.size/2, tc.size/2-6).RGBA()
			if r|g|b != 0 {
				t.Fatalf("template icon must be black, got %d %d %d", r, g, b)
			}
		}
	}
}
