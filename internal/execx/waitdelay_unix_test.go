//go:build !windows

package execx

import (
	"context"
	"testing"
	"time"
)

// FU-Wb: a grandchild that keeps the output pipe open must not keep Run waiting.
func TestRunDoesNotWaitForAGrandchildHoldingThePipe(t *testing.T) {
	old := waitDelay
	waitDelay = 300 * time.Millisecond
	t.Cleanup(func() { waitDelay = old })
	// sh exits at once; the background sleep inherits stdout and stderr and lives for 30 s
	cmd := Command(context.Background(), "/bin/sh", "-c", "(sleep 30 &) ; echo hello")
	start := time.Now()
	res, err := Run(cmd, nil)
	if err != nil || res.ExitCode != 0 || string(res.Stdout) != "hello\n" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("Run waited %s for the grandchild", d)
	}
}

// The same when the context ends: the command is killed, and Run returns although a grandchild holds the pipe.
func TestRunReturnsAfterTheContextEndsEvenIfAGrandchildHoldsThePipe(t *testing.T) {
	old := waitDelay
	waitDelay = 300 * time.Millisecond
	t.Cleanup(func() { waitDelay = old })
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	cmd := Command(ctx, "/bin/sh", "-c", "sleep 30 & wait")
	start := time.Now()
	_, err := Run(cmd, nil)
	if err == nil {
		t.Fatal("a command killed by its context is an error")
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("Run waited %s", d)
	}
}
