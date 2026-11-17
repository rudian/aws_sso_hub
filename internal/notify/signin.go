package notify

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/rudian/codepass/internal/api"
)

// The two sign-in kinds (plan 4.15, design 6). Both have one button, "Sign in again".
const (
	KindSignInHeadsUp Kind = "signin.headsup"
	KindSignInEnded   Kind = "signin.ended"
)

// Record kinds in notifications.json and in the engine's notice log.
const (
	RecordHeadsUp = "headsup"
	RecordEnded   = "ended"
)

// The keys of a notice's data map: the kind, the portal name and the sign-in time, nothing else.
const (
	DataKind   = "kind"
	DataPortal = "portal"
	DataTime   = "time"
)

// SignInID is the notification id of a portal. The heads-up and the "ended" notice share it, so on macOS the
// second replaces the first.
func SignInID(portal string) string { return "signin:" + portal }

// SignInButton is the text of the one button.
func SignInButton(lang string) string {
	if lang == "zh-CN" {
		return "重新登录"
	}
	return "Sign in again"
}

// Account is one account of a portal as the app knows it. Only Name may reach a notification; AccountID and
// Role are here so the builder can make sure they never do.
type Account struct {
	Name      string
	AccountID string
	Role      string
}

// SignInInfo is everything the sign-in texts are built from. Fields the texts must never show (StartURL, an
// account's ID and role) are part of it on purpose: the builder is tested with secrets in every one of them.
type SignInInfo struct {
	Portal      string // the portal's display name (the sso-session name)
	StartURL    string
	User        string    // the Identity Center user name, "" when the app does not know it
	Accounts    []Account // in the order of the Accounts page
	LeadMinutes int       // 15, 30 or 60 (heads-up only)
	SignInTime  string    // RFC 3339 time of the sign-in this notice is about, "" when unknown
}

// SignInHeadsUp builds title and body of the heads-up.
func SignInHeadsUp(lang string, in SignInInfo) (title, body string) {
	zh := lang == "zh-CN"
	name := in.portalName()
	if zh {
		title = join("登录快过期了", "：", name)
	} else {
		title = join("Sign-in expires soon", ": ", name)
	}
	var second string
	switch {
	case zh && in.LeadMinutes >= 60:
		second = "大约 1 小时后过期。"
	case zh:
		second = fmt.Sprintf("大约 %d 分钟后过期。", in.LeadMinutes)
	case in.LeadMinutes >= 60:
		second = "Expires in about 1 hour."
	default:
		second = fmt.Sprintf("Expires in about %d minutes.", in.LeadMinutes)
	}
	return title, in.names(lang) + second
}

// SignInEnded builds title and body of the "ended" notice.
func SignInEnded(lang string, in SignInInfo) (title, body string) {
	zh := lang == "zh-CN"
	name := in.portalName()
	if zh {
		title = join("登录已过期", "：", name)
		return title, in.names(lang) + "重新登录后才能继续用。"
	}
	title = join("Sign-in expired", ": ", name)
	return title, in.names(lang) + "Sign in again to continue."
}

// SignInData is the notice's data map: the kind, the portal name and the sign-in time. The portal name is
// the routing key of a click; it is the only free text in the map.
func SignInData(kind Kind, in SignInInfo) map[string]string {
	return map[string]string{DataKind: string(kind), DataPortal: in.Portal, DataTime: in.SignInTime}
}

// Tone and stickiness of the toast: a warning that goes away, and an error that stays until dismissed.
func SignInHeadsUpNotice(lang string, in SignInInfo) Notice {
	t, b := SignInHeadsUp(lang, in)
	return Notice{Kind: KindSignInHeadsUp, ID: SignInID(in.Portal), Title: t, Body: b, Data: SignInData(KindSignInHeadsUp, in), Silent: true, Tone: api.NotifyToneWarn}
}

// SignInEndedNotice builds the "ended" notice: default sound, a toast that stays.
func SignInEndedNotice(lang string, in SignInInfo) Notice {
	t, b := SignInEnded(lang, in)
	return Notice{Kind: KindSignInEnded, ID: SignInID(in.Portal), Title: t, Body: b, Data: SignInData(KindSignInEnded, in), Tone: api.NotifyToneDanger, Sticky: true}
}

func join(head, sep, tail string) string {
	if tail == "" {
		return head
	}
	return head + sep + tail
}

func (in SignInInfo) portalName() string { return in.clean(in.Portal) }

// names is "alex · acme-prod and 2 more. " (with its trailing separator), or "" when no account name is safe.
func (in SignInInfo) names(lang string) string {
	zh := lang == "zh-CN"
	var names []string
	seen := map[string]bool{}
	for _, a := range in.Accounts {
		n := in.cleanAccount(a)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		names = append(names, n)
	}
	if len(names) == 0 {
		return ""
	}
	var accts string
	switch {
	case len(names) == 1:
		accts = names[0]
	case len(names) == 2 && zh:
		accts = names[0] + "、" + names[1]
	case len(names) == 2:
		accts = names[0] + " and " + names[1]
	case zh:
		accts = fmt.Sprintf("%s 等 %d 个账号", names[0], len(names))
	default:
		accts = fmt.Sprintf("%s and %d more", names[0], len(names)-1)
	}
	if user := in.clean(in.User); user != "" {
		accts = user + " · " + accts
	}
	if zh {
		return accts + "。"
	}
	return accts + ". "
}

// secrets are the values of this sign-in that no text may contain.
func (in SignInInfo) secrets(a *Account) []string {
	out := []string{in.StartURL, startURLHost(in.StartURL)}
	for _, x := range in.Accounts {
		out = append(out, x.AccountID, x.Role)
	}
	if a != nil {
		out = append(out, a.AccountID, a.Role)
	}
	return out
}

func (in SignInInfo) cleanAccount(a Account) string { return cleanText(a.Name, in.secrets(&a), false) }

// clean returns s when it may appear in a notification, else "".
func (in SignInInfo) clean(s string) string { return cleanText(s, in.secrets(nil), true) }

func startURLHost(raw string) string {
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
}

var (
	reDigits12 = regexp.MustCompile(`\d{12}`)
	// A token: a run longer than any legal name (an account name has up to 50 characters, a session name up to
	// 64), or the start of a JWT. A name of 32 characters is a name.
	reTokenish = regexp.MustCompile(`[A-Za-z0-9_+=/.\-]{65,}|\beyJ[A-Za-z0-9_\-]{8,}`)
	reURLish   = regexp.MustCompile(`(?i)(://|www\.|\.awsapps\.|\.amazonaws\.|\.app\.aws|\.amazonwebservices\.)`)
	rePathish  = regexp.MustCompile(`(^[~/\\])|([A-Za-z]:\\)|(\\\\)|(/Users/)|(/home/)`)
)

// cleanText drops a text that looks like something a notification must never show: a URL or start-URL host, an
// account ID, a role name, a token, a file path; and, unless allowEmail, an e-mail address (the Identity Center
// user name may be one). A dropped text is replaced by "": the notification then says less, never more.
func cleanText(s string, secrets []string, allowEmail bool) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	low := strings.ToLower(s)
	for _, sec := range secrets {
		sec = strings.TrimSpace(sec)
		if sec == "" || len(sec) < 3 {
			continue
		}
		ls := strings.ToLower(sec)
		// a role name is a name of its own: the same word inside an account name is allowed only when it is
		// not the whole text; every other secret may not appear anywhere in the text
		if low == ls || strings.Contains(low, ls) && len(ls) >= 8 {
			return ""
		}
	}
	if reDigits12.MatchString(s) || reTokenish.MatchString(s) || reURLish.MatchString(s) || rePathish.MatchString(s) {
		return ""
	}
	if !allowEmail && strings.Contains(s, "@") {
		return ""
	}
	if strings.ContainsAny(s, "\n\r\t") || strings.Contains(low, "awsreservedsso_") {
		return ""
	}
	return s
}
