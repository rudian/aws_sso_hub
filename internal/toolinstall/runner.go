package toolinstall

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Cmd is one program to run, with everything it inherits spelled out.
type Cmd struct {
	Path string
	Args []string
	Env  []string // the complete environment; nothing else is inherited
}

// Runner starts a program and calls onLine for every non-empty line of its output (stdout and stderr together).
// A non-zero exit is not an error: it is returned as the exit code. An error means it could not start, or the
// context ended it (exit is then -1).
type Runner interface {
	Run(ctx context.Context, c Cmd, onLine func(string)) (exit int, err error)
}

type execRunner struct{}

// RealRunner runs programs on this machine.
func RealRunner() Runner { return execRunner{} }

func (execRunner) Run(ctx context.Context, c Cmd, onLine func(string)) (int, error) {
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	cmd.Env = append([]string(nil), c.Env...)
	// A stop asks politely first (brew in the middle of pouring a bottle can leave a half-installed keg); the
	// hard kill comes after WaitDelay.
	cmd.Cancel = func() error { return softStop(cmd.Process) }
	cmd.WaitDelay = 5 * time.Second
	hideConsole(cmd)
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		sc.Split(scanLinesCR)
		for sc.Scan() {
			if l := CleanLine(sc.Text()); l != "" && onLine != nil {
				onLine(l)
			}
		}
		io.Copy(io.Discard, pr) // never let the child block on a full pipe
	}()
	err := cmd.Start()
	if err != nil {
		pw.Close()
		wg.Wait()
		return -1, err
	}
	err = cmd.Wait()
	pw.Close()
	wg.Wait()
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if asExit(err, &ee) {
		if ee.ExitCode() == -1 {
			return -1, ctx.Err()
		}
		return ee.ExitCode(), nil
	}
	return -1, err
}

// scanLinesCR splits on \n and on \r, so a progress bar that rewrites its line becomes separate lines.
func scanLinesCR(data []byte, atEOF bool) (int, []byte, error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// CleanLine removes terminal escape sequences and control characters, trims, and caps the length. A line that
// is only a progress bar ("####  12.3%") is dropped.
func CleanLine(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == 0x1b: // ESC [ ... final byte
			if i+1 < len(rs) && rs[i+1] == '[' {
				i += 2
				for i < len(rs) && !(rs[i] >= 0x40 && rs[i] <= 0x7e) {
					i++
				}
			}
		case r == '\t':
			b.WriteRune(' ')
		case r < 0x20 || r == 0x7f || r == utf8.RuneError:
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" || strings.Trim(out, "#=-> 0123456789.%") == "" {
		return ""
	}
	if len([]rune(out)) > 400 {
		out = string([]rune(out)[:400]) + "..."
	}
	return out
}
