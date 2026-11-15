//go:build windows

package update

import (
	"os"
	"path/filepath"
	"strings"
)

// GateSupported reports whether this platform has a payload check. Without one, updates are not offered at all.
func GateSupported() bool { return true }

// VerifyBundle is plan 4.14 item 4 for Windows. Trust here rests on the ed25519-signed manifest and the sha256 of
// the archive, both already checked before the download is unpacked (platform-independent code); this adds the one
// thing the manifest cannot say by itself: the unpacked exe really is the release the manifest names, so an archive
// swapped for an older or different build is refused. It reads the staged file only and never touches the installed
// exe; every failure is a *Refusal, and the caller removes the staging folder.
//
// g.BundleID and g.Requirement are macOS values and are ignored here.
func VerifyBundle(path, wantVersion string, g Gate) error {
	if err := verifyExe(path, wantVersion); err != nil {
		return refuseErr(err)
	}
	return nil
}

func verifyExe(path, wantVersion string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return codedf(CodeBundleInvalid, "staged executable: %v", err)
	}
	if !fi.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(path), ".exe") {
		return codedf(CodeBundleInvalid, "staged payload is not an .exe file")
	}
	if err := checkExeVersion(path, wantVersion); err != nil {
		return err
	}
	// AUTHENTICODE: not checked. The Windows build ships unsigned in 1.0 (owner decision 3), so there is no
	// publisher to verify. If a certificate is ever bought, the signature check (WinVerifyTrust on `path`, with the
	// expected subject pinned) goes HERE, after the version check and before returning nil, and the release pipeline
	// gains a signing step.
	return nil
}
