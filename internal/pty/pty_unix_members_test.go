//go:build !windows

package pty

import (
	"testing"
	"time"
)

// FU-01a: Members() must not wait for a whole KillSession. The old kill() held k.mu through the grace period
// and the SIGKILL step, so a concurrent Members() waited for all of it.
func TestMembersDoesNotWaitForKill(t *testing.T) {
	f := newFakeKiller(54321)
	f.grace = 600 * time.Millisecond
	f.hardWait = 300 * time.Millisecond
	f.table = func() ([]int, error) { return []int{900}, nil } // a member that never goes away
	done := make(chan error, 1)
	go func() { done <- f.kill() }()
	time.Sleep(100 * time.Millisecond) // kill is now inside its grace wait
	start := time.Now()
	m, err := f.members()
	took := time.Since(start)
	if err != nil || len(m) != 1 {
		t.Fatalf("members = %v, %v", m, err)
	}
	if took > 150*time.Millisecond {
		t.Errorf("Members waited %v for the running kill", took)
	}
	if err := <-done; err == nil {
		t.Error("kill reported success although the member survived")
	}
}

// FU-01a review: kill() drops k.mu between listings, so a concurrent members() can latch ended. The running
// kill must then stop acting: a new session that reuses the number is never signalled.
func TestKillStopsWhenMembersLatchesEnded(t *testing.T) {
	f := newFakeKiller(54321)
	f.grace = 600 * time.Millisecond
	f.hardWait = 300 * time.Millisecond
	f.table = func() ([]int, error) { return []int{900}, nil }
	done := make(chan error, 1)
	go func() { done <- f.kill() }()
	time.Sleep(100 * time.Millisecond)
	f.mu.Lock()
	f.table = func() ([]int, error) { return nil, nil } // the session empties
	f.mu.Unlock()
	if m, _ := f.members(); len(m) != 0 { // latches ended
		t.Fatalf("members = %v", m)
	}
	f.mu.Lock()
	f.table = func() ([]int, error) { return []int{777}, nil } // the number is reused by a new leader
	f.mu.Unlock()
	<-done
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.signals {
		if s == -777 || s == 777 {
			t.Fatalf("a new session was signalled: %v", f.signals)
		}
	}
}
