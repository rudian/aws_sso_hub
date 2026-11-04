package paths

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestForRootLayout(t *testing.T) {
	root := t.TempDir()
	p := ForRoot(root)
	home := filepath.Join(root, "home")
	app := filepath.Join(root, "appdata")
	want := map[string]string{
		"Home":               home,
		"AWSConfigFile":      filepath.Join(home, ".aws", "config"),
		"AWSCredentialsFile": filepath.Join(home, ".aws", "credentials"),
		"SSOCacheDir":        filepath.Join(home, ".aws", "sso", "cache"),
		"CLICacheDir":        filepath.Join(home, ".aws", "cli", "cache"),
		"AppDataDir":         app,
		"SettingsFile":       filepath.Join(app, "settings.json"),
		"BackupDir":          filepath.Join(app, "backups"),
		"RegistrationsDir":   filepath.Join(app, "registrations"),
		"LogDir":             filepath.Join(app, "logs"),
		"WebViewDir":         filepath.Join(app, "WebView2"),
		"DefaultScanRoot":    filepath.Join(home, "Documents", "code"),
		"GitGlobalFile":      filepath.Join(home, ".gitconfig"),
		"SandboxRoot":        root,
	}
	v := reflect.ValueOf(p)
	for field, exp := range want {
		if got := v.FieldByName(field).String(); got != exp {
			t.Errorf("%s = %q, want %q", field, got, exp)
		}
	}
	if !p.Sandbox {
		t.Error("Sandbox must be true")
	}
	wantEnv := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"AWS_CONFIG_FILE=" + filepath.Join(home, ".aws", "config"),
		"AWS_SHARED_CREDENTIALS_FILE=" + filepath.Join(home, ".aws", "credentials"),
		"GIT_CONFIG_GLOBAL=" + filepath.Join(home, ".gitconfig"),
		"GIT_CONFIG_NOSYSTEM=1",
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
	}
	got := p.ChildEnv()
	if len(got) != len(wantEnv) {
		t.Fatalf("ChildEnv has %d entries, want %d: %v", len(got), len(wantEnv), got)
	}
	gotSet := map[string]bool{}
	for _, e := range got {
		gotSet[e] = true
	}
	for _, e := range wantEnv {
		if !gotSet[e] {
			t.Errorf("ChildEnv missing %q", e)
		}
	}
}

func TestChildEnvNilOutsideSandbox(t *testing.T) {
	if env := (Paths{Home: "/x"}).ChildEnv(); env != nil {
		t.Errorf("ChildEnv outside sandbox = %v, want nil", env)
	}
}

func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv(SandboxEnv, "")
	t.Setenv("AWS_CONFIG_FILE", "")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "")
	return home
}

func TestResolveDefaults(t *testing.T) {
	home := isolateHome(t)
	p, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if p.Sandbox || p.Home != home {
		t.Fatalf("unexpected Paths: %+v", p)
	}
	if p.AWSConfigFile != filepath.Join(home, ".aws", "config") ||
		p.AWSCredentialsFile != filepath.Join(home, ".aws", "credentials") ||
		p.SSOCacheDir != filepath.Join(home, ".aws", "sso", "cache") ||
		p.CLICacheDir != filepath.Join(home, ".aws", "cli", "cache") {
		t.Errorf("aws paths wrong: %+v", p)
	}
	if filepath.Base(p.AppDataDir) != AppDirName {
		t.Errorf("AppDataDir = %q, want base %q", p.AppDataDir, AppDirName)
	}
	if p.GitGlobalFile != "" {
		t.Errorf("GitGlobalFile must be empty outside sandbox, got %q", p.GitGlobalFile)
	}
	if p.DefaultScanRoot != filepath.Join(home, "Documents", "code") {
		t.Errorf("DefaultScanRoot = %q", p.DefaultScanRoot)
	}
}

func TestResolveHonoursAWSEnv(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "myconfig")
	creds := filepath.Join(dir, "mycreds")
	t.Setenv("AWS_CONFIG_FILE", cfg)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", creds)
	p, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if p.AWSConfigFile != cfg {
		t.Errorf("AWSConfigFile = %q, want %q", p.AWSConfigFile, cfg)
	}
	if p.AWSCredentialsFile != creds {
		t.Errorf("AWSCredentialsFile = %q, want %q", p.AWSCredentialsFile, creds)
	}
}

func TestDisplay(t *testing.T) {
	home := filepath.Join(string(filepath.Separator)+"Users", "alex")
	p := Paths{Home: home}
	in := filepath.Join(home, "Documents", "code", "x")
	want := "~" + string(filepath.Separator) + filepath.Join("Documents", "code", "x")
	if got := p.Display(in); got != want {
		t.Errorf("Display(%q) = %q, want %q", in, got, want)
	}
	if got := p.Display(home); got != "~" {
		t.Errorf("Display(home) = %q, want ~", got)
	}
	outside := filepath.Join(string(filepath.Separator)+"opt", "x")
	if got := p.Display(outside); got != outside {
		t.Errorf("Display(%q) = %q, want unchanged", outside, got)
	}
	// A sibling that merely shares the prefix is not inside home.
	sibling := home + "2" + string(filepath.Separator) + "a"
	if got := p.Display(sibling); got != sibling {
		t.Errorf("Display(%q) = %q, want unchanged", sibling, got)
	}
}

func TestEnsureAppDirs(t *testing.T) {
	p := ForRoot(t.TempDir())
	if err := p.EnsureAppDirs(); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{p.AppDataDir, p.BackupDir, p.RegistrationsDir, p.LogDir} {
		st, err := os.Stat(d)
		if err != nil || !st.IsDir() {
			t.Fatalf("%s not created: %v", d, err)
		}
		if mode := st.Mode().Perm(); mode != 0o700 && !strings.Contains(strings.ToLower(os.Getenv("OS")), "windows") {
			t.Errorf("%s mode = %o, want 700", d, mode)
		}
	}
}

func TestExpand(t *testing.T) {
	home := filepath.Join(string(filepath.Separator)+"Users", "alex")
	p := Paths{Home: home}
	cases := map[string]string{
		"~":             home,
		"~/.zshrc":      filepath.Join(home, ".zshrc"),
		"~/.aws/config": filepath.Join(home, ".aws", "config"),
		"~other/x":      "~other/x",
		"/etc/hosts":    "/etc/hosts",
		"relative/path": "relative/path",
		"":              "",
	}
	for in, want := range cases {
		if got := p.Expand(in); got != want {
			t.Errorf("Expand(%q) = %q, want %q", in, got, want)
		}
	}
	if got := (Paths{}).Expand("~/x"); got != "~/x" {
		t.Errorf("empty home must leave the input alone, got %q", got)
	}
}
