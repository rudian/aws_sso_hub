//go:build !windows

package pty

import (
	"errors"
	"testing"

	"github.com/rudian/codepass/internal/shellguard"
)

func TestStartRefusesALoginShellWithANonTempHome(t *testing.T) {
	shellguard.ResetRefused()
	for _, argv := range [][]string{LaunchArgv("/bin/zsh", "true"), {"/bin/bash", "-lic", "true"}} {
		s, err := Start(Options{Argv: argv, Env: []string{"HOME=/Users/someone", "PATH=/usr/bin:/bin"}})
		if !errors.Is(err, shellguard.ErrRefused) || s != nil {
			t.Fatalf("%v: %v", argv, err)
		}
	}
	if len(shellguard.Refused()) != 2 {
		t.Fatalf("refused = %v", shellguard.Refused())
	}
}

// A shell with no -c and no script on a terminal is interactive and reads the rc files.
func TestStartRefusesABareShellWithANonTempHome(t *testing.T) {
	shellguard.ResetRefused()
	for _, sh := range []string{"/bin/zsh", "/bin/bash"} {
		s, err := Start(Options{Argv: []string{sh}, Env: []string{"HOME=/Users/someone", "PATH=/usr/bin:/bin"}})
		if !errors.Is(err, shellguard.ErrRefused) || s != nil {
			t.Fatalf("%s: %v", sh, err)
		}
	}
}
