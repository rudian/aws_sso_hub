//go:build !windows

package taskstate

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// PSTable lists processes with one `ps` call (never one call per process).
type PSTable struct{}

func (PSTable) List() ([]Process, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/bin/ps", "-axo", "pid=,pgid=,comm=").Output()
	if err != nil {
		return nil, err
	}
	return parsePS(out), nil
}

func parsePS(b []byte) []Process {
	var ps []Process
	for _, ln := range bytes.Split(b, []byte("\n")) {
		f := strings.Fields(string(ln))
		if len(f) < 3 {
			continue
		}
		pid, e1 := strconv.Atoi(f[0])
		pgid, e2 := strconv.Atoi(f[1])
		if e1 != nil || e2 != nil {
			continue
		}
		ps = append(ps, Process{PID: pid, PGID: pgid, Name: strings.Join(f[2:], " ")})
	}
	return ps
}
