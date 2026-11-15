package appguard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SystemDeps returns the production dependencies. Tests never call it: it lists real processes and can quit
// and move real files.
func SystemDeps() Deps {
	self, _ := os.Executable()
	return Deps{
		Lister: sysLister{}, Term: sysTerm{}, Files: sysFiles{}, Clock: sysClock{},
		Prompter: newSysPrompter(), SelfPID: os.Getpid(), SelfPath: self,
	}
}

type sysClock struct{}

func (sysClock) Now() time.Time        { return time.Now() }
func (sysClock) Sleep(d time.Duration) { time.Sleep(d) }

type sysFiles struct{}

func (sysFiles) Exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
func (sysFiles) Rename(a, b string) error { return os.Rename(a, b) }
func (sysFiles) TrashDir() string         { return trashDir() }
func (sysFiles) CanRecycle() bool         { return canRecycle }
func (sysFiles) Recycle(p string) error   { return recycleFile(p) }

func runOut(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...) // #nosec: argument list, fixed system tools
	hideConsole(cmd)
	out, err := cmd.Output()
	return string(out), err
}

// parsePS parses `ps -x -o pid=,uid=,comm=` output ("  123  501 /path/with spaces/exe") and keeps only the
// rows owned by uid: another user's app (fast user switching) is none of this launch's business.
func parsePS(out string, uid int) []Proc {
	var procs []Proc
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		u, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil || u != uid {
			continue
		}
		rest := strings.TrimSpace(line)
		rest = strings.TrimSpace(strings.TrimPrefix(rest, f[0]))
		rest = strings.TrimSpace(strings.TrimPrefix(rest, f[1]))
		procs = append(procs, Proc{PID: pid, Path: rest})
	}
	return procs
}

// parseTasklist parses `tasklist /FO CSV /NH`: "name.exe","123",... (image names only, no paths).
func parseTasklist(out string) []Proc {
	var procs []Proc
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(strings.TrimSpace(line), `","`)
		if len(parts) < 2 {
			continue
		}
		pid, err := strconv.Atoi(strings.Trim(parts[1], `"`))
		if err != nil {
			continue
		}
		procs = append(procs, Proc{PID: pid, Path: strings.Trim(parts[0], `"`)})
	}
	return procs
}

func homeTrash() string {
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return ""
	}
	return filepath.Join(h, ".Trash")
}
