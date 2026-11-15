// Package credstore lists and clears the cached CodeCommit passwords that credential helpers leave in the macOS
// keychain or the Windows Credential Manager. It never reads or logs secret values: output of `security` is
// discarded and only exit codes are used. Commands run through an injectable runner so tests assert argv only.
package credstore

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/execx"
	"github.com/rudian/codepass/internal/gitx"
)

// Store is the platform credential cache.
type Store interface {
	Supported() bool
	List(ctx context.Context, hosts []string) ([]api.SavedCredential, error)
	Clear(ctx context.Context, ids []string) (int, error)
}

// runner executes one command. capture=false discards stdout and stderr entirely.
type runner func(ctx context.Context, name string, args []string, stdin []byte, capture bool) (stdout []byte, exit int, err error)

// run is replaced in tests; they never execute security, cmdkey or git.
var run runner = defaultRun

func defaultRun(ctx context.Context, name string, args []string, stdin []byte, capture bool) ([]byte, int, error) {
	res, err := execx.Run(execx.Command(ctx, name, args...), stdin)
	if err != nil {
		return nil, -1, err
	}
	if !capture {
		return nil, res.ExitCode, nil
	}
	return res.Stdout, res.ExitCode, nil
}

var reHost = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)

// reCodeCommitHost matches the Git hosts of CodeCommit: regional, FIPS and VPC endpoint names, global and China.
var reCodeCommitHost = regexp.MustCompile(`^(vpce-[a-z0-9-]+\.)?git-codecommit(-fips)?\.[a-z0-9-]+\.(vpce\.)?amazonaws\.com(\.cn)?$`)

// IsCodeCommitHost reports whether host is a CodeCommit Git endpoint. Only such hosts are ever cleared.
func IsCodeCommitHost(host string) bool { return reCodeCommitHost.MatchString(strings.ToLower(host)) }

// New returns the store for goos. sandbox disables it (the real keychain must never be touched in sandbox mode).
func New(goos string, git func(ctx context.Context) (*gitx.Git, error), sandbox bool) Store {
	switch {
	case sandbox:
		return &disabled{err: api.NewError(api.ErrSandbox, "saved credentials are not available in sandbox mode")}
	case goos == "darwin":
		return &darwin{git: git}
	case goos == "windows":
		return &windows{}
	}
	return &disabled{err: api.NewError(api.ErrUnsupportedOS, "saved credentials are not supported on this system")}
}

type disabled struct{ err *api.AppError }

func (d *disabled) Supported() bool { return false }
func (d *disabled) List(ctx context.Context, hosts []string) ([]api.SavedCredential, error) {
	return []api.SavedCredential{}, nil
}
func (d *disabled) Clear(ctx context.Context, ids []string) (int, error) { return 0, d.err }

// ---- macOS ----

type darwin struct {
	git func(ctx context.Context) (*gitx.Git, error)
}

func (d *darwin) Supported() bool { return true }

func cleanHosts(hosts []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "https://")))
		if h == "" || seen[h] || !reHost.MatchString(h) {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

func (d *darwin) found(ctx context.Context, host string) bool {
	_, exit, err := run(ctx, "security", []string{"find-internet-password", "-s", host}, nil, false)
	return err == nil && exit == 0
}

func (d *darwin) List(ctx context.Context, hosts []string) ([]api.SavedCredential, error) {
	out := make([]api.SavedCredential, 0)
	for _, h := range cleanHosts(hosts) {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if d.found(ctx, h) {
			out = append(out, api.SavedCredential{ID: "keychain:" + h, Host: h, Store: api.StoreKeychain})
		}
	}
	return out, nil
}

func (d *darwin) Clear(ctx context.Context, ids []string) (int, error) {
	var hosts []string
	for _, id := range ids {
		if h, ok := strings.CutPrefix(id, "keychain:"); ok {
			hosts = append(hosts, h)
		}
	}
	cleared := 0
	for _, h := range cleanHosts(hosts) {
		if !IsCodeCommitHost(h) {
			continue // never touch passwords of other services
		}
		if err := ctx.Err(); err != nil {
			return cleared, err
		}
		if d.git != nil {
			if g, err := d.git(ctx); err == nil && g != nil {
				// Let the helper erase what it stored itself; failures are not fatal.
				_, _, _ = run(ctx, g.Bin, []string{"-c", "credential.helper=", "-c", "credential.helper=osxkeychain", "credential", "reject"},
					[]byte("protocol=https\nhost="+h+"\n\n"), false)
			}
		}
		for i := 0; i < 20; i++ {
			_, exit, err := run(ctx, "security", []string{"delete-internet-password", "-s", h}, nil, false)
			if err != nil || exit != 0 {
				break
			}
		}
		if !d.found(ctx, h) {
			cleared++
		}
	}
	return cleared, nil
}

// ---- Windows ----

type windows struct{}

func (w *windows) Supported() bool { return true }

var reCmdkeyTarget = regexp.MustCompile(`(?i)(?:Target|目标)\s*[:：]\s*(.+?)\s*$`)

// ParseCmdkeyList extracts the stored targets that belong to CodeCommit from `cmdkey /list` output.
func ParseCmdkeyList(out string) []string {
	targets := make([]string, 0)
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		m := reCmdkeyTarget.FindStringSubmatch(line)
		if m == nil || !strings.Contains(m[1], "git-codecommit.") || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		targets = append(targets, m[1])
	}
	return targets
}

func hostAndPath(target string) (host, path string) {
	i := strings.Index(target, "https://")
	if i < 0 {
		return "", ""
	}
	rest := target[i+len("https://"):]
	if at := strings.IndexByte(rest, '@'); at >= 0 && at < strings.IndexAny(rest+"/", "/") {
		rest = rest[at+1:]
	}
	host, path, _ = strings.Cut(rest, "/")
	return strings.ToLower(host), path
}

func (w *windows) List(ctx context.Context, hosts []string) ([]api.SavedCredential, error) {
	stdout, exit, err := run(ctx, "cmdkey", []string{"/list"}, nil, true)
	if err != nil || exit != 0 {
		return []api.SavedCredential{}, nil
	}
	out := make([]api.SavedCredential, 0)
	for _, t := range ParseCmdkeyList(string(stdout)) {
		h, p := hostAndPath(t)
		if !IsCodeCommitHost(h) {
			continue
		}
		out = append(out, api.SavedCredential{ID: "cmdkey:" + t, Host: h, Path: p, Store: api.StoreCredentialManager})
	}
	return out, nil
}

func (w *windows) Clear(ctx context.Context, ids []string) (int, error) {
	// Only targets that are stored right now and belong to a CodeCommit host are deleted.
	stdout, exit, err := run(ctx, "cmdkey", []string{"/list"}, nil, true)
	if err != nil || exit != 0 {
		return 0, nil
	}
	current := map[string]bool{}
	for _, t := range ParseCmdkeyList(string(stdout)) {
		current[t] = true
	}
	cleared := 0
	for _, id := range ids {
		t, ok := strings.CutPrefix(id, "cmdkey:")
		if !ok || !current[t] {
			continue
		}
		if h, _ := hostAndPath(t); !IsCodeCommitHost(h) {
			continue
		}
		_, exit, err := run(ctx, "cmdkey", []string{"/delete:" + t}, nil, false)
		if err == nil && exit == 0 {
			cleared++
		}
	}
	return cleared, nil
}
