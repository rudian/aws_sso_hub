package toolinstall

import (
	"testing"

	"github.com/rudian/codepass/internal/api"
)

func TestMacPrivileged(t *testing.T) {
	cases := []struct {
		name string
		exit int
		out  string
		want api.FailureCause
	}{
		// recorded: osascript -e 'error "User canceled." number -128'
		{"cancel", 1, "6:22: execution error: User canceled. (-128)", api.CauseCancelled},
		{"cancel in the security window", 1, "execution error: ... (-60006)", api.CauseCancelled},
		// recorded with the test script: stderr text then the exit code
		{"wrong team", 1, "7:211: execution error: codepass-pkg: Team ID is ABCDEFGHIJ, expected 927JGANW46 (13)", api.CauseUnknownSigner},
		{"tampered", 1, "7:211: execution error: codepass-pkg: signature: Status: package is invalid (checksum did not verify) (12)", api.CauseBadSignature},
		{"not notarized", 1, "7:211: execution error: codepass-pkg: not notarized (14)", api.CauseNotNotarized},
		{"size", 1, "7:211: execution error: codepass-pkg: size 5, expected 6 (15)", api.CauseDamagedDownload},
		{"disk", 1, "7:211: execution error: codepass-pkg: disk full (11)", api.CauseDiskFull},
		{"prefix", 1, "7:211: execution error: codepass-pkg: /opt/homebrew already exists (20)", api.CausePrefixExists},
		{"copy failed", 1, "7:211: execution error: codepass-pkg: not a regular file (10)", api.CauseInstallerFailed},
		{"brew missing", 1, "7:211: execution error: codepass-pkg: the installer finished but brew is missing (31)", api.CauseInstallerFailed},
		// header values from Authorization.h
		{"denied", 1, "execution error: The administrator user name or password was incorrect. (-60005)", api.CauseAuthDenied},
		{"no session", 1, "execution error: ... (-60007)", api.CauseNoSession},
		{"installer disk", 1, "execution error: codepass-pkg: installer failed (exit 1). Last lines:\ninstaller: No space left on device\n (30)", api.CauseDiskFull},
		{"installer policy", 1, "execution error: codepass-pkg: installer failed (exit 1). Last lines:\ninstaller: installation is blocked by your administrator (30)", api.CausePolicy},
		{"installer other", 1, "execution error: codepass-pkg: installer failed (exit 1). Last lines:\nx (30)", api.CauseInstallerFailed},
		{"not root", 1, "execution error: codepass-pkg: must run as root (21)", api.CauseUnknown},
		{"ok", 0, "", ""},
		{"garbage", 1, "something else", api.CauseUnknown},
	}
	for _, c := range cases {
		if got := MacPrivileged(c.exit, c.out); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestEveryScriptExitCodeHasItsOwnCause(t *testing.T) {
	seen := map[api.FailureCause][]int{}
	for _, code := range []int{ExitDiskFull, ExitNotSigned, ExitWrongTeam, ExitNotNotarized, ExitSizeMismatch, ExitPrefixExists} {
		c := MacPrivileged(1, "execution error: x ("+itoa(code)+")")
		seen[c] = append(seen[c], code)
	}
	for c, codes := range seen {
		if len(codes) != 1 {
			t.Errorf("cause %q is shared by exit codes %v", c, codes)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func TestCurlAndBrew(t *testing.T) {
	// exit codes recorded on the S0-04 Mac
	for code, want := range map[int]api.FailureCause{7: api.CauseNetwork, 28: api.CauseNetwork, 6: api.CauseNetwork, 60: api.CauseNetwork, 56: api.CauseNetwork, 0: "", 99: api.CauseUnknown} {
		if got := Curl(code, ""); got != want {
			t.Errorf("curl %d: got %q", code, got)
		}
	}
	if got := Curl(22, "curl: (22) The requested URL returned error: 407"); got != api.CauseProxyAuth {
		t.Errorf("407: got %q", got)
	}
	// recorded: brew with HTTPS_PROXY=http://127.0.0.1:1
	brewDead := "Error: Failure while executing; `/usr/bin/env /x/shims/shared/curl --disable --fail ... https://formulae.brew.sh/api/x.jws.json` exited with 7."
	cases := []struct {
		out  string
		want api.FailureCause
	}{
		{brewDead, api.CauseNetwork},
		{"Error: Failure while executing; `curl --fail https://ghcr.io/x` exited with 22.\ncurl: (22) The requested URL returned error: 407", api.CauseProxyAuth},
		{"Error: No space left on device @ io_write - /opt/homebrew/Cellar/x", api.CauseDiskFull},
		{"Error: something odd", api.CauseInstallerFailed},
	}
	for _, c := range cases {
		if got := Brew(1, c.out); got != c.want {
			t.Errorf("brew %q: got %q want %q", c.out, got, c.want)
		}
	}
	if Brew(0, "x") != "" {
		t.Error("exit 0 is not a failure")
	}
}

func TestGoNet(t *testing.T) {
	for in, want := range map[string]api.FailureCause{
		`Get "https://x": proxyconnect tcp: dial tcp 127.0.0.1:1: connect: connection refused`:              api.CauseNetwork,
		`Get "https://x": dial tcp: lookup x.invalid: no such host`:                                         api.CauseNetwork,
		`Get "https://x": context deadline exceeded`:                                                        api.CauseNetwork,
		`Get "https://x": tls: failed to verify certificate: x509: certificate signed by unknown authority`: api.CauseNetwork,
		`Get "https://x": Proxy Authentication Required`:                                                    api.CauseProxyAuth,
		`write /x: no space left on device`:                                                                 api.CauseDiskFull,
		`something else`:                                                                                    api.CauseUnknown,
	} {
		if got := GoNet(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestWinget(t *testing.T) {
	for code, want := range map[uint32]api.FailureCause{
		0: "", 0x8A15010D: "", 0x8A15002B: "", 0x8A150019: api.CauseNeedsAdmin, 0x8A150105: api.CauseDiskFull, 0x8A150008: api.CauseNetwork,
		0x8A150107: api.CauseNetwork, 0x8A15010F: api.CausePolicy, 0x8A15003A: api.CausePolicy, 0x8A150011: api.CauseBadSignature,
		0x8A15010C: api.CauseCancelled, 0x8A150077: api.CauseCancelled, 0x8A150109: api.CauseReboot, 0x8A150049: api.CauseInstallerFailed,
	} {
		if got := Winget(code); got != want {
			t.Errorf("%#x: got %q want %q", code, got, want)
		}
	}
	if WingetStart(`exec: "winget": executable file not found in %PATH%`) != api.CauseWingetMissing {
		t.Error("winget missing")
	}
	if WingetStart("access denied") != api.CauseUnknown {
		t.Error("other start errors stay unknown")
	}
}

func TestScriptCode(t *testing.T) {
	for in, want := range map[string]int{"x (13)": 13, "x (-128)\n": -128, "x (-60005)": -60005} {
		if got, ok := ScriptCode(in); !ok || got != want {
			t.Errorf("%q: %d %v", in, got, ok)
		}
	}
	if _, ok := ScriptCode("no code"); ok {
		t.Error("no code")
	}
}
