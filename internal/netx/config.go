// Package netx is Codepass's one view of the network: the system proxy, one
// HTTP client with timeouts and an ETag cache, and the proxy variables handed
// to child processes. Proxy credentials may sit in URLs here; they are never
// logged and String() masks them.
package netx

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// Config is a proxy setup: one proxy URL per scheme and a NO_PROXY list.
type Config struct {
	HTTP    string // http proxy URL, "" for none
	HTTPS   string // https proxy URL, "" for none
	NoProxy string // comma-separated hosts, domains (".example.com") and CIDRs
	PAC     string // PAC URL when the system uses one (not evaluated)
	// BypassSimple sends host names without a dot (not IP addresses) direct:
	// WinINET "<local>" and macOS ExcludeSimpleHostnames. NO_PROXY cannot say
	// this, so child processes do not get the rule.
	BypassSimple bool
	Source       string // "env", "system" or "none"
	// Unsupported names a system setup Codepass cannot use ("socks" or
	// "autodiscovery") when no usable proxy is set, so the UI can ask for a
	// manual proxy as it does for a PAC file.
	Unsupported string
}

// Empty reports whether no proxy is configured.
func (c Config) Empty() bool { return c.HTTP == "" && c.HTTPS == "" }

// PACOnly reports a setup that only names a PAC file: neither Go nor curl can
// use it, so the UI should ask for a manual proxy.
func (c Config) PACOnly() bool { return c.Empty() && c.PAC != "" }

// String is safe to log: userinfo (user name and password) is never printed.
func (c Config) String() string {
	return "proxy{http=" + safeURL(c.HTTP) + " https=" + safeURL(c.HTTPS) +
		" no_proxy=" + c.NoProxy + " pac=" + safeURL(c.PAC) + " source=" + c.Source + "}"
}

// GoString keeps %#v from printing credentials.
func (c Config) GoString() string { return c.String() }

// MarshalJSON writes the masked form: credentials never leave through JSON
// (logs, Wails bindings). A Config does not survive a JSON round trip; a manual
// proxy's credentials belong in the Keychain.
func (c Config) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		HTTP         string
		HTTPS        string
		NoProxy      string
		PAC          string
		BypassSimple bool
		Source       string
		Unsupported  string
	}{safeURL(c.HTTP), safeURL(c.HTTPS), c.NoProxy, safeURL(c.PAC), c.BypassSimple, c.Source, c.Unsupported})
}

// HostPort is the proxy address for display ("proxy.acme.example:8080"),
// without scheme or credentials. The HTTPS proxy wins; "" when none is set.
func (c Config) HostPort() string {
	for _, raw := range []string{c.HTTPS, c.HTTP} {
		if raw == "" {
			continue
		}
		if u, err := url.Parse(raw); err == nil && u.Host != "" {
			return u.Host
		}
		return ""
	}
	return ""
}

// safeURL renders scheme://host only. Anything with userinfo gets a marker,
// an unparseable value a fixed placeholder, never the raw text.
func safeURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "<unparseable>"
	}
	s := u.Scheme + "://" + u.Host
	if u.User != nil {
		s += " (credentials hidden)"
	}
	return s
}

// FromEnv reads HTTP(S)_PROXY / NO_PROXY in either case. Upper case wins.
func FromEnv(get func(string) string) Config {
	if get == nil {
		get = os.Getenv
	}
	pick := func(names ...string) string {
		for _, n := range names {
			if v := strings.TrimSpace(get(n)); v != "" {
				return v
			}
		}
		return ""
	}
	c := Config{
		HTTP:    normalizeProxyURL(pick("HTTP_PROXY", "http_proxy")),
		HTTPS:   normalizeProxyURL(pick("HTTPS_PROXY", "https_proxy")),
		NoProxy: pick("NO_PROXY", "no_proxy"),
	}
	// Like curl: ALL_PROXY covers each scheme that has no proxy of its own.
	if all := normalizeProxyURL(pick("ALL_PROXY", "all_proxy")); all != "" {
		if c.HTTP == "" {
			c.HTTP = all
		}
		if c.HTTPS == "" {
			c.HTTPS = all
		}
	}
	if !c.Empty() {
		c.Source = "env"
	} else {
		c.Source = "none"
	}
	return c
}

// normalizeProxyURL adds http:// to a bare host:port.
func normalizeProxyURL(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	return s
}

// ProxyFunc returns an http.Transport Proxy function for the config. NO_PROXY
// is honoured; loopback is never proxied.
func (c Config) ProxyFunc() func(*http.Request) (*url.URL, error) {
	rules := parseNoProxy(c.NoProxy)
	// A configured but unparseable proxy fails the request (no URL text in the
	// error), like http.ProxyFromEnvironment and curl, instead of going direct.
	errBad := errors.New("netx: the proxy setting is not a valid URL")
	parse := func(raw string) (*url.URL, error) {
		if raw == "" {
			return nil, nil
		}
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return nil, errBad
		}
		return u, nil
	}
	httpU, httpErr := parse(c.HTTP)
	httpsU, httpsErr := parse(c.HTTPS)
	return func(r *http.Request) (*url.URL, error) {
		var u *url.URL
		switch r.URL.Scheme {
		case "https":
			u = httpsU
			if httpsErr != nil {
				return nil, httpsErr
			}
		case "http":
			u = httpU
			if httpErr != nil {
				return nil, httpErr
			}
		}
		if u == nil || bypass(rules, c.BypassSimple, r.URL.Hostname(), effectivePort(r.URL)) {
			return nil, nil
		}
		return u, nil
	}
}

func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

type noProxyRule struct {
	all    bool
	host   string // host; with suffix also its subdomains (without leading dot)
	suffix bool
	ip     net.IP
	cidr   *net.IPNet
	port   string
}

func parseNoProxy(list string) []noProxyRule {
	var rules []noProxyRule
	for _, tok := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		tok = strings.ToLower(strings.TrimSpace(tok))
		if tok == "" {
			continue
		}
		if tok == "*" {
			rules = append(rules, noProxyRule{all: true})
			continue
		}
		if _, n, err := net.ParseCIDR(tok); err == nil {
			rules = append(rules, noProxyRule{cidr: n})
			continue
		}
		var port string
		if h, p, err := net.SplitHostPort(tok); err == nil {
			tok, port = h, p
		}
		if ip := net.ParseIP(strings.Trim(tok, "[]")); ip != nil {
			rules = append(rules, noProxyRule{ip: ip, port: port})
			continue
		}
		r := noProxyRule{port: port}
		switch {
		case strings.HasPrefix(tok, "*."):
			r.host, r.suffix = tok[2:], true
		case strings.HasPrefix(tok, "."):
			r.host, r.suffix = tok[1:], true
		default:
			// Like net/http and curl: a plain name covers its subdomains.
			r.host, r.suffix = tok, true
		}
		rules = append(rules, r)
	}
	return rules
}

func bypass(rules []noProxyRule, simple bool, host, port string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return true
	}
	if simple && ip == nil && !strings.Contains(host, ".") {
		return true
	}
	for _, r := range rules {
		if r.port != "" && r.port != port {
			continue
		}
		switch {
		case r.all:
			return true
		case r.cidr != nil:
			if ip != nil && r.cidr.Contains(ip) {
				return true
			}
		case r.ip != nil:
			if ip != nil && r.ip.Equal(ip) {
				return true
			}
		default:
			if host == r.host || strings.HasSuffix(host, "."+r.host) {
				return true
			}
		}
	}
	return false
}

// ChildEnv returns the proxy variables, in upper and lower case, for a child
// process (brew, curl, winget, git). It is empty when no proxy is set. NO_PROXY
// always starts with the loopback names, as ProxyFunc never proxies loopback,
// so a child's localhost traffic stays local too. BypassSimple cannot be
// written in NO_PROXY, so children do not get that rule. The values may hold
// credentials: never log them.
func (c Config) ChildEnv() []string {
	var env []string
	add := func(name, val string) {
		if val != "" {
			env = append(env, name+"="+val, strings.ToLower(name)+"="+val)
		}
	}
	add("HTTP_PROXY", c.HTTP)
	add("HTTPS_PROXY", c.HTTPS)
	if !c.Empty() {
		add("NO_PROXY", childNoProxy(c.NoProxy))
	}
	return env
}

func childNoProxy(list string) string {
	out := []string{"localhost", "127.0.0.1", "::1"}
	seen := map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true}
	for _, e := range strings.Split(list, ",") {
		if e = strings.TrimSpace(e); e != "" && !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	return strings.Join(out, ",")
}

// StripProxyEnv removes proxy variables from env so ChildEnv can replace them.
func StripProxyEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		switch strings.ToUpper(name) {
		case "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY":
			continue
		}
		out = append(out, kv)
	}
	return out
}
