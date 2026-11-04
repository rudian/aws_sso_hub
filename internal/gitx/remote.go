package gitx

import (
	"regexp"
	"strings"

	"github.com/rudian/aws_sso_hub/internal/api"
)

// RemoteInfo describes a parsed CodeCommit remote URL.
type RemoteInfo struct {
	Kind                              api.RemoteKind
	Host, Region, Repo, User, Profile string // Profile only for GRC ("" = default)
	FIPS, VPCE, China                 bool
	HTTPSURL                          string // https://<host>/v1/repos/<repo> ("" for GRC)
}

// Regexes for the CodeCommit remote forms, made case-insensitive for the scheme and host (the repository name
// keeps its case because it is read from the original text).
var (
	reHTTPS = regexp.MustCompile(`(?i)^https://(?:([^@/]+)@)?((?:vpce-[a-z0-9-]+\.)?git-codecommit(-fips)?\.([a-z0-9-]+)\.(vpce\.)?amazonaws\.com(?:\.cn)?)(?::443)?/v1/repos/([\w.-]{1,100})/?$`)
	reSSH   = regexp.MustCompile(`(?i)^(?:ssh://(?:([^@/]+)@)?((?:vpce-[a-z0-9-]+\.)?git-codecommit(?:-fips)?\.([a-z0-9-]+)\.(?:vpce\.)?amazonaws\.com(?:\.cn)?)(?::22)?/|(?:([^@/:]+)@)?((?:vpce-[a-z0-9-]+\.)?git-codecommit(?:-fips)?\.([a-z0-9-]+)\.(?:vpce\.)?amazonaws\.com(?:\.cn)?):/?)v1/repos/([\w.-]{1,100})/?$`)
	reGRC   = regexp.MustCompile(`^codecommit(?:::([a-z]{2}(?:-[a-z]+)+-\d+))?://(?:([^@/]+)@)?([\w.-]{1,100})/?$`)
)

func hostFlags(host string) (fips, vpce, china bool) {
	h := strings.ToLower(host)
	return strings.Contains(h, "git-codecommit-fips."), strings.Contains(h, ".vpce.") || strings.HasPrefix(h, "vpce-"),
		strings.HasSuffix(h, ".cn")
}

// ParseRemote parses a git remote URL. false means it is not a CodeCommit URL.
func ParseRemote(raw string) (RemoteInfo, bool) {
	raw = strings.TrimSpace(raw)
	if m := reHTTPS.FindStringSubmatch(raw); m != nil {
		host := strings.ToLower(m[2])
		user := m[1]
		if i := strings.IndexByte(user, ':'); i >= 0 {
			user = user[:i] // never keep a password
		}
		fips, vpce, china := hostFlags(host)
		return RemoteInfo{
			Kind: api.RemoteHTTPS, Host: host, Region: strings.ToLower(m[4]), Repo: m[6], User: user,
			FIPS: fips, VPCE: vpce, China: china,
			HTTPSURL: "https://" + host + "/v1/repos/" + m[6],
		}, true
	}
	if m := reSSH.FindStringSubmatch(raw); m != nil {
		user, host, region := m[1], m[2], m[3]
		if host == "" {
			user, host, region = m[4], m[5], m[6]
		}
		host = strings.ToLower(host)
		fips, vpce, china := hostFlags(host)
		return RemoteInfo{
			Kind: api.RemoteSSH, Host: host, Region: strings.ToLower(region), Repo: m[7], User: user,
			FIPS: fips, VPCE: vpce, China: china,
			HTTPSURL: "https://" + host + "/v1/repos/" + m[7],
		}, true
	}
	if m := reGRC.FindStringSubmatch(raw); m != nil {
		return RemoteInfo{Kind: api.RemoteGRC, Region: m[1], Profile: m[2], Repo: m[3]}, true
	}
	return RemoteInfo{}, false
}
