package version_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/paths"
	"github.com/rudian/codepass/internal/version"
)

// TestPinnedLegacyIdentity pins every identifier that predates the rename to Codepass and must survive it:
// the BundleID (updater code requirement and single-instance ID), the data folder name (existing settings and
// backups), and the strings the app wrote into users' git config (the awsssohub section and the managed
// helper text, which IsFallback and IsGuard recognise by prefix). Changing any of them silently orphans data
// written by 0.0.1, so a change here needs a migration, not an edit of this test.
func TestPinnedLegacyIdentity(t *testing.T) {
	if version.BundleID != "com.rudian.awsssohub" {
		t.Errorf("BundleID = %q", version.BundleID)
	}
	for _, f := range []string{"../../build/darwin/Info.plist", "../../build/darwin/Info.dev.plist"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		re := regexp.MustCompile(`<key>CFBundleIdentifier</key>\s*<string>` + regexp.QuoteMeta(version.BundleID) + `</string>`)
		if !re.Match(b) {
			t.Errorf("%s: CFBundleIdentifier is not %q", f, version.BundleID)
		}
	}
	for f, want := range map[string]string{
		"../../build/config.yml":                 `productIdentifier: "` + version.BundleID + `"`,
		"../../build/windows/wails.exe.manifest": `name="` + version.BundleID + `"`,
	} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), want) {
			t.Errorf("%s no longer contains %s", f, want)
		}
	}
	if paths.AppDirName != "AWS SSO Hub" {
		t.Errorf("paths.AppDirName = %q", paths.AppDirName)
	}
	if version.ProductName != "Codepass" {
		t.Errorf("ProductName = %q", version.ProductName)
	}
	const prefix = `!f() { test "$1" = get || return 0; echo "AWS SSO Hub`
	for _, lang := range []string{"en", "zh-CN"} {
		for _, goos := range []string{"darwin", "windows"} {
			fb, gd := gitx.FallbackValue("acme", lang, goos), gitx.GuardValue(lang, goos)
			if !strings.HasPrefix(fb, prefix) || !strings.HasSuffix(fb, `echo quit=1; }; f`) || !gitx.IsFallback(fb) {
				t.Errorf("fallback helper (%s/%s) no longer carries the managed marker: %q", lang, goos, fb)
			}
			if !strings.HasPrefix(gd, prefix) || !gitx.IsGuard(gd) {
				t.Errorf("guard helper (%s/%s) no longer carries the managed marker: %q", lang, goos, gd)
			}
		}
	}
	// Helpers written by 0.0.1 (English text) must still be recognised.
	old := `!f() { test "$1" = get || return 0; echo "AWS SSO Hub: cannot get CodeCommit credentials for AWS profile 'acme' (not logged in, session expired, or AWS CLI missing). Click 'SSO sign-in' for this profile in AWS SSO Hub, or run: aws sso login --profile acme" >&2; echo quit=1; }; f`
	if !gitx.IsFallback(old) {
		t.Errorf("a 0.0.1 fallback helper is no longer recognised")
	}
	meta := gitx.LocalMetaFrom([]gitx.Entry{
		{Scope: "local", Key: "awsssohub.profile", Value: "acme"},
		{Scope: "local", Key: "awsssohub.accountid", Value: "111122223333"},
		{Scope: "local", Key: "awsssohub.version", Value: "1"},
	})
	if meta.Profile != "acme" || meta.AccountID != "111122223333" || meta.Version != "1" {
		t.Errorf("awsssohub.* marker keys are no longer read: %+v", meta)
	}
}
