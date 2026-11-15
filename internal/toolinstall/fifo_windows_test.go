//go:build windows

package toolinstall

import "errors"

func mkfifo(string) error { return errors.New("no FIFOs on Windows") }
