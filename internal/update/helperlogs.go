package update

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Wails' helper writes $TMPDIR/wails-update-<pid>.log, where <pid> is the process that called Restart, and never
// removes it. Every Wails v3 app writes that name, so a folder-wide pattern would also match other apps' logs.
// Instead the app records its own pid in a small file in its data folder right before Restart; at the next start
// it removes exactly $TMPDIR/wails-update-<recorded pid>.log for each recorded pid, then clears the record.

const maxPIDRecords = 64

// RecordRestart appends pid to the record file. Called right before the helper is started.
func RecordRestart(recordPath string, pid int) error {
	if recordPath == "" || pid <= 0 {
		return errors.New("no helper-log record configured")
	}
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o700); err != nil {
		return err
	}
	pids := readPIDRecord(recordPath)
	if len(pids) >= maxPIDRecords {
		pids = pids[len(pids)-maxPIDRecords+1:]
	}
	pids = append(pids, pid)
	var b strings.Builder
	for _, p := range pids {
		fmt.Fprintf(&b, "%d\n", p)
	}
	tmp := recordPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, recordPath)
}

// readPIDRecord returns the pids in the record; unreadable or malformed lines are ignored.
func readPIDRecord(recordPath string) []int {
	b, err := os.ReadFile(recordPath)
	if err != nil || len(b) > 4096 {
		return nil
	}
	var out []int
	for _, ln := range strings.Fields(string(b)) {
		if n, err := strconv.Atoi(ln); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}

func helperLogName(pid int) string { return "wails-update-" + strconv.Itoa(pid) + ".log" }

// cleanHelperLogs removes the helper log of each recorded pid that is a regular file at least minAge old (a helper
// that is running now has just written to its log), and clears the record. It returns the removed paths.
func cleanHelperLogs(recordPath, tmpDir string, self int, now time.Time, minAge time.Duration) []string {
	if recordPath == "" {
		return nil
	}
	pids := readPIDRecord(recordPath)
	var done []string
	var keep []int
	for _, pid := range pids {
		p := filepath.Join(tmpDir, helperLogName(pid))
		fi, err := os.Lstat(p)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue // nothing to remove, drop the record
		case err != nil:
			keep = append(keep, pid)
			continue
		case !fi.Mode().IsRegular():
			continue // not ours to delete; drop the record
		case pid == self || now.Sub(fi.ModTime()) < minAge:
			keep = append(keep, pid) // may still be written to: try again at the next start
			continue
		}
		if os.Remove(p) == nil {
			done = append(done, p)
		} else {
			keep = append(keep, pid)
		}
	}
	if len(keep) == 0 {
		_ = os.Remove(recordPath)
		return done
	}
	var b strings.Builder
	for _, p := range keep {
		fmt.Fprintf(&b, "%d\n", p)
	}
	_ = os.WriteFile(recordPath, []byte(b.String()), 0o600)
	return done
}

// CleanHelperLogs removes the helper logs of the restarts this app recorded and returns the exact paths it removed,
// so the caller can log them.
func CleanHelperLogs(recordPath string) []string {
	return cleanHelperLogs(recordPath, os.TempDir(), os.Getpid(), time.Now(), time.Hour)
}
