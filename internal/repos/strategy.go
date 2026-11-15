package repos

import (
	"net/url"
	"strings"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/projects"
)

// Strategy is what the engine needs to know about one kind of remote (plan 3.2: provider strategies behind one
// interface). Classification, owner detection and binding of AWS accounts exist for CodeCommit only; the
// others follow the machine's own git setup until a later milestone gives GitHub and Bitbucket their own
// bindings. Adding a provider means adding a Strategy to strategies.
type Strategy interface {
	// Provider is the host family, cached on the repository's record.
	Provider() projects.Provider
	// Match reports whether a remote URL belongs to this provider.
	Match(rawURL string) bool
	// Binding is the binding source a repository of this provider shows when it has no binding of its own.
	// CodeCommit answers "" (the engine classifies it from git's credential configuration).
	Binding() api.BindingSource
	// AWSAccounts reports (not consulted yet: Engine.rec refuses account operations on a row by whether it is in
	// the CodeCommit set, which gitx.Inspect decides; the M6 binding seam routes classification, detection and
	// binding through this method) whether the engine can bind, test and detect an owner for this provider.
	AWSAccounts() bool
}

type codeCommitStrategy struct{}

func (codeCommitStrategy) Provider() projects.Provider { return projects.ProviderCodeCommit }
func (codeCommitStrategy) Match(raw string) bool {
	_, ok := gitx.ParseRemote(raw)
	return ok
}
func (codeCommitStrategy) Binding() api.BindingSource { return "" }
func (codeCommitStrategy) AWSAccounts() bool          { return true }

// hostStrategy matches a fixed set of host names (and nothing else).
type hostStrategy struct {
	provider projects.Provider
	hosts    map[string]bool
}

func (s hostStrategy) Provider() projects.Provider { return s.provider }
func (s hostStrategy) Match(raw string) bool {
	for _, h := range remoteHostsOf([]string{raw}) {
		if s.hosts[h] {
			return true
		}
	}
	return false
}
func (hostStrategy) Binding() api.BindingSource { return api.BindingMachine }
func (hostStrategy) AWSAccounts() bool          { return false }

type otherStrategy struct{}

func (otherStrategy) Provider() projects.Provider { return projects.ProviderOther }
func (otherStrategy) Match(string) bool           { return true }
func (otherStrategy) Binding() api.BindingSource  { return api.BindingMachine }
func (otherStrategy) AWSAccounts() bool           { return false }

// strategies is tried in order; otherStrategy matches everything and comes last.
var strategies = []Strategy{
	codeCommitStrategy{},
	hostStrategy{projects.ProviderGitHub, map[string]bool{"github.com": true, "www.github.com": true, "ssh.github.com": true}},
	hostStrategy{projects.ProviderBitbucket, map[string]bool{"bitbucket.org": true, "www.bitbucket.org": true, "altssh.bitbucket.org": true}},
	otherStrategy{},
}

// StrategyFor returns the strategy of a remote URL ("" gives the "other" strategy).
func StrategyFor(rawURL string) Strategy {
	for _, s := range strategies {
		if s.Match(strings.TrimSpace(rawURL)) {
			return s
		}
	}
	return otherStrategy{}
}

// ProviderFor is the provider of a remote URL.
func ProviderFor(rawURL string) projects.Provider { return StrategyFor(rawURL).Provider() }

// remoteNameOf is the last segment of a remote URL without ".git" ("" for no URL): "acme_api" for
// https://example.com/org/acme_api.git, git@example.com:org/acme_api, codecommit::us-east-1://p@acme_api and a
// local path.
func remoteNameOf(rawURL string) string {
	s := strings.TrimSpace(rawURL)
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimRight(s, `/\`)
	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimRight(s, `/\`)
	if i := strings.LastIndexAny(s, `/\:@`); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// remoteKindOf tells https from ssh from anything else for a non-CodeCommit URL.
func remoteKindOf(rawURL string) api.RemoteKind {
	raw := strings.TrimSpace(rawURL)
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Host != "" {
		switch strings.ToLower(u.Scheme) {
		case "https", "http":
			return api.RemoteHTTPS
		case "ssh", "git+ssh", "ssh+git":
			return api.RemoteSSH
		}
		return api.RemoteOther
	}
	if scpRemoteRE.MatchString(raw) && !strings.Contains(raw, "://") && !isWindowsPath(raw) {
		return api.RemoteSSH
	}
	return api.RemoteOther
}

func isWindowsPath(s string) bool {
	return len(s) >= 2 && s[1] == ':' && (s[0] >= 'a' && s[0] <= 'z' || s[0] >= 'A' && s[0] <= 'Z') && (len(s) == 2 || s[2] == '\\' || s[2] == '/')
}
