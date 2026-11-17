//go:build !darwin && !windows

package pty

import (
	"bytes"
	"os"
	"strconv"
	"strings"
)

// listPIDs lists live processes from /proc (Linux and similar; the app ships for macOS and Windows, this
// keeps the package building and testable elsewhere). An error means the list is unknown.
func listPIDs() ([]int, error) {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var out []int
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if b, err := os.ReadFile("/proc/" + e.Name() + "/stat"); err == nil {
			if i := bytes.LastIndexByte(b, ')'); i >= 0 && i+2 < len(b) && b[i+2] == 'Z' {
				continue
			}
		}
		out = append(out, pid)
	}
	return out, nil
}

// startTime is field 22 of /proc/<pid>/stat (clock ticks since boot), or false when the process is gone.
func startTime(pid int) (int64, bool) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, false
	}
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return 0, false
	}
	f := strings.Fields(string(b[i+1:])) // f[0] is field 3 (state), so field 22 is f[19]
	if len(f) < 20 {
		return 0, false
	}
	v, err := strconv.ParseInt(f[19], 10, 64)
	return v, err == nil
}
