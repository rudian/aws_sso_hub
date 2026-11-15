package sso

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"html"
	"math/big"
	"net/url"
	"strings"
)

const (
	defaultScope     = "sso:account:access"
	redirectPath     = "/oauth/callback"
	redirectNoPort   = "http://127.0.0.1/oauth/callback" // registered (port-less); used with the port
	grantAuthCode    = "authorization_code"
	grantRefresh     = "refresh_token"
	grantDeviceCode  = "urn:ietf:params:oauth:grant-type:device_code"
	clientTypePublic = "public"
)

// pctEncode is botocore's percent_encode: urllib.parse.quote(s, safe='-._~').
func pctEncode(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || strings.IndexByte("-._~", c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// NewPKCE returns a 64 char verifier from [A-Za-z0-9-._~] and its S256 challenge (base64url, no padding).
func NewPKCE() (verifier, challenge string, err error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"
	buf := make([]byte, 64)
	for i := range buf {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", "", err
		}
		buf[i] = alphabet[n.Int64()]
	}
	sum := sha256.Sum256(buf)
	return string(buf), base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// NewState returns a UUID v4 string (the CLI uses str(uuid.uuid4())).
func NewState() string {
	var u [16]byte
	_, _ = rand.Read(u[:])
	u[6] = (u[6] & 0x0f) | 0x40
	u[8] = (u[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

// AuthorizeURL reproduces botocore's _get_authorization_uri byte for byte (parameter order, "scopes", raw challenge).
func AuthorizeURL(base, clientID, redirectWithPort, state, challenge string, scopes []string) string {
	sc := defaultScope
	if len(scopes) > 0 {
		sc = strings.Join(scopes, " ")
	}
	return base + "/authorize?" +
		"response_type=code" +
		"&client_id=" + pctEncode(clientID) +
		"&redirect_uri=" + pctEncode(redirectWithPort) +
		"&state=" + pctEncode(state) +
		"&code_challenge_method=S256" +
		"&scopes=" + pctEncode(sc) +
		"&code_challenge=" + challenge
}

// ShortcutURL builds the access portal shortcut link (no secrets): <start>/#/console?account_id=..&role_name=..[&destination=..].
func ShortcutURL(startURL, accountID, role, destination string) string {
	u := strings.TrimRight(startURL, "/") + "/#/console?account_id=" + url.QueryEscape(accountID)
	if role != "" {
		u += "&role_name=" + url.QueryEscape(role)
	}
	if destination != "" {
		u += "&destination=" + url.QueryEscape(destination)
	}
	return u
}

// CallbackPage renders the self-contained browser page shown after the redirect (no JS, no external URLs).
func CallbackPage(lang string, ok bool, errorValue string) []byte {
	zh := lang == "zh-CN"
	title, heading, body, htmlLang := "Codepass", "", "", "en"
	switch {
	case zh && ok:
		title, htmlLang = "Codepass", "zh-CN"
		heading, body = "登录成功", "Codepass 已经收到授权，可以关闭这个页面，回到应用继续。"
	case zh:
		title, htmlLang = "Codepass", "zh-CN"
		heading, body = "登录没有完成", "浏览器返回了："+html.EscapeString(errorValue)+"。回到 Codepass 重试即可。"
	case ok:
		heading, body = "You're signed in", "Codepass received the authorization. You can close this tab and go back to the app."
	default:
		heading, body = "Sign-in didn't finish", "The browser returned: "+html.EscapeString(errorValue)+". Go back to Codepass and try again."
	}
	colour := "#1a7f4b"
	if !ok {
		colour = "#b42318"
	}
	page := `<!DOCTYPE html>
<html lang="` + htmlLang + `">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>` + html.EscapeString(title) + `</title>
<style>
:root { color-scheme: light dark; }
body { margin: 0; min-height: 100vh; display: flex; align-items: center; justify-content: center;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
  background: Canvas; color: CanvasText; }
main { max-width: 28rem; padding: 2rem; text-align: center; }
h1 { font-size: 1.5rem; margin: 0 0 .75rem; color: ` + colour + `; }
p { margin: 0; line-height: 1.6; opacity: .85; }
</style>
</head>
<body>
<main>
<h1>` + heading + `</h1>
<p>` + body + `</p>
</main>
</body>
</html>
`
	return []byte(page)
}
