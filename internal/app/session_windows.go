//go:build windows

package app

func sessionEnding() bool { return sessionEndFlag.Load() }
