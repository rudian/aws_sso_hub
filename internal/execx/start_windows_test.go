//go:build windows

package execx

import "testing"

func TestStartWithoutConsoleLeavesSysProcAttrUnset(t *testing.T) {
	cmd := startCmd("wt.exe", []string{"-d", `C:\work`}, "", nil, false)
	if cmd.SysProcAttr != nil {
		t.Fatalf("SysProcAttr = %+v, want nil so the launched GUI program keeps its normal window state", cmd.SysProcAttr)
	}
}

func TestStartWithNewConsoleSetsCreationFlag(t *testing.T) {
	cmd := startCmd("cmd.exe", nil, "", nil, true)
	if cmd.SysProcAttr == nil || cmd.SysProcAttr.CreationFlags&0x10 == 0 || cmd.SysProcAttr.HideWindow {
		t.Fatalf("SysProcAttr = %+v, want CREATE_NEW_CONSOLE without HideWindow", cmd.SysProcAttr)
	}
}
