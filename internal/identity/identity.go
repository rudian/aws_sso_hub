// Package identity assigns the colour slot and monogram that identify an AWS account in the UI.
package identity

import (
	"strings"
	"unicode"
)

// AutoOrder is the order automatic colours are handed out in; slot 8 is only ever chosen by the user.
var AutoOrder = []int{1, 3, 2, 6, 5, 4, 7}

// ProfileKeyPrefix keys the colour of a profile whose account id is not known yet.
const ProfileKeyPrefix = "profile:"

// Input is a profile in config file order.
type Input struct{ Profile, AccountID string }

// Result is the identity of one profile.
type Result struct {
	Slot     int
	Monogram string
}

func isAlnum(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// ValidMonogram: exactly 2 runes of [A-Za-z0-9], or exactly 1 rune in U+4E00..U+9FFF.
func ValidMonogram(m string) bool {
	rs := []rune(m)
	switch len(rs) {
	case 1:
		return rs[0] >= 0x4E00 && rs[0] <= 0x9FFF
	case 2:
		for _, r := range rs {
			if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
				return false
			}
		}
		return true
	}
	return false
}

func alnums(s string) []rune {
	var out []rune
	for _, r := range s {
		if isAlnum(r) {
			out = append(out, r)
		}
	}
	return out
}

// AutoMonogram derives a two character monogram from a profile name that is not in taken (uppercase pairs).
func AutoMonogram(name string, taken map[string]bool) string {
	segs := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	if len(segs) == 0 {
		segs = []string{name}
	}
	first := alnums(segs[0])
	if len(first) == 0 {
		first = []rune{'X'}
	}
	f := unicode.ToUpper(first[0])
	var cands []rune
	if len(segs) >= 2 {
		cands = alnums(segs[len(segs)-1])
	} else if len(first) > 1 {
		cands = first[1:]
	}
	for _, c := range cands {
		if m := string([]rune{f, unicode.ToUpper(c)}); !taken[m] {
			return m
		}
	}
	for d := '2'; d <= '9'; d++ {
		if m := string([]rune{f, d}); !taken[m] {
			return m
		}
	}
	return string([]rune{f, '1'})
}

// Assign computes colour slots and monograms in config order. colors is keyed by account id or
// "profile:<name>"; the returned newColors contains every assignment (and the re-keyed entries).
func Assign(inputs []Input, colors map[string]int, monogramOverrides map[string]string) (byProfile map[string]Result, newColors map[string]int) {
	newColors = make(map[string]int, len(colors)+len(inputs))
	for k, v := range colors {
		newColors[k] = v
	}
	byProfile = make(map[string]Result, len(inputs))

	taken := map[string]bool{}
	for _, in := range inputs {
		if m := monogramOverrides[in.Profile]; ValidMonogram(m) {
			taken[strings.ToUpper(m)] = true
		}
	}
	used := func() map[int]bool {
		u := map[int]bool{}
		for _, v := range newColors {
			u[v] = true
		}
		return u
	}
	for _, in := range inputs {
		pkey := ProfileKeyPrefix + in.Profile
		key := in.AccountID
		if key == "" {
			key = pkey
		}
		if in.AccountID != "" {
			if v, ok := newColors[pkey]; ok {
				if _, has := newColors[in.AccountID]; !has {
					newColors[in.AccountID] = v
				}
				delete(newColors, pkey)
			}
		}
		slot, ok := newColors[key]
		if !ok {
			u := used()
			for _, s := range AutoOrder {
				if !u[s] {
					slot, ok = s, true
					break
				}
			}
			if !ok {
				slot = AutoOrder[len(newColors)%len(AutoOrder)]
			}
			newColors[key] = slot
		}
		mono := ""
		if m := monogramOverrides[in.Profile]; ValidMonogram(m) {
			mono = strings.ToUpper(m)
		} else {
			mono = AutoMonogram(in.Profile, taken)
			taken[mono] = true
		}
		byProfile[in.Profile] = Result{Slot: slot, Monogram: mono}
	}
	return byProfile, newColors
}
