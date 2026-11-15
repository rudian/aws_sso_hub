//go:build !windows && !darwin

package app

func sessionEnding() bool { return false }
