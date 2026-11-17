//go:build darwin

package pty

import "golang.org/x/sys/unix"

const szomb = 5 // p_stat of a zombie in <sys/proc.h>

// listPIDs returns every live process ID (zombies are left out: they are dead, only waiting to be reaped).
// ps cannot be used for the session: on macOS "ps -o sess" prints 0. getsid(pid) works for any process of
// the same user. An error means the list is unknown, which is not the same as an empty list.
func listPIDs() ([]int, error) {
	ps, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	out := make([]int, 0, len(ps))
	for i := range ps {
		if ps[i].Proc.P_stat == szomb {
			continue
		}
		out = append(out, int(ps[i].Proc.P_pid))
	}
	return out, nil
}

// startTime is the start time of process pid in microseconds (kinfo_proc p_starttime), or false when no such
// process exists. Together with the pid it identifies one process for its whole life.
func startTime(pid int) (int64, bool) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || int(kp.Proc.P_pid) != pid {
		return 0, false
	}
	return int64(kp.Proc.P_starttime.Sec)*1_000_000 + int64(kp.Proc.P_starttime.Usec), true
}
