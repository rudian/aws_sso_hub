package envcheck

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/execx"
	"github.com/rudian/codepass/internal/gitx"
)

func TestParseAWSVersion(t *testing.T) {
	v, major, ok := ParseAWSVersion("aws-cli/2.40.0 Python/3.13.5 Darwin/24.6.0 source/arm64")
	if !ok || v != [3]int{2, 40, 0} || major != 2 {
		t.Errorf("v2: %v %d %v", v, major, ok)
	}
	v, major, ok = ParseAWSVersion("aws-cli/1.32.0 Python/3.11.6 Linux/5.15 botocore/1.34.0")
	if !ok || v != [3]int{1, 32, 0} || major != 1 {
		t.Errorf("v1: %v %d %v", v, major, ok)
	}
	if _, _, ok := ParseAWSVersion("command not found"); ok {
		t.Error("garbage parsed")
	}
}

func TestAWSCLIStatus(t *testing.T) {
	cases := []struct {
		v     [3]int
		major int
		want  api.ToolStatus
	}{
		{[3]int{2, 8, 9}, 2, api.ToolTooOld}, {[3]int{2, 9, 0}, 2, api.ToolOutdated}, {[3]int{2, 21, 9}, 2, api.ToolOutdated},
		{[3]int{2, 22, 0}, 2, api.ToolOK}, {[3]int{2, 40, 0}, 2, api.ToolOK}, {[3]int{1, 32, 0}, 1, api.ToolV1},
	}
	for _, c := range cases {
		if got := AWSCLIStatus(c.v, c.major); got != c.want {
			t.Errorf("%v: got %s want %s", c.v, got, c.want)
		}
	}
}

func TestParseGitVersion(t *testing.T) {
	for in, want := range map[string][3]int{
		"git version 2.54.0 (Apple Git-157)": {2, 54, 0},
		"git version 2.47.0.windows.1":       {2, 47, 0},
		"git version 2.47.1":                 {2, 47, 1},
	} {
		if v, ok := ParseGitVersion(in); !ok || v != want {
			t.Errorf("%q: %v %v", in, v, ok)
		}
	}
	if _, ok := ParseGitVersion("nope"); ok {
		t.Error("garbage parsed")
	}
	if Compare([3]int{2, 26, 0}, [3]int{2, 26, 0}) != 0 || Compare([3]int{2, 9, 0}, [3]int{2, 22, 0}) != -1 || Compare([3]int{3, 0, 0}, [3]int{2, 99, 9}) != 1 {
		t.Error("Compare")
	}
}

func TestScanShellFiles(t *testing.T) {
	home := t.TempDir()
	lines := []string{"# header", "export PATH=$PATH:/x", "echo hi", "# export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE"}
	for len(lines) < 41 {
		lines = append(lines, "alias ll='ls -l'")
	}
	lines = append(lines, "export AWS_SECRET_ACCESS_KEY=fakeSecretValue123", "export AWS_PROFILE=acme")
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".config", "fish"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "fish", "config.fish"), []byte("set -gx AWS_ACCESS_KEY_ID AKIAIOSFODNN7EXAMPLE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := ScanShellFiles(home, "darwin")
	if len(got) != 2 {
		t.Fatalf("findings: %+v", got)
	}
	if f := got[0]; f.Kind != api.HealthShellExport || f.Severity != api.SeverityDanger || f.File != "~/.zshrc" || f.Line != 42 {
		t.Errorf("zshrc finding: %+v", f)
	}
	if f := got[1]; f.File != "~/.config/fish/config.fish" || f.Line != 1 {
		t.Errorf("fish finding: %+v", f)
	}
	b, _ := json.Marshal(got)
	for _, secret := range []string{"fakeSecretValue123", "AKIAIOSF", "AWS_SECRET_ACCESS_KEY", "export AWS"} {
		if strings.Contains(string(b), secret) {
			t.Errorf("DTO leaks part of the line (%q): %s", secret, b)
		}
	}
	if n := len(ScanShellFiles(t.TempDir(), "darwin")); n != 0 {
		t.Errorf("empty home: %d", n)
	}
}

func TestScanShellFilesWindows(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "Documents", "PowerShell")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Microsoft.PowerShell_profile.ps1"), []byte("$env:AWS_ACCESS_KEY_ID = 'x'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := ScanShellFiles(home, "windows")
	if len(got) != 1 || got[0].File != `~\Documents\PowerShell\Microsoft.PowerShell_profile.ps1` || got[0].Line != 1 {
		t.Errorf("%+v", got)
	}
}

func TestEnvVarFindings(t *testing.T) {
	got := EnvVarFindings([]string{"HOME=/x", "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE", "AWS_SECRET_ACCESS_KEY=", "AWS_SESSION_TOKEN=tok", "AWS_PROFILE=p"})
	if len(got) != 2 || got[0].Params["name"] != "AWS_ACCESS_KEY_ID" || got[1].Params["name"] != "AWS_SESSION_TOKEN" || got[0].Severity != api.SeverityDanger {
		t.Errorf("%+v", got)
	}
	b, _ := json.Marshal(got)
	if strings.Contains(string(b), "AKIA") || strings.Contains(string(b), "tok\"") {
		t.Errorf("leak: %s", b)
	}
	if len(EnvVarFindings(nil)) != 0 {
		t.Error("nil environ")
	}
}

func writeFake(t *testing.T, dir, name, out string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho '"+out+"' >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCheckToolsWithFakeBinaries(t *testing.T) {
	dir := t.TempDir()
	execx.Configure(execx.Options{PreferredDirs: []string{dir}, DisableLoginShell: true})
	t.Cleanup(func() { execx.Configure(execx.Options{}) })
	aws := writeFake(t, dir, "aws", "aws-cli/2.21.9 Python/3.12 Darwin/24 source/arm64")
	info := CheckAWSCLI(context.Background(), aws)
	if info.Status != api.ToolOutdated || info.Version != "2.21.9" || info.Path != aws || info.Min != AWSMin || info.Recommended != AWSRecommended {
		t.Errorf("aws: %+v", info)
	}
	if info := CheckAWSCLI(context.Background(), filepath.Join(dir, "missing")); info.Status != api.ToolMissing {
		t.Errorf("missing: %+v", info)
	}
	// An override must be a program named aws or git, so each fake gets its own folder.
	sub := func(name string) string {
		d := filepath.Join(dir, name)
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
		return d
	}
	bad := writeFake(t, sub("garbage"), "aws", "who knows")
	if info := CheckAWSCLI(context.Background(), bad); info.Status != api.ToolError {
		t.Errorf("garbage: %+v", info)
	}
	git := writeFake(t, dir, "git", "git version 2.20.1")
	if info := CheckGit(context.Background(), git); info.Status != api.ToolTooOld || info.Version != "2.20.1" || info.Min != GitMin {
		t.Errorf("git: %+v", info)
	}
	git2 := writeFake(t, sub("newer"), "git", "git version 2.54.0 (Apple Git-157)")
	if info := CheckGit(context.Background(), git2); info.Status != api.ToolOK {
		t.Errorf("git ok: %+v", info)
	}
	if info := CheckGRC(context.Background()); info.Status != api.ToolMissing || info.Name != "git-remote-codecommit" {
		// not installed in the isolated PATH of this test unless inherited PATH has it
		t.Logf("grc: %+v", info)
	}
}

// FU-12d: the gate's minimum git and the git layer's minimum are one number, and the README says it.
func TestGitMinimumIsOneNumberEverywhere(t *testing.T) {
	if GitMin != gitx.MinVersion {
		t.Fatalf("envcheck.GitMin %q, gitx.MinVersion %q", GitMin, gitx.MinVersion)
	}
	for _, f := range []string{"../../README.md", "../../README.zh-CN.md", "../../docs/guide.md", "../../docs/guide.zh-CN.md"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "Git "+GitMin) {
			t.Errorf("%s does not say Git %s", f, GitMin)
		}
	}
}
