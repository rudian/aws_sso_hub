//go:build !darwin

package update

// GateSupported is false until the Windows check (the exe's file version equals the manifest version) exists
// (F1-20, gate_windows.go). F1-20 changes this file's build tag to !darwin && !windows when it adds that file.
func GateSupported() bool { return false }

// VerifyBundle refuses everything on platforms without a check: failing closed.
func VerifyBundle(path, wantVersion string, g Gate) error {
	return &Refusal{Reason: "payload verification is not implemented on this platform", Code: CodePlatform}
}
