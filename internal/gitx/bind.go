package gitx

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/execx"
	"github.com/rudian/aws_sso_hub/internal/redact"
)

// BindSpec describes one repository binding. Hosts are bare CodeCommit host names
// (git-codecommit.<region>.amazonaws.com); a leading "https://" is accepted.
type BindSpec struct {
	RepoDir, Profile, AccountID, AWSPath, Lang, GOOS string
	Hosts                                            []string
}

func hostURL(h string) string {
	if strings.HasPrefix(h, "https://") {
		return h
	}
	return "https://" + h
}

func (s BindSpec) validate() (awsHelper, fallback string, hosts []string, err error) {
	awsHelper, err = AWSHelperValue(s.AWSPath, s.Profile, s.GOOS)
	if err != nil {
		return "", "", nil, err
	}
	if len(s.Hosts) == 0 {
		return "", "", nil, api.NewError(api.ErrInvalidInput, "no CodeCommit host to bind").WithParam("field", "hosts")
	}
	seen := map[string]bool{}
	for _, h := range s.Hosts {
		u := hostURL(strings.ToLower(strings.TrimSpace(h)))
		if strings.ContainsAny(u, " \"'`$\\") {
			return "", "", nil, unsafeErr(h)
		}
		if !seen[u] {
			seen[u] = true
			hosts = append(hosts, u)
		}
	}
	sort.Strings(hosts)
	if s.AccountID != "" && !isAccountID(s.AccountID) {
		return "", "", nil, api.NewError(api.ErrInvalidInput, "invalid account id").WithParam("field", "accountId")
	}
	return awsHelper, FallbackValue(s.Profile, s.Lang, s.GOOS), hosts, nil
}

func isAccountID(s string) bool {
	if len(s) != 12 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// BindArgs returns the unconditional `git config` invocations (arguments after `-C <repo>`) of the bind
// sequence: the optional legacy-helper removal when legacyPlain, then the host-scoped helper, fallback and
// marker settings. Bind additionally performs the conditional clean-up of the plain local section.
func BindArgs(spec BindSpec, legacyPlain bool) ([][]string, error) {
	plan, err := makeBindPlan(spec, legacyPlain)
	if err != nil {
		return nil, err
	}
	steps := plan.all()
	out := make([][]string, len(steps))
	for i, s := range steps {
		out[i] = s.args
	}
	return out, nil
}

type step struct {
	args []string
	ok   []int // non-zero exit codes that are fine
}

// bindPlan is the bind sequence split around the conditional legacy clean-up.
type bindPlan struct{ pre, post []step }

func (p bindPlan) all() []step { return append(append([]step(nil), p.pre...), p.post...) }

func makeBindPlan(spec BindSpec, legacyPlain bool) (bindPlan, error) {
	aws, fb, hosts, err := spec.validate()
	if err != nil {
		return bindPlan{}, err
	}
	var p bindPlan
	for _, h := range hosts {
		p.pre = append(p.pre, step{[]string{"config", "--local", "--remove-section", "credential." + h}, []int{128}})
	}
	if legacyPlain {
		p.pre = append(p.pre, step{[]string{"config", "--local", "--unset-all", "credential.helper", "codecommit[[:space:]]+credential-helper"}, []int{5}})
	}
	for _, h := range hosts {
		k := "credential." + h
		p.post = append(p.post,
			step{args: []string{"config", "--local", k + ".helper", ""}},
			step{args: []string{"config", "--local", "--add", k + ".helper", aws}},
			step{args: []string{"config", "--local", "--add", k + ".helper", fb}},
			step{args: []string{"config", "--local", k + ".useHttpPath", "true"}},
			step{args: []string{"config", "--local", k + ".interactive", "false"}},
		)
	}
	p.post = append(p.post, step{[]string{"config", "--local", "--remove-section", "awsssohub"}, []int{128}},
		step{args: []string{"config", "--local", "awsssohub.profile", spec.Profile}})
	if spec.AccountID != "" {
		p.post = append(p.post, step{args: []string{"config", "--local", "awsssohub.accountId", spec.AccountID}})
	}
	p.post = append(p.post, step{args: []string{"config", "--local", "awsssohub.version", "1"}})
	return p, nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// DisplayCommands renders the writing steps (3 to 11) as `git config --local ...` lines for the preview.
func DisplayCommands(spec BindSpec) []string {
	plan, err := makeBindPlan(spec, false)
	if err != nil {
		return []string{}
	}
	out := make([]string, 0, len(plan.post))
	for _, s := range plan.post {
		if len(s.ok) > 0 { // removals are not shown
			continue
		}
		a := s.args[1:] // drop "config"
		var b strings.Builder
		b.WriteString("git config")
		for _, x := range a[:len(a)-1] {
			b.WriteString(" " + x)
		}
		b.WriteString(" " + shellQuote(a[len(a)-1]))
		out = append(out, b.String())
	}
	return out
}

func (g *Git) runSteps(ctx context.Context, dir string, steps []step) error {
	for _, s := range steps {
		if _, err := g.runOK(ctx, dir, nil, s.ok, s.args...); err != nil {
			return err
		}
	}
	return nil
}

// Bind writes the host-scoped credential section and the awsssohub marker. The caller backs up
// .git/config first.
func (g *Git) Bind(ctx context.Context, spec BindSpec) error {
	entries, err := g.CredentialEntries(ctx, spec.RepoDir)
	if err != nil {
		return err
	}
	legacy := hasLocalPlainAWS(entries)
	plan, err := makeBindPlan(spec, legacy)
	if err != nil {
		return err
	}
	if err := g.runSteps(ctx, spec.RepoDir, plan.pre); err != nil {
		return err
	}
	if legacy {
		if err := g.cleanLegacyPlain(ctx, spec.RepoDir); err != nil {
			return err
		}
	}
	return g.runSteps(ctx, spec.RepoDir, plan.post)
}

func hasLocalPlainAWS(entries []Entry) bool {
	for _, e := range entries {
		if e.Scope == "local" && e.Subsection == "" && e.Key == "credential.helper" && ParseAWSHelper(e.Value).IsAWS {
			return true
		}
	}
	return false
}

// cleanLegacyPlain finishes the legacy removal: after the aws helper lines were removed from the plain local section,
// drop the leftover reset/usehttppath and the empty section, but keep unrelated helpers such as `store`.
func (g *Git) cleanLegacyPlain(ctx context.Context, dir string) error {
	res, err := g.runOK(ctx, dir, nil, []int{1}, "config", "--local", "--get-all", "credential.helper")
	if err != nil {
		return err
	}
	remaining := false
	for _, l := range strings.Split(string(res.Stdout), "\n") {
		if strings.TrimSpace(l) != "" {
			remaining = true
		}
	}
	if !remaining {
		if _, err := g.runOK(ctx, dir, nil, []int{5}, "config", "--local", "--unset-all", "credential.helper", "^$"); err != nil {
			return err
		}
		if _, err := g.runOK(ctx, dir, nil, []int{5}, "config", "--local", "--unset-all", "credential.usehttppath"); err != nil {
			return err
		}
	}
	res, err = g.runOK(ctx, dir, nil, []int{1}, "config", "--local", "--get-regexp", `^credential\.[^.]+$`)
	if err != nil {
		return err
	}
	if res.ExitCode == 1 {
		if _, err := g.runOK(ctx, dir, nil, []int{128}, "config", "--local", "--remove-section", "credential"); err != nil {
			return err
		}
	}
	return nil
}

// Unbind removes every local managed credential section and the awsssohub marker. The caller backs up first.
func (g *Git) Unbind(ctx context.Context, repoDir string) error {
	entries, err := g.CredentialEntries(ctx, repoDir)
	if err != nil {
		return err
	}
	managed := map[string]bool{}
	var subs []string
	for _, e := range entries {
		if e.Scope != "local" || e.Subsection == "" || e.Var != "helper" {
			continue
		}
		if ParseAWSHelper(e.Value).IsAWS || IsFallback(e.Value) {
			if !managed[e.Subsection] {
				managed[e.Subsection] = true
				subs = append(subs, e.Subsection)
			}
		}
	}
	for _, s := range subs {
		if _, err := g.runOK(ctx, repoDir, nil, []int{128}, "config", "--local", "--remove-section", "credential."+s); err != nil {
			return err
		}
	}
	if hasLocalPlainAWS(entries) {
		if _, err := g.runOK(ctx, repoDir, nil, []int{5}, "config", "--local", "--unset-all", "credential.helper", "codecommit[[:space:]]+credential-helper"); err != nil {
			return err
		}
		if err := g.cleanLegacyPlain(ctx, repoDir); err != nil {
			return err
		}
	}
	_, err = g.runOK(ctx, repoDir, nil, []int{128}, "config", "--local", "--remove-section", "awsssohub")
	return err
}

// ConfigPath returns the absolute path of the repository's config file (worktree-safe).
func (g *Git) ConfigPath(ctx context.Context, repoDir string) (string, error) {
	res, err := g.runOK(ctx, repoDir, nil, nil, "rev-parse", "--git-path", "config")
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(string(res.Stdout))
	if !filepath.IsAbs(p) {
		p = filepath.Join(repoDir, p)
	}
	return filepath.Clean(p), nil
}

var remoteNameRE = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

// ValidRemoteName reports whether name is safe to pass to git as a remote name: the usual name characters and
// no leading "-" (which git would read as an option).
func ValidRemoteName(name string) bool {
	return remoteNameRE.MatchString(name) && !strings.HasPrefix(name, "-")
}

// SetRemoteURL changes the fetch (or push) URL of a remote.
func (g *Git) SetRemoteURL(ctx context.Context, repoDir, remote, url string, push bool) error {
	if !ValidRemoteName(remote) {
		return api.NewError(api.ErrInvalidInput, "invalid remote name").WithParam("field", "remote")
	}
	args := []string{"remote", "set-url"}
	if push {
		args = append(args, "--push")
	}
	_, err := g.runOK(ctx, repoDir, nil, nil, append(args, "--", remote, url)...)
	return err
}

// CloneSpec describes `git clone` with the binding applied through -c options.
type CloneSpec struct {
	URL, Dest string
	Bind      BindSpec
}

// CloneArgs builds the argv (without the git binary) of the bound clone.
func CloneArgs(spec CloneSpec) ([]string, error) {
	aws, fb, hosts, err := spec.Bind.validate()
	if err != nil {
		return nil, err
	}
	k := "credential." + hosts[0]
	args := []string{"clone", "--progress",
		"-c", k + ".helper=",
		"-c", k + ".helper=" + aws,
		"-c", k + ".helper=" + fb,
		"-c", k + ".useHttpPath=true",
		"-c", k + ".interactive=false",
		"-c", "awsssohub.profile=" + spec.Bind.Profile,
	}
	if spec.Bind.AccountID != "" {
		args = append(args, "-c", "awsssohub.accountId="+spec.Bind.AccountID)
	}
	// "--" keeps a URL or destination that starts with "-" from being read as an option.
	args = append(args, "-c", "awsssohub.version=1", "--", spec.URL, spec.Dest)
	return args, nil
}

// Clone runs the bound clone and reports git's progress lines (split on \r and \n, empty lines dropped).
func (g *Git) Clone(ctx context.Context, spec CloneSpec, progress func(line string)) error {
	args, err := CloneArgs(spec)
	if err != nil {
		return err
	}
	cmd := g.command(ctx, "", args...)
	stderr, perr := cmd.StderrPipe()
	if perr != nil {
		return api.NewError(api.ErrCloneFailed, "cannot start git clone").WithDetail(perr.Error())
	}
	if err := cmd.Start(); err != nil {
		return api.NewError(api.ErrCloneFailed, "cannot start git clone").WithDetail(err.Error())
	}
	var all bytes.Buffer
	sc := bufio.NewScanner(io.TeeReader(stderr, &all))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	sc.Split(splitCRLF)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" && progress != nil {
			progress(redact.String(line))
		}
	}
	_, _ = io.Copy(io.Discard, stderr)
	if werr := cmd.Wait(); werr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return api.NewError(api.ErrCloneFailed, "git clone failed").WithDetail(redact.Tail(all.String(), 3))
	}
	return nil
}

func splitCRLF(data []byte, atEOF bool) (int, []byte, error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// LsRemote lists branch heads of target (a URL). It runs outside any repository. extraConfig entries become
// `-c key=value` options. refs maps full ref names ("refs/heads/main") to SHAs.
func (g *Git) LsRemote(ctx context.Context, target string, extraConfig []string, timeout time.Duration) (map[string]string, execx.Result, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	var args []string
	for _, c := range extraConfig {
		args = append(args, "-c", c)
	}
	args = append(args, "ls-remote", "--heads", "--", target)
	res, err := g.Run(ctx, "", nil, args...)
	refs := map[string]string{}
	if err != nil {
		return refs, res, err
	}
	if res.ExitCode == 0 {
		for _, l := range strings.Split(string(res.Stdout), "\n") {
			f := strings.Fields(l)
			if len(f) == 2 {
				refs[f[1]] = f[0]
			}
		}
	}
	return refs, res, nil
}

// TestRemoteConfig returns the `-c` settings of a connection test against the CodeCommit HTTPS remote info:
// only https is allowed, certificates are verified, nothing prompts, and the only credential helper is the aws
// one for profile. The test runs outside the repository, so its local config (helpers, URL rewrites, proxy
// settings, ext:: transports) never applies.
func TestRemoteConfig(info RemoteInfo, profile, awsPath, goos string) ([]string, error) {
	helper, err := AWSHelperValue(awsPath, profile, goos)
	if err != nil {
		return nil, err
	}
	k := "credential." + hostURL(info.Host)
	return []string{
		"protocol.allow=never",
		"protocol.https.allow=always",
		"http.sslVerify=true",
		"core.askPass=",
		"credential.interactive=false",
		k + ".helper=",
		k + ".helper=" + helper,
		k + ".useHttpPath=true",
	}, nil
}

// TestRemote runs `ls-remote --heads` against the canonical HTTPS URL of a CodeCommit remote with the aws
// credential helper of profile, and classifies the outcome. The string is the redacted three-line stderr tail.
// Anything that is not an HTTPS CodeCommit remote is refused without running git.
func (g *Git) TestRemote(ctx context.Context, remoteURL, profile, awsPath, goos string, timeout time.Duration) (api.TestStatus, string) {
	info, ok := ParseRemote(remoteURL)
	if !ok || info.Kind != api.RemoteHTTPS {
		return api.TestError, "Only HTTPS CodeCommit remotes can be tested."
	}
	cfg, err := TestRemoteConfig(info, profile, awsPath, goos)
	if err != nil {
		return api.TestError, "The AWS CLI path or profile name cannot be used in a credential helper."
	}
	_, res, err := g.LsRemote(ctx, info.HTTPSURL, cfg, timeout)
	timedOut := err != nil
	if timedOut && ctx.Err() != nil {
		return api.TestError, ""
	}
	return ClassifyTestStderr(res.ExitCode, string(res.Stderr), timedOut), redact.Tail(string(res.Stderr), 3)
}

// ClassifyTestStderr maps the result of an ls-remote test to a status.
func ClassifyTestStderr(exit int, stderr string, timedOut bool) api.TestStatus {
	if timedOut {
		return api.TestTimeout
	}
	if exit == 0 {
		return api.TestOK
	}
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(stderr, s) {
				return true
			}
		}
		return false
	}
	low := strings.ToLower(stderr)
	switch {
	case has("told us to quit", "Error loading SSO Token", "Token has expired", "SSO session associated with this profile has expired",
		"Unable to locate credentials", "could not be found"):
		return api.TestSignedOut
	case has("returned error: 403"):
		return api.TestAuth
	case has("returned error: 404") || strings.Contains(low, "not found") || has("does not exist"):
		return api.TestNotFound
	case has("Could not resolve host", "Failed to connect", "Connection timed out", "Connection refused", "SSL"):
		return api.TestNetwork
	}
	return api.TestError
}

// RemoteTrackingRefs maps branch names ("main", "feature/x") to the SHA of refs/remotes/<remote>/<branch>.
func (g *Git) RemoteTrackingRefs(ctx context.Context, repoDir, remote string) (map[string]string, error) {
	prefix := "refs/remotes/" + remote + "/"
	res, err := g.runOK(ctx, repoDir, nil, nil, "for-each-ref", "--format=%(objectname) %(refname)", prefix)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, l := range strings.Split(string(res.Stdout), "\n") {
		f := strings.Fields(l)
		if len(f) != 2 || !strings.HasPrefix(f[1], prefix) {
			continue
		}
		name := strings.TrimPrefix(f[1], prefix)
		if name == "HEAD" {
			continue
		}
		out[name] = f[0]
	}
	return out, nil
}
