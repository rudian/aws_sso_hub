//go:build !windows

package browser

import "errors"

func winDefaultCommand() (string, error) { return "", errors.New("not Windows") }

func winPolicy(string, string) (uint32, bool) { return 0, false }
