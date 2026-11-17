package taskstate

import "strings"

// CanAnswer reports whether bytes written to a task's terminal can answer a permission prompt or a question:
// they hold a carriage return (Enter), a lone Esc, or a digit, outside the terminal's own escape sequences.
//
// FU-A2-05c: the terminal page's Write also carries what xterm sends by itself: focus in and out reports (CSI I
// and CSI O once a tool turns on mode 1004), cursor position and device attribute replies and OSC replies, and
// the first arrow key in a menu (CSI A). None of these answers anything; calling OwnerInput or Touch for them
// would turn a waiting task into a running one before the person answered, and a task nobody answers would
// then hold the computer awake until the hard ceiling. The escape sequences are skipped before the rest is
// looked at, so the digits of a cursor position report do not count.
func CanAnswer(b []byte) bool {
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case c == 0x1b:
			if i+1 == len(b) {
				return true // a lone Esc, which closes a question
			}
			switch b[i+1] {
			case '[': // CSI: parameters, intermediates, one final byte 0x40-0x7e
				j := i + 2
				for j < len(b) && b[j] >= 0x20 && b[j] <= 0x3f {
					j++
				}
				if j < len(b) {
					j++ // the final byte
				}
				i = j - 1
			case ']', 'P', 'X', '^', '_': // OSC, DCS, SOS, PM, APC: up to BEL or ST (ESC \)
				j := i + 2
				for j < len(b) && b[j] != 0x07 && !(b[j] == 0x1b && j+1 < len(b) && b[j+1] == '\\') {
					j++
				}
				if j < len(b) && b[j] == 0x1b {
					j++
				}
				i = j
			case 'O': // SS3 (application cursor keys): ESC O and one byte
				i += 2
			default: // Alt+key: ESC and one byte
				i++
			}
		case c == '\r' || (c >= '0' && c <= '9'):
			return true
		}
	}
	return false
}

// IsTyping reports whether bytes written to a task's terminal come from the person: text, Enter, Esc, or a
// cursor or editing key (CSI with the final byte A, B, C, D, H, F or "~", and no "?", "<" or ">" in front of the
// parameters, which are the terminal's replies and mouse reports). Focus reports, cursor position and device
// attribute replies and OSC replies are not typing. Keep-awake's hard ceiling counts typing as input from the
// owner (plan 4.10), and nothing else.
func IsTyping(b []byte) bool {
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c != 0x1b {
			return true // text, Enter, a control key
		}
		if i+1 == len(b) {
			return true // a lone Esc
		}
		switch b[i+1] {
		case '[':
			j := i + 2
			private := j < len(b) && (b[j] == '?' || b[j] == '<' || b[j] == '>' || b[j] == '=')
			for j < len(b) && b[j] >= 0x20 && b[j] <= 0x3f {
				j++
			}
			if j < len(b) {
				if !private && strings.IndexByte("ABCDHF~", b[j]) >= 0 {
					return true
				}
				j++
			}
			i = j - 1
		case ']', 'P', 'X', '^', '_':
			j := i + 2
			for j < len(b) && b[j] != 0x07 && !(b[j] == 0x1b && j+1 < len(b) && b[j+1] == '\\') {
				j++
			}
			if j < len(b) && b[j] == 0x1b {
				j++
			}
			i = j
		default: // SS3 keys (ESC O A) and Alt+key
			return true
		}
	}
	return false
}
