package terminal

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultLogCap is the default size of one session's byte log (plan 5): memory stays near it.
	DefaultLogCap = 16 << 20
	chunkSize     = 64 << 10
)

// chunk is one fixed-size piece of the ring. Bytes before start have been trimmed.
type chunk struct {
	abs   int64 // absolute offset of buf[0]
	buf   []byte
	start int

	safeAt    int // first index in ground state on a character boundary (-1 none)
	safeModes modeSet
	prefAt    int // start of the latest full clear / reset / alt-screen entry (-1 none)
	prefModes modeSet
}

func (c *chunk) end() int64 { return c.abs + int64(len(c.buf)) }

// Log is an append-only byte log addressed by absolute offsets. It is a ring of fixed chunks, so memory stays
// near the cap. Append has one caller (the session's reader goroutine). Readers copy out under a read lock
// and never hold it while the bytes travel anywhere.
//
// Trimming follows the rules of plan 5: only where the escape parser is in ground state and on a UTF-8
// boundary, preferring the latest full clear or alt-screen entry shortly before the cut. The terminal modes
// as of the new base are kept, so a read of trimmed bytes can carry a prologue.
type Log struct {
	mu        sync.RWMutex
	cap       int
	chunkSize int
	chunks    []*chunk
	free      []*chunk
	base      int64
	baseModes modeSet
	trims     int64

	p *parser // owned by the appender

	// lockWaitMax is the longest time Append waited for the write lock (ns): a page reader can make it wait
	// for at most one copy of the bytes it reads.
	lockWaitMax atomic.Int64
}

// NewLog makes a log that keeps about capBytes (it is trimmed once it passes capBytes + capBytes/8 + capBytes/16).
func NewLog(capBytes int) *Log { return newLog(capBytes, chunkSize) }

func newLog(capBytes, chunk int) *Log { return newLogAt(capBytes, chunk, 0, defaultModes()) }

// newLogAt makes a log whose first byte has absolute offset base and whose terminal state there is modes. A
// session that reuses an ID starts at the previous run's tail, and an archive starts where its file says, so
// an offset means the same bytes for as long as the app runs (a page's old offset gets a gap, never other bytes).
func newLogAt(capBytes, chunk int, base int64, modes modeSet) *Log {
	if capBytes < 1 {
		capBytes = DefaultLogCap
	}
	p := newParser()
	p.cur = modes
	return &Log{cap: capBytes, chunkSize: chunk, p: p, base: base, baseModes: modes}
}

// Append adds bytes. It never blocks on a reader for longer than one copy.
func (l *Log) Append(p []byte) {
	for len(p) > 0 {
		var c *chunk
		if n := len(l.chunks); n > 0 {
			c = l.chunks[n-1]
		}
		if c == nil || len(c.buf) >= l.chunkSize {
			c = l.newChunk()
			c.abs = l.tailLocked() // the appender is the only writer: no lock needed to read the tail
		}
		n := l.chunkSize - len(c.buf)
		if n > len(p) {
			n = len(p)
		}
		piece := p[:n]
		out := l.p.feed(piece, c.end()) // no lock: the parser belongs to the appender

		t0 := time.Now()
		l.mu.Lock()
		if w := int64(time.Since(t0)); w > l.lockWaitMax.Load() {
			l.lockWaitMax.Store(w)
		}
		if len(l.chunks) == 0 || l.chunks[len(l.chunks)-1] != c {
			l.chunks = append(l.chunks, c)
		}
		at := len(c.buf)
		c.buf = append(c.buf, piece...)
		if c.safeAt < 0 && out.safe >= 0 {
			c.safeAt, c.safeModes = at+out.safe, out.safeModes
		}
		if out.pref.ok {
			l.placeMarker(out.pref)
		}
		l.mu.Unlock()
		p = p[n:]
	}
	l.mu.Lock()
	l.trimLocked()
	l.mu.Unlock()
}

func (l *Log) newChunk() *chunk {
	if n := len(l.free); n > 0 {
		c := l.free[n-1]
		l.free = l.free[:n-1]
		return c
	}
	return &chunk{buf: make([]byte, 0, l.chunkSize), safeAt: -1, prefAt: -1}
}

func (l *Log) recycle(c *chunk) {
	c.buf, c.start, c.safeAt, c.prefAt = c.buf[:0], 0, -1, -1
	if len(l.free) < 4 {
		l.free = append(l.free, c)
	}
}

// placeMarker stores a full-clear marker in the chunk that holds its first byte (the latest chunks only: a
// marker sequence is a few bytes long). Called with the write lock held.
func (l *Log) placeMarker(m marker) {
	for i := len(l.chunks) - 1; i >= 0 && i >= len(l.chunks)-3; i-- {
		c := l.chunks[i]
		if m.abs >= c.abs+int64(c.start) && m.abs < c.abs+int64(l.chunkSize) {
			if at := int(m.abs - c.abs); at > c.prefAt {
				c.prefAt, c.prefModes = at, m.modes
			}
			return
		}
	}
}

func (l *Log) tailLocked() int64 {
	if n := len(l.chunks); n > 0 {
		return l.chunks[n-1].end()
	}
	return l.base
}

// trimLocked drops the front when the log has grown a slack past its cap.
func (l *Log) trimLocked() {
	slack := l.cap / 16
	if slack < l.chunkSize {
		slack = l.chunkSize
	}
	tail := l.tailLocked()
	if tail-l.base <= int64(l.cap+l.cap/8+slack) {
		return
	}
	abs, modes, ok := l.findCutLocked(tail-int64(l.cap), l.cap/8)
	if !ok || abs <= l.base {
		return
	}
	l.cutLocked(abs, modes)
}

// findCutLocked picks the cut for "keep about the last cap bytes": the latest full clear or alt-screen entry
// within lookBack bytes before ideal, else the first safe point at or after ideal.
func (l *Log) findCutLocked(ideal int64, lookBack int) (abs int64, m modeSet, ok bool) {
	look := ideal - int64(lookBack)
	for i := len(l.chunks) - 1; i >= 0; i-- {
		c := l.chunks[i]
		if c.end() < look {
			break
		}
		if c.prefAt >= c.start && c.prefAt >= 0 {
			if a := c.abs + int64(c.prefAt); a <= ideal && a >= look && a > l.base {
				return a, c.prefModes, true
			}
		}
	}
	for i, c := range l.chunks {
		if c.end() <= ideal {
			continue
		}
		if ideal >= c.abs { // the chunk that holds ideal: look for the first safe point at or after it
			if a, m, ok := l.exactSafeLocked(i, ideal); ok {
				return a, m, true
			}
			continue
		}
		if c.safeAt >= 0 && c.safeAt >= c.start { // a later chunk: its first safe point
			return c.abs + int64(c.safeAt), c.safeModes, true
		}
	}
	return 0, modeSet{}, false
}

// exactSafeLocked finds the first position at or after ideal inside chunk i where the stream is in ground state
// on a character boundary, by running a parser from the chunk's first safe point. It reports false when the
// chunk has none (the caller then tries the next chunk).
func (l *Log) exactSafeLocked(i int, ideal int64) (int64, modeSet, bool) {
	c := l.chunks[i]
	var from int
	var modes modeSet
	switch {
	case c.safeAt >= 0 && c.safeAt >= c.start:
		from, modes = c.safeAt, c.safeModes
	case i == 0:
		from, modes = c.start, l.baseModes // the base itself is such a point
	default:
		return 0, modeSet{}, false
	}
	target := int(ideal - c.abs)
	p := parser{cur: modes}
	for j := from; j < len(c.buf); j++ {
		if j >= target && p.ground() {
			return c.abs + int64(j), p.cur, true
		}
		p.feed(c.buf[j:j+1], c.abs+int64(j))
	}
	if p.ground() && len(c.buf) >= target {
		return c.end(), p.cur, true
	}
	return 0, modeSet{}, false
}

// cutLocked makes abs the new base. abs lies in a chunk, or at the end of the last one.
func (l *Log) cutLocked(abs int64, modes modeSet) {
	k := 0
	for k < len(l.chunks)-1 && l.chunks[k].end() <= abs {
		k++
	}
	for _, c := range l.chunks[:k] {
		l.recycle(c)
	}
	l.chunks = append(l.chunks[:0], l.chunks[k:]...)
	l.chunks[0].start = int(abs - l.chunks[0].abs)
	l.base, l.baseModes = abs, modes
	l.trims++
}

// Result is what Read returns.
type Result struct {
	Data     []byte
	From     int64
	Next     int64
	Tail     int64
	Gap      int64
	Prologue []byte // set only when Gap > 0: the modes as of From
}

// Read copies up to max bytes starting at offset. The returned bytes are a copy: no lock is held while the
// caller encodes or sends them. An offset before the base returns the bytes from the base with Gap set and
// the mode prologue; an offset past the tail is clamped to the tail.
func (l *Log) Read(offset int64, max int) Result {
	l.mu.RLock()
	defer l.mu.RUnlock()
	tail := l.tailLocked()
	r := Result{Tail: tail}
	if offset < l.base {
		r.Gap = l.base - offset
		r.Prologue = l.baseModes.prologue()
		offset = l.base
	}
	if offset > tail {
		offset = tail
	}
	end := tail
	if max >= 0 && end-offset > int64(max) {
		end = offset + int64(max)
	}
	r.From, r.Next = offset, end
	if end > offset {
		r.Data = make([]byte, 0, end-offset)
		for off := offset; off < end; {
			c := l.chunks[int((off-l.chunks[0].abs)/int64(l.chunkSize))]
			lo := int(off - c.abs)
			hi := len(c.buf)
			if rem := int(end - off); hi-lo > rem {
				hi = lo + rem
			}
			r.Data = append(r.Data, c.buf[lo:hi]...)
			off += int64(hi - lo)
		}
	}
	return r
}

// Tail is the offset one past the last byte.
func (l *Log) Tail() int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.tailLocked()
}

// Base is the offset of the first byte still held.
func (l *Log) Base() int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.base
}

// Trims is the number of times the front has been cut.
func (l *Log) Trims() int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.trims
}

// Size is the number of bytes held.
func (l *Log) Size() int64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.tailLocked() - l.base
}

// Snapshot returns about the last max bytes (cut like a trim would) with the mode prologue when anything
// before them was left out (from is the absolute offset of the first byte returned). It is what the scrollback
// file keeps.
func (l *Log) Snapshot(max int) (data, prologue []byte, from int64) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	tail := l.tailLocked()
	from, modes := l.base, l.baseModes
	if tail-l.base > int64(max) {
		if a, m, ok := l.findCutLocked(tail-int64(max), max/8); ok {
			from, modes = a, m
		}
	}
	if from > 0 {
		prologue = modes.prologue()
	}
	for off := from; off < tail; {
		c := l.chunks[int((off-l.chunks[0].abs)/int64(l.chunkSize))]
		lo := int(off - c.abs)
		data = append(data, c.buf[lo:]...)
		off += int64(len(c.buf) - lo)
	}
	return data, prologue, from
}
