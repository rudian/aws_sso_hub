// Package browser opens a URL in a private (incognito) window of the user's default web browser, so that an
// old web session of the browser is never reused for a sign-in. It never uses a shell: the URL is passed as
// one argument to a program launched by absolute path.
package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rudian/codepass/internal/execx"
)

// ErrInvalidURL is returned for anything that is not an absolute https URL with a host.
var ErrInvalidURL = errors.New("browser: only absolute https URLs can be opened")

// Opener detects the default browser and launches it with its private-window switch. The zero value works
// for production use on the running system; tests inject every field.
type Opener struct {
	GOOS string // "" = runtime.GOOS
	Home string // "" = the user's home directory

	// Output runs a program and returns its stdout. nil = execx.
	Output func(ctx context.Context, name string, args ...string) ([]byte, error)
	// Start launches a program without waiting for it. nil = execx.
	Start func(ctx context.Context, name string, args ...string) error

	// WinDefaultCommand returns the shell\open\command of the default https browser (Windows only).
	// nil = the Windows registry; other systems report an error.
	WinDefaultCommand func() (string, error)
	// WinPolicy reads a DWORD of a browser policy key (HKLM first, then HKCU). nil = the Windows registry.
	WinPolicy func(subkey, name string) (uint32, bool)
}

const (
	plutilPath = "/usr/bin/plutil"
	openPath   = "/usr/bin/open"
	lsPlist    = "Library/Preferences/com.apple.LaunchServices/com.apple.launchservices.secure.plist"
	safariID   = "com.apple.safari"

	managedPrefsDir = "/Library/Managed Preferences"
)

// browserKind describes a browser that has a private-window switch.
type browserKind struct {
	name string
	flag string
	// newInstance: macOS launches with `open -n` (a fresh process receiving the flag).
	newInstance bool
	// noMac: no reliable way to open a private window on macOS. `open --args` is dropped when the app is
	// already running, and a second `-n` Firefox process is not confirmed to forward to the running one, so
	// the caller falls back and says so instead of claiming a private window.
	noMac bool
	// Policy that can switch private browsing off: a Windows registry key (under HKLM or HKCU), the value
	// name, and the macOS managed-preferences key (same name). The value 1 means disabled.
	winPolicy, policyKey string
	// macDomain is the preference domain managed policy is read from on macOS. It is not always the bundle
	// id (Edge: bundle com.microsoft.edgemac, policy domain com.microsoft.Edge).
	macDomain string
}

var (
	chrome   = browserKind{name: "chrome", flag: "--incognito", newInstance: true, macDomain: "com.google.Chrome", winPolicy: `SOFTWARE\Policies\Google\Chrome`, policyKey: "IncognitoModeAvailability"}
	edge     = browserKind{name: "edge", flag: "--inprivate", newInstance: true, macDomain: "com.microsoft.Edge", winPolicy: `SOFTWARE\Policies\Microsoft\Edge`, policyKey: "InPrivateModeAvailability"}
	brave    = browserKind{name: "brave", flag: "--incognito", newInstance: true, macDomain: "com.brave.Browser", winPolicy: `SOFTWARE\Policies\BraveSoftware\Brave`, policyKey: "IncognitoModeAvailability"}
	chromium = browserKind{name: "chromium", flag: "--incognito", newInstance: true}
	vivaldi  = browserKind{name: "vivaldi", flag: "--incognito", newInstance: true}
	firefox  = browserKind{name: "firefox", flag: "--private-window", noMac: true, macDomain: "org.mozilla.firefox", winPolicy: `SOFTWARE\Policies\Mozilla\Firefox`, policyKey: "DisablePrivateBrowsing"}
	opera    = browserKind{name: "opera", flag: "--private", newInstance: true}
)

// macBundles maps a lower-case bundle id (and its beta/dev/canary variants) to a browser.
var macBundles = []struct {
	id   string
	kind browserKind
}{
	{"com.google.chrome", chrome},
	{"com.microsoft.edgemac", edge},
	{"com.brave.browser", brave},
	{"org.chromium.chromium", chromium},
	{"com.vivaldi.vivaldi", vivaldi},
	{"org.mozilla.firefox", firefox},
	{"org.mozilla.firefoxdeveloperedition", firefox},
	{"org.mozilla.nightly", firefox},
	{"com.operasoftware.opera", opera},
}

func macKind(bundleID string) (browserKind, bool) {
	id := strings.ToLower(strings.TrimSpace(bundleID))
	for _, b := range macBundles {
		if id == b.id || strings.HasPrefix(id, b.id+".") {
			return b.kind, true
		}
	}
	return browserKind{}, false
}

// winKind identifies a browser by the base name of its executable (and, for Opera's launcher, its folder).
func winKind(exe string) (browserKind, bool) {
	lower := strings.ToLower(exe)
	base := lower
	if i := strings.LastIndexAny(base, `\/`); i >= 0 {
		base = base[i+1:]
	}
	switch base {
	case "chrome.exe":
		return chrome, true
	case "msedge.exe":
		return edge, true
	case "brave.exe":
		return brave, true
	case "vivaldi.exe":
		return vivaldi, true
	case "firefox.exe":
		return firefox, true
	case "opera.exe":
		return opera, true
	case "launcher.exe":
		if strings.Contains(lower, "opera") {
			return opera, true
		}
	}
	return browserKind{}, false
}

// CheckURL accepts only absolute https URLs with a host and no control characters.
func CheckURL(raw string) error {
	if raw == "" || strings.ContainsAny(raw, "\x00\r\n") {
		return ErrInvalidURL
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return ErrInvalidURL
	}
	return nil
}

// OpenPrivate opens rawURL in a private window of the default browser. (true, nil): opened privately.
// (false, nil): the default browser has no private-window switch (or the system is not supported); nothing
// was launched and the caller falls back. (false, err): detection or launch failed; the caller falls back.
func (o *Opener) OpenPrivate(ctx context.Context, rawURL string) (bool, error) {
	if err := CheckURL(rawURL); err != nil {
		return false, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	goos := o.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	switch goos {
	case "darwin":
		return o.openDarwin(ctx, rawURL)
	case "windows":
		return o.openWindows(ctx, rawURL)
	}
	return false, nil
}

func (o *Opener) output(ctx context.Context, name string, args ...string) ([]byte, error) {
	if o.Output != nil {
		return o.Output(ctx, name, args...)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := execx.Run(execx.Command(ctx, name, args...), nil)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("%s exited with code %d", filepath.Base(name), res.ExitCode)
	}
	return res.Stdout, nil
}

func (o *Opener) start(ctx context.Context, name string, args ...string) error {
	if o.Start != nil {
		return o.Start(ctx, name, args...)
	}
	return execx.Start(name, args, "", nil, false)
}

// ---- macOS ----

type lsHandler struct {
	Scheme  string `json:"LSHandlerURLScheme"`
	RoleAll string `json:"LSHandlerRoleAll"`
}

// defaultMacBundle returns the bundle id registered for https (else http); Safari when there is no entry.
func (o *Opener) defaultMacBundle(ctx context.Context) (string, error) {
	home := o.homeDir()
	if home == "" {
		return "", errors.New("no home directory")
	}
	out, err := o.output(ctx, plutilPath, "-convert", "json", "-o", "-", filepath.Join(home, lsPlist))
	if err != nil {
		return "", fmt.Errorf("cannot read the default browser: %w", err)
	}
	var doc struct {
		Handlers []lsHandler `json:"LSHandlers"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return "", fmt.Errorf("cannot parse the default browser settings: %w", err)
	}
	for _, scheme := range []string{"https", "http"} {
		for _, h := range doc.Handlers {
			if strings.EqualFold(h.Scheme, scheme) && h.RoleAll != "" && h.RoleAll != "-" {
				return h.RoleAll, nil
			}
		}
	}
	return safariID, nil
}

func (o *Opener) openDarwin(ctx context.Context, rawURL string) (bool, error) {
	id, err := o.defaultMacBundle(ctx)
	if err != nil {
		return false, err
	}
	kind, ok := macKind(id)
	if !ok || kind.noMac {
		return false, nil
	}
	if o.macPolicyDisabled(ctx, kind) {
		return false, nil
	}
	args := []string{}
	if kind.newInstance {
		args = append(args, "-n")
	}
	args = append(args, "-b", id, "--args", kind.flag, rawURL)
	// open returns once LaunchServices has launched the app (or failed to find it), so wait for it and
	// check the exit code: a browser that was uninstalled must not be reported as a private window.
	if _, err := o.output(ctx, openPath, args...); err != nil {
		return false, fmt.Errorf("cannot start %s: %w", kind.name, err)
	}
	return true, nil
}

// macPolicyDisabled reports whether a managed preference turns private browsing off for the browser.
func (o *Opener) macPolicyDisabled(ctx context.Context, kind browserKind) bool {
	if kind.policyKey == "" || kind.macDomain == "" {
		return false
	}
	dirs := []string{managedPrefsDir}
	if u := filepath.Base(o.homeDir()); u != "" && u != "." && u != string(filepath.Separator) {
		dirs = append(dirs, filepath.Join(managedPrefsDir, u))
	}
	for _, d := range dirs {
		out, err := o.output(ctx, plutilPath, "-extract", kind.policyKey, "raw", "-o", "-", filepath.Join(d, kind.macDomain+".plist"))
		if err == nil && strings.TrimSpace(string(out)) == "1" {
			return true
		}
	}
	return false
}

func (o *Opener) homeDir() string {
	if o.Home != "" {
		return o.Home
	}
	h, _ := os.UserHomeDir()
	return h
}

// ---- Windows ----

// exeFromCommand returns the executable of a shell\open\command value: a quoted path, else up to ".exe".
func exeFromCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if strings.HasPrefix(cmd, `"`) {
		if j := strings.Index(cmd[1:], `"`); j >= 0 {
			return cmd[1 : 1+j]
		}
		return ""
	}
	if i := strings.Index(strings.ToLower(cmd), ".exe"); i >= 0 {
		return cmd[:i+4]
	}
	return ""
}

// Registry keys (under HKCU) holding the user's https choice. Newer Windows 11 builds with the UserChoice
// hash protection keep the current choice in UserChoiceLatest, so it is read first.
var winChoiceKeys = []string{
	`Software\Microsoft\Windows\Shell\Associations\UrlAssociations\https\UserChoiceLatest`,
	`Software\Microsoft\Windows\Shell\Associations\UrlAssociations\https\UserChoice`,
}

// pickProgID returns the first usable ProgId of winChoiceKeys; read returns the ProgId value of a key.
func pickProgID(read func(key string) (string, error)) (string, error) {
	var last error = errors.New("no ProgId")
	for _, key := range winChoiceKeys {
		id, err := read(key)
		if err != nil {
			last = err
			continue
		}
		if id == "" || strings.ContainsAny(id, `"\/`) {
			last = errors.New("no ProgId")
			continue
		}
		return id, nil
	}
	return "", last
}

// winAbs reports whether p is an absolute Windows path (drive letter or UNC); it is independent of the
// running system so that it can be tested everywhere.
func winAbs(p string) bool {
	if len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') &&
		(p[0] >= 'a' && p[0] <= 'z' || p[0] >= 'A' && p[0] <= 'Z') {
		return true
	}
	return strings.HasPrefix(p, `\\`)
}

func (o *Opener) openWindows(ctx context.Context, rawURL string) (bool, error) {
	read := o.WinDefaultCommand
	if read == nil {
		read = winDefaultCommand
	}
	cmd, err := read()
	if err != nil {
		return false, fmt.Errorf("cannot read the default browser: %w", err)
	}
	exe := exeFromCommand(cmd)
	if exe == "" {
		return false, errors.New("cannot find the default browser program")
	}
	kind, ok := winKind(exe)
	if !ok {
		return false, nil
	}
	if !winAbs(exe) {
		// Never launch a program found through the search path or the working directory.
		return false, nil
	}
	if kind.policyKey != "" {
		policy := o.WinPolicy
		if policy == nil {
			policy = winPolicy
		}
		if v, found := policy(kind.winPolicy, kind.policyKey); found && v == 1 {
			return false, nil
		}
	}
	if err := o.start(ctx, exe, kind.flag, rawURL); err != nil {
		return false, fmt.Errorf("cannot start %s: %w", kind.name, err)
	}
	return true, nil
}
