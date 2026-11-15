//go:build !darwin && !windows

package update

import "testing"

// The platforms without a payload check refuse everything.
func TestOtherPlatformsRefuseEverything(t *testing.T) {
	if GateSupported() {
		t.Fatal("gate reported as supported")
	}
	err := VerifyBundle("/anything", "1.0.0", Gate{})
	if !IsRefusal(err) || RefusalCode(err) != CodePlatform {
		t.Fatalf("not refused with the platform code: %v", err)
	}
}
