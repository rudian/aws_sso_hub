package aitools

import (
	"context"
	"errors"
	"testing"

	"github.com/rudian/codepass/internal/shellguard"
)

func TestRealRunRefusesALoginShellWithANonTempHome(t *testing.T) {
	shellguard.ResetRefused()
	_, code, err := RealRun(context.Background(), []string{"/bin/zsh", "-l", "-i", "-c", "true"}, "", []string{"HOME=/Users/someone"})
	if !errors.Is(err, shellguard.ErrRefused) || code != -1 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if len(shellguard.Refused()) != 1 {
		t.Fatalf("refused = %v", shellguard.Refused())
	}
}
