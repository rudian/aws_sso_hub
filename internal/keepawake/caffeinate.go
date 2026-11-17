package keepawake

import (
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// CaffeinatePath is the system's own tool; never looked up through PATH.
const CaffeinatePath = "/usr/bin/caffeinate"

// Proc is a started child process.
type Proc interface {
	// Wait blocks until the process has exited.
	Wait() error
	// Kill ends the process. It is only ever called on a process this package started.
	Kill() error
}

// Runner starts a program. The argument list is the whole command: no shell is involved.
type Runner interface {
	Start(argv []string) (Proc, error)
}

type execRunner struct{}

// ExecRunner starts real processes with an empty environment and no input or output.
func ExecRunner() Runner { return execRunner{} }

func (execRunner) Start(argv []string) (Proc, error) {
	if len(argv) == 0 {
		return nil, errors.New("keepawake: empty command")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execProc{cmd: cmd}, nil
}

type execProc struct{ cmd *exec.Cmd }

func (p *execProc) Wait() error { return p.cmd.Wait() }
func (p *execProc) Kill() error { return p.cmd.Process.Kill() }

// Caffeinate holds the macOS idle-sleep (and, with screen, display-sleep) assertion by running
// `caffeinate -d -i -w <app pid>` (plan 4.10). -w makes caffeinate exit by itself when the app dies, however it
// dies. If caffeinate is killed from outside while the hold is wanted, it is started again.
type Caffeinate struct {
	Runner Runner
	Clock  Clock
	Log    *slog.Logger
	Path   string // default CaffeinatePath
	PID    int    // the app's process ID, for -w

	mu     sync.Mutex
	want   bool
	screen bool
	alive  bool // the current process runs (false while it waits to be started again)
	notify func()
	gen    int
	proc   Proc
	stop   chan struct{} // closed to end the loop of the current generation
	done   chan struct{} // closed when that loop has ended
}

func (c *Caffeinate) Name() string { return "caffeinate" }

func (c *Caffeinate) argv(screen bool) []string {
	p := c.Path
	if p == "" {
		p = CaffeinatePath
	}
	a := []string{p}
	if screen {
		a = append(a, "-d")
	}
	return append(a, "-i", "-w", strconv.Itoa(c.PID))
}

// SetNotify sets the function called, in a goroutine of its own, when the process ended or was started again by
// itself (so the Manager sees a lost hold). The Manager sets it.
func (c *Caffeinate) SetNotify(f func()) { c.mu.Lock(); c.notify = f; c.mu.Unlock() }

// Holding tells whether a caffeinate process runs now.
func (c *Caffeinate) Holding() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.want && c.alive }

// HoldsScreen tells whether the process that runs holds the display too (-d).
func (c *Caffeinate) HoldsScreen() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.want && c.alive && c.screen
}

func (c *Caffeinate) tell() {
	c.mu.Lock()
	f := c.notify
	c.mu.Unlock()
	if f != nil {
		go f() // its own goroutine: Release waits for the loop, and the Manager holds its lock around Release
	}
}

// Acquire starts caffeinate (or starts it again with the new arguments when the screen option changed: the new
// process starts first and the old one ends only after that, so a change that fails keeps the old hold). The first
// start happens before it returns, so a missing tool is reported. With the same option it only checks that the
// process runs; one that died and is waiting to be started again is an error.
func (c *Caffeinate) Acquire(screen bool) error {
	c.mu.Lock()
	if c.want && c.screen == screen && c.proc != nil {
		alive := c.alive
		c.mu.Unlock()
		if !alive {
			return errors.New("caffeinate is not running")
		}
		return nil
	}
	c.mu.Unlock()

	startedAt := c.Clock.Now()
	proc, err := c.Runner.Start(c.argv(screen))
	if err != nil {
		return fmt.Errorf("starting caffeinate: %w", err)
	}
	c.mu.Lock()
	oldWant, oldProc, oldStop, oldDone := c.want, c.proc, c.stop, c.done
	c.want, c.screen, c.proc, c.alive = true, screen, proc, true
	c.gen++
	gen := c.gen
	c.stop, c.done = make(chan struct{}), make(chan struct{})
	stop, done := c.stop, c.done
	c.mu.Unlock()
	if oldWant {
		close(oldStop)
		if oldProc != nil {
			_ = oldProc.Kill()
		}
		<-oldDone
	}
	go c.loop(gen, proc, startedAt, stop, done)
	return nil
}

// Release stops caffeinate and waits for the loop to end.
func (c *Caffeinate) Release() {
	c.mu.Lock()
	if !c.want {
		c.mu.Unlock()
		return
	}
	c.want, c.alive = false, false
	proc, stop, done := c.proc, c.stop, c.done
	c.proc = nil
	c.mu.Unlock()
	close(stop)
	if proc != nil {
		_ = proc.Kill()
	}
	<-done
}

const (
	restartMinRun   = 2 * time.Second
	restartMaxDelay = 30 * time.Second
)

// loop waits for the process and starts it again when it ends while the hold is still wanted.
func (c *Caffeinate) loop(gen int, proc Proc, started time.Time, stop, done chan struct{}) {
	defer close(done)
	delay := time.Duration(0)
	for {
		exited := make(chan struct{})
		go func(p Proc) { _ = p.Wait(); close(exited) }(proc)
		select {
		case <-stop:
			<-exited // Release killed it
			return
		case <-exited:
		}
		// It ended by itself: killed from outside, or crashed.
		c.mu.Lock()
		mine := c.gen == gen && c.want
		if mine {
			c.alive = false
		}
		c.mu.Unlock()
		if mine {
			c.tell()
		}
		if c.Clock.Now().Sub(started) < restartMinRun {
			if delay == 0 {
				delay = time.Second
			} else if delay *= 2; delay > restartMaxDelay {
				delay = restartMaxDelay
			}
		} else {
			delay = 0
		}
		if c.Log != nil {
			c.Log.Warn("keepawake: caffeinate ended; starting it again", "after", delay)
		}
		for {
			if delay > 0 {
				wake := make(chan struct{})
				t := c.Clock.AfterFunc(delay, func() { close(wake) })
				select {
				case <-stop:
					t.Stop()
					return
				case <-wake:
				}
			}
			select {
			case <-stop:
				return
			default:
			}
			c.mu.Lock()
			if c.gen != gen || !c.want {
				c.mu.Unlock()
				return
			}
			screen := c.screen
			c.mu.Unlock()
			startedAt := c.Clock.Now()
			p, err := c.Runner.Start(c.argv(screen))
			if err != nil {
				if c.Log != nil {
					c.Log.Warn("keepawake: starting caffeinate again failed", "err", err)
				}
				if delay == 0 {
					delay = time.Second
				} else if delay *= 2; delay > restartMaxDelay {
					delay = restartMaxDelay
				}
				continue
			}
			c.mu.Lock()
			if c.gen != gen || !c.want {
				c.mu.Unlock()
				_ = p.Kill()
				_ = p.Wait()
				return
			}
			c.proc, c.alive = p, true
			c.mu.Unlock()
			c.tell()
			proc, started = p, startedAt
			break
		}
	}
}
