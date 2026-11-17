//go:build !windows

package proctrack

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// PSSource runs `ps` once per listing (macOS and, for tests, Linux). The environment is cut down to a plain C
// locale so lstart always has the same shape.
type PSSource struct{}

// NewSource is the process-table source of this platform.
func NewSource() Source { return PSSource{} }

func (PSSource) Read(_ []int) ([]Proc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/ps", PSArgs...)
	// TZ=UTC0: lstart is then printed and parsed in UTC, so the repeated hour at a daylight-saving change
	// cannot make a child look older than its parent.
	cmd.Env = []string{"LC_ALL=C", "LANG=C", "TZ=UTC0", "PATH=/usr/bin:/bin"}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return ParsePS(out, time.UTC)
}

// CommandArgs is the on-demand command-line listing: pid, lstart and the full arguments of chosen PIDs.
var CommandArgs = []string{"-ww", "-o", "pid=,lstart=,args=", "-p"}

// ReadCommands reads the full command lines of pids with one `ps` call. It is for the expanded list in the UI:
// the lines are never cached, saved or logged.
func ReadCommands(pids []int) ([]CommandLine, error) {
	if len(pids) == 0 {
		return nil, nil
	}
	strs := make([]string, len(pids))
	for i, p := range pids {
		strs[i] = strconv.Itoa(p)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	args := append(append([]string(nil), CommandArgs...), strings.Join(strs, ","))
	cmd := exec.CommandContext(ctx, "/bin/ps", args...)
	cmd.Env = []string{"LC_ALL=C", "LANG=C", "TZ=UTC0", "PATH=/usr/bin:/bin"}
	out, _ := cmd.Output() // ps exits 1 when some PID is gone; the rows it did print are good
	return ParseCommands(out, time.UTC), nil
}

func defaultCommands(pids []int) ([]CommandLine, error) { return ReadCommands(pids) }
