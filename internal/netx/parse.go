package netx

import (
	"net"
	"strconv"
	"strings"
)

// ParseScutil reads the output of `scutil --proxy` (macOS). The output is a
// tree: the effective settings sit at depth 1 of the root dictionary, while
// __SCOPED__ (per interface), __SERVICES__ and __SUPPLEMENTAL__ (VPN, per
// domain) hold nested dictionaries that must not override them. Only depth 1
// is read.
func ParseScutil(out string) Config {
	kv := map[string]string{}
	var exceptions []string
	var stack []string // names of the open braces; the root dictionary has ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "}" {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			continue
		}
		if strings.HasSuffix(line, "{") {
			name := ""
			if k, _, ok := strings.Cut(line, " : "); ok {
				name = strings.TrimSpace(k)
			}
			stack = append(stack, name)
			continue
		}
		k, v, ok := strings.Cut(line, " : ")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch {
		case len(stack) == 1:
			kv[k] = v
		case len(stack) == 2 && stack[1] == "ExceptionsList":
			exceptions = append(exceptions, v)
		}
	}
	c := Config{Source: "system"}
	if kv["HTTPEnable"] == "1" && kv["HTTPProxy"] != "" {
		c.HTTP = hostPortURL(kv["HTTPProxy"], kv["HTTPPort"])
	}
	if kv["HTTPSEnable"] == "1" && kv["HTTPSProxy"] != "" {
		c.HTTPS = hostPortURL(kv["HTTPSProxy"], kv["HTTPSPort"])
	}
	if kv["ProxyAutoConfigEnable"] == "1" {
		c.PAC = kv["ProxyAutoConfigURLString"]
	}
	c.NoProxy, c.BypassSimple = joinNoProxy(exceptions)
	if kv["ExcludeSimpleHostnames"] == "1" {
		c.BypassSimple = true
	}
	if c.Empty() && c.PAC == "" {
		switch {
		case kv["SOCKSEnable"] == "1" && kv["SOCKSProxy"] != "":
			c.Unsupported = "socks"
		case kv["ProxyAutoDiscoveryEnable"] == "1":
			c.Unsupported = "autodiscovery"
		default:
			c.Source = "none"
		}
	}
	return c
}

// WinInternetConfig builds a Config from the WinINET "Internet Settings"
// values (HKCU): ProxyEnable, ProxyServer, ProxyOverride, AutoConfigURL.
func WinInternetConfig(enabled bool, server, override, pac string) Config {
	c := Config{Source: "system", PAC: strings.TrimSpace(pac)}
	if enabled {
		c.HTTP, c.HTTPS = parseWinProxyServer(server)
		c.NoProxy, c.BypassSimple = joinNoProxy(strings.Split(override, ";"))
	}
	if c.Empty() && c.PAC == "" {
		return Config{Source: "none"}
	}
	return c
}

// WinHTTPConfig builds a Config from the WinHTTP default proxy (server and
// bypass list as WinHttpGetDefaultProxyConfiguration returns them).
func WinHTTPConfig(server, bypass string) Config {
	c := Config{Source: "system"}
	c.HTTP, c.HTTPS = parseWinProxyServer(server)
	c.NoProxy, c.BypassSimple = joinNoProxy(strings.Split(bypass, ";"))
	if c.Empty() {
		return Config{Source: "none"}
	}
	return c
}

// parseWinProxyServer handles "host:port" and "http=h:p;https=h:p;socks=h:p".
func parseWinProxyServer(s string) (httpURL, httpsURL string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if !strings.Contains(s, "=") {
		u := normalizeProxyURL(s)
		return u, u
	}
	for _, part := range strings.Split(s, ";") {
		scheme, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || v == "" {
			continue
		}
		switch strings.ToLower(scheme) {
		case "http":
			httpURL = normalizeProxyURL(v)
		case "https":
			httpsURL = normalizeProxyURL(v)
		}
	}
	// A per-scheme entry covers that scheme only (WinINET, WinHTTP).
	return
}

func hostPortURL(host, port string) string {
	if port != "" {
		host = net.JoinHostPort(host, port)
	}
	return "http://" + host
}

// joinNoProxy converts system exception entries into NO_PROXY syntax:
// "*.local" -> ".local", "169.254/16" -> "169.254.0.0/16", "10.*" ->
// "10.0.0.0/8". "<local>" (every name without a dot) cannot be written in
// NO_PROXY, so it is returned as the simple flag. Other wildcard shapes are
// skipped, because neither bypass() nor curl understands them.
func joinNoProxy(items []string) (list string, simple bool) {
	var out []string
	seen := map[string]bool{}
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		switch {
		case strings.EqualFold(it, "<local>"):
			simple = true
			continue
		case strings.HasPrefix(it, "<"):
			continue // <-loopback> and friends
		case it == "*":
		case strings.HasPrefix(it, "*."):
			it = it[1:]
		case strings.Contains(it, "/"):
			it = padCIDR(it)
		case strings.Contains(it, "*"):
			cidr, ok := wildcardCIDR(it)
			if !ok {
				continue
			}
			it = cidr
		}
		if strings.Contains(it, "*") && it != "*" {
			continue
		}
		if !seen[it] {
			seen[it] = true
			out = append(out, it)
		}
	}
	return strings.Join(out, ","), simple
}

// wildcardCIDR turns an IPv4 prefix pattern ("10.*", "192.168.*", "a.b.c.*")
// into a CIDR.
func wildcardCIDR(s string) (string, bool) {
	parts := strings.Split(s, ".")
	for len(parts) > 0 && parts[len(parts)-1] == "*" {
		parts = parts[:len(parts)-1]
	}
	n := len(parts)
	if n == 0 || n > 3 || n == len(strings.Split(s, ".")) {
		return "", false
	}
	for _, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 || v > 255 {
			return "", false
		}
	}
	for len(parts) < 4 {
		parts = append(parts, "0")
	}
	return strings.Join(parts, ".") + "/" + strconv.Itoa(n*8), true
}

func padCIDR(s string) string {
	addr, bits, _ := strings.Cut(s, "/")
	parts := strings.Split(addr, ".")
	if len(parts) >= 4 || strings.Contains(addr, ":") {
		return s
	}
	for len(parts) < 4 {
		parts = append(parts, "0")
	}
	return strings.Join(parts, ".") + "/" + bits
}
