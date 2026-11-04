package ini

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../awsconfig/testdata/config.fixture")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustParse(t *testing.T, s string) *Doc {
	t.Helper()
	d, err := Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestRoundTripUnmodified(t *testing.T) {
	f := fixture(t)
	variants := map[string]string{
		"plain":     f,
		"crlf":      strings.ReplaceAll(f, "\n", "\r\n"),
		"bom":       "\xef\xbb\xbf" + f,
		"nofinalnl": strings.TrimSuffix(f, "\n"),
		"empty":     "",
		"nested":    "[default]\ns3 =\n  max_concurrent_requests = 20\n  # note\n  max_queue_size = 1\nregion = x\n",
	}
	for name, in := range variants {
		d := mustParse(t, in)
		if string(d.Bytes()) != in {
			t.Errorf("%s: round trip differs", name)
		}
	}
}

func TestParseErrors(t *testing.T) {
	_, err := Parse([]byte("key = v\n[a]\n"))
	if pe, ok := err.(*ParseError); !ok || pe.Line != 1 {
		t.Fatalf("got %v", err)
	}
	_, err = Parse([]byte("[a]\nx = 1\ngarbage\n"))
	if pe, ok := err.(*ParseError); !ok || pe.Line != 3 {
		t.Fatalf("got %v", err)
	}
	if _, err = Parse([]byte("[a\nx=1\n")); err == nil {
		t.Fatal("unterminated header must fail")
	}
}

func TestSetKeepsSpacing(t *testing.T) {
	d := mustParse(t, fixture(t))
	if !d.Set("sso-session initech", []KV{{"sso_region", "us-east-1"}}, InsertTop) {
		t.Fatal("missing section")
	}
	want := strings.Replace(fixture(t), "sso_region     = us-west-2", "sso_region     = us-east-1", 1)
	if string(d.Bytes()) != want {
		t.Fatalf("unexpected:\n%s", d.Bytes())
	}
	if v, _ := d.Get("sso-session initech", "SSO_REGION"); v != "us-east-1" {
		t.Fatal(v)
	}
	if d.Set("nope", nil, InsertTop) {
		t.Fatal("Set on missing section must return false")
	}
}

func TestInsertBatchAlignment(t *testing.T) {
	// aligned section: '=' column kept
	d := mustParse(t, "[profile a]\nsso_session    = s\nsso_role_name  = r\n")
	d.Set("profile a", []KV{{"region", "x"}, {"output", "json"}}, InsertBottom)
	want := "[profile a]\nsso_session    = s\nsso_role_name  = r\nregion         = x\noutput         = json\n"
	if string(d.Bytes()) != want {
		t.Fatalf("aligned:\n%q", d.Bytes())
	}
	// unaligned, 3 keys: padded to W
	d = mustParse(t, "[profile b]\nregion = x\n")
	d.Set("profile b", []KV{{"sso_session", "s"}, {"sso_account_id", "1"}, {"sso_role_name", "r"}}, InsertTop)
	want = "[profile b]\nsso_session    = s\nsso_account_id = 1\nsso_role_name  = r\nregion = x\n"
	if string(d.Bytes()) != want {
		t.Fatalf("unaligned:\n%q", d.Bytes())
	}
	// single key
	d = mustParse(t, "[profile c]\nregion = x\n")
	d.Set("profile c", []KV{{"output", "json"}}, InsertBottom)
	if string(d.Bytes()) != "[profile c]\nregion = x\noutput = json\n" {
		t.Fatalf("single:\n%q", d.Bytes())
	}
	// bottom insert keeps trailing comments after the batch
	d = mustParse(t, "[a]\nx = 1\n# tail\n\n[b]\n")
	d.Set("a", []KV{{"y", "2"}}, InsertBottom)
	if string(d.Bytes()) != "[a]\nx = 1\ny = 2\n# tail\n\n[b]\n" {
		t.Fatalf("bottom:\n%q", d.Bytes())
	}
}

func TestAddSectionBlankLine(t *testing.T) {
	d := mustParse(t, "[a]\nx = 1")
	d.AddSection("b", []KV{{"y", "2"}})
	if string(d.Bytes()) != "[a]\nx = 1\n\n[b]\ny = 2\n" {
		t.Fatalf("%q", d.Bytes())
	}
	d = mustParse(t, "[a]\nx = 1\n\n")
	d.AddSection("b", []KV{{"y", "2"}})
	if string(d.Bytes()) != "[a]\nx = 1\n\n[b]\ny = 2\n" {
		t.Fatalf("%q", d.Bytes())
	}
	d = mustParse(t, "")
	d.AddSection("b", []KV{{"y", "2"}})
	if string(d.Bytes()) != "[b]\ny = 2\n" {
		t.Fatalf("%q", d.Bytes())
	}
	d = mustParse(t, "[a]\r\nx = 1\r\n")
	d.AddSection("b", []KV{{"y", "2"}})
	if string(d.Bytes()) != "[a]\r\nx = 1\r\n\r\n[b]\r\ny = 2\r\n" {
		t.Fatalf("crlf %q", d.Bytes())
	}
}

func TestDeleteSectionCollapsesBlank(t *testing.T) {
	d := mustParse(t, "[a]\nx = 1\n\n[b]\ny = 2\n\n[c]\nz = 3\n")
	d.DeleteSection("b")
	if string(d.Bytes()) != "[a]\nx = 1\n\n[c]\nz = 3\n" {
		t.Fatalf("%q", d.Bytes())
	}
	d = mustParse(t, "[a]\nx = 1\n\n[b]\ny = 2\n")
	d.DeleteSection("a")
	if string(d.Bytes()) != "[b]\ny = 2\n" {
		t.Fatalf("%q", d.Bytes())
	}
	if d.DeleteSection("zzz") {
		t.Fatal("missing section")
	}
}

func TestCommentAboveSectionStays(t *testing.T) {
	d := mustParse(t, fixture(t))
	if !d.DeleteSection("profile globex") {
		t.Fatal("not deleted")
	}
	got := string(d.Bytes())
	want := strings.Replace(fixture(t), "[profile globex]\nregion = eu-west-1\n\n", "", 1)
	if got != want {
		t.Fatalf("got:\n%s", got)
	}
	if !strings.Contains(got, "# ===== 平台基础设施（2026-03-01 添加）=====\n# 平台基础设施：通过 IAM Identity Center 登录，下面两个配置文件共用同一个门户\n[sso-session initech]") {
		t.Fatal("comment block moved")
	}
}

func TestNestedContinuation(t *testing.T) {
	d := mustParse(t, "[default]\ns3 =\n  max_concurrent_requests = 20\nregion = x\n")
	kv := d.Keys("default")
	if len(kv) != 2 || kv[0].Key != "s3" || kv[0].Value != "" || kv[1].Value != "x" {
		t.Fatalf("%v", kv)
	}
	if n := d.DeleteKeys("default", "S3"); n != 1 {
		t.Fatal(n)
	}
	if string(d.Bytes()) != "[default]\nregion = x\n" {
		t.Fatalf("%q", d.Bytes())
	}
}

func TestRenameSection(t *testing.T) {
	d := mustParse(t, "# c\n[sso-session  old]  # trailing\nx = 1\n")
	if !d.RenameSection("sso-session old", "sso-session new") {
		t.Fatal("not renamed")
	}
	if string(d.Bytes()) != "# c\n[sso-session new]  # trailing\nx = 1\n" {
		t.Fatalf("%q", d.Bytes())
	}
	if !d.Has("sso-session new") || d.SectionLine("sso-session new") != 2 {
		t.Fatal("lookup after rename")
	}
}

func TestSectionsAndGet(t *testing.T) {
	d := mustParse(t, fixture(t))
	got := strings.Join(d.Sections(), "|")
	if got != "profile acme|profile globex|sso-session initech|profile initech-admin|profile initech-readonly" {
		t.Fatal(got)
	}
	if v, ok := d.Get("profile initech-admin", "sso_account_id"); !ok || v != "777788889999" {
		t.Fatal(v)
	}
}

var unsafeTexts = map[string]string{
	"CR":     "a\rb",
	"LF":     "a\nb",
	"CRLF":   "a\r\nb",
	"NUL":    "a\x00b",
	"NEL":    "a\u0085b",
	"LS":     "a b",
	"PS":     "a b",
	"tab":    "a\tb",
	"inject": "AdministratorAccess\ncredential_process = /usr/bin/id",
}

func TestUnsafeTextIsRefusedAndDocumentUnchanged(t *testing.T) {
	const base = "[profile acme]\nregion = us-east-1\n"
	for name, bad := range unsafeTexts {
		t.Run("value/"+name, func(t *testing.T) {
			d := mustParse(t, base)
			if d.Set("profile acme", []KV{{Key: "sso_role_name", Value: bad}}, InsertBottom) {
				t.Fatal("Set must report failure")
			}
			assertUnsafe(t, d, "sso_role_name")
			if string(d.Bytes()) != base {
				t.Fatalf("document changed:\n%s", d.Bytes())
			}
		})
		t.Run("set-existing-value/"+name, func(t *testing.T) {
			d := mustParse(t, base)
			d.Set("profile acme", []KV{{Key: "region", Value: bad}}, InsertTop)
			assertUnsafe(t, d, "region")
			if string(d.Bytes()) != base {
				t.Fatalf("document changed:\n%s", d.Bytes())
			}
		})
		t.Run("add-section-value/"+name, func(t *testing.T) {
			d := mustParse(t, base)
			d.AddSection("profile other", []KV{{Key: "region", Value: bad}})
			assertUnsafe(t, d, "region")
			if string(d.Bytes()) != base {
				t.Fatalf("document changed:\n%s", d.Bytes())
			}
		})
		t.Run("section-name/"+name, func(t *testing.T) {
			d := mustParse(t, base)
			d.AddSection("profile "+bad, []KV{{Key: "region", Value: "x"}})
			assertUnsafe(t, d, "section")
			if string(d.Bytes()) != base {
				t.Fatalf("document changed:\n%s", d.Bytes())
			}
		})
		t.Run("key/"+name, func(t *testing.T) {
			d := mustParse(t, base)
			d.AddSection("profile other", []KV{{Key: "k" + bad, Value: "x"}})
			assertUnsafe(t, d, "key")
			if string(d.Bytes()) != base {
				t.Fatalf("document changed:\n%s", d.Bytes())
			}
		})
		t.Run("rename/"+name, func(t *testing.T) {
			d := mustParse(t, base)
			if d.RenameSection("profile acme", "profile "+bad) {
				t.Fatal("rename must report failure")
			}
			assertUnsafe(t, d, "section")
			if string(d.Bytes()) != base {
				t.Fatalf("document changed:\n%s", d.Bytes())
			}
		})
	}
}

func TestStructuralCharactersAreRefused(t *testing.T) {
	const base = "[profile acme]\nregion = us-east-1\n"
	cases := []struct {
		name string
		edit func(d *Doc)
		want string
	}{
		{"section bracket", func(d *Doc) { d.AddSection("profile x]y", nil) }, "section"},
		{"section open bracket", func(d *Doc) { d.AddSection("profile x[y", nil) }, "section"},
		{"rename bracket", func(d *Doc) { d.RenameSection("profile acme", "x]\n[profile y") }, "section"},
		{"key equals", func(d *Doc) { d.AddSection("profile x", []KV{{Key: "a=b", Value: "1"}}) }, "key"},
		{"key comment", func(d *Doc) { d.AddSection("profile x", []KV{{Key: "#a", Value: "1"}}) }, "key"},
		{"key semicolon", func(d *Doc) { d.AddSection("profile x", []KV{{Key: ";a", Value: "1"}}) }, "key"},
		{"key empty", func(d *Doc) { d.AddSection("profile x", []KV{{Key: "", Value: "1"}}) }, "key"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := mustParse(t, base)
			c.edit(d)
			assertUnsafe(t, d, c.want)
			if string(d.Bytes()) != base {
				t.Fatalf("document changed:\n%s", d.Bytes())
			}
		})
	}
}

func TestSafeEditsStillWork(t *testing.T) {
	d := mustParse(t, "[profile acme]\nregion = us-east-1\n")
	d.AddSection("sso-session acme", []KV{{Key: "sso_start_url", Value: "https://acme.awsapps.com/start"}, {Key: "sso_registration_scopes", Value: "sso:account:access,x"}})
	d.Set("profile acme", []KV{{Key: "sso_role_name", Value: "acme.Admin+ops@x"}}, InsertBottom)
	if d.Err() != nil {
		t.Fatal(d.Err())
	}
	if !d.RenameSection("sso-session acme", "sso-session globex") || d.Err() != nil {
		t.Fatal("rename failed", d.Err())
	}
	if !strings.Contains(string(d.Bytes()), "[sso-session globex]") || !strings.Contains(string(d.Bytes()), "sso_role_name = acme.Admin+ops@x") {
		t.Fatalf("%s", d.Bytes())
	}
}

func TestUnsafeTextClassification(t *testing.T) {
	for _, s := range []string{"a\x00", "\r", "\n", "\x1b[0m", "\x7f", "\u0085", " ", " "} {
		if !UnsafeText(s) {
			t.Errorf("%q must be unsafe", s)
		}
	}
	for _, s := range []string{"", "AdministratorAccess", "acme.Admin+ops@x", "https://acme.awsapps.com/start", "名称", "a b"} {
		if UnsafeText(s) {
			t.Errorf("%q must be safe", s)
		}
	}
}

func assertUnsafe(t *testing.T, d *Doc, field string) {
	t.Helper()
	var ue *UnsafeError
	if !errors.As(d.Err(), &ue) {
		t.Fatalf("want UnsafeError, got %v", d.Err())
	}
	if ue.Field != field {
		t.Fatalf("field %q, want %q", ue.Field, field)
	}
	if strings.Contains(d.Err().Error(), "credential_process") {
		t.Fatal("the error must not echo the value")
	}
}
