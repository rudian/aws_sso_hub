package gitx

import (
	"regexp"
	"strings"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/execx"
)

var (
	reAwsHelper = regexp.MustCompile(`(?:^|[\s"'/\\])aws(?:\.exe|\.cmd)?(?:\\?["'])?\s.*?\bcodecommit\s+credential-helper\b`)
	reProfile   = regexp.MustCompile(`--profile(?:=|\s+)(?:'([^']*)'|"([^"]*)"|\\"([^"\\]*)\\"|([^\s'"]+))`)
	reDriveAbs  = regexp.MustCompile(`^[A-Za-z]:[/\\]`)
)

// AWSHelper is what ParseAWSHelper learned about one credential.helper value.
type AWSHelper struct {
	IsAWS      bool
	Profile    string
	HasProfile bool
	AWSPath    string // the executable as written ("aws" when not a path)
	Absolute   bool
}

// ParseAWSHelper recognises `aws ... codecommit credential-helper` helpers in any of the spellings in
// Matches (profile before or after, quoted or not, absolute path or bare `aws`).
func ParseAWSHelper(value string) AWSHelper {
	v := strings.TrimSpace(value)
	v = strings.TrimPrefix(v, "!")
	v = strings.TrimSpace(v)
	if !reAwsHelper.MatchString(v) {
		return AWSHelper{}
	}
	h := AWSHelper{IsAWS: true}
	if m := reProfile.FindStringSubmatch(v); m != nil {
		h.HasProfile = true
		for _, g := range m[1:] {
			if g != "" {
				h.Profile = g
				break
			}
		}
	}
	h.AWSPath = helperExecutable(v)
	h.Absolute = strings.HasPrefix(h.AWSPath, "/") || reDriveAbs.MatchString(h.AWSPath)
	return h
}

// helperExecutable extracts the first word of a helper command, honouring quotes and \" escapes.
func helperExecutable(v string) string {
	switch {
	case strings.HasPrefix(v, `\"`):
		if i := strings.Index(v[2:], `\"`); i >= 0 {
			return v[2 : 2+i]
		}
	case strings.HasPrefix(v, `"`), strings.HasPrefix(v, `'`):
		q := v[:1]
		if i := strings.Index(v[1:], q); i >= 0 {
			return v[1 : 1+i]
		}
	}
	if i := strings.IndexAny(v, " \t"); i >= 0 {
		return v[:i]
	}
	return v
}

const (
	fallbackPrefix = `!f() { test "$1" = get || return 0; echo "AWS SSO Hub`
	fallbackSuffix = `echo quit=1; }; f`
	loginHint      = `aws sso login --profile `
)

// IsFallback reports whether value is the managed "cannot get credentials" fallback helper.
func IsFallback(value string) bool {
	return strings.HasPrefix(value, fallbackPrefix) && strings.HasSuffix(value, fallbackSuffix) && strings.Contains(value, loginHint)
}

// IsGuard reports whether value is the managed global "not bound" guard helper.
func IsGuard(value string) bool {
	return strings.HasPrefix(value, fallbackPrefix) && strings.HasSuffix(value, fallbackSuffix) && !strings.Contains(value, loginHint)
}

// LocalMeta is the [awsssohub] marker section of a repository.
type LocalMeta struct{ Profile, AccountID, Version string }

// LocalMetaFrom reads awsssohub.* from local-scope entries.
func LocalMetaFrom(entries []Entry) LocalMeta {
	var m LocalMeta
	for _, e := range entries {
		if e.Scope != "" && e.Scope != "local" && e.Scope != "worktree" {
			continue
		}
		switch e.Key {
		case "awsssohub.profile":
			m.Profile = e.Value
		case "awsssohub.accountid":
			m.AccountID = e.Value
		case "awsssohub.version":
			m.Version = e.Value
		}
	}
	return m
}

// Classified is the binding of a repository derived from the resolved helpers.
type Classified struct {
	Source                                  api.BindingSource
	Profile, URLRule, AWSPath               string
	NoProfileFlag, LegacyPlain, NeedsRepair bool
}

func isLocalScope(s string) bool { return s == "local" || s == "worktree" || s == "command" }

// Classify turns the effective helper list into a binding description.
func Classify(eff Effective, meta LocalMeta, kind api.RemoteKind, grcProfile string, fileExists func(string) bool) Classified {
	switch kind {
	case api.RemoteGRC:
		p := grcProfile
		if p == "" {
			p = "default"
		}
		return Classified{Source: api.BindingRemoteURL, Profile: p}
	case api.RemoteSSH:
		return Classified{Source: api.BindingNone}
	}
	first := -1
	var firstAWS AWSHelper
	for i, h := range eff.Helpers {
		if a := ParseAWSHelper(h.Value); a.IsAWS {
			first, firstAWS = i, a
			break
		}
	}
	if first < 0 {
		if n := len(eff.Helpers); n > 0 && IsGuard(eff.Helpers[n-1].Value) {
			return Classified{Source: api.BindingGuard}
		}
		for _, h := range eff.Helpers {
			if isLocalScope(h.Scope) && IsFallback(h.Value) {
				return Classified{Source: api.BindingLocal, Profile: meta.Profile, NeedsRepair: true}
			}
		}
		return Classified{Source: api.BindingNone}
	}
	ref := eff.Helpers[first]
	c := Classified{AWSPath: firstAWS.AWSPath, NoProfileFlag: !firstAWS.HasProfile, Profile: firstAWS.Profile}
	if isLocalScope(ref.Scope) {
		c.Source = api.BindingLocal
		c.LegacyPlain = ref.Subsection == ""
		if !firstAWS.HasProfile {
			c.Profile = meta.Profile
		}
		if !c.LegacyPlain {
			last := eff.Helpers[len(eff.Helpers)-1].Value
			exists := fileExists != nil && fileExists(firstAWS.AWSPath)
			c.NeedsRepair = !firstAWS.Absolute || !exists || !IsFallback(last) || eff.ResetScope != "local"
		}
		return c
	}
	if ref.Subsection != "" {
		c.Source, c.URLRule = api.BindingGlobalURL, ref.Subsection
	} else {
		c.Source = api.BindingGlobalDefault
	}
	return c
}

// UnsafeValue reports whether s contains characters that cannot be embedded safely in a helper value.
func unsafeInHelper(s string) bool { return strings.ContainsAny(s, "\"`$\\") }

func unsafeErr(value string) *api.AppError {
	return api.NewError(api.ErrUnsafeValue, "value cannot be written to git config safely").WithParam("value", value)
}

// AWSHelperValue builds `!"<aws>" --profile '<P>' codecommit credential-helper $@`.
func AWSHelperValue(awsPath, profile, goos string) (string, error) {
	if !ProfileNameRE.MatchString(profile) {
		return "", unsafeErr(profile)
	}
	// A relative path would resolve against whatever directory git runs in.
	if !execx.IsAbsPath(awsPath, goos) {
		return "", unsafeErr(awsPath)
	}
	p := execx.HelperPath(awsPath, goos)
	if p == "" || unsafeInHelper(p) || execx.HasControlChars(p) {
		return "", unsafeErr(awsPath)
	}
	return `!"` + p + `" --profile '` + profile + `' codecommit credential-helper $@`, nil
}

// FallbackValue returns the helper that prints a friendly message and quits instead of prompting.
func FallbackValue(profile, lang, goos string) string {
	if goos == "darwin" && lang == string(api.LanguageZhCN) {
		return fallbackPrefix + `: 无法获取 AWS 配置文件 '` + profile + `' 的 CodeCommit 凭证（未登录、登录已过期或缺少 AWS CLI）。请在 AWS SSO Hub 中点击该配置文件的“SSO 登录”，或运行: ` + loginHint + profile + `" >&2; ` + fallbackSuffix
	}
	return fallbackPrefix + `: cannot get CodeCommit credentials for AWS profile '` + profile + `' (not logged in, session expired, or AWS CLI missing). Click 'SSO sign-in' for this profile in AWS SSO Hub, or run: ` + loginHint + profile + `" >&2; ` + fallbackSuffix
}

// GuardValue returns the global helper that blocks unbound CodeCommit repositories with a message.
func GuardValue(lang, goos string) string {
	if goos == "darwin" && lang == string(api.LanguageZhCN) {
		return fallbackPrefix + `: 这个仓库还没有绑定 AWS 账号。打开 AWS SSO Hub > 仓库，绑定后再试。" >&2; ` + fallbackSuffix
	}
	return fallbackPrefix + `: this repository is not bound to an AWS account (no CodeCommit credential helper). Open AWS SSO Hub > Repositories and bind it." >&2; ` + fallbackSuffix
}
