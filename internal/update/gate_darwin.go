//go:build darwin

package update

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GateSupported reports whether this platform has a payload check. Without one, updates are not offered at all.
func GateSupported() bool { return true }

// Absolute tool paths: nothing is looked up through PATH.
const (
	codesignPath = "/usr/bin/codesign"
	spctlPath    = "/usr/sbin/spctl"
	plutilPath   = "/usr/bin/plutil"
)

// runTool runs one system tool; tests replace it to assert the argument lists.
var runTool = func(d time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + os.Getenv("HOME"), "LANG=C"}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.Bytes(), err
}

// VerifyBundle is plan 4.14 item 4 for macOS. It runs on the staged copy, before the helper is started and again
// inside Restart, so a failure leaves the installed app alone. Every failure is a *Refusal.
func VerifyBundle(path, wantVersion string, g Gate) error {
	if err := verifyBundle(path, wantVersion, g); err != nil {
		return refuseErr(err)
	}
	return nil
}

func verifyBundle(path, wantVersion string, g Gate) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return codedf(CodeBundleInvalid, "staged bundle: %v", err)
	}
	if !fi.IsDir() || filepath.Ext(path) != ".app" {
		return codedf(CodeBundleInvalid, "staged payload is not an .app bundle")
	}
	if err := rejectAppleDouble(path); err != nil {
		return &codedError{code: CodeAppleDouble, err: err}
	}
	plist := filepath.Join(path, "Contents", "Info.plist")
	id, err := plistValue(plist, "CFBundleIdentifier")
	if err != nil {
		return err
	}
	if id != g.BundleID {
		return codedf(CodeBundleID, "bundle identifier %q, expected %q", id, g.BundleID)
	}
	ver, err := plistValue(plist, "CFBundleShortVersionString")
	if err != nil {
		return err
	}
	if ver != wantVersion {
		return codedf(CodeBundleVersion, "bundle version %q does not equal manifest version %q", ver, wantVersion)
	}
	if g.Requirement == "" {
		return errNoRequirement
	}
	// codesign --verify with a requirement checks the sealed contents AND that the signature satisfies the
	// requirement. Ad-hoc signatures have no certificate chain, so they cannot satisfy "anchor apple generic".
	if out, err := runTool(30*time.Second, codesignPath, "--verify", "--deep", "--strict", "-R="+g.Requirement, path); err != nil {
		return codedf(CodeCodesign, "codesign requirement failed: %s", oneLine(out, err))
	}
	// Second check, never the only one: spctl answers "accepted" when Gatekeeper is switched off.
	if g.RequireGatekeeper {
		if out, err := runTool(60*time.Second, spctlPath, "-a", "-t", "exec", path); err != nil {
			return codedf(CodeGatekeeper, "gatekeeper assessment failed: %s", oneLine(out, err))
		}
	}
	return nil
}

func plistValue(plist, key string) (string, error) {
	out, err := runTool(10*time.Second, plutilPath, "-extract", key, "raw", "-o", "-", plist)
	if err != nil {
		return "", codedf(CodeBundleInvalid, "Info.plist %s: %s", key, oneLine(out, err))
	}
	return strings.TrimSpace(string(out)), nil
}

func oneLine(out []byte, err error) string {
	s := strings.TrimSpace(string(out))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return err.Error()
	}
	return s
}
