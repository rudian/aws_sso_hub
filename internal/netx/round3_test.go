package netx

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestConfigNeverMarshalsCredentials(t *testing.T) {
	c := Config{HTTP: "http://alex:s3cret@proxy.acme.example.com:8080", HTTPS: "http://alex:s3cret@proxy.acme.example.com:8080", PAC: "http://u:s3cret@w.example.com/p.pac", NoProxy: "a.example", Source: "env"}
	j1, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	j2, _ := json.Marshal(struct{ P Config }{c})
	j3, _ := json.Marshal([]Config{c})
	for _, s := range []string{fmt.Sprintf("%#v", c), fmt.Sprintf("%+v", c), fmt.Sprintf("%v", c), string(j1), string(j2), string(j3)} {
		if strings.Contains(s, "s3cret") || strings.Contains(s, "alex") {
			t.Errorf("leaked: %s", s)
		}
	}
	if !strings.Contains(string(j1), "proxy.acme.example.com:8080") || !strings.Contains(string(j1), "a.example") {
		t.Errorf("masked JSON lost the host or NO_PROXY: %s", j1)
	}
}

func TestProxyFuncSchemes(t *testing.T) {
	c := Config{HTTP: "http://h.example:1", HTTPS: "http://s.example:2"}
	f := c.ProxyFunc()
	for raw, want := range map[string]string{"http://x.example/": "h.example:1", "https://x.example/": "s.example:2"} {
		r, _ := http.NewRequest("GET", raw, nil)
		u, err := f(r)
		if err != nil || u == nil || u.Host != want {
			t.Errorf("%s: got %v %v want %s", raw, u, err, want)
		}
	}
	onlyHTTP := Config{HTTP: "http://h.example:1"}.ProxyFunc()
	r, _ := http.NewRequest("GET", "https://x.example/", nil)
	if u, err := onlyHTTP(r); u != nil || err != nil {
		t.Errorf("https must not use the http proxy: %v %v", u, err)
	}
}

func TestProxyFuncBadURLFails(t *testing.T) {
	f := Config{HTTPS: "http://u:pa#ss@p.example.com:1:x", HTTP: "http://ok.example:1"}.ProxyFunc()
	r, _ := http.NewRequest("GET", "https://x.example/", nil)
	u, err := f(r)
	if u != nil || err == nil || strings.Contains(err.Error(), "pa#ss") || strings.Contains(err.Error(), "p.example") {
		t.Fatalf("u=%v err=%v", u, err)
	}
	r, _ = http.NewRequest("GET", "http://x.example/", nil)
	if u, err := f(r); err != nil || u == nil {
		t.Fatalf("the valid http proxy must still work: %v %v", u, err)
	}
}

func TestScutilDisabledKeysIgnored(t *testing.T) {
	out := `<dictionary> {
  HTTPEnable : 0
  HTTPPort : 8080
  HTTPProxy : proxy.acme.example
  HTTPSEnable : 0
  HTTPSPort : 8443
  HTTPSProxy : secure.acme.example
  ProxyAutoConfigEnable : 0
  ProxyAutoConfigURLString : http://wpad.acme.example/p.pac
}
`
	c := ParseScutil(out)
	if !c.Empty() || c.PAC != "" || c.Source != "none" || c.Unsupported != "" {
		t.Fatalf("got %+v", c)
	}
}

func TestScutilUnsupported(t *testing.T) {
	socks := ParseScutil("<dictionary> {\n  SOCKSEnable : 1\n  SOCKSPort : 1080\n  SOCKSProxy : 127.0.0.1\n}\n")
	if !socks.Empty() || socks.Unsupported != "socks" || socks.PACOnly() {
		t.Fatalf("got %+v", socks)
	}
	auto := ParseScutil("<dictionary> {\n  ProxyAutoDiscoveryEnable : 1\n}\n")
	if !auto.Empty() || auto.Unsupported != "autodiscovery" {
		t.Fatalf("got %+v", auto)
	}
	// A real HTTP proxy wins over SOCKS.
	both := ParseScutil("<dictionary> {\n  HTTPEnable : 1\n  HTTPProxy : p.example\n  HTTPPort : 1\n  SOCKSEnable : 1\n  SOCKSProxy : s.example\n}\n")
	if both.Empty() || both.Unsupported != "" {
		t.Fatalf("got %+v", both)
	}
}

func TestWinPerSchemeEntryCoversThatSchemeOnly(t *testing.T) {
	c := WinInternetConfig(true, "http=a.example:1", "", "")
	if c.HTTP != "http://a.example:1" || c.HTTPS != "" {
		t.Fatalf("got %+v", c)
	}
}

func TestFromEnvAllProxyBothSchemes(t *testing.T) {
	env := map[string]string{"ALL_PROXY": "a.example:9", "HTTPS_PROXY": "s.example:1"}
	c := FromEnv(func(k string) string { return env[k] })
	if c.HTTP != "http://a.example:9" || c.HTTPS != "http://s.example:1" {
		t.Fatalf("got %+v", c)
	}
}

func TestGetCachedBodyIsNotShared(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v"`)
		if r.Header.Get("If-None-Match") != "" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte("abc"))
	}))
	defer srv.Close()
	c := New(Options{})
	r1, err := c.GetCached(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	r1.Body[0] = 'X'
	r2, _ := c.GetCached(context.Background(), srv.URL, nil)
	if string(r2.Body) != "abc" || !r2.FromCache {
		t.Fatalf("got %q", r2.Body)
	}
	r2.Body[0] = 'Y'
	r3, _ := c.GetCached(context.Background(), srv.URL, nil)
	if string(r3.Body) != "abc" {
		t.Fatalf("got %q", r3.Body)
	}
}

// A minimal CONNECT proxy: it records the request line and answers 502.
func TestHTTPSGoesThroughConnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var method, host, auth atomic.Value
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				req, err := http.ReadRequest(bufio.NewReader(conn))
				if err != nil {
					return
				}
				method.Store(req.Method)
				host.Store(req.Host)
				auth.Store(req.Header.Get("Proxy-Authorization"))
				_, _ = conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n"))
			}()
		}
	}()
	c := New(Options{Proxy: Config{HTTP: "http://127.0.0.1:1", HTTPS: "http://u:pw@" + ln.Addr().String()}, Timeout: 3 * time.Second})
	_, err = c.GetCached(context.Background(), "https://svc.example.test/x", nil)
	if err == nil {
		t.Fatal("expected the 502 to fail the request")
	}
	if method.Load() != "CONNECT" || host.Load() != "svc.example.test:443" {
		t.Fatalf("proxy saw %v %v", method.Load(), host.Load())
	}
	if a, _ := auth.Load().(string); !strings.HasPrefix(a, "Basic ") {
		t.Fatalf("no Proxy-Authorization: %q", a)
	}
}

func TestDownloadSharesProxyWithoutTimeout(t *testing.T) {
	var hits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("dl"))
	}))
	defer proxy.Close()
	c := New(Options{Timeout: time.Second})
	d := c.Download()
	if d.Timeout != 0 {
		t.Fatalf("timeout %v", d.Timeout)
	}
	c.SetProxy(Config{HTTP: proxy.URL})
	resp, err := d.Get("http://svc.example.test/pkg")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if hits.Load() != 1 {
		t.Fatalf("hits %d", hits.Load())
	}
}

// FU-06a: a request that NO_PROXY or loopback sends direct never meets the broken proxy setting.
func TestProxyFuncBadURLDoesNotFailBypassedRequests(t *testing.T) {
	f := Config{HTTPS: "http://u:pa#ss@p.example.com:1:x", NoProxy: "internal.example"}.ProxyFunc()
	for _, raw := range []string{"https://internal.example/", "https://git.internal.example/", "https://localhost/", "https://127.0.0.1:8443/"} {
		r, _ := http.NewRequest("GET", raw, nil)
		if u, err := f(r); u != nil || err != nil {
			t.Errorf("%s: u=%v err=%v", raw, u, err)
		}
	}
	r, _ := http.NewRequest("GET", "https://other.example/", nil)
	if _, err := f(r); err == nil {
		t.Error("a request that would use the proxy must still fail")
	}
	r, _ = http.NewRequest("GET", "http://other.example/", nil) // no http proxy at all: direct
	if u, err := f(r); u != nil || err != nil {
		t.Errorf("u=%v err=%v", u, err)
	}
}

// FU-06b: on Windows a SOCKS-only proxy is reported as unsupported, as on macOS, instead of going direct silently.
func TestWindowsSocksOnlyIsUnsupported(t *testing.T) {
	for name, c := range map[string]Config{
		"WinINET":  WinInternetConfig(true, "socks=127.0.0.1:1080", "", ""),
		"WinHTTP":  WinHTTPConfig("socks=127.0.0.1:1080", ""),
		"socks5=":  WinInternetConfig(true, "socks5=127.0.0.1:1080", "", ""),
		"disabled": {},
	} {
		if name == "disabled" {
			continue
		}
		if !c.Empty() || c.Unsupported != "socks" || c.Source != "system" || c.PACOnly() {
			t.Errorf("%s: %+v", name, c)
		}
	}
	// a SOCKS entry next to an http one: the http proxy is used and nothing is unsupported
	if c := WinInternetConfig(true, "http=a.example:1;socks=c.example:3", "", ""); c.HTTP == "" || c.Unsupported != "" {
		t.Errorf("%+v", c)
	}
	// a SOCKS entry with the proxy switched off, and with a PAC file, does not matter
	if c := WinInternetConfig(false, "socks=c.example:3", "", ""); c.Unsupported != "" || c.Source != "none" {
		t.Errorf("%+v", c)
	}
	if c := WinInternetConfig(true, "socks=c.example:3", "", "http://wpad/p.pac"); c.Unsupported != "" || !c.PACOnly() {
		t.Errorf("%+v", c)
	}
}
