//go:build production

package notify

import "testing"

// A release build (-tags production, build/darwin/Taskfile.yml BUILD_FLAGS) uses the system's service.
func TestProductionBuildChoosesTheSystem(t *testing.T) {
	if !productionBuild || !ChooseReal(productionBuild, false, func(string) string { return "" }, false) {
		t.Fatal("a production build must use the system's notification service")
	}
	if ChooseReal(productionBuild, true, func(string) string { return "1" }, false) {
		t.Fatal("sandbox mode never reaches the system's service")
	}
}
