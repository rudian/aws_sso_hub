package proctrack

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"syscall"
	"time"
)

// ForceStop ends every program the session started: SIGTERM to every tracked program (and to a process group
// only when every member of that group is tracked), Grace, then SIGKILL repeatedly, then one more snapshot to
// confirm. On Windows the session's job is terminated instead. It blocks for up to Grace plus Hard: never
// call it from a UI thread. It signals only programs of this session's set that the table shows alive with
// the recorded start time, so a reused PID is never touched. Concurrent calls for one session run one after
// the other.
func (t *Tracker) ForceStop(id string) (StopResult, error) {
	res := StopResult{Session: id}
	t.mu.Lock()
	s := t.sess[id]
	t.mu.Unlock()
	if s == nil {
		return res, nil
	}
	dirty := false
	defer func() { // runs after stopMu is released: OnChange may call ForceStop for this session
		if dirty {
			t.changed()
		}
	}()
	s.stopMu.Lock()
	defer s.stopMu.Unlock()

	table, err := t.tickQuiet(&dirty) // fresh set, including children forked a moment ago
	if err != nil {
		res.Err = fmt.Errorf("%w: %v", errUnreadable, err)
		return res, res.Err
	}
	t.mu.Lock()
	first := reportOf(s)
	g := s.g
	t.mu.Unlock()
	res.Found = first.Count
	res.Names = distinctNames(first.Procs)
	if first.Count == 0 && g == nil {
		return res, nil
	}

	var stopErr error
	if t.useJob() {
		if g == nil {
			stopErr = fmt.Errorf("proctrack: session %s has no job to end", id)
		} else {
			stopErr = g.KillSession() // TerminateJobObject: no polite step
			deadline := time.Now().Add(t.o.Hard)
			for {
				if _, err := t.tickQuiet(&dirty); err != nil {
					break
				}
				if t.count(s) == 0 || time.Now().After(deadline) {
					break
				}
				time.Sleep(t.o.Poll)
			}
		}
	} else {
		stopErr = t.signalLoop(s, table, &dirty)
	}

	if _, err := t.tickQuiet(&dirty); err != nil && stopErr == nil { // the confirming snapshot
		stopErr = fmt.Errorf("%w: %v", errUnreadable, err)
	}
	t.mu.Lock()
	if cur := t.sess[id]; cur == s {
		res.Survivors = reportOf(s).Procs
	}
	t.mu.Unlock()
	res.Stopped = res.Found - len(res.Survivors)
	if res.Stopped < 0 {
		res.Stopped = 0
	}
	res.Err = stopErr
	return res, stopErr
}

// ForceStopAll stops every session that has tracked programs at the same time (each session has its own
// stop lock and ticks are serialized), so "stop all and quit" takes about one Grace plus Hard, not one per
// session. Results come back in session order. The error is the first failure; every result is returned.
func (t *Tracker) ForceStopAll() ([]StopResult, error) {
	rs := t.All()
	out := make([]StopResult, len(rs))
	errs := make([]error, len(rs))
	var wg sync.WaitGroup
	for i, r := range rs {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			out[i], errs[i] = t.ForceStop(id)
		}(i, r.Session)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func (t *Tracker) count(s *session) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(s.entries)
}

func distinctNames(ps []Process) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range ps {
		if p.Name != "" && !seen[p.Name] {
			seen[p.Name] = true
			out = append(out, p.Name)
		}
	}
	sort.Strings(out)
	return out
}

// signalLoop is the Unix stop. table is the snapshot that the caller's tick produced.
func (t *Tracker) signalLoop(s *session, table map[int]Proc, dirty *bool) error {
	termed := map[[2]int64]bool{} // pid, start: already sent the polite signal
	t.sendTracked(s, table, syscall.SIGTERM, termed)
	deadline := time.Now().Add(t.o.Grace)
	for time.Now().Before(deadline) {
		time.Sleep(t.o.Poll)
		tb, err := t.tickQuiet(dirty)
		if err != nil {
			return fmt.Errorf("%w: %v", errUnreadable, err)
		}
		if t.count(s) == 0 {
			return nil
		}
		t.sendTracked(s, tb, syscall.SIGTERM, termed) // a program forked after the first round
	}
	hard := time.Now().Add(t.o.Hard)
	for time.Now().Before(hard) {
		tb, err := t.tickQuiet(dirty)
		if err != nil {
			return fmt.Errorf("%w: %v", errUnreadable, err)
		}
		if t.count(s) == 0 {
			return nil
		}
		t.sendTracked(s, tb, syscall.SIGKILL, nil)
		time.Sleep(t.o.Poll)
	}
	return nil
}

// sendTracked signals the session's tracked programs. A process group gets the signal only when every program
// the table shows in that group belongs to this session's set; otherwise each program is signalled by itself.
// With only != nil (SIGTERM), programs already signalled are skipped.
func (t *Tracker) sendTracked(s *session, table map[int]Proc, sig syscall.Signal, only map[[2]int64]bool) {
	t.mu.Lock()
	mine := map[int]*entry{}
	for pid, e := range s.entries {
		if p, ok := table[pid]; ok && p.Start == e.Start { // alive with the recorded start time
			mine[pid] = e
		}
	}
	t.mu.Unlock()
	groupAll := map[int]int{}  // pgid -> programs in the table
	groupMine := map[int]int{} // pgid -> programs of this set
	for _, p := range table {
		groupAll[p.PGID]++
		if _, ok := mine[p.PID]; ok {
			groupMine[p.PGID]++
		}
	}
	ownPG := ownPgrp()
	sent := map[int]bool{}
	for pid, e := range mine {
		key := [2]int64{int64(pid), e.Start}
		if only != nil {
			if only[key] {
				continue
			}
			only[key] = true
		}
		pg := table[pid].PGID
		if pg > 1 && pg != ownPG && groupAll[pg] == groupMine[pg] {
			if !sent[-pg] {
				sent[-pg] = true
				t.send(-pg, sig)
			}
			continue
		}
		t.send(pid, sig)
	}
}

// send signals one process (pid > 1) or one process group (pid < -1), never this app's own process or group.
func (t *Tracker) send(pid int, sig syscall.Signal) {
	target := pid
	if pid < 0 {
		target = -pid
		if target <= 1 || target == ownPgrp() {
			return
		}
	} else if pid <= 1 || pid == os.Getpid() {
		return
	}
	_ = t.o.Signal(pid, sig)
	if sig == syscall.SIGTERM { // a stopped job cannot act on SIGTERM until it continues
		_ = t.o.Signal(pid, sigCont)
	}
}
