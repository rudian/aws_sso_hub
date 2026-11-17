package terminal

import (
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// xtermModel is an oracle for the target terminal, written from how xterm.js 6 (src/common/InputHandler.ts)
// treats these modes, and independent of modeSet and the parser: several modes share one state there, so a
// reset of any member clears the group ("?1002h" then "?1000l" leaves no mouse; any of "?47l", "?1047l" and
// "?1049l" returns to the normal buffer; "?66" and ESC =/> are one switch).
type xtermModel struct {
	alt, mouse, enc             int // alt: 0/1; mouse and enc: the active mode or 0
	keypad, appCursor, cursorOn bool
	focus, altScroll, paste     bool
}

func newXtermModel() xtermModel { return xtermModel{cursorOn: true} }

var modelSeq = regexp.MustCompile(`\x1b\[\?[0-9;]+[hl]|\x1b[=>c]|\x1b\[!p`)

func (x *xtermModel) set(n int, on bool) {
	b := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	switch n {
	case 1:
		x.appCursor = on
	case 9, 1000, 1002, 1003:
		x.mouse = b(on) * n
	case 25:
		x.cursorOn = on
	case 47, 1047, 1049:
		x.alt = b(on)
	case 66:
		x.keypad = on
	case 1004:
		x.focus = on
	case 1005, 1006, 1015, 1016:
		x.enc = b(on) * n
	case 1007:
		x.altScroll = on
	case 2004:
		x.paste = on
	}
}

func (x *xtermModel) write(stream string) {
	for _, m := range modelSeq.FindAllString(stream, -1) {
		switch {
		case m == "\x1b=":
			x.keypad = true
		case m == "\x1b>":
			x.keypad = false
		case m == "\x1bc":
			*x = newXtermModel()
		case m == "\x1b[!p":
			x.appCursor, x.cursorOn, x.keypad = false, true, false
		default:
			on := m[len(m)-1] == 'h'
			for _, f := range strings.Split(m[3:len(m)-1], ";") {
				n, _ := strconv.Atoi(f)
				x.set(n, on)
			}
		}
	}
}

func modelOf(streams ...string) xtermModel {
	x := newXtermModel()
	for _, s := range streams {
		x.write(s)
	}
	return x
}

func prologueOf(stream string) string {
	p := newParser()
	p.feed([]byte(stream), 0)
	return string(p.cur.prologue())
}

func TestPrologueGivesXtermTheRightGroupStates(t *testing.T) {
	cases := []struct {
		name, stream string
		want         xtermModel
	}{
		{"vim", "\x1b[?1049h\x1b[?1000h\x1b[?1006h\x1b[?2004h\x1b[?25l", xtermModel{alt: 1, mouse: 1000, enc: 1006, paste: true}},
		{"drag TUI", "\x1b[?1049h\x1b[?1002h\x1b[?1006h", xtermModel{alt: 1, mouse: 1002, enc: 1006, cursorOn: true}},
		{"any motion and pixels", "\x1b[?1003h\x1b[?1016h", xtermModel{mouse: 1003, enc: 1016, cursorOn: true}},
		{"alt via 1047", "\x1b[?1047h", xtermModel{alt: 1, cursorOn: true}},
		{"alt via 47", "\x1b[?47h\x1b[?1000h", xtermModel{alt: 1, mouse: 1000, cursorOn: true}},
		{"keypad via ?66", "\x1b[?66h", xtermModel{keypad: true, cursorOn: true}},
		{"keypad via ESC =, off via ?66l", "\x1b=\x1b[?66l", xtermModel{cursorOn: true}},
		{"set 1002 then reset 1000: no mouse", "\x1b[?1002h\x1b[?1000l", xtermModel{cursorOn: true}},
		{"set 1000 then 1002: the last wins", "\x1b[?1000h\x1b[?1002h", xtermModel{mouse: 1002, cursorOn: true}},
		{"encoding reset by another member", "\x1b[?1006h\x1b[?1016l", xtermModel{cursorOn: true}},
		{"left the alt screen", "\x1b[?1049h\x1b[?1049l", xtermModel{cursorOn: true}},
		{"entered by 1049, left by 47", "\x1b[?1049h\x1b[?47l", xtermModel{cursorOn: true}},
		{"focus, scroll, cursor keys", "\x1b[?1004h\x1b[?1007h\x1b[?1h", xtermModel{focus: true, altScroll: true, appCursor: true, cursorOn: true}},
	}
	dirty := modelOf("\x1b[?1003h\x1b[?1015h\x1b[?1049h\x1b=\x1b[?2004h\x1b[?25l\x1b[?1h")
	for _, c := range cases {
		if got := modelOf(c.stream); got != c.want {
			t.Fatalf("%s: the oracle itself gives %+v, want %+v", c.name, got, c.want)
		}
		pro := prologueOf(c.stream)
		if got := modelOf(pro); got != c.want {
			t.Errorf("%s: prologue %q gives %+v, want %+v", c.name, pro, got, c.want)
		}
		// the page's terminal need not be fresh: the prologue also overrides whatever it held
		d := dirty
		d.write(pro)
		if d != c.want {
			t.Errorf("%s: on a used terminal the prologue gives %+v, want %+v", c.name, d, c.want)
		}
	}
}

// For every point where the stream is in ground state: the prologue of the modes there, then the rest of the
// stream, leaves the target terminal as the whole stream does.
func TestPrologueThenRestMatchesTheWholeStreamInXterm(t *testing.T) {
	for seed := int64(200); seed < 260; seed++ {
		rng := rand.New(rand.NewSource(seed))
		stream := randomStream(rng, 3000)
		ref := newReference(stream)
		want := modelOf(string(stream))
		for cut := 0; cut <= len(stream); cut += 1 + rng.Intn(40) {
			if !ref.ground[cut] {
				continue
			}
			got := modelOf(string(ref.modes[cut].prologue()), string(stream[cut:]))
			if got != want {
				t.Fatalf("seed %d cut %d: got %+v, want %+v", seed, cut, got, want)
			}
		}
	}
}

// The same through the log: the prologue of a read of trimmed bytes plus the data it returns.
func TestTrimmedReadReplaysLikeTheWholeStreamInXterm(t *testing.T) {
	for seed := int64(300); seed < 330; seed++ {
		rng := rand.New(rand.NewSource(seed))
		stream := randomStream(rng, 5000)
		l := newLog(400, 64)
		l.Append(stream)
		r := l.Read(0, 1<<20)
		if r.Gap == 0 {
			t.Fatalf("seed %d: no trim", seed)
		}
		if got, want := modelOf(string(r.Prologue), string(r.Data)), modelOf(string(stream)); got != want {
			t.Fatalf("seed %d: replay gives %+v, the whole stream %+v", seed, got, want)
		}
	}
}

func TestPrologueRestoresAltScreenAfterTrim(t *testing.T) {
	l := newLog(1000, 64)
	l.Append([]byte("\x1b[?1049h\x1b[?1000h\x1b[?1006h\x1b[?2004h\x1b[?25l"))
	l.Append([]byte(strings.Repeat("repaint ", 800)))
	r := l.Read(0, 1<<20)
	if r.Gap == 0 {
		t.Fatal("no trim")
	}
	want := xtermModel{alt: 1, mouse: 1000, enc: 1006, paste: true}
	if got := modelOf(string(r.Prologue)); got != want {
		t.Fatalf("%q gives %+v", r.Prologue, got)
	}
	// leave the alt screen later: the next prologue says so
	l.Append([]byte("\x1b[?1049l\x1b[?25h\x1b[?2004l\x1b[?1000l\x1b[?1006l"))
	l.Append([]byte(strings.Repeat("shell ", 800)))
	r = l.Read(0, 1<<20)
	if got := modelOf(string(r.Prologue)); got != newXtermModel() {
		t.Fatalf("%q gives %+v", r.Prologue, got)
	}
	if strings.Contains(string(r.Prologue), "h") && strings.Contains(string(r.Prologue), "\x1b[?1049h") {
		t.Errorf("second prologue still enters the alt screen: %q", r.Prologue)
	}
}
