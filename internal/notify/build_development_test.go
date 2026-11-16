//go:build !production

package notify

import "testing"

// A development build never uses the system's service without the one opt-in.
func TestDevelopmentBuildNeedsTheOptIn(t *testing.T) {
	if productionBuild {
		t.Fatal("productionBuild must be false without the production tag")
	}
	if ChooseReal(productionBuild, false, func(string) string { return "" }, false) {
		t.Fatal("a development build must use the fake")
	}
	if !ChooseReal(productionBuild, false, func(k string) string {
		if k == EnvRealNotifications {
			return "1"
		}
		return ""
	}, false) {
		t.Fatal("AWSSSOHUB_REAL_NOTIFICATIONS=1 opts in")
	}
}
