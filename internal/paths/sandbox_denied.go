//go:build production && !sandbox

package paths

// sandboxAllowed is false in release builds: an environment variable must not be able to move the app onto a
// different home folder or redirect the sign-in endpoints of a shipped binary.
const sandboxAllowed = false
