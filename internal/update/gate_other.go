//go:build !darwin && !windows

package update

// GateSupported is false: the macOS check is gate_darwin.go and the Windows check is gate_windows.go; every other
// platform has no payload check, so no update is offered.
func GateSupported() bool { return false }

// VerifyBundle refuses everything on platforms without a check: failing closed.
func VerifyBundle(path, wantVersion string, g Gate) error {
	return &Refusal{Reason: "payload verification is not implemented on this platform", Code: CodePlatform}
}
