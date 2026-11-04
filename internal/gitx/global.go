package gitx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

func envLookup(env []string, key string) (string, bool) {
	val, ok := "", false
	for _, kv := range env {
		if strings.HasPrefix(kv, key+"=") {
			val, ok = kv[len(key)+1:], true
		}
	}
	return val, ok
}

func (g *Git) getenv(key string) string {
	if v, ok := envLookup(g.Env, key); ok {
		return v
	}
	return os.Getenv(key)
}

// GlobalConfigPath returns the file `git config --global` writes: override (sandbox), $GIT_CONFIG_GLOBAL,
// ~/.gitconfig if it exists, $XDG_CONFIG_HOME/git/config if it exists, else ~/.gitconfig.
func (g *Git) GlobalConfigPath(home, override string) string {
	if override != "" {
		return override
	}
	if v := g.getenv("GIT_CONFIG_GLOBAL"); v != "" {
		return v
	}
	dot := filepath.Join(home, ".gitconfig")
	if _, err := os.Stat(dot); err == nil {
		return dot
	}
	xdg := g.getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".config")
	}
	x := filepath.Join(xdg, "git", "config")
	if _, err := os.Stat(x); err == nil {
		return x
	}
	return dot
}

// GuardPatterns returns the credential URL patterns the global guard section must cover for the given hosts.
func GuardPatterns(hosts []string) []string {
	out := []string{"https://git-codecommit.*.amazonaws.com"}
	var fips, china, vpce bool
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimPrefix(h, "https://"))
		f, v, c := hostFlags(h)
		fips, vpce, china = fips || f, vpce || v, china || c
	}
	if fips {
		out = append(out, "https://git-codecommit-fips.*.amazonaws.com")
	}
	if china {
		out = append(out, "https://git-codecommit.*.amazonaws.com.cn")
	}
	if vpce {
		out = append(out, "https://*.git-codecommit.*.vpce.amazonaws.com")
	}
	return out
}

func (g *Git) global(ctx context.Context, ok []int, args ...string) (string, error) {
	res, err := g.runOK(ctx, "", nil, ok, append([]string{"config", "--global"}, args...)...)
	return string(res.Stdout), err
}

// RemoveGlobalAWSDefault removes the global `aws codecommit credential-helper` lines and keeps the plain
// reset and usehttppath.
func (g *Git) RemoveGlobalAWSDefault(ctx context.Context) error {
	_, err := g.global(ctx, []int{5}, "--unset-all", "credential.helper", "codecommit[[:space:]]+credential-helper")
	return err
}

// AddGlobalDefault appends an aws helper as the global default (with a reset in front when addReset).
func (g *Git) AddGlobalDefault(ctx context.Context, awsHelper string, addReset bool) error {
	if addReset {
		if _, err := g.global(ctx, nil, "--add", "credential.helper", ""); err != nil {
			return err
		}
	}
	if _, err := g.global(ctx, nil, "--add", "credential.helper", awsHelper); err != nil {
		return err
	}
	out, err := g.global(ctx, []int{1}, "--get", "credential.usehttppath")
	if err != nil {
		return err
	}
	if !isTrue(out) {
		_, err = g.global(ctx, nil, "credential.useHttpPath", "true")
	}
	return err
}

// InstallGuard writes one host-pattern section per pattern: reset, guard helper, useHttpPath.
func (g *Git) InstallGuard(ctx context.Context, patterns []string, guardValue string) error {
	for _, p := range patterns {
		k := "credential." + p
		if _, err := g.global(ctx, []int{128}, "--remove-section", k); err != nil {
			return err
		}
		if _, err := g.global(ctx, nil, k+".helper", ""); err != nil {
			return err
		}
		if _, err := g.global(ctx, nil, "--add", k+".helper", guardValue); err != nil {
			return err
		}
		if _, err := g.global(ctx, nil, k+".useHttpPath", "true"); err != nil {
			return err
		}
	}
	return nil
}

// RemoveURLRule removes a global URL-scoped rule. keepOtherKeys removes only its helper values.
func (g *Git) RemoveURLRule(ctx context.Context, pattern string, keepOtherKeys bool) error {
	var err error
	if keepOtherKeys {
		_, err = g.global(ctx, []int{5}, "--unset-all", "credential."+pattern+".helper")
	} else {
		_, err = g.global(ctx, []int{128}, "--remove-section", "credential."+pattern)
	}
	return err
}
