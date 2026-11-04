package gitx

import (
	"strings"
	"testing"

	"github.com/rudian/aws_sso_hub/internal/api"
)

func TestHelperValuesExact(t *testing.T) {
	aws, err := AWSHelperValue("/opt/homebrew/bin/aws", "acme", "darwin")
	if err != nil || aws != `!"/opt/homebrew/bin/aws" --profile 'acme' codecommit credential-helper $@` {
		t.Errorf("aws helper darwin: %q %v", aws, err)
	}
	aws, err = AWSHelperValue(`C:\Program Files\Amazon\AWSCLIV2\aws.exe`, "initech-admin", "windows")
	if err != nil || aws != `!"C:/Program Files/Amazon/AWSCLIV2/aws.exe" --profile 'initech-admin' codecommit credential-helper $@` {
		t.Errorf("aws helper windows: %q %v", aws, err)
	}
	wantEN := `!f() { test "$1" = get || return 0; echo "AWS SSO Hub: cannot get CodeCommit credentials for AWS profile 'acme' (not logged in, session expired, or AWS CLI missing). Click 'SSO sign-in' for this profile in AWS SSO Hub, or run: aws sso login --profile acme" >&2; echo quit=1; }; f`
	wantZH := `!f() { test "$1" = get || return 0; echo "AWS SSO Hub: 无法获取 AWS 配置文件 'acme' 的 CodeCommit 凭证（未登录、登录已过期或缺少 AWS CLI）。请在 AWS SSO Hub 中点击该配置文件的“SSO 登录”，或运行: aws sso login --profile acme" >&2; echo quit=1; }; f`
	for _, c := range []struct{ lang, goos, want string }{
		{"en", "darwin", wantEN}, {"zh-CN", "darwin", wantZH}, {"zh-CN", "windows", wantEN}, {"en", "windows", wantEN}, {"", "darwin", wantEN},
	} {
		if got := FallbackValue("acme", c.lang, c.goos); got != c.want {
			t.Errorf("fallback(%s,%s):\n got %s\nwant %s", c.lang, c.goos, got, c.want)
		}
	}
	guardEN := `!f() { test "$1" = get || return 0; echo "AWS SSO Hub: this repository is not bound to an AWS account (no CodeCommit credential helper). Open AWS SSO Hub > Repositories and bind it." >&2; echo quit=1; }; f`
	guardZH := `!f() { test "$1" = get || return 0; echo "AWS SSO Hub: 这个仓库还没有绑定 AWS 账号。打开 AWS SSO Hub > 仓库，绑定后再试。" >&2; echo quit=1; }; f`
	if GuardValue("en", "darwin") != guardEN || GuardValue("zh-CN", "darwin") != guardZH || GuardValue("zh-CN", "windows") != guardEN {
		t.Error("guard values differ from the documented helper section")
	}
	for _, v := range []string{wantZH, guardZH} {
		inner := v[strings.Index(v, `echo "`)+6 : strings.Index(v, `" >&2`)]
		if strings.ContainsAny(inner, "\"`$\\") {
			t.Errorf("zh text has forbidden ASCII characters: %q", inner)
		}
	}
	if !IsFallback(wantEN) || !IsFallback(wantZH) || IsGuard(wantEN) {
		t.Error("IsFallback/IsGuard confusion for fallback values")
	}
	if !IsGuard(guardEN) || !IsGuard(guardZH) || IsFallback(guardEN) {
		t.Error("IsFallback/IsGuard confusion for guard values")
	}
	if IsFallback("!aws codecommit credential-helper $@") || IsGuard("osxkeychain") {
		t.Error("false positives")
	}
}

func TestUnsafeValues(t *testing.T) {
	for _, p := range []string{"a'b", "a b", "", "a\"b", "a$b"} {
		_, err := AWSHelperValue("/opt/homebrew/bin/aws", p, "darwin")
		ae, ok := err.(*api.AppError)
		if !ok || ae.Code != api.ErrUnsafeValue {
			t.Errorf("profile %q: %v", p, err)
		}
	}
	for _, path := range []string{`/x/a"ws`, "/x/a$ws", "/x/a`ws", ``, `C:\x\aws.exe"`} {
		goos := "darwin"
		if strings.HasPrefix(path, "C:") {
			goos = "windows"
		}
		_, err := AWSHelperValue(path, "p", goos)
		ae, ok := err.(*api.AppError)
		if !ok || ae.Code != api.ErrUnsafeValue {
			t.Errorf("path %q: %v", path, err)
		}
	}
	// a space and a Windows drive path are fine
	if _, err := AWSHelperValue("/tmp/Program Files/aws", "p", "darwin"); err != nil {
		t.Errorf("space in path: %v", err)
	}
	if _, err := BindArgs(BindSpec{RepoDir: "/r", Hosts: []string{ccHost}, Profile: "a'b", AWSPath: "/x/aws"}, false); err == nil {
		t.Error("BindArgs must reject an unsafe profile")
	}
}

func TestParseAWSHelper(t *testing.T) {
	cases := []struct {
		v    string
		want AWSHelper
	}{
		{`!"/opt/homebrew/bin/aws" --profile 'acme' codecommit credential-helper $@`, AWSHelper{true, "acme", true, "/opt/homebrew/bin/aws", true}},
		{`!\"/opt/homebrew/bin/aws\" --profile 'acme' codecommit credential-helper $@`, AWSHelper{true, "acme", true, "/opt/homebrew/bin/aws", true}},
		{`!aws codecommit credential-helper $@ --profile acme`, AWSHelper{true, "acme", true, "aws", false}},
		{`!aws --profile initech-admin codecommit credential-helper $@`, AWSHelper{true, "initech-admin", true, "aws", false}},
		{`!aws --profile=x codecommit credential-helper $@`, AWSHelper{true, "x", true, "aws", false}},
		{`!aws codecommit credential-helper $@`, AWSHelper{true, "", false, "aws", false}},
		{`!"C:/Program Files/Amazon/AWSCLIV2/aws.exe" --profile 'p' codecommit credential-helper $@`, AWSHelper{true, "p", true, "C:/Program Files/Amazon/AWSCLIV2/aws.exe", true}},
		{`osxkeychain`, AWSHelper{}},
		{`!gh auth git-credential`, AWSHelper{}},
		{FallbackValue("p", "en", "darwin"), AWSHelper{}},
	}
	for _, c := range cases {
		if got := ParseAWSHelper(c.v); got != c.want {
			t.Errorf("%s\n got %+v\nwant %+v", c.v, got, c.want)
		}
	}
}

func hr(scope, sub, v string) HelperRef {
	return HelperRef{Value: v, Scope: scope, Origin: "o", Subsection: sub}
}

func TestClassify(t *testing.T) {
	aws := func(p string) string { v, _ := AWSHelperValue("/opt/homebrew/bin/aws", p, "darwin"); return v }
	fb := func(p string) string { return FallbackValue(p, "en", "darwin") }
	exists := func(p string) bool { return p == "/opt/homebrew/bin/aws" }
	guard := GuardValue("en", "darwin")
	meta := LocalMeta{Profile: "acme"}

	type tc struct {
		name string
		eff  Effective
		meta LocalMeta
		kind api.RemoteKind
		grc  string
		want Classified
	}
	cases := []tc{
		{"local managed", Effective{Helpers: []HelperRef{hr("local", "https://h", aws("acme")), hr("local", "https://h", fb("acme"))}, ResetScope: "local", ResetSubsection: "https://h"},
			meta, api.RemoteHTTPS, "", Classified{Source: api.BindingLocal, Profile: "acme", AWSPath: "/opt/homebrew/bin/aws"}},
		{"legacy plain", Effective{Helpers: []HelperRef{hr("local", "", "!aws codecommit credential-helper $@ --profile acme")}, ResetScope: "local"},
			LocalMeta{}, api.RemoteHTTPS, "", Classified{Source: api.BindingLocal, Profile: "acme", AWSPath: "aws", LegacyPlain: true}},
		{"global url", Effective{Helpers: []HelperRef{hr("global", "https://h/v1/repos/x", "!aws --profile initech-admin codecommit credential-helper $@")}, ResetScope: "global"},
			LocalMeta{}, api.RemoteHTTPS, "", Classified{Source: api.BindingGlobalURL, Profile: "initech-admin", URLRule: "https://h/v1/repos/x", AWSPath: "aws"}},
		{"global default", Effective{Helpers: []HelperRef{hr("global", "", "!aws codecommit credential-helper $@ --profile acme")}, ResetScope: "global"},
			LocalMeta{}, api.RemoteHTTPS, "", Classified{Source: api.BindingGlobalDefault, Profile: "acme", AWSPath: "aws"}},
		{"global default without profile", Effective{Helpers: []HelperRef{hr("global", "", "!aws codecommit credential-helper $@")}},
			LocalMeta{}, api.RemoteHTTPS, "", Classified{Source: api.BindingGlobalDefault, AWSPath: "aws", NoProfileFlag: true}},
		{"guard", Effective{Helpers: []HelperRef{hr("global", "https://git-codecommit.*.amazonaws.com", guard)}, ResetScope: "global"},
			LocalMeta{}, api.RemoteHTTPS, "", Classified{Source: api.BindingGuard}},
		{"none", Effective{Helpers: []HelperRef{hr("system", "", "osxkeychain")}}, LocalMeta{}, api.RemoteHTTPS, "", Classified{Source: api.BindingNone}},
		{"grc", Effective{}, LocalMeta{}, api.RemoteGRC, "globex", Classified{Source: api.BindingRemoteURL, Profile: "globex"}},
		{"grc default", Effective{}, LocalMeta{}, api.RemoteGRC, "", Classified{Source: api.BindingRemoteURL, Profile: "default"}},
		{"ssh", Effective{Helpers: []HelperRef{hr("global", "", aws("p"))}}, LocalMeta{}, api.RemoteSSH, "", Classified{Source: api.BindingNone}},
		{"bare aws needs repair", Effective{Helpers: []HelperRef{hr("local", "https://h", `!aws --profile 'p' codecommit credential-helper $@`), hr("local", "https://h", fb("p"))}, ResetScope: "local"},
			LocalMeta{Profile: "p"}, api.RemoteHTTPS, "", Classified{Source: api.BindingLocal, Profile: "p", AWSPath: "aws", NeedsRepair: true}},
		{"missing aws path needs repair", Effective{Helpers: []HelperRef{hr("local", "https://h", `!"/gone/aws" --profile 'p' codecommit credential-helper $@`), hr("local", "https://h", fb("p"))}, ResetScope: "local"},
			LocalMeta{Profile: "p"}, api.RemoteHTTPS, "", Classified{Source: api.BindingLocal, Profile: "p", AWSPath: "/gone/aws", NeedsRepair: true}},
		{"no fallback needs repair", Effective{Helpers: []HelperRef{hr("local", "https://h", aws("p"))}, ResetScope: "local"},
			LocalMeta{Profile: "p"}, api.RemoteHTTPS, "", Classified{Source: api.BindingLocal, Profile: "p", AWSPath: "/opt/homebrew/bin/aws", NeedsRepair: true}},
		{"no local reset needs repair", Effective{Helpers: []HelperRef{hr("global", "https://x", "!aws --profile q codecommit credential-helper $@"), hr("local", "https://h", aws("p")), hr("local", "https://h", fb("p"))}, ResetScope: "global"},
			LocalMeta{Profile: "p"}, api.RemoteHTTPS, "", Classified{Source: api.BindingGlobalURL, Profile: "q", URLRule: "https://x", AWSPath: "aws"}},
		{"fallback without aws helper", Effective{Helpers: []HelperRef{hr("local", "https://h", fb("p"))}, ResetScope: "local"},
			LocalMeta{Profile: "p"}, api.RemoteHTTPS, "", Classified{Source: api.BindingLocal, Profile: "p", NeedsRepair: true}},
	}
	for _, c := range cases {
		got := Classify(c.eff, c.meta, c.kind, c.grc, exists)
		if got != c.want {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, got, c.want)
		}
	}
	// A healthy local binding where the aws helper is also not preceded by an earlier global one.
	_ = tc{}
}

func TestLocalMetaFrom(t *testing.T) {
	m := LocalMetaFrom([]Entry{
		{Scope: "global", Key: "awsssohub.profile", Value: "ignored"},
		{Scope: "local", Key: "awsssohub.profile", Value: "acme"},
		{Scope: "local", Key: "awsssohub.accountid", Value: "111122223333"},
		{Scope: "local", Key: "awsssohub.version", Value: "1"},
	})
	if m != (LocalMeta{Profile: "acme", AccountID: "111122223333", Version: "1"}) {
		t.Errorf("%+v", m)
	}
}

func TestGuardPatterns(t *testing.T) {
	got := GuardPatterns([]string{"git-codecommit.us-east-1.amazonaws.com"})
	if strings.Join(got, ",") != "https://git-codecommit.*.amazonaws.com" {
		t.Errorf("plain: %v", got)
	}
	got = GuardPatterns([]string{
		"git-codecommit-fips.us-east-2.amazonaws.com", "git-codecommit.cn-north-1.amazonaws.com.cn",
		"vpce-0b7d-abc.git-codecommit.us-east-2.vpce.amazonaws.com", "https://git-codecommit.us-east-1.amazonaws.com",
	})
	want := "https://git-codecommit.*.amazonaws.com,https://git-codecommit-fips.*.amazonaws.com,https://git-codecommit.*.amazonaws.com.cn,https://*.git-codecommit.*.vpce.amazonaws.com"
	if strings.Join(got, ",") != want {
		t.Errorf("all: %v", got)
	}
}

func TestClassifyTestStderr(t *testing.T) {
	cases := []struct {
		exit     int
		stderr   string
		timedOut bool
		want     api.TestStatus
	}{
		{0, "", false, api.TestOK},
		{128, "", true, api.TestTimeout},
		{128, "aws: [ERROR]: Error loading SSO Token: Token for fake-portal does not exist\nfatal: credential helper '!f() {...}' told us to quit", false, api.TestSignedOut},
		{128, "aws: [ERROR]: Error when retrieving token from sso: Token has expired and refresh failed", false, api.TestSignedOut},
		{128, "aws: [ERROR]: The SSO session associated with this profile has expired or is otherwise invalid.", false, api.TestSignedOut},
		{128, "Unable to locate credentials", false, api.TestSignedOut},
		{128, "aws: [ERROR]: The config profile (x) could not be found", false, api.TestSignedOut},
		{128, "fatal: unable to access 'https://h/': The requested URL returned error: 403", false, api.TestAuth},
		{128, "fatal: unable to access 'https://h/': The requested URL returned error: 404", false, api.TestNotFound},
		{128, "fatal: repository 'https://h/v1/repos/x' not found", false, api.TestNotFound},
		{128, "fatal: Repository does not exist", false, api.TestNotFound},
		{128, "fatal: unable to access: Could not resolve host: git-codecommit.us-east-1.amazonaws.com", false, api.TestNetwork},
		{128, "Failed to connect to host port 443", false, api.TestNetwork},
		{128, "fatal: unable to access: SSL certificate problem", false, api.TestNetwork},
		{128, "fatal: unable to get password from user", false, api.TestError},
	}
	for _, c := range cases {
		if got := ClassifyTestStderr(c.exit, c.stderr, c.timedOut); got != c.want {
			t.Errorf("%q: got %s want %s", c.stderr, got, c.want)
		}
	}
}

func TestAWSHelperValueRequiresAbsolutePath(t *testing.T) {
	for _, tc := range []struct{ path, goos string }{
		{"aws", "darwin"}, {"bin/aws", "darwin"}, {"./aws", "darwin"}, {`bin\aws.exe`, "windows"}, {`aws.exe`, "windows"},
		{"/opt/homebrew/bin/aws\n", "darwin"}, {"/opt/homebrew/bin/aws\x00", "darwin"},
	} {
		_, err := AWSHelperValue(tc.path, "p", tc.goos)
		ae, ok := err.(*api.AppError)
		if !ok || ae.Code != api.ErrUnsafeValue {
			t.Errorf("%q on %s: %v", tc.path, tc.goos, err)
		}
	}
	if _, err := AWSHelperValue(`C:\Program Files\Amazon\AWSCLIV2\aws.exe`, "p", "windows"); err != nil {
		t.Errorf("absolute windows path: %v", err)
	}
}
