//go:build windows

package appguard

import (
	"strings"
	"testing"
)

// FU-03d: SHFileOperation's own codes are shown as they are, never read as Win32 errors.
func TestShellErrorIsNotAWin32Error(t *testing.T) {
	if got := shellError(0x78).Error(); !strings.Contains(got, "0x78") || !strings.Contains(got, "access denied") {
		t.Fatal(got)
	}
	if got := shellError(0x1234).Error(); got != "SHFileOperation error 0x1234" {
		t.Fatal(got)
	}
}
