package terminal

import "strconv"

// The escape parser of the byte log. It follows the stream in the reader goroutine and answers three
// questions without ever holding up the reader:
//
//  1. Where is the stream in "ground" state, on a UTF-8 character boundary? Only there may the log be trimmed.
//  2. Which terminal modes are on at such a point? (DEC private modes and a few more.) After a trim, a page
//     that asks for trimmed bytes gets these as a prologue, so alt screen, mouse, bracketed paste and the like
//     are right before the child repaints.
//  3. Where did the latest full clear or alt-screen entry start? A trim prefers to begin there.
//
// The parser is a small state machine in the style of the DEC ANSI parser (vt100.net). It never allocates.

// Plain DEC private modes the parser follows (CSI ? Ps h / l), one bit each. The modes that xterm.js keeps
// in one shared state are not here: they are groups with one value each (see modeSet).
var trackedDEC = [...]int{
	1,    // application cursor keys
	25,   // cursor visible (on by default)
	1004, // focus reporting
	1007, // alternate scroll
	2004, // bracketed paste
}

const (
	modeCursorVisible = 25
	kittyDepth        = 4
)

func decIndex(n int) int {
	for i, m := range trackedDEC {
		if m == n {
			return i
		}
	}
	return -1
}

// The grouped modes. xterm.js (InputHandler.ts) gives each group one state, so the terminal's behaviour is:
//   - alt buffer: ?47, ?1047 and ?1049 set switch to the alternate buffer, a reset of any of them back to the
//     normal one (the last set wins);
//   - mouse protocol: ?9, ?1000, ?1002 and ?1003 set the protocol, a reset of any of them means none;
//   - mouse encoding: ?1005, ?1006, ?1015 and ?1016 set the encoding, a reset of any of them means default;
//   - keypad: ?66 and ESC = / ESC > are one switch.
var (
	altModes   = [...]int{47, 1047, 1049}
	mouseModes = [...]int{9, 1000, 1002, 1003}
	encModes   = [...]int{1005, 1006, 1015, 1016}
)

func inGroup(g []int, n int) bool {
	for _, m := range g {
		if m == n {
			return true
		}
	}
	return false
}

// modeSet is the tracked terminal state. It is small and comparable, so a snapshot is a plain copy.
type modeSet struct {
	dec      uint32 // bit i: trackedDEC[i] is on
	alt      uint16 // 0 = normal buffer, else the mode that entered the alternate one (47, 1047 or 1049)
	mouse    uint16 // 0 = no mouse reporting, else the protocol mode (9, 1000, 1002 or 1003)
	enc      uint16 // 0 = default encoding, else the encoding mode (1005, 1006, 1015 or 1016)
	keypad   bool   // application keypad (ESC = and ?66)
	modOther uint8  // xterm modifyOtherKeys level (CSI > 4 ; n m)
	kittyN   uint8  // depth of the kitty keyboard flag stack (CSI > n u pushes, CSI < n u pops)
	kitty    [kittyDepth]uint8
}

func defaultModes() modeSet {
	var m modeSet
	m.setDEC(modeCursorVisible, true)
	return m
}

func (m *modeSet) setDEC(n int, on bool) {
	switch {
	case inGroup(altModes[:], n):
		m.alt = 0
		if on {
			m.alt = uint16(n)
		}
	case inGroup(mouseModes[:], n):
		m.mouse = 0
		if on {
			m.mouse = uint16(n)
		}
	case inGroup(encModes[:], n):
		m.enc = 0
		if on {
			m.enc = uint16(n)
		}
	case n == 66:
		m.keypad = on
	default:
		i := decIndex(n)
		if i < 0 {
			return
		}
		if on {
			m.dec |= 1 << uint(i)
		} else {
			m.dec &^= 1 << uint(i)
		}
	}
}

// isDEC reports whether mode n is on. For a grouped mode, it is on only when it is the group's current value.
func (m modeSet) isDEC(n int) bool {
	switch {
	case inGroup(altModes[:], n):
		return m.alt == uint16(n)
	case inGroup(mouseModes[:], n):
		return m.mouse == uint16(n)
	case inGroup(encModes[:], n):
		return m.enc == uint16(n)
	case n == 66:
		return m.keypad
	}
	i := decIndex(n)
	return i >= 0 && m.dec&(1<<uint(i)) != 0
}

// prologue is the byte string that brings a terminal into this state. For every group it writes the reset
// first and then the single set that is active, so the shared state of xterm.js ends up right whatever order
// the modes were turned on in, and whatever the page's terminal looked like before.
func (m modeSet) prologue() []byte {
	b := make([]byte, 0, 160)
	// alt screen first: entering it clears the alt buffer and saves the cursor, and nothing else is affected
	b = appendMode(b, 47, false)
	if m.alt != 0 {
		b = appendMode(b, int(m.alt), true)
	}
	b = appendMode(b, 1000, false) // any member's reset means "no protocol"
	if m.mouse != 0 {
		b = appendMode(b, int(m.mouse), true)
	}
	b = appendMode(b, 1006, false) // any member's reset means "default encoding"
	if m.enc != 0 {
		b = appendMode(b, int(m.enc), true)
	}
	for _, n := range trackedDEC {
		b = appendMode(b, n, m.isDEC(n))
	}
	if m.keypad {
		b = append(b, 0x1b, '=')
	} else {
		b = append(b, 0x1b, '>')
	}
	b = append(b, "\x1b[>4;"...)
	b = strconv.AppendInt(b, int64(m.modOther), 10)
	b = append(b, 'm')
	for i := 0; i < int(m.kittyN) && i < kittyDepth; i++ {
		b = append(b, "\x1b[>"...)
		b = strconv.AppendInt(b, int64(m.kitty[i]), 10)
		b = append(b, 'u')
	}
	return b
}

func appendMode(b []byte, n int, on bool) []byte {
	b = append(b, "\x1b[?"...)
	b = strconv.AppendInt(b, int64(n), 10)
	if on {
		return append(b, 'h')
	}
	return append(b, 'l')
}

const (
	sGround = iota
	sEsc
	sEscInter
	sCSI
	sCSIIgnore
	sStr    // OSC, DCS, APC, PM, SOS: up to BEL or ST
	sStrEsc // ESC inside a string, waiting for the backslash
)

// maxStr: a string sequence longer than this is treated as ended. xterm drops overlong strings too, and it
// keeps "ground" reachable, so the log can always be trimmed.
const maxStr = 1 << 20

// maxSeq: an ESC or CSI sequence that has not ended after this many bytes is treated as ended, for the same
// reason (a stream of NULs, digits or newlines after an ESC would otherwise never reach ground).
const maxSeq = 4096

// marker is a position where a trim would be welcome: the start of a full clear, a reset or an alt-screen entry.
type marker struct {
	ok    bool
	abs   int64
	modes modeSet // the state before the sequence
}

// scanOut is what one call of feed found.
type scanOut struct {
	safe      int // first index (0..len) at which the stream is in ground state on a character boundary; -1 none
	safeModes modeSet
	pref      marker // the latest marker whose sequence was completed in this piece
}

type parser struct {
	st   uint8
	need uint8 // UTF-8 continuation bytes still expected
	cur  modeSet

	// CSI collection
	prefix byte // '?', '>', '<', '=' or 0
	inter  byte // last intermediate byte
	params [6]int32
	np     int
	num    int32
	hasNum bool

	strLen   int
	run      int   // bytes since the parser was last in ground (a pending UTF-8 character counts); never reset by begin()
	seqStart int64 // absolute offset of the ESC that began the current sequence
	seqModes modeSet
}

func newParser() *parser { return &parser{cur: defaultModes()} }

func (p *parser) ground() bool { return p.st == sGround && p.need == 0 }

func (p *parser) begin(abs int64) {
	p.seqStart = abs
	p.seqModes = p.cur
	p.st = sEsc
	p.need = 0
}

func (p *parser) resetCSI() {
	p.prefix, p.inter = 0, 0
	p.np, p.num, p.hasNum = 0, 0, false
}

// feed advances the parser over data, which starts at absolute offset abs.
func (p *parser) feed(data []byte, abs int64) scanOut {
	out := scanOut{safe: -1}
	if p.ground() {
		out.safe, out.safeModes = 0, p.cur
	}
	for i := 0; i < len(data); i++ {
		b := data[i]
		if b < 0x80 && b != 0x1b && p.st == sGround && p.need == 0 { // plain text: the common case
			if out.safe < 0 {
				out.safe, out.safeModes = i+1, p.cur
			}
			continue
		}
	again:
		switch p.st {
		case sGround:
			switch {
			case b == 0x1b:
				p.begin(abs + int64(i))
			case p.need > 0 && b&0xC0 == 0x80:
				p.need--
			default:
				p.need = 0 // a lead byte, or garbage that ends a pending character
				switch {
				case b >= 0xC2 && b <= 0xDF:
					p.need = 1
				case b >= 0xE0 && b <= 0xEF:
					p.need = 2
				case b >= 0xF0 && b <= 0xF4:
					p.need = 3
				}
			}
		case sEsc:
			switch {
			case b == 0x1b:
				p.begin(abs + int64(i))
			case b == 0x18 || b == 0x1a:
				p.st = sGround
			case b == '[':
				p.st = sCSI
				p.resetCSI()
			case b == ']' || b == 'P' || b == 'X' || b == '^' || b == '_':
				p.st, p.strLen = sStr, 0
			case b >= 0x20 && b <= 0x2f:
				p.st = sEscInter
			case b == 'c': // RIS: full reset
				p.cur = defaultModes()
				out.pref = marker{true, p.seqStart, p.seqModes}
				p.st = sGround
			case b == '=':
				p.cur.keypad = true
				p.st = sGround
			case b == '>':
				p.cur.keypad = false
				p.st = sGround
			case b >= 0x30 && b <= 0x7e:
				p.st = sGround
			}
		case sEscInter:
			switch {
			case b == 0x1b:
				p.begin(abs + int64(i))
			case b == 0x18 || b == 0x1a:
				p.st = sGround
			case b >= 0x30 && b <= 0x7e:
				p.st = sGround
			}
		case sCSI:
			switch {
			case b == 0x1b:
				p.begin(abs + int64(i))
			case b == 0x18 || b == 0x1a:
				p.st = sGround
			case b >= '0' && b <= '9':
				p.hasNum = true
				if p.num < 100000 {
					p.num = p.num*10 + int32(b-'0')
				}
			case b == ';' || b == ':':
				p.pushParam()
			case b >= 0x3c && b <= 0x3f:
				if p.prefix == 0 && p.np == 0 && !p.hasNum {
					p.prefix = b
				} else {
					p.st = sCSIIgnore
				}
			case b >= 0x20 && b <= 0x2f:
				p.inter = b
			case b >= 0x40 && b <= 0x7e:
				p.pushParamLast()
				if m := p.csi(b); m {
					out.pref = marker{true, p.seqStart, p.seqModes}
				}
				p.st = sGround
			}
		case sCSIIgnore:
			switch {
			case b == 0x1b:
				p.begin(abs + int64(i))
			case b == 0x18 || b == 0x1a:
				p.st = sGround
			case b >= 0x40 && b <= 0x7e:
				p.st = sGround
			}
		case sStr:
			p.strLen++
			switch {
			case b == 0x07 || b == 0x18 || b == 0x1a:
				p.st = sGround
			case b == 0x1b:
				p.st = sStrEsc
			case p.strLen > maxStr:
				p.st = sGround
			}
		case sStrEsc:
			if b == '\\' {
				p.st = sGround
			} else {
				p.begin(abs + int64(i) - 1) // the ESC was the previous byte
				goto again
			}
		}
		if p.ground() {
			p.run = 0
		} else {
			p.run++
			limit := maxSeq
			if p.st == sStr || p.st == sStrEsc {
				limit = maxStr
			}
			if p.run > limit { // never reaching ground would pin the log: treat the sequence as ended
				p.st, p.need, p.run = sGround, 0, 0
			}
		}
		if out.safe < 0 && p.ground() {
			out.safe, out.safeModes = i+1, p.cur
		}
	}
	return out
}

func (p *parser) pushParam() {
	if p.np < len(p.params) {
		p.params[p.np] = p.num
		p.np++
	}
	p.num, p.hasNum = 0, false
}

func (p *parser) pushParamLast() {
	if p.hasNum || p.np > 0 {
		p.pushParam()
	}
}

// param returns parameter i, or def when the sequence has fewer parameters.
func (p *parser) param(i, def int) int {
	if i < p.np {
		return int(p.params[i])
	}
	return def
}

// csi applies a finished CSI sequence. It reports true when the sequence is a full clear or an alt-screen entry.
func (p *parser) csi(final byte) (marked bool) {
	switch p.prefix {
	case '?':
		if p.inter != 0 || (final != 'h' && final != 'l') {
			return false
		}
		on := final == 'h'
		for i := 0; i < p.np; i++ {
			n := int(p.params[i])
			p.cur.setDEC(n, on)
			if on && inGroup(altModes[:], n) {
				marked = true
			}
		}
	case '>':
		switch final {
		case 'u': // push kitty keyboard flags
			if p.cur.kittyN < kittyDepth {
				p.cur.kitty[p.cur.kittyN] = uint8(p.param(0, 0))
				p.cur.kittyN++
			} else {
				copy(p.cur.kitty[:], p.cur.kitty[1:])
				p.cur.kitty[kittyDepth-1] = uint8(p.param(0, 0))
			}
		case 'm': // modifyOtherKeys
			if p.param(0, 0) == 4 {
				p.cur.modOther = uint8(p.param(1, 0))
			}
		}
	case '<':
		if final == 'u' {
			n := p.param(0, 1)
			if n <= 0 {
				n = 1
			}
			if n > int(p.cur.kittyN) {
				n = int(p.cur.kittyN)
			}
			for i := int(p.cur.kittyN) - n; i < int(p.cur.kittyN); i++ {
				p.cur.kitty[i] = 0 // keep modeSet comparable: nothing stale above the top
			}
			p.cur.kittyN -= uint8(n)
		}
	case '=':
		if final == 'u' {
			if p.cur.kittyN == 0 {
				p.cur.kittyN = 1
			}
			top := &p.cur.kitty[p.cur.kittyN-1]
			f := uint8(p.param(0, 0))
			switch p.param(1, 1) {
			case 2:
				*top |= f
			case 3:
				*top &^= f
			default:
				*top = f
			}
		}
	case 0:
		switch {
		case final == 'J' && p.inter == 0:
			if n := p.param(0, 0); n == 2 || n == 3 {
				marked = true
			}
		case final == 'p' && p.inter == '!': // DECSTR soft reset
			p.cur.setDEC(1, false)
			p.cur.setDEC(modeCursorVisible, true)
			p.cur.keypad = false
		}
	}
	return marked
}
