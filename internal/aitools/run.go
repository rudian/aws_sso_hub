package aitools

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/rudian/codepass/internal/shellguard"
)

// RunFunc runs one program to its end and returns stdout and stderr together (capped). A non-zero exit is not
// an error: it is returned as the exit code. err is set when the program could not start or the context ended
// it (exit is then -1). env is the complete environment; nothing else is inherited.
type RunFunc func(ctx context.Context, argv []string, dir string, env []string) (out []byte, exit int, err error)

const outputCap = 1 << 20

type capBuf struct{ b bytes.Buffer }

func (c *capBuf) Write(p []byte) (int, error) {
	if room := outputCap - c.b.Len(); room > 0 {
		if len(p) > room {
			c.b.Write(p[:room])
		} else {
			c.b.Write(p)
		}
	}
	return len(p), nil // never block the child on a full buffer
}

// RealRun runs the program on this machine with no standard input. When the context ends, the program and the
// processes in its process group are killed (macOS; Windows: the whole job), and the call returns within a
// couple of seconds even if a grandchild still holds the output pipe.
func RealRun(ctx context.Context, argv []string, dir string, env []string) ([]byte, int, error) {
	if len(argv) == 0 {
		return nil, -1, errors.New("aitools: empty command")
	}
	if err := shellguard.Check(argv[0], argv[1:], env); err != nil {
		return nil, -1, err // a test binary never starts the owner's login shell
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = append([]string(nil), env...)
	var buf capBuf
	cmd.Stdout, cmd.Stderr = &buf, &buf
	cmd.WaitDelay = 2 * time.Second
	afterStart, release := prepare(cmd)
	defer release()
	var err error
	if err = cmd.Start(); err == nil {
		afterStart()
		err = cmd.Wait()
	}
	if err == nil || errors.Is(err, exec.ErrWaitDelay) {
		if ctx.Err() != nil {
			return buf.b.Bytes(), -1, ctx.Err()
		}
		return buf.b.Bytes(), 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ee.ExitCode() == -1 || ctx.Err() != nil {
			return buf.b.Bytes(), -1, ctxErr(ctx)
		}
		return buf.b.Bytes(), ee.ExitCode(), nil
	}
	return buf.b.Bytes(), -1, err
}

func ctxErr(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	return errors.New("aitools: the program was ended by a signal")
}
