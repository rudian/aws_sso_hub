//go:build windows

package taskstate

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// PSTable lists processes with one Toolhelp snapshot.
type PSTable struct{}

func (PSTable) List() ([]Process, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	var out []Process
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		out = append(out, Process{PID: int(pe.ProcessID), Name: windows.UTF16ToString(pe.ExeFile[:])})
	}
	return out, nil
}
