// Package paths resolves every location the app reads or writes, including the sandbox layout used for
// manual runs and tests (AWSSSOHUB_SANDBOX=<root>).
package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const AppDirName = "AWS SSO Hub"
const SandboxEnv = "AWSSSOHUB_SANDBOX"

type Paths struct {
	Home, AWSConfigFile, AWSCredentialsFile, SSOCacheDir, CLICacheDir         string
	AppDataDir, SettingsFile, BackupDir, RegistrationsDir, LogDir, WebViewDir string
	DefaultScanRoot                                                           string // <Home>/Documents/code
	GitGlobalFile                                                             string // sandbox only: <root>/home/.gitconfig; "" otherwise (git decides)
	Sandbox                                                                   bool
	SandboxRoot                                                               string
}

// Resolve honours AWSSSOHUB_SANDBOX (sandbox layout, AWS env overrides ignored) in development and sandbox
// builds only, plus AWS_CONFIG_FILE and AWS_SHARED_CREDENTIALS_FILE.
func Resolve() (Paths, error) {
	if root := os.Getenv(SandboxEnv); root != "" && sandboxAllowed {
		abs, err := filepath.Abs(root)
		if err != nil {
			return Paths{}, err
		}
		return ForRoot(abs), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, err
	}
	p := build(home, filepath.Join(cfgDir, AppDirName))
	if v := os.Getenv("AWS_CONFIG_FILE"); v != "" {
		p.AWSConfigFile = v
	}
	if v := os.Getenv("AWS_SHARED_CREDENTIALS_FILE"); v != "" {
		p.AWSCredentialsFile = v
	}
	return p, nil
}

// ForRoot returns the sandbox layout: Home=<root>/home, AppDataDir=<root>/appdata, Sandbox=true.
func ForRoot(root string) Paths {
	p := build(filepath.Join(root, "home"), filepath.Join(root, "appdata"))
	p.GitGlobalFile = filepath.Join(p.Home, ".gitconfig")
	p.Sandbox = true
	p.SandboxRoot = root
	return p
}

func build(home, appData string) Paths {
	return Paths{
		Home:               home,
		AWSConfigFile:      filepath.Join(home, ".aws", "config"),
		AWSCredentialsFile: filepath.Join(home, ".aws", "credentials"),
		SSOCacheDir:        filepath.Join(home, ".aws", "sso", "cache"),
		CLICacheDir:        filepath.Join(home, ".aws", "cli", "cache"),
		AppDataDir:         appData,
		SettingsFile:       filepath.Join(appData, "settings.json"),
		BackupDir:          filepath.Join(appData, "backups"),
		RegistrationsDir:   filepath.Join(appData, "registrations"),
		LogDir:             filepath.Join(appData, "logs"),
		WebViewDir:         filepath.Join(appData, "WebView2"),
		DefaultScanRoot:    filepath.Join(home, "Documents", "code"),
	}
}

// Display returns "~" + rest when abs is under Home (OS separator kept); otherwise abs unchanged.
func (p Paths) Display(abs string) string {
	if p.Home == "" || abs == "" {
		return abs
	}
	home := filepath.Clean(p.Home)
	clean := filepath.Clean(abs)
	eq := func(a, b string) bool { return a == b }
	if runtime.GOOS == "windows" {
		eq = strings.EqualFold
	}
	if eq(clean, home) {
		return "~"
	}
	prefix := home
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	if len(clean) > len(prefix) && eq(clean[:len(prefix)], prefix) {
		return "~" + string(filepath.Separator) + clean[len(prefix):]
	}
	return abs
}

// Expand is the inverse of Display: a leading "~", "~/" (and "~\\" on Windows) becomes Home. Anything else is
// returned unchanged.
func (p Paths) Expand(display string) string {
	if p.Home == "" {
		return display
	}
	if display == "~" {
		return p.Home
	}
	if strings.HasPrefix(display, "~/") || (runtime.GOOS == "windows" && strings.HasPrefix(display, "~\\")) {
		rest := display[2:]
		if runtime.GOOS == "windows" {
			rest = strings.ReplaceAll(rest, "\\", "/")
		}
		return filepath.Join(p.Home, filepath.FromSlash(rest))
	}
	return display
}

// ChildEnv returns the environment overrides for child processes in sandbox mode; nil otherwise.
func (p Paths) ChildEnv() []string {
	if !p.Sandbox {
		return nil
	}
	return []string{
		"HOME=" + p.Home,
		"USERPROFILE=" + p.Home,
		"AWS_CONFIG_FILE=" + p.AWSConfigFile,
		"AWS_SHARED_CREDENTIALS_FILE=" + p.AWSCredentialsFile,
		"GIT_CONFIG_GLOBAL=" + p.GitGlobalFile,
		"GIT_CONFIG_NOSYSTEM=1",
		"XDG_CONFIG_HOME=" + filepath.Join(p.Home, ".config"),
	}
}

// EnsureAppDirs creates AppDataDir, BackupDir, RegistrationsDir and LogDir with mode 0700.
func (p Paths) EnsureAppDirs() error {
	for _, d := range []string{p.AppDataDir, p.BackupDir, p.RegistrationsDir, p.LogDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	return nil
}
