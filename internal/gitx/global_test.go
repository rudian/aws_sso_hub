package gitx_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/gitx/gitxtest"
)

const realLayout = "[credential]\n\thelper = \n\thelper = !aws codecommit credential-helper $@ --profile acme\n\tusehttppath = true\n" +
	"[credential \"https://github.com\"]\n\thelper = \n\thelper = !gh auth git-credential\n" +
	"[credential \"https://git-codecommit.us-west-2.amazonaws.com/v1/repos/initech_infra\"]\n\thelper = \n\thelper = !aws --profile initech-admin codecommit credential-helper $@\n"

func readGlobal(t *testing.T, sb *gitxtest.Sandbox) string {
	t.Helper()
	b, err := os.ReadFile(sb.GlobalConfig)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestGlobalEntriesAndOps(t *testing.T) {
	ctx := context.Background()
	sb := gitxtest.New(t)
	sb.WriteGlobal(t, realLayout)
	es, err := sb.Git.GlobalEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 7 {
		t.Fatalf("entries: %d %+v", len(es), es)
	}
	if err := sb.Git.RemoveGlobalAWSDefault(ctx); err != nil {
		t.Fatal(err)
	}
	g := readGlobal(t, sb)
	if strings.Contains(g, "--profile acme") || !strings.Contains(g, "usehttppath = true") || !strings.Contains(g, "--profile initech-admin") {
		t.Errorf("after RemoveGlobalAWSDefault:\n%s", g)
	}
	if err := sb.Git.InstallGuard(ctx, gitx.GuardPatterns([]string{host}), gitx.GuardValue("en", "darwin")); err != nil {
		t.Fatal(err)
	}
	if err := sb.Git.InstallGuard(ctx, gitx.GuardPatterns([]string{host}), gitx.GuardValue("en", "darwin")); err != nil { // idempotent
		t.Fatal(err)
	}
	g = readGlobal(t, sb)
	if strings.Count(g, `[credential "https://git-codecommit.*.amazonaws.com"]`) != 1 || !strings.Contains(g, "not bound to an AWS account") {
		t.Errorf("guard:\n%s", g)
	}
	es, _ = sb.Git.GlobalEntries(ctx)
	eff := gitx.Resolve(es, mustCred(t, "https://"+host+"/v1/repos/other"))
	if len(eff.Helpers) != 1 || !gitx.IsGuard(eff.Helpers[0].Value) {
		t.Errorf("guard must win for an unbound repo: %+v", eff.Helpers)
	}
}

func TestAddGlobalDefaultAndRemoveURLRule(t *testing.T) {
	ctx := context.Background()
	sb := gitxtest.New(t)
	sb.WriteGlobal(t, realLayout)
	if err := sb.Git.RemoveGlobalAWSDefault(ctx); err != nil {
		t.Fatal(err)
	}
	aws, _ := gitx.AWSHelperValue("/opt/homebrew/bin/aws", "initech-admin", "darwin")
	if err := sb.Git.AddGlobalDefault(ctx, aws, false); err != nil {
		t.Fatal(err)
	}
	g := readGlobal(t, sb)
	if !strings.Contains(g, "--profile 'initech-admin'") || strings.Count(g, "usehttppath") != 1 {
		t.Errorf("default:\n%s", g)
	}
	pattern := "https://git-codecommit.us-west-2.amazonaws.com/v1/repos/initech_infra"
	if err := sb.Git.RemoveURLRule(ctx, pattern, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readGlobal(t, sb), "initech_infra") {
		t.Errorf("rule not removed:\n%s", readGlobal(t, sb))
	}
	// keepOtherKeys only removes helpers
	sb.WriteGlobal(t, "[credential \""+pattern+"\"]\n\thelper = \n\thelper = !aws --profile x codecommit credential-helper $@\n\tusehttppath = true\n")
	if err := sb.Git.RemoveURLRule(ctx, pattern, true); err != nil {
		t.Fatal(err)
	}
	g = readGlobal(t, sb)
	if strings.Contains(g, "helper") || !strings.Contains(g, "usehttppath = true") {
		t.Errorf("keepOtherKeys:\n%s", g)
	}
	// AddGlobalDefault with reset on an empty file
	sb.WriteGlobal(t, "")
	if err := sb.Git.AddGlobalDefault(ctx, aws, true); err != nil {
		t.Fatal(err)
	}
	g = readGlobal(t, sb)
	if !strings.Contains(g, "helper = \n\thelper = !") {
		t.Errorf("reset missing:\n%s", g)
	}
}

func TestGlobalConfigPath(t *testing.T) {
	sb := gitxtest.New(t)
	home := sb.Home
	if got := sb.Git.GlobalConfigPath(home, "/override"); got != "/override" {
		t.Errorf("override: %s", got)
	}
	if got := sb.Git.GlobalConfigPath(home, ""); got != sb.GlobalConfig {
		t.Errorf("GIT_CONFIG_GLOBAL from env: %s", got)
	}
	plain := &gitx.Git{Bin: sb.Git.Bin}
	t.Setenv("GIT_CONFIG_GLOBAL", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if got := plain.GlobalConfigPath(home, ""); got != filepath.Join(home, ".gitconfig") {
		t.Errorf("default: %s", got)
	}
	xdg := filepath.Join(home, ".config", "git", "config")
	if err := os.MkdirAll(filepath.Dir(xdg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(xdg, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := plain.GlobalConfigPath(home, ""); got != xdg {
		t.Errorf("xdg: %s", got)
	}
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := plain.GlobalConfigPath(home, ""); got != filepath.Join(home, ".gitconfig") {
		t.Errorf("dotfile wins: %s", got)
	}
}

func TestNewRejectsMissingGit(t *testing.T) {
	_, err := gitx.New(context.Background(), filepath.Join(t.TempDir(), "nogit"), nil)
	if err == nil {
		t.Error("expected E_GIT_MISSING")
	}
}
