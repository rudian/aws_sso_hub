//go:build windows

package app

func sessionEnding() bool { return sessionEndFlag.Load() }

// watchSessionEnd: the Windows session-end flag is set by the message window; nothing to register here.
func watchSessionEnd() {}
