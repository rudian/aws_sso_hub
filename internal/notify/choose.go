package notify

import (
	"os"
	"testing"
)

// EnvRealNotifications is the one opt-in for real notifications in a development build. The development
// build shares its bundle identifier with the installed app, so a real notification from it would use, and
// could change, the installed app's permission (docs/research/notifications.md limit 10).
const EnvRealNotifications = "AWSSSOHUB_REAL_NOTIFICATIONS"

// ChooseReal decides whether the system's notification service is used.
//
//   - sandbox mode: never (a fake that also shows each notice as a toast in the window);
//   - a production build (-tags production): always;
//   - any other build: only with AWSSSOHUB_REAL_NOTIFICATIONS=1, and never inside a test binary.
func ChooseReal(production, sandbox bool, env func(string) string, inTest bool) bool {
	switch {
	case sandbox || inTest:
		return false
	case production:
		return true
	}
	return env(EnvRealNotifications) == "1"
}

// UseReal is ChooseReal for this build and this process.
func UseReal(sandbox bool) bool {
	return ChooseReal(productionBuild, sandbox, os.Getenv, testing.Testing())
}
