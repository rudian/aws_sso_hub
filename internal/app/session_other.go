//go:build !windows && !darwin

package app

func sessionEnding() bool { return false }

// watchSessionEnd: nothing to register here.
func watchSessionEnd() {}
