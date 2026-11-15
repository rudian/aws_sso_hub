package sso

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
)

type fakeResp struct {
	status  int
	errType string
	body    string
}

// fakeOIDC is an httptest OIDC server (REST-JSON).
type fakeOIDC struct {
	srv *httptest.Server

	mu         sync.Mutex
	regBodies  []map[string]any
	tokBodies  []map[string]any
	devBodies  []map[string]any
	regCount   int
	tokQueue   []fakeResp
	regErrOnce *fakeResp
	expiresIn  int
	onToken    func()
}

func newFakeOIDC(t *testing.T) *fakeOIDC {
	f := &fakeOIDC{expiresIn: 3600}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func writeErr(w http.ResponseWriter, r fakeResp) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(r.status)
	fmt.Fprintf(w, `{"__type":%q,"error":"x","error_description":"x"}`, r.errType)
}

func (f *fakeOIDC) serve(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(b, &body)
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/client/register":
		f.regBodies = append(f.regBodies, body)
		f.regCount++
		if f.regErrOnce != nil {
			e := *f.regErrOnce
			f.regErrOnce = nil
			writeErr(w, e)
			return
		}
		fmt.Fprintf(w, `{"clientId":"CID-%d","clientSecret":"CS-%d","clientIdIssuedAt":1,"clientSecretExpiresAt":%d}`,
			f.regCount, f.regCount, time.Now().Add(80*24*time.Hour).Unix())
	case "/device_authorization":
		f.devBodies = append(f.devBodies, body)
		fmt.Fprint(w, `{"deviceCode":"DC","userCode":"ABCD-EFGH","verificationUri":"https://device.example/","verificationUriComplete":"https://device.example/?user_code=ABCD-EFGH","expiresIn":600,"interval":1}`)
	case "/token":
		f.tokBodies = append(f.tokBodies, body)
		if f.onToken != nil {
			f.onToken()
		}
		if len(f.tokQueue) > 0 {
			e := f.tokQueue[0]
			f.tokQueue = f.tokQueue[1:]
			if e.status != 200 {
				writeErr(w, e)
				return
			}
		}
		fmt.Fprintf(w, `{"accessToken":"AT-%d","expiresIn":%d,"refreshToken":"RT-%d","tokenType":"Bearer"}`, len(f.tokBodies), f.expiresIn, len(f.tokBodies))
	default:
		w.WriteHeader(404)
	}
}

func (f *fakeOIDC) queueToken(rs ...fakeResp) {
	f.mu.Lock()
	f.tokQueue = append(f.tokQueue, rs...)
	f.mu.Unlock()
}

func (f *fakeOIDC) tokenCalls() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.tokBodies...)
}

func pending() fakeResp  { return fakeResp{400, "AuthorizationPendingException", ""} }
func slowDown() fakeResp { return fakeResp{400, "SlowDownException", ""} }
func okResp() fakeResp   { return fakeResp{status: 200} }

// fakePortal is the REST-JSON SSO portal API.
type fakePortal struct {
	srv *httptest.Server

	mu           sync.Mutex
	accounts     [][]map[string]string // pages
	roles        map[string][]string
	credCalls    int
	logoutCalls  int
	lastToken    string
	expireMillis int64
	unauthorized bool
}

func newFakePortal(t *testing.T) *fakePortal {
	p := &fakePortal{roles: map[string][]string{}, expireMillis: time.Now().Add(time.Hour).UnixMilli()}
	p.srv = httptest.NewServer(http.HandlerFunc(p.serve))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *fakePortal) serve(w http.ResponseWriter, r *http.Request) {
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
		out := map[string]any{"accountList": p.accounts[page]}
		if page+1 < len(p.accounts) {
			out["nextToken"] = fmt.Sprintf("p%d", page+1)
		}
		_ = json.NewEncoder(w).Encode(out)
	case "/assignment/roles":
		var rl []map[string]string
		for _, n := range p.roles[r.URL.Query().Get("account_id")] {
			rl = append(rl, map[string]string{"accountId": r.URL.Query().Get("account_id"), "roleName": n})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"roleList": rl})
	case "/federation/credentials":
		p.credCalls++
		_ = json.NewEncoder(w).Encode(map[string]any{"roleCredentials": map[string]any{
			"accessKeyId": "ASIAIOSFODNN7EXAMPLE", "secretAccessKey": "roleSecret", "sessionToken": "roleSessionTok", "expiration": p.expireMillis}})
	case "/logout":
		p.logoutCalls++
		w.WriteHeader(200)
		fmt.Fprint(w, `{}`)
	default:
		w.WriteHeader(404)
	}
}

type env struct {
	home   string
	oidc   *fakeOIDC
	por    *fakePortal
	m      *Manager
	sess   Session
	opened []string
	mu     sync.Mutex
	prog   []api.LoginProgress
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{home: t.TempDir(), oidc: newFakeOIDC(t), por: newFakePortal(t)}
	e.sess = Session{Name: "initech", StartURL: "https://d-1234567890.awsapps.com/start", Region: "us-west-2", Scopes: []string{"sso:account:access"}, ScopesSet: true}
	e.m = NewManager(Config{
		Home: e.home, RegistrationsDir: filepath.Join(t.TempDir(), "registrations"),
		Endpoints: Endpoints{OIDC: e.oidc.srv.URL, Portal: e.por.srv.URL},
		Lang:      func() string { return "en" },
	})
	return e
}

func (e *env) progress(p api.LoginProgress) {
	e.mu.Lock()
	e.prog = append(e.prog, p)
	e.mu.Unlock()
}

func (e *env) phases() []api.LoginPhase {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []api.LoginPhase
	for _, p := range e.prog {
		out = append(out, p.Phase)
	}
	return out
}

func (e *env) writeToken(t *testing.T, tok *CachedToken) {
	t.Helper()
	if err := WriteToken(TokenCacheDir(e.home), TokenCacheKey(e.sess.Name, e.sess.StartURL), tok); err != nil {
		t.Fatal(err)
	}
}

func (e *env) readToken(t *testing.T) *CachedToken {
	t.Helper()
	tok, err := ReadToken(TokenCacheDir(e.home), TokenCacheKey(e.sess.Name, e.sess.StartURL))
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func goodToken(remaining time.Duration) *CachedToken {
	now := time.Now().UTC().Truncate(time.Second)
	return &CachedToken{StartURL: "https://d-1234567890.awsapps.com/start", Region: "us-west-2", AccessToken: "AT-old",
		ExpiresAt: now.Add(remaining), ClientID: "CID-old", ClientSecret: "CS-old",
		RegistrationExpiresAt: now.Add(80 * 24 * time.Hour), RefreshToken: "RT-old"}
}
