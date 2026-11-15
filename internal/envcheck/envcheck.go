// Package envcheck inspects the tools the app depends on (AWS CLI, git, git-remote-codecommit) and scans
// shell start-up files and the process environment for long-lived AWS keys. It never reads or returns the
// content of a scanned line.
package envcheck

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/execx"
)

const (
	AWSMinBlocking = "2.9.0"
	AWSMin         = "2.22.0"
	AWSRecommended = "2.38.2"
	GitMin         = "2.39.1"
)

var (
	reAWSVersion = regexp.MustCompile(`aws-cli/(\d+)\.(\d+)\.(\d+)`)
	reGitVersion = regexp.MustCompile(`git version (\d+)\.(\d+)\.(\d+)`)
)

func atoi3(m []string) [3]int {
	var v [3]int
	for i := 0; i < 3; i++ {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return v
}

// ParseAWSVersion parses `aws --version` output (stdout and stderr together). major is v[0].
func ParseAWSVersion(out string) (v [3]int, major int, ok bool) {
	m := reAWSVersion.FindStringSubmatch(out)
	if m == nil {
		return [3]int{}, 0, false
	}
	v = atoi3(m)
	return v, v[0], true
}

// ParseGitVersion parses `git --version` output, accepting "(Apple Git-157)" and ".windows.1" suffixes.
func ParseGitVersion(out string) ([3]int, bool) {
	m := reGitVersion.FindStringSubmatch(out)
	if m == nil {
		return [3]int{}, false
	}
	return atoi3(m), true
}

// Compare returns -1, 0 or 1.
func Compare(a, b [3]int) int {
	for i := 0; i < 3; i++ {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	return 0
}

func mustVersion(s string) [3]int {
	var v [3]int
	for i, p := range strings.Split(s, ".") {
		v[i], _ = strconv.Atoi(p)
	}
	return v
}

// AWSCLIStatus classifies an AWS CLI version: v1, too-old (< 2.9.0), outdated (< 2.22.0) or ok.
func AWSCLIStatus(v [3]int, major int) api.ToolStatus {
	switch {
	case major == 1:
		return api.ToolV1
	case major < 2 || Compare(v, mustVersion(AWSMinBlocking)) < 0:
		return api.ToolTooOld
	case Compare(v, mustVersion(AWSMin)) < 0:
		return api.ToolOutdated
	}
	return api.ToolOK
}

func versionString(v [3]int) string {
	return strconv.Itoa(v[0]) + "." + strconv.Itoa(v[1]) + "." + strconv.Itoa(v[2])
}

func runVersion(ctx context.Context, bin string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := execx.Run(execx.Command(ctx, bin, "--version"), nil)
	if err != nil {
		return "", err
	}
	return string(res.Stdout) + "\n" + string(res.Stderr), nil
}

// CheckAWSCLI locates and classifies the AWS CLI.
func CheckAWSCLI(ctx context.Context, override string) api.ToolInfo {
	info := api.ToolInfo{Name: "aws", Min: AWSMin, Recommended: AWSRecommended}
	path, err := execx.FindAWS(override)
	if err != nil {
		info.Status = api.ToolMissing
		return info
	}
	info.Path = path
	out, err := runVersion(ctx, path)
	if err != nil {
		info.Status = api.ToolError
		return info
	}
	v, major, ok := ParseAWSVersion(out)
	if !ok {
		info.Status = api.ToolError
		return info
	}
	info.Version = versionString(v)
	info.Status = AWSCLIStatus(v, major)
	return info
}

// CheckGit locates and classifies git.
func CheckGit(ctx context.Context, override string) api.ToolInfo {
	info := api.ToolInfo{Name: "git", Min: GitMin}
	path, err := execx.FindGit(ctx, override)
	if err != nil {
		info.Status = api.ToolMissing
		return info
	}
	info.Path = path
	out, err := runVersion(ctx, path)
	if err != nil {
		info.Status = api.ToolError
		return info
	}
	v, ok := ParseGitVersion(out)
	if !ok {
		info.Status = api.ToolError
		return info
	}
	info.Version = versionString(v)
	info.Status = api.ToolOK
	if Compare(v, mustVersion(GitMin)) < 0 {
		info.Status = api.ToolTooOld
	}
	return info
}

// CheckGRC reports whether git-remote-codecommit is installed (informational).
func CheckGRC(ctx context.Context) api.ToolInfo {
	info := api.ToolInfo{Name: "git-remote-codecommit"}
	name := "git-remote-codecommit"
	if p, err := execx.LookPath(name); err == nil {
		info.Path, info.Status = p, api.ToolOK
	} else {
		info.Status = api.ToolMissing
	}
	return info
}

var reShellExport = regexp.MustCompile(`(?i)^\s*(export\s+|set\s+-[a-z]*x[a-z]*\s+|setenv\s+|\$env:)?AWS_(ACCESS_KEY_ID|SECRET_ACCESS_KEY|SESSION_TOKEN)\b\s*[= ]`)

func shellFiles(goos string) []string {
	if goos == "windows" {
		return []string{
			"Documents/PowerShell/Microsoft.PowerShell_profile.ps1",
			"Documents/WindowsPowerShell/Microsoft.PowerShell_profile.ps1",
		}
	}
	return []string{".zshrc", ".zprofile", ".zshenv", ".bashrc", ".bash_profile", ".profile", ".config/fish/config.fish"}
}

// ScanShellFiles looks for lines that export long-lived AWS keys. Findings carry the display path and the line
// number only; the line text is never kept.
func ScanShellFiles(home, goos string) []api.HealthFinding {
	out := make([]api.HealthFinding, 0)
	sep := "/"
	if goos == "windows" {
		sep = `\`
	}
	for _, rel := range shellFiles(goos) {
		f, err := os.Open(filepath.Join(home, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		display := "~" + sep + strings.ReplaceAll(rel, "/", sep)
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
		line := 0
		for sc.Scan() {
			line++
			text := sc.Text()
			if t := strings.TrimSpace(text); strings.HasPrefix(t, "#") {
				continue
			}
			if reShellExport.MatchString(text) {
				out = append(out, api.HealthFinding{
					ID: "shell-export:" + display + ":" + strconv.Itoa(line), Kind: api.HealthShellExport,
					Severity: api.SeverityDanger, File: display, Line: line,
				})
			}
		}
		f.Close()
	}
	return out
}

// EnvVarFindings flags long-lived AWS credentials present in the process environment.
func EnvVarFindings(environ []string) []api.HealthFinding {
	out := make([]api.HealthFinding, 0)
	for _, name := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"} {
		for _, kv := range environ {
			k, v, ok := strings.Cut(kv, "=")
			if ok && strings.EqualFold(k, name) && v != "" {
				out = append(out, api.HealthFinding{
					ID: "env-var:" + name, Kind: api.HealthEnvVar, Severity: api.SeverityDanger,
					Params: map[string]string{"name": name},
				})
				break
			}
		}
	}
	return out
}
