package sso

import (
	"crypto/rand"
	"encoding/hex"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// SignInIntro is the app's own page that a sign-in the app started by itself shows first, in the same browser
// window, before AWS's sign-in page. AWS's pages do not say which portal a sign-in is for; this page names the
// portal and the profiles the sign-in unlocks, and its button continues to AWS.
type SignInIntro struct {
	Portal   string
	Profiles []IntroProfile
	EndsAt   time.Time // when the current session ends
}

// IntroProfile is one profile on the intro page.
type IntroProfile struct {
	Name     string
	Monogram string
	Slot     int // account colour 1..8; anything else shows no colour
}

// LoginOptions tune an interactive sign-in. The zero value is the sign-in the SSO sign-in button starts.
type LoginOptions struct {
	// Portal names the portal on the browser pages ("" = the session name).
	Portal string
	// Intro, when set, makes a browser sign-in open the intro page first (only when Config.OpenIntro is set).
	Intro *SignInIntro
	// Background marks a sign-in the app started by itself: Status keeps reporting the current sign-in's
	// state, with Waiting set, instead of signing-in.
	Background bool
	// Wait is how long a browser sign-in waits for AWS to send the user back (0 = Config.CallbackTimeout).
	Wait time.Duration
}

// IntroPathPrefix starts the path of every intro page: IntroPathPrefix + 32 lower-case hex digits.
const IntroPathPrefix = "/start/"

// newIntroPath returns an unguessable intro page path. It is not the OAuth state, which travels to AWS.
func newIntroPath() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return IntroPathPrefix + hex.EncodeToString(b[:]), nil
}

// setPageHeaders sets the headers of the pages the sign-in's loopback listener serves: never cached, never
// framed, no script, no referrer for the link to AWS.
func setPageHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	h.Set("X-Frame-Options", "DENY")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
}

// acctColors mirrors --acct-1 … --acct-8 of frontend/src/styles/tokens.css: the light colour, then the dark one.
var acctColors = [8][2]string{
	{"#3459D6", "#7E9BFF"}, // Cobalt
	{"#0F8565", "#3CC79E"}, // Jade
	{"#6E4FD8", "#A792FF"}, // Iris
	{"#B83C8C", "#F07CC4"}, // Orchid
	{"#0A7E99", "#3EC3E0"}, // Lagoon
	{"#A65A24", "#E59A5E"}, // Copper
	{"#6A7A12", "#B5C64A"}, // Olive
	{"#C23A4A", "#F27884"}, // Ruby
}

// introEnd formats the session end as a local time, with the date when it is not today.
func introEnd(end, now time.Time) string {
	end, now = end.Local(), now.Local()
	if end.Year() != now.Year() || end.YearDay() != now.YearDay() {
		return end.Format("2006-01-02 15:04")
	}
	return end.Format("15:04")
}

// IntroPage renders the intro page (self-contained: no script, no external URL). authURL is the AWS authorize
// URL the button continues to.
func IntroPage(lang string, in SignInIntro, authURL string, now time.Time) []byte {
	zh := lang == "zh-CN"
	portal := html.EscapeString(in.Portal)
	end := html.EscapeString(introEnd(in.EndsAt, now))
	htmlLang, heading, lead, button, foot := "en", "Sign in again: "+portal, "", "Continue to AWS sign-in",
		"AWS SSO Hub opened this page because the session ends soon. To skip it, close this window."
	switch {
	case zh && len(in.Profiles) > 0:
		lead = "这次登录 " + end + " 结束。重新登录会为下面这些配置文件开始新的会话："
	case zh:
		lead = "这次登录 " + end + " 结束。重新登录会开始新的会话。"
	case len(in.Profiles) > 0:
		lead = "This sign-in ends at " + end + ". Signing in again starts a new session for these profiles:"
	default:
		lead = "This sign-in ends at " + end + ". Signing in again starts a new session."
	}
	if zh {
		htmlLang, heading, button = "zh-CN", "重新登录："+portal, "继续到 AWS 登录"
		foot = "AWS SSO Hub 打开这个页面，是因为会话快结束了。不想登录的话，关掉这个窗口就行。"
	}

	var css, light, dark strings.Builder
	for i, c := range acctColors {
		n := strconv.Itoa(i + 1)
		light.WriteString(".s" + n + "{background:" + c[0] + "}")
		dark.WriteString(".s" + n + "{background:" + c[1] + "}")
	}
	css.WriteString(light.String())
	css.WriteString("@media (prefers-color-scheme: dark){.tile{color:#10131A}" + dark.String() + "}")

	var list strings.Builder
	if len(in.Profiles) > 0 {
		list.WriteString("<ul>")
		for _, p := range in.Profiles {
			cls := "tile"
			if p.Slot >= 1 && p.Slot <= len(acctColors) {
				cls += " s" + strconv.Itoa(p.Slot)
			}
			list.WriteString(`<li><span class="` + cls + `">` + html.EscapeString(p.Monogram) + `</span>` + html.EscapeString(p.Name) + `</li>`)
		}
		list.WriteString("</ul>")
	}

	page := `<!DOCTYPE html>
<html lang="` + htmlLang + `">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>` + heading + ` · AWS SSO Hub</title>
<style>
:root { color-scheme: light dark; }
body { margin: 0; min-height: 100vh; display: flex; align-items: center; justify-content: center;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
  background: Canvas; color: CanvasText; }
main { max-width: 32rem; padding: 2rem; }
.app { margin: 0 0 .5rem; font-size: .875rem; opacity: .7; }
h1 { font-size: 1.75rem; margin: 0 0 1rem; overflow-wrap: anywhere; }
p { margin: 0 0 1rem; line-height: 1.6; }
ul { list-style: none; margin: 0 0 1.5rem; padding: 0; }
li { display: flex; align-items: center; gap: .625rem; margin: 0 0 .5rem; overflow-wrap: anywhere; }
.tile { flex: none; display: inline-flex; align-items: center; justify-content: center; min-width: 2rem; height: 2rem;
  padding: 0 .25rem; box-sizing: border-box; border-radius: .5rem; font-size: .75rem; font-weight: 600;
  color: #FFFFFF; background: GrayText; }
.btn { display: inline-block; padding: .75rem 1.25rem; border-radius: .5rem; background: #3459D6; color: #FFFFFF;
  font-weight: 600; text-decoration: none; }
.btn:focus-visible { outline: 3px solid #7E9BFF; outline-offset: 2px; }
.foot { margin: 1.5rem 0 0; font-size: .8125rem; opacity: .7; }
` + css.String() + `
</style>
</head>
<body>
<main>
<p class="app">AWS SSO Hub</p>
<h1>` + heading + `</h1>
<p>` + lead + `</p>
` + list.String() + `
<a class="btn" href="` + html.EscapeString(authURL) + `">` + button + ` →</a>
<p class="foot">` + foot + `</p>
</main>
</body>
</html>
`
	return []byte(page)
}
