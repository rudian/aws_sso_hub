package terminal

import (
	"bytes"
	"math/rand"
	"strings"
	"sync"
	"testing"
)

func TestLogOffsetsAndGap(t *testing.T) {
	l := newLog(64, 16)
	l.Append([]byte("0123456789"))
	r := l.Read(4, 3)
	if string(r.Data) != "456" || r.From != 4 || r.Next != 7 || r.Tail != 10 || r.Gap != 0 || r.Prologue != nil {
		t.Fatalf("%+v", r)
	}
	r = l.Read(10, 5) // at the tail
	if len(r.Data) != 0 || r.Next != 10 {
		t.Fatalf("tail read: %+v", r)
	}
	r = l.Read(99, 5) // past the tail is clamped
	if len(r.Data) != 0 || r.From != 10 || r.Next != 10 {
		t.Fatalf("past tail: %+v", r)
	}
	// across chunk boundaries
	l.Append([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ"))
	if r = l.Read(8, 12); string(r.Data) != "89ABCDEFGHIJ" {
		t.Fatalf("%q", r.Data)
	}
	// the returned bytes are a copy
	r.Data[0] = 'x'
	if r = l.Read(8, 2); string(r.Data) != "89" {
		t.Fatalf("log changed through a returned slice: %q", r.Data)
	}
}

func TestLogTrimsAndReportsGap(t *testing.T) {
	l := newLog(100, 16)
	var all []byte
	for i := 0; i < 50; i++ {
		b := []byte(strings.Repeat(string(rune('a'+i%26)), 10))
		all = append(all, b...)
		l.Append(b)
	}
	if l.Base() == 0 || l.Trims() == 0 {
		t.Fatal("expected a trim")
	}
	if l.Tail() != int64(len(all)) {
		t.Fatalf("tail %d want %d", l.Tail(), len(all))
	}
	if sz := l.Size(); sz > 100+100/8+100/16+16 || sz < 50 {
		t.Fatalf("size %d not near cap", sz)
	}
	r := l.Read(0, 1<<20)
	if r.Gap != l.Base() || r.From != l.Base() || len(r.Prologue) == 0 {
		t.Fatalf("gap=%d from=%d base=%d prologue=%q", r.Gap, r.From, l.Base(), r.Prologue)
	}
	if !bytes.Equal(r.Data, all[r.From:]) {
		t.Fatal("retained bytes differ from the stream")
	}
	// an offset inside the retained range has no gap and no prologue
	r2 := l.Read(l.Base()+5, 10)
	if r2.Gap != 0 || r2.Prologue != nil || !bytes.Equal(r2.Data, all[l.Base()+5:l.Base()+15]) {
		t.Fatalf("%+v", r2)
	}
}

// reference runs a second parser byte by byte to know, for every offset, whether the stream is in ground state
// on a character boundary there and which modes are on.
type reference struct {
	ground []bool
	modes  []modeSet
}

func newReference(stream []byte) reference {
	p := newParser()
	r := reference{ground: []bool{true}, modes: []modeSet{p.cur}}
	for i := range stream {
		p.feed(stream[i:i+1], int64(i))
		r.ground = append(r.ground, p.ground())
		r.modes = append(r.modes, p.cur)
	}
	return r
}

var fragments = []string{
	"hello world ", "line\r\n", "é", "世界", "😀", "\x1b[31m", "\x1b[0m", "\x1b[38;5;81m", "\x1b[?1049h", "\x1b[?1049l",
	"\x1b[?2004h", "\x1b[?2004l", "\x1b[?25l", "\x1b[?25h", "\x1b[?1000;1006h", "\x1b[?1000;1006l", "\x1b[2J", "\x1b[H",
	"\x1b]0;a title\x07", "\x1b]8;;http://example.com\x1b\\link\x1b]8;;\x1b\\", "\x1b=", "\x1b>", "\x1bc", "\x1b[?1002h", "\x1b[?1003h", "\x1b[?1000l", "\x1b[?1047h", "\x1b[?1047l", "\x1b[?47h", "\x1b[?66h", "\x1b[?66l", "\x1b[?1h", "\x1b[?1l", "\x1b[?1016h", "\x1b[?1015h", "\x1b[?1006l", "\x1b[!p", "\x1b[>1u", "\x1b[<u",
	"\x1b(B", "\x1bP1$r0m\x1b\\", "\x1b[>4;2m",
}

func randomStream(rng *rand.Rand, n int) []byte {
	var b []byte
	for len(b) < n {
		b = append(b, fragments[rng.Intn(len(fragments))]...)
	}
	return b
}

// The log is trimmed only in ground state, on a UTF-8 boundary, and the mode state kept for the new base is the
// state of the stream at that offset, whatever the chunking of the appends.
func TestTrimNeverCutsInsideASequenceOrCharacter(t *testing.T) {
	for seed := int64(1); seed <= 40; seed++ {
		rng := rand.New(rand.NewSource(seed))
		stream := randomStream(rng, 6000)
		ref := newReference(stream)
		l := newLog(300, 32)
		for off := 0; off < len(stream); {
			n := 1 + rng.Intn(70)
			if off+n > len(stream) {
				n = len(stream) - off
			}
			l.Append(stream[off : off+n])
			off += n
			base := l.Base()
			if !ref.ground[base] {
				t.Fatalf("seed %d: base %d is inside a sequence or character (%q)", seed, base, stream[max(0, base-6):min(int64(len(stream)), base+6)])
			}
			if l.baseModes != ref.modes[base] {
				t.Fatalf("seed %d: modes at base %d: got %+v want %+v", seed, base, l.baseModes, ref.modes[base])
			}
			if got := l.Read(base, 1<<20); !bytes.Equal(got.Data, stream[base:off]) {
				t.Fatalf("seed %d: retained bytes differ at base %d", seed, base)
			}
			if sz := l.Size(); sz > 300+300/8+300/16+32+200 {
				t.Fatalf("seed %d: size %d not bounded", seed, sz)
			}
		}
		if l.Trims() == 0 {
			t.Fatalf("seed %d: never trimmed", seed)
		}
	}
}

func TestTrimPrefersTheLatestClearBeforeTheCut(t *testing.T) {
	l := newLog(1000, 64)
	l.Append([]byte(strings.Repeat("a", 100)))
	l.Append([]byte("\x1b[2J"))
	l.Append([]byte(strings.Repeat("b", 1190-104)))
	if l.Base() != 100 {
		t.Fatalf("base %d, want the start of the clear at 100", l.Base())
	}
	if r := l.Read(100, 4); string(r.Data) != "\x1b[2J" {
		t.Fatalf("%q", r.Data)
	}
}

func TestAltScreenEntryIsAPreferredCut(t *testing.T) {
	l := newLog(1000, 64)
	l.Append([]byte(strings.Repeat("a", 120)))
	l.Append([]byte("\x1b[?1049h"))
	l.Append([]byte(strings.Repeat("b", 1190-120-8)))
	if l.Base() != 120 {
		t.Fatalf("base %d, want the alt-screen entry at 120", l.Base())
	}
	// cut before the entry: the prologue must not already be in the alt screen
	if strings.Contains(string(l.Read(0, 10).Prologue), "\x1b[?1049h") {
		t.Fatal("prologue is in the alt screen although the entry is part of the data")
	}
}

func TestSplitSequencesAcrossAppends(t *testing.T) {
	l := newLog(1<<20, 64)
	for _, c := range []string{"hello \x1b[?10", "49h more \x1b", "[?2004", ";1006h \x1b[?25l \x1b[?2004l"} {
		l.Append([]byte(c))
	}
	m := l.p.cur
	if !m.isDEC(1049) || !m.isDEC(1006) || m.isDEC(2004) || m.isDEC(25) {
		t.Fatalf("%+v", m)
	}
	p := string(m.prologue())
	if !strings.Contains(p, "\x1b[?1049h") || !strings.Contains(p, "\x1b[?1006h") || !strings.Contains(p, "\x1b[?25l") || !strings.Contains(p, "\x1b[?2004l") {
		t.Fatalf("%q", p)
	}
}

func TestOtherModes(t *testing.T) {
	p := newParser()
	p.feed([]byte("\x1b=\x1b[>4;2m\x1b[>5u\x1b[>1u"), 0)
	if !p.cur.keypad || p.cur.modOther != 2 || p.cur.kittyN != 2 || p.cur.kitty[0] != 5 || p.cur.kitty[1] != 1 {
		t.Fatalf("%+v", p.cur)
	}
	p.feed([]byte("\x1b[<u"), 0)
	if p.cur.kittyN != 1 {
		t.Fatalf("%+v", p.cur)
	}
	pro := string(p.cur.prologue())
	if !strings.Contains(pro, "\x1b=") || !strings.Contains(pro, "\x1b[>4;2m") || !strings.Contains(pro, "\x1b[>5u") {
		t.Fatalf("%q", pro)
	}
	p.feed([]byte("\x1b[?1h\x1bc"), 0) // RIS resets everything
	if p.cur != defaultModes() {
		t.Fatalf("%+v", p.cur)
	}
	p.feed([]byte("\x1b[?1h\x1b[?25l\x1b="), 0)
	p.feed([]byte("\x1b[!p"), 0) // DECSTR
	if p.cur.isDEC(1) || !p.cur.isDEC(25) || p.cur.keypad {
		t.Fatalf("%+v", p.cur)
	}
}

func TestOverlongStringDoesNotPinTheLog(t *testing.T) {
	p := newParser()
	p.feed([]byte("\x1b]0;"), 0)
	if p.ground() {
		t.Fatal("inside an OSC")
	}
	p.feed(bytes.Repeat([]byte("x"), maxStr+10), 5)
	if !p.ground() {
		t.Fatal("an overlong string must end so that the log can be trimmed")
	}
}

func TestConcurrentReadersWhileAppending(t *testing.T) {
	l := newLog(4096, 256)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var off int64
			for {
				select {
				case <-stop:
					return
				default:
				}
				r := l.Read(off, 1000)
				for _, b := range r.Data { // the bytes are the stream's: all 'x' or the fixed sequences
					if b != 'x' && b != '\x1b' && b != '[' && b != '2' && b != 'J' {
						t.Errorf("unexpected byte %q", b)
						return
					}
				}
				off = r.Next
			}
		}()
	}
	chunk := []byte(strings.Repeat("x", 500) + "\x1b[2J")
	for i := 0; i < 2000; i++ {
		l.Append(chunk)
	}
	close(stop)
	wg.Wait()
	if l.Tail() != int64(2000*len(chunk)) {
		t.Fatal(l.Tail())
	}
}

func BenchmarkAppendEscapeDense(b *testing.B) {
	chunk := bytes.Repeat([]byte("\x1b[38;5;81mabc\x1b[0m def\x1b[?25l\x1b[H"), 64*1024/30)
	l := NewLog(16 << 20)
	b.SetBytes(int64(len(chunk)))
	for i := 0; i < b.N; i++ {
		l.Append(chunk)
	}
}

func BenchmarkAppendPlain(b *testing.B) {
	chunk := bytes.Repeat([]byte("0123456789 the quick brown fox\r\n"), 64*1024/32)
	l := NewLog(16 << 20)
	b.SetBytes(int64(len(chunk)))
	for i := 0; i < b.N; i++ {
		l.Append(chunk)
	}
}

func BenchmarkAppendWithClearsEveryFrame(b *testing.B) {
	chunk := bytes.Repeat([]byte("\x1b[2J\x1b[Hframe \x1b[1;31mred\x1b[0m\r\n"), 64*1024/32)
	l := NewLog(16 << 20)
	b.SetBytes(int64(len(chunk)))
	for i := 0; i < b.N; i++ {
		l.Append(chunk)
	}
}
