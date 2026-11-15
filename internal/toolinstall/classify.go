// Package toolinstall installs the tools the gate needs (plan 4.1, design 5.1): Homebrew.pkg through one
// privileged step on macOS, brew and winget as the user. Every failure ends in one api.FailureCause.
package toolinstall

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/rudian/codepass/internal/api"
)

// Exit codes of the privileged script (script.go).
const (
	ExitOK            = 0
	ExitCopyFailed    = 10
	ExitDiskFull      = 11
	ExitNotSigned     = 12
	ExitWrongTeam     = 13
	ExitNotNotarized  = 14
	ExitSizeMismatch  = 15
	ExitPrefixExists  = 20
	ExitNotRoot       = 21
	ExitInstallFailed = 30
	ExitBrewMissing   = 31
)

var (
	// osascript puts the AppleScript error number last: "... execution error: <text> (13)".
	osaCode = regexp.MustCompile(`\((-?\d+)\)\s*$`)
	// brew prints: "Failure while executing; `... curl ...` exited with 7."
	curlExit = regexp.MustCompile(`curl[^\n]*exited with (\d+)`)
)

// Authorization.h: -60005 denied (wrong name or password), -60006 cancelled, -60007 no interaction possible.
// -128 is AppleScript's "User canceled."
const (
	osaCancelled     = -128
	osaAuthCancelled = -60006
	osaAuthDenied    = -60005
	osaNoInteraction = -60007
)

// ScriptCode extracts the number in the last parentheses of osascript's error text; ok is false when absent.
func ScriptCode(stderr string) (code int, ok bool) {
	m := osaCode.FindStringSubmatch(strings.TrimSpace(stderr))
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// MacPrivileged classifies the osascript call that runs the privileged script. stderr is osascript's error text
// (stdout is discarded by "do shell script" on a non-zero exit, so the script writes every reason to stderr).
func MacPrivileged(exit int, stderr string) api.FailureCause {
	if exit == 0 {
		return ""
	}
	code, ok := ScriptCode(stderr)
	if !ok {
		return api.CauseUnknown
	}
	switch code {
	case osaCancelled, osaAuthCancelled:
		return api.CauseCancelled
	case osaAuthDenied:
		return api.CauseAuthDenied
	case osaNoInteraction:
		return api.CauseNoSession
	case ExitCopyFailed:
		return api.CauseInstallerFailed // a plain copy failure: the log shows why
	case ExitDiskFull:
		return api.CauseDiskFull
	case ExitNotSigned:
		return api.CauseBadSignature
	case ExitWrongTeam:
		return api.CauseUnknownSigner
	case ExitNotNotarized:
		return api.CauseNotNotarized
	case ExitSizeMismatch:
		return api.CauseDamagedDownload
	case ExitPrefixExists:
		return api.CausePrefixExists
	case ExitInstallFailed:
		return macInstallerText(stderr)
	case ExitBrewMissing:
		return api.CauseInstallerFailed
	}
	return api.CauseUnknown
}

// macInstallerText looks inside the installer's own output. Only the disk-full text was reproduced; the policy
// words are unverified (they need an MDM-managed Mac, check B14).
func macInstallerText(out string) api.FailureCause {
	l := strings.ToLower(out)
	switch {
	case strings.Contains(l, "no space left"), strings.Contains(l, "not enough space"):
		return api.CauseDiskFull
	case strings.Contains(l, "restricted by"), strings.Contains(l, "blocked by"), strings.Contains(l, "not allowed"):
		return api.CausePolicy
	}
	return api.CauseInstallerFailed
}

// Curl classifies a curl exit code (also the code brew reports for its downloads). Codes were reproduced with
// the S0-04 network-failure script.
func Curl(code int, out string) api.FailureCause {
	switch code {
	case 0:
		return ""
	case 6, 7, 28, 35, 52, 55, 56, 60:
		return api.CauseNetwork
	case 22:
		if strings.Contains(out, "407") {
			return api.CauseProxyAuth
		}
		return api.CauseNetwork
	}
	return api.CauseUnknown
}

// Brew classifies a failed brew run from its combined output.
func Brew(exit int, out string) api.FailureCause {
	if exit == 0 {
		return ""
	}
	if m := curlExit.FindStringSubmatch(out); m != nil {
		c, _ := strconv.Atoi(m[1])
		if cause := Curl(c, out); cause != api.CauseUnknown {
			return cause
		}
	}
	l := strings.ToLower(out)
	switch {
	case strings.Contains(l, "no space left on device"):
		return api.CauseDiskFull
	case strings.Contains(l, "407 proxy authentication"), strings.Contains(l, "proxy authentication required"):
		return api.CauseProxyAuth
	}
	return api.CauseInstallerFailed
}

// GoNet classifies an error string from Go's net/http (the app's own downloads).
func GoNet(err string) api.FailureCause {
	l := strings.ToLower(err)
	switch {
	case strings.Contains(l, "proxy authentication required"), strings.Contains(l, "407"):
		return api.CauseProxyAuth
	case strings.Contains(l, "proxyconnect"), strings.Contains(l, "no such host"),
		strings.Contains(l, "i/o timeout"), strings.Contains(l, "deadline exceeded"),
		strings.Contains(l, "connection refused"), strings.Contains(l, "connection reset"), strings.Contains(l, "x509:"),
		strings.Contains(l, "tls:"), strings.Contains(l, "network is unreachable"), strings.Contains(l, "unexpected eof"):
		return api.CauseNetwork
	case strings.Contains(l, "no space left"):
		return api.CauseDiskFull
	}
	return api.CauseUnknown
}

// Winget classifies a winget exit code. Names are from microsoft/winget-cli returnCodes.md; none of these has
// run on a Windows machine yet (check B11): they are the documented values.
func Winget(code uint32) api.FailureCause {
	switch code {
	case 0, 0x8A15002B, 0x8A15010D:
		return "" // installed, nothing to update, or already installed
	case 0x8A150019:
		return api.CauseNeedsAdmin
	case 0x8A150105:
		return api.CauseDiskFull
	case 0x8A150008, 0x8A150107, 0x8A150086, 0x8A15002E, 0x8A15004B, 0x8A150045:
		return api.CauseNetwork
	case 0x8A15003A, 0x8A15010F, 0x8A15001B, 0x8A15001C:
		return api.CausePolicy
	case 0x8A150011:
		return api.CauseBadSignature // the installer's hash differs from the manifest
	case 0x8A15010C, 0x8A150077:
		return api.CauseCancelled // UAC declined or cancelled
	case 0x8A150109, 0x8A15010A:
		return api.CauseReboot
	}
	return api.CauseInstallerFailed
}

// WingetStart classifies a failure to start winget at all.
func WingetStart(err string) api.FailureCause {
	l := strings.ToLower(err)
	if strings.Contains(l, "executable file not found") || strings.Contains(l, "not recognized") ||
		strings.Contains(l, "cannot find the file") || strings.Contains(l, "no such file") {
		return api.CauseWingetMissing
	}
	return api.CauseUnknown
}
