package gitx

import (
	"fmt"
	"strings"
)

// CredURL is the credential description git builds from a remote URL. Port is only set when the URL spells it
// out; Path has no leading slash.
type CredURL struct{ Protocol, Host, Port, Path, User string }

func defaultPort(proto string) string {
	switch strings.ToLower(proto) {
	case "https":
		return "443"
	case "http":
		return "80"
	}
	return ""
}

func isUnreserved(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '.' || c == '_' || c == '~'
}

func unhex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// normPath decodes percent-escapes of unreserved characters only (git's urlmatch normalisation).
func normPath(p string) string {
	if !strings.Contains(p, "%") {
		return p
	}
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		if p[i] == '%' && i+2 < len(p) {
			hi, ok1 := unhex(p[i+1])
			lo, ok2 := unhex(p[i+2])
			if ok1 && ok2 {
				if c := hi<<4 | lo; isUnreserved(c) {
					b.WriteByte(c)
					i += 2
					continue
				}
			}
		}
		b.WriteByte(p[i])
	}
	return b.String()
}

// splitAuthority parses "[user[:pass]@]host[:port]".
func splitAuthority(auth string) (user, host, port string) {
	if at := strings.LastIndexByte(auth, '@'); at >= 0 {
		user = auth[:at]
		if c := strings.IndexByte(user, ':'); c >= 0 {
			user = user[:c]
		}
		auth = auth[at+1:]
	}
	host = auth
	if !strings.HasPrefix(auth, "[") {
		if c := strings.LastIndexByte(auth, ':'); c >= 0 {
			host, port = auth[:c], auth[c+1:]
		}
	}
	return user, strings.ToLower(host), port
}

// ParseCredURL parses a remote URL the way `git credential` would (query and fragment are dropped).
func ParseCredURL(raw string) (CredURL, error) {
	i := strings.Index(raw, "://")
	if i <= 0 {
		return CredURL{}, fmt.Errorf("gitx: not a URL: %q", raw)
	}
	u := CredURL{Protocol: strings.ToLower(raw[:i])}
	rest := raw[i+3:]
	if j := strings.IndexAny(rest, "?#"); j >= 0 {
		rest = rest[:j]
	}
	auth, path := rest, ""
	if j := strings.IndexByte(rest, '/'); j >= 0 {
		auth, path = rest[:j], rest[j+1:]
	}
	u.User, u.Host, u.Port = splitAuthority(auth)
	u.Path = normPath(path)
	if u.Host == "" {
		return CredURL{}, fmt.Errorf("gitx: URL without host: %q", raw)
	}
	return u, nil
}

func hostWithPort(u CredURL) string {
	if u.Port != "" {
		return u.Host + ":" + u.Port
	}
	return u.Host
}

// Matches reports whether a `credential.<pattern>.*` section applies to u, following git's urlmatch rules
// URL patterns compare scheme, host (`*` = one label), port, user and a path prefix on
// a "/" boundary; scheme-less (partial) patterns need exact equality of every field they specify.
func Matches(pattern string, u CredURL) bool {
	if pattern == "" {
		return true
	}
	if strings.Contains(pattern, "://") {
		return matchURLPattern(pattern, u)
	}
	return matchPartial(pattern, u)
}

func matchURLPattern(pattern string, u CredURL) bool {
	i := strings.Index(pattern, "://")
	scheme := strings.ToLower(pattern[:i])
	rest := pattern[i+3:]
	if strings.ContainsAny(rest, "?#") {
		return false // query and fragment never match
	}
	if scheme != strings.ToLower(u.Protocol) {
		return false
	}
	auth, path := rest, ""
	if j := strings.IndexByte(rest, '/'); j >= 0 {
		auth, path = rest[:j], rest[j+1:]
	}
	puser, phost, pport := splitAuthority(auth)
	if phost != "" {
		if !matchHost(phost, strings.ToLower(u.Host)) {
			return false
		}
		pp, up := pport, u.Port
		if pp == defaultPort(scheme) {
			pp = ""
		}
		if up == defaultPort(u.Protocol) {
			up = ""
		}
		if pp != up {
			return false
		}
	}
	if puser != "" && puser != u.User {
		return false
	}
	path = strings.TrimSuffix(normPath(path), "/")
	if path == "" {
		return true
	}
	up := strings.TrimPrefix(u.Path, "/")
	return up == path || strings.HasPrefix(up, path+"/")
}

func matchHost(pattern, host string) bool {
	pl, hl := strings.Split(pattern, "."), strings.Split(host, ".")
	if len(pl) != len(hl) {
		return false
	}
	for i := range pl {
		if pl[i] != "*" && pl[i] != hl[i] {
			return false
		}
	}
	return true
}

func matchPartial(pattern string, u CredURL) bool {
	p := pattern
	var puser, ppath string
	if at := strings.IndexByte(p, '@'); at >= 0 {
		puser, p = p[:at], p[at+1:]
	}
	if j := strings.IndexByte(p, '/'); j >= 0 {
		p, ppath = p[:j], p[j+1:]
	}
	if puser != "" && puser != u.User {
		return false
	}
	if p != "" && strings.ToLower(p) != hostWithPort(u) {
		return false
	}
	if ppath != "" && normPath(ppath) != strings.TrimPrefix(u.Path, "/") {
		return false
	}
	return true
}

// HelperRef is one helper that survived the resets, with where it was configured.
type HelperRef struct{ Value, Scope, Origin, Subsection string }

// Effective is the resolved helper list for one URL.
type Effective struct {
	Helpers         []HelperRef
	UseHTTPPath     bool
	ResetScope      string // where the last empty-value reset happened ("" = none)
	ResetSubsection string
}

// Resolve folds the credential.* entries that match u in file order: an empty helper value clears the list,
// others append; usehttppath is last-wins.
func Resolve(entries []Entry, u CredURL) Effective {
	eff := Effective{Helpers: make([]HelperRef, 0)}
	for _, e := range entries {
		if !strings.HasPrefix(e.Key, "credential.") {
			continue
		}
		if e.Subsection != "" && !Matches(e.Subsection, u) {
			continue
		}
		switch e.Var {
		case "helper":
			if e.NoValue {
				continue
			}
			if e.Value == "" {
				eff.Helpers = eff.Helpers[:0]
				eff.ResetScope, eff.ResetSubsection = e.Scope, e.Subsection
				continue
			}
			eff.Helpers = append(eff.Helpers, HelperRef{Value: e.Value, Scope: e.Scope, Origin: e.Origin, Subsection: e.Subsection})
		case "usehttppath":
			eff.UseHTTPPath = e.NoValue || isTrue(e.Value)
		}
	}
	return eff
}

func isTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "yes", "on", "1":
		return true
	}
	return false
}
