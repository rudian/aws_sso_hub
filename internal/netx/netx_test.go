package netx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const scutilSample = `<dictionary> {
  ExceptionsList : <array> {
    0 : *.local
    1 : 169.254/16
  }
  HTTPEnable : 1
  HTTPPort : 8080
  HTTPProxy : proxy.acme.example
  HTTPSEnable : 1
  HTTPSPort : 8443
  HTTPSProxy : secure.acme.example
  ProxyAutoConfigEnable : 0
}
`

func TestParseScutil(t *testing.T) {
	c := ParseScutil(scutilSample)
	if c.HTTP != "http://proxy.acme.example:8080" || c.HTTPS != "http://secure.acme.example:8443" {
		t.Fatalf("got %+v", c)
	}
	if c.NoProxy != ".local,169.254.0.0/16" {
		t.Fatalf("no_proxy %q", c.NoProxy)
	}
	empty := ParseScutil("<dictionary> {\n  ExceptionsList : <array> {\n    0 : *.local\n  }\n  FTPPassive : 1\n}\n")
	if !empty.Empty() || empty.Source != "none" {
		t.Fatalf("got %+v", empty)
	}
	pac := ParseScutil("<dictionary> {\n  ProxyAutoConfigEnable : 1\n  ProxyAutoConfigURLString : http://wpad.acme.example/proxy.pac\n}\n")
	if !pac.PACOnly() || pac.PAC == "" {
		t.Fatalf("got %+v", pac)
	}
}

const scutilScoped = `<dictionary> {
  ExceptionsList : <array> {
    0 : *.local
  }
  ExcludeSimpleHostnames : 1
  HTTPEnable : 1
  HTTPPort : 8080
  HTTPProxy : proxy.acme.example
  __SCOPED__ : <dictionary> {
    en0 : <dictionary> {
      HTTPEnable : 1
      HTTPPort : 8080
      HTTPProxy : proxy.acme.example
    }
    en7 : <dictionary> {
      ExceptionsList : <array> {
        0 : evil.example
      }
      HTTPEnable : 0
    }
  }
}
`

const scutilSupplemental = `<dictionary> {
  HTTPEnable : 0
  __SUPPLEMENTAL__ : <array> {
    0 : <dictionary> {
      HTTPEnable : 1
      HTTPPort : 3128
      HTTPProxy : vpn.initech.example
      __MATCH_DOMAIN__ : initech.example
    }
  }
}
`

func TestParseScutilNested(t *testing.T) {
	c := ParseScutil(scutilScoped)
	if c.HTTP != "http://proxy.acme.example:8080" || c.NoProxy != ".local" || !c.BypassSimple {
		t.Fatalf("scoped entry overrode root: %+v", c)
	}
	s := ParseScutil(scutilSupplemental)
	if !s.Empty() {
		t.Fatalf("supplemental proxy leaked to every host: %+v", s)
	}
}

func TestWinInternetConfig(t *testing.T) {
	c := WinInternetConfig(true, "proxy.acme.example:3128", "*.acme.example;10.*;192.168.*;<local>", "")
	if c.HTTP != "http://proxy.acme.example:3128" || c.HTTPS != c.HTTP {
		t.Fatalf("got %+v", c)
	}
	if c.NoProxy != ".acme.example,10.0.0.0/8,192.168.0.0/16" || !c.BypassSimple {
		t.Fatalf("no_proxy %q simple %v", c.NoProxy, c.BypassSimple)
	}
	per := WinInternetConfig(true, "http=a.example:1;https=b.example:2;socks=c.example:3", "", "")
	if per.HTTP != "http://a.example:1" || per.HTTPS != "http://b.example:2" {
		t.Fatalf("got %+v", per)
	}
	if off := WinInternetConfig(false, "a.example:1", "", ""); !off.Empty() || off.Source != "none" {
		t.Fatalf("got %+v", off)
	}
	if pac := WinInternetConfig(false, "", "", "http://wpad/p.pac"); !pac.PACOnly() {
		t.Fatalf("got %+v", pac)
	}
	if c := WinInternetConfig(true, "p:1", "a.b.c.*;1.*x;foo*bar", ""); c.NoProxy != "" {
		t.Fatalf("odd wildcards must be skipped: %q", c.NoProxy)
	}
	if c := WinInternetConfig(true, "p:1", "172.16.5.*", ""); c.NoProxy != "172.16.5.0/24" {
		t.Fatalf("got %q", c.NoProxy)
	}
}

func TestWinHTTPConfig(t *testing.T) {
	c := WinHTTPConfig("proxy.acme.example:8080", "*.acme.example;<local>")
	if c.HTTP != "http://proxy.acme.example:8080" || c.NoProxy != ".acme.example" || !c.BypassSimple {
		t.Fatalf("got %+v", c)
	}
	if d := WinHTTPConfig("", ""); !d.Empty() || d.Source != "none" {
		t.Fatalf("got %+v", d)
	}
}

func TestFromEnv(t *testing.T) {
	env := map[string]string{"https_proxy": "p.example:3128", "no_proxy": "a.example"}
	c := FromEnv(func(k string) string { return env[k] })
	if c.HTTPS != "http://p.example:3128" || c.NoProxy != "a.example" || c.Source != "env" {
		t.Fatalf("got %+v", c)
	}
	if !FromEnv(func(string) string { return "" }).Empty() {
		t.Fatal("expected empty")
	}
}

func TestProxyFuncNoProxy(t *testing.T) {
	c := Config{HTTP: "http://p.example:1", HTTPS: "http://p.example:1", NoProxy: ".acme.example,10.0.0.0/8,exact.example,svc.example:8080"}
	f := c.ProxyFunc()
	cases := map[string]bool{
		"https://github.com/x":        true,
		"https://api.acme.example/x":  false,
		"https://acme.example/x":      false,
		"https://notacme.example/x":   true,
		"http://10.2.3.4/x":           false,
		"http://11.2.3.4/x":           true,
		"https://exact.example/x":     false,
		"https://sub.exact.example/x": false,
		"https://svc.example/x":       true,
		"https://svc.example:8080/x":  false,
		"https://svc.example:9/x":     true,
		"http://127.0.0.1:9/x":        false,
		"http://localhost/x":          false,
		"ftp://github.com/x":          false,
	}
	for raw, wantProxy := range cases {
		req, _ := http.NewRequest("GET", raw, nil)
		u, _ := f(req)
		if (u != nil) != wantProxy {
			t.Errorf("%s: proxied=%v want %v", raw, u != nil, wantProxy)
		}
	}
	// Implicit port: svc.example:443 matches https://svc.example/.
	imp := Config{HTTPS: "http://p.example:1", NoProxy: "svc.example:443"}.ProxyFunc()
	req0, _ := http.NewRequest("GET", "https://svc.example/", nil)
	if u, _ := imp(req0); u != nil {
		t.Fatal("implicit port 443 must match")
	}
	simple := Config{HTTP: "http://p.example:1", BypassSimple: true}.ProxyFunc()
	for raw, want := range map[string]bool{"http://intranet/": false, "http://a.example/": true, "http://10.1.2.3/": true} {
		r, _ := http.NewRequest("GET", raw, nil)
		if u, _ := simple(r); (u != nil) != want {
			t.Errorf("%s: proxied=%v want %v", raw, u != nil, want)
		}
	}
	cidr := Config{HTTP: "http://p.example:1", NoProxy: "10.0.0.0/8,192.168.0.0/16"}.ProxyFunc()
	for raw, want := range map[string]bool{"http://10.1.2.3/": false, "http://192.168.1.5/": false, "http://11.1.1.1/": true} {
		r, _ := http.NewRequest("GET", raw, nil)
		if u, _ := cidr(r); (u != nil) != want {
			t.Errorf("%s: proxied=%v want %v", raw, u != nil, want)
		}
	}
	all := Config{HTTPS: "http://p.example:1", NoProxy: "*"}.ProxyFunc()
	req, _ := http.NewRequest("GET", "https://github.com", nil)
	if u, _ := all(req); u != nil {
		t.Fatal("* must bypass everything")
	}
}

func TestChildEnv(t *testing.T) {
	if len(Config{}.ChildEnv()) != 0 {
		t.Fatal("empty config must give no env")
	}
	env := Config{HTTP: "http://h:1", HTTPS: "http://s:2", NoProxy: "a,b"}.ChildEnv()
	joined := strings.Join(env, "\n")
	for _, want := range []string{"HTTP_PROXY=http://h:1", "http_proxy=http://h:1", "HTTPS_PROXY=http://s:2", "https_proxy=http://s:2", "NO_PROXY=localhost,127.0.0.1,::1,a,b", "no_proxy=localhost,127.0.0.1,::1,a,b"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %s in %v", want, env)
		}
	}
	mac := ParseScutil(scutilSample)
	if j := strings.Join(mac.ChildEnv(), "\n"); !strings.Contains(j, "NO_PROXY=localhost,127.0.0.1,::1,.local,169.254.0.0/16") {
		t.Errorf("mac defaults: %s", j)
	}
	dup := strings.Join(Config{HTTP: "http://h:1", NoProxy: "localhost,x"}.ChildEnv(), "\n")
	if !strings.Contains(dup, "NO_PROXY=localhost,127.0.0.1,::1,x\n") {
		t.Errorf("dedupe: %s", dup)
	}
	got := StripProxyEnv([]string{"PATH=/bin", "http_proxy=x", "NO_PROXY=y", "ALL_PROXY=z", "HOME=/h"})
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestStringMasksCredentials(t *testing.T) {
	for _, raw := range []string{
		"http://user:s3cret@p.example.com:1",
		"http://alex:p@ss@proxy.acme.example.com:8080",
		"http://tok3n-secret@proxy.acme.example.com:8080",
		"http://user:s3cret@wpad.acme.example.com/proxy.pac?k=abc",
		"http://bad:s3cret@host:99999999999 x",
	} {
		c := Config{HTTP: raw, HTTPS: raw, PAC: raw}
		s := c.String()
		for _, leak := range []string{"s3cret", "ss@", "p@ss", "tok3n", "alex", "abc"} {
			if strings.Contains(s, leak) {
				t.Errorf("%q leaked %q: %s", raw, leak, s)
			}
		}
	}
	c := Config{HTTPS: "http://alex:p@ss@proxy.acme.example.com:8080"}
	if c.HostPort() != "proxy.acme.example.com:8080" {
		t.Fatalf("hostport %q", c.HostPort())
	}
}

// A local server stands in for the proxy: it sees absolute-form requests.
func TestClientThroughProxy(t *testing.T) {
	var hits atomic.Int32
	var seen atomic.Value
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		seen.Store(r.URL.String())
		w.Header().Set("ETag", `"v1"`)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte("hello"))
	}))
	defer proxy.Close()

	c := New(Options{Proxy: Config{HTTP: proxy.URL, NoProxy: ".direct.test"}})
	res, err := c.GetCached(context.Background(), "http://svc.example.test/data", nil)
	if err != nil || string(res.Body) != "hello" || res.FromCache {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if seen.Load() != "http://svc.example.test/data" {
		t.Fatalf("proxy saw %v", seen.Load())
	}
	res, err = c.GetCached(context.Background(), "http://svc.example.test/data", nil)
	if err != nil || string(res.Body) != "hello" || !res.FromCache {
		t.Fatalf("second: res=%+v err=%v", res, err)
	}

	before := hits.Load()
	c2 := New(Options{Proxy: Config{HTTP: proxy.URL, NoProxy: ".direct.test"}, Timeout: 2 * time.Second})
	if _, err := c2.GetCached(context.Background(), "http://x.direct.test/data", nil); err == nil {
		t.Fatal("expected a direct dial failure")
	}
	if hits.Load() != before {
		t.Fatal("NO_PROXY host went through the proxy")
	}
}

func TestClientStatusAndSize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/big" {
			_, _ = w.Write([]byte(strings.Repeat("x", 100)))
			return
		}
		http.Error(w, "no", http.StatusNotFound)
	}))
	defer srv.Close()
	c := New(Options{MaxBody: 10})
	if _, err := c.GetCached(context.Background(), srv.URL+"/nope", nil); err == nil || strings.Contains(err.Error(), srv.URL) {
		t.Fatalf("err=%v", err)
	}
	if _, err := c.GetCached(context.Background(), srv.URL+"/big", nil); err != ErrTooLarge {
		t.Fatalf("err=%v", err)
	}
}

func TestSetProxyAffectsNextRequest(t *testing.T) {
	var hits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("via proxy"))
	}))
	defer proxy.Close()
	c := New(Options{Timeout: time.Second})
	if _, err := c.GetCached(context.Background(), "http://svc.example.test/x", nil); err == nil {
		t.Fatal("expected a direct failure")
	}
	c.SetProxy(Config{HTTP: proxy.URL})
	if c.Proxy().HTTP != proxy.URL {
		t.Fatal("Proxy() not updated")
	}
	res, err := c.GetCached(context.Background(), "http://svc.example.test/x", nil)
	if err != nil || string(res.Body) != "via proxy" || hits.Load() != 1 {
		t.Fatalf("res=%+v err=%v hits=%d", res, err, hits.Load())
	}
}

func TestErrStatusHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Accepted-OAuth-Scopes", "repo")
		w.Header().Set("X-OAuth-Scopes", "read:user")
		http.Error(w, "no", http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := New(Options{}).GetCached(context.Background(), srv.URL, nil)
	var es *ErrStatus
	if !errors.As(err, &es) || es.Code != 404 || es.Header.Get("X-Accepted-OAuth-Scopes") != "repo" || es.Header.Get("X-OAuth-Scopes") != "read:user" {
		t.Fatalf("err=%#v", err)
	}
}

func TestCacheKeysAndHeaders(t *testing.T) {
	var full atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tag := `"` + r.Header.Get("Authorization") + r.Header.Get("Accept") + `"`
		w.Header().Set("ETag", tag)
		w.Header().Set("Link", `<next>; rel="next"`)
		w.Header().Set("X-RateLimit-Remaining", "42")
		if r.Header.Get("If-None-Match") == tag {
			w.Header().Del("Link") // a 304 may omit it
			w.Header().Set("X-RateLimit-Remaining", "41")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		full.Add(1)
		_, _ = w.Write([]byte("body-" + tag))
	}))
	defer srv.Close()
	c := New(Options{})
	get := func(auth, accept string) Result {
		h := http.Header{"Authorization": {auth}, "Accept": {accept}}
		res, err := c.GetCached(context.Background(), srv.URL, h)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	get("a", "json")
	get("b", "json")
	get("a", "diff")
	if full.Load() != 3 {
		t.Fatalf("full=%d", full.Load())
	}
	for _, args := range [][2]string{{"a", "json"}, {"b", "json"}, {"a", "diff"}} {
		r := get(args[0], args[1])
		if !r.FromCache {
			t.Errorf("%v: entry was evicted", args)
		}
		if r.Header.Get("Link") == "" || r.Header.Get("X-RateLimit-Remaining") != "41" {
			t.Errorf("%v: headers %v", args, r.Header)
		}
	}
	if full.Load() != 3 {
		t.Fatalf("full=%d after revalidation", full.Load())
	}
}

func TestErrStatusBody(t *testing.T) {
	const msg = `{"message":"Resource protected by organization SAML enforcement."}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(msg))
	}))
	defer srv.Close()
	_, err := New(Options{}).GetCached(context.Background(), srv.URL, nil)
	var es *ErrStatus
	if !errors.As(err, &es) || es.Code != 403 || string(es.Body) != msg {
		t.Fatalf("err=%#v", err)
	}
	if strings.Contains(err.Error(), "SAML") {
		t.Fatalf("body leaked into Error(): %q", err.Error())
	}
}
