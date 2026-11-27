// Package ssotest provides fake IAM Identity Center servers (OIDC and portal, REST-JSON) for tests of
// packages that build on internal/sso. It must not import internal/sso.
package ssotest

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sync"
	"testing"
	"time"
)

// TokenResp scripts one /token response. Status 200 = success.
type TokenResp struct {
	Status  int
	ErrType string // e.g. "AuthorizationPendingException"
}

// OIDC is a fake OIDC endpoint (client/register, device_authorization, token).
type OIDC struct {
	URL string

	mu        sync.Mutex
	regBodies []map[string]any
	tokBodies []map[string]any
	regCount  int
	queue     []TokenResp
	expiresIn int
	onToken   func()
}

func NewOIDC(t *testing.T) *OIDC {
	o := &OIDC{expiresIn: 3600}
	srv := httptest.NewServer(http.HandlerFunc(o.serve))
	t.Cleanup(srv.Close)
	o.URL = srv.URL
	return o
}

func (o *OIDC) QueueToken(rs ...TokenResp) {
	o.mu.Lock()
	o.queue = append(o.queue, rs...)
	o.mu.Unlock()
}
func (o *OIDC) SetExpiresIn(s int) { o.mu.Lock(); o.expiresIn = s; o.mu.Unlock() }
func (o *OIDC) OnToken(f func())   { o.mu.Lock(); o.onToken = f; o.mu.Unlock() }
func (o *OIDC) RegCount() int      { o.mu.Lock(); defer o.mu.Unlock(); return o.regCount }
func (o *OIDC) TokenCalls() []map[string]any {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]map[string]any(nil), o.tokBodies...)
}

func (o *OIDC) serve(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(b, &body)
	o.mu.Lock()
	defer o.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/client/register":
		o.regBodies = append(o.regBodies, body)
		o.regCount++
		fmt.Fprintf(w, `{"clientId":"CID-%d","clientSecret":"CS-%d","clientIdIssuedAt":1,"clientSecretExpiresAt":%d}`,
			o.regCount, o.regCount, time.Now().Add(80*24*time.Hour).Unix())
	case "/device_authorization":
		fmt.Fprint(w, `{"deviceCode":"DC","userCode":"ABCD-EFGH","verificationUri":"https://device.example/","verificationUriComplete":"https://device.example/?user_code=ABCD-EFGH","expiresIn":600,"interval":1}`)
	case "/token":
		o.tokBodies = append(o.tokBodies, body)
		if o.onToken != nil {
			o.onToken()
		}
		if len(o.queue) > 0 {
			e := o.queue[0]
			o.queue = o.queue[1:]
			if e.Status != 200 && e.Status != 0 {
				w.WriteHeader(e.Status)
				fmt.Fprintf(w, `{"__type":%q,"error":"x","error_description":"x"}`, e.ErrType)
				return
			}
		}
		fmt.Fprintf(w, `{"accessToken":"AT-%d","expiresIn":%d,"refreshToken":"RT-%d","tokenType":"Bearer"}`, len(o.tokBodies), o.expiresIn, len(o.tokBodies))
	default:
		w.WriteHeader(404)
	}
}

// Account is one entry of the fake account list.
type Account struct{ ID, Name, Email string }

// Portal is a fake SSO portal API.
type Portal struct {
	URL string

	mu           sync.Mutex
	pages        [][]Account
	roles        map[string][]string
	rolesFail    map[string]bool
	akid         string
	credCalls    int
	logoutCalls  int
	lastToken    string
	expireMillis int64
	unauthorized bool
}

func NewPortal(t *testing.T) *Portal {
	p := &Portal{roles: map[string][]string{}, rolesFail: map[string]bool{}, akid: "ASIAIOSFODNN7EXAMPLE", expireMillis: time.Now().Add(time.Hour).UnixMilli()}
	srv := httptest.NewServer(http.HandlerFunc(p.serve))
	t.Cleanup(srv.Close)
	p.URL = srv.URL
	return p
}

func (p *Portal) SetAccounts(pages ...[]Account) { p.mu.Lock(); p.pages = pages; p.mu.Unlock() }
func (p *Portal) SetRoles(account string, roles ...string) {
	p.mu.Lock()
	p.roles[account] = roles
	p.mu.Unlock()
}
func (p *Portal) FailRoles(account string) { p.mu.Lock(); p.rolesFail[account] = true; p.mu.Unlock() }
func (p *Portal) SetKeyID(akid string)     { p.mu.Lock(); p.akid = akid; p.mu.Unlock() }
func (p *Portal) SetExpiration(t time.Time) {
	p.mu.Lock()
	p.expireMillis = t.UnixMilli()
	p.mu.Unlock()
}
func (p *Portal) SetUnauthorized(b bool) { p.mu.Lock(); p.unauthorized = b; p.mu.Unlock() }
func (p *Portal) CredCalls() int         { p.mu.Lock(); defer p.mu.Unlock(); return p.credCalls }
func (p *Portal) LogoutCalls() int       { p.mu.Lock(); defer p.mu.Unlock(); return p.logoutCalls }
func (p *Portal) LastToken() string      { p.mu.Lock(); defer p.mu.Unlock(); return p.lastToken }

func (p *Portal) serve(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	p.lastToken = r.Header.Get("x-amz-sso_bearer_token")
	if p.unauthorized && r.URL.Path != "/logout" {
		w.WriteHeader(401)
		fmt.Fprint(w, `{"__type":"UnauthorizedException","message":"no"}`)
		return
	}
	switch r.URL.Path {
	case "/assignment/accounts":
		page := 0
		if nt := r.URL.Query().Get("next_token"); nt != "" {
			fmt.Sscanf(nt, "p%d", &page)
		}
		list := []map[string]string{}
		if page < len(p.pages) {
			for _, a := range p.pages[page] {
				list = append(list, map[string]string{"accountId": a.ID, "accountName": a.Name, "emailAddress": a.Email})
			}
		}
		out := map[string]any{"accountList": list}
		if page+1 < len(p.pages) {
			out["nextToken"] = fmt.Sprintf("p%d", page+1)
		}
		_ = json.NewEncoder(w).Encode(out)
	case "/assignment/roles":
		acct := r.URL.Query().Get("account_id")
		if p.rolesFail[acct] {
			w.WriteHeader(500)
			fmt.Fprint(w, `{"__type":"InternalFailure","message":"boom"}`)
			return
		}
		rl := []map[string]string{}
		for _, n := range p.roles[acct] {
			rl = append(rl, map[string]string{"accountId": acct, "roleName": n})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"roleList": rl})
	case "/federation/credentials":
		p.credCalls++
		_ = json.NewEncoder(w).Encode(map[string]any{"roleCredentials": map[string]any{
			"accessKeyId": p.akid, "secretAccessKey": "roleSecretNotReal", "sessionToken": "roleSessionTokNotReal", "expiration": p.expireMillis}})
	case "/logout":
		p.logoutCalls++
		fmt.Fprint(w, `{}`)
	default:
		w.WriteHeader(404)
	}
}

// Browser returns an OpenBrowser function that approves the authorization request (simulated user).
func Browser() func(string) error {
	return func(u string) error {
		pu, err := url.Parse(u)
		if err != nil {
			return err
		}
		q := pu.Query()
		go func() {
			time.Sleep(20 * time.Millisecond)
			resp, err := http.Get(q.Get("redirect_uri") + "?code=THECODE&state=" + url.QueryEscape(q.Get("state")))
			if err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}()
		return nil
	}
}

// introButtonRE finds the link of the intro page's button.
var introButtonRE = regexp.MustCompile(`class="btn" href="([^"]+)"`)

// IntroBrowser simulates the user's browser for a sign-in with an intro page: it loads the page, hands its HTML
// to seen (when not nil), then follows the page's button to the authorize URL, whose redirect it calls with a
// code, as Browser does.
func IntroBrowser(seen func(page string)) func(string) error {
	return func(u string) error {
		go func() {
			time.Sleep(20 * time.Millisecond)
			resp, err := http.Get(u)
			if err != nil {
				return
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if seen != nil {
				seen(string(b))
			}
			m := introButtonRE.FindSubmatch(b)
			if m == nil {
				return
			}
			au, err := url.Parse(html.UnescapeString(string(m[1])))
			if err != nil {
				return
			}
			q := au.Query()
			r2, err := http.Get(q.Get("redirect_uri") + "?code=THECODE&state=" + url.QueryEscape(q.Get("state")))
			if err == nil {
				io.Copy(io.Discard, r2.Body)
				r2.Body.Close()
			}
		}()
		return nil
	}
}
