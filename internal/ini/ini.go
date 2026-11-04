// Package ini is a line-preserving INI editor for ~/.aws/config, ~/.aws/credentials and the backup merge.
// An unmodified Doc serialises byte-identically to its input; edits touch only the lines they must.
package ini

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ParseError reports the first line (1-based) that does not fit the supported INI grammar.
type ParseError struct {
	Line int
	Msg  string
}

func (e *ParseError) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Msg) }

// UnsafeError reports an edit that was refused because a section name, key or value could not be written
// to an INI file without changing its structure (for example a value holding a line break). Field names the
// part that was refused; it never carries the offending text.
type UnsafeError struct{ Field string }

func (e *UnsafeError) Error() string { return "unsafe " + e.Field + " for an INI file" }

// KV is a key/value pair for Set and AddSection.
type KV struct{ Key, Value string }

// InsertPos selects where Set places new keys.
type InsertPos int

const (
	InsertTop InsertPos = iota
	InsertBottom
)

type kind int

const (
	kBlank kind = iota
	kComment
	kHeader
	kKey
	kCont
)

type line struct {
	kind   kind
	raw    string // line text without line ending
	eol    string // "\n", "\r\n" or "" (last line without final newline)
	name   string // header: normalized section name
	key    string // key line: key text
	prefix string // key line: text up to the start of the value, e.g. "key  =  "
}

// Doc is a parsed INI document.
//
// Edits that would put unsafe text (line breaks or other control characters, bracket or "=" inside names)
// into the file are refused: the document stays as it was and the first refusal is kept in Err. Callers must
// check Err before writing Bytes to disk.
type Doc struct {
	bom   string
	lines []line
	nl    string // line ending used for inserted lines
	err   error
}

// Err returns the first refused edit, nil when every edit was applied.
func (d *Doc) Err() error { return d.err }

func (d *Doc) refuse(field string) {
	if d.err == nil {
		d.err = &UnsafeError{Field: field}
	}
}

// UnsafeText reports whether s contains a Unicode control character (which includes CR, LF, NUL and tab) or
// one of the line separators U+0085, U+2028 and U+2029. Such text must never be written into an INI file.
func UnsafeText(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cc, r) || r == 0x85 || r == 0x2028 || r == 0x2029 {
			return true
		}
	}
	return false
}

func unsafeSection(name string) bool {
	return UnsafeText(name) || strings.ContainsAny(name, "[]")
}

func unsafeKey(key string) bool {
	return key == "" || UnsafeText(key) || strings.Contains(key, "=") || key[0] == '#' || key[0] == ';' || strings.TrimSpace(key) != key
}

// checkKVs refuses the whole batch when any key or value is unsafe.
func (d *Doc) checkKVs(kvs []KV) bool {
	for _, kv := range kvs {
		if unsafeKey(kv.Key) {
			d.refuse("key")
			return false
		}
		if UnsafeText(kv.Value) {
			d.refuse(kv.Key)
			return false
		}
	}
	return true
}

var bomBytes = "\xef\xbb\xbf"

func normalizeName(s string) string { return strings.Join(strings.Fields(s), " ") }

// Parse parses data. Section headers, key/value pairs and comments are preserved byte for byte.
func Parse(data []byte) (*Doc, error) {
	s := string(data)
	d := &Doc{nl: "\n"}
	if strings.HasPrefix(s, bomBytes) {
		d.bom = bomBytes
		s = s[len(bomBytes):]
	}
	nlSet := false
	inSection := false
	inBlock := false
	n := 0
	for len(s) > 0 {
		n++
		i := strings.IndexByte(s, '\n')
		var text, eol string
		if i < 0 {
			text, s = s, ""
		} else {
			text, s = s[:i], s[i+1:]
			eol = "\n"
			if strings.HasSuffix(text, "\r") {
				text = text[:len(text)-1]
				eol = "\r\n"
			}
		}
		if !nlSet && eol != "" {
			d.nl, nlSet = eol, true
		}
		l := line{raw: text, eol: eol}
		trimmed := strings.TrimLeft(text, " \t")
		switch {
		case strings.TrimSpace(text) == "":
			l.kind = kBlank
			inBlock = false
		case trimmed[0] == '#' || trimmed[0] == ';':
			l.kind = kComment
			if text[0] != ' ' && text[0] != '\t' {
				inBlock = false
			}
		case trimmed[0] == '[':
			end := strings.IndexByte(trimmed, ']')
			if end < 0 {
				return nil, &ParseError{Line: n, Msg: "unterminated section header"}
			}
			rest := strings.TrimSpace(trimmed[end+1:])
			if rest != "" && rest[0] != '#' && rest[0] != ';' {
				return nil, &ParseError{Line: n, Msg: "unexpected text after section header"}
			}
			l.kind = kHeader
			l.name = normalizeName(trimmed[1:end])
			inSection = true
			inBlock = false
		case text[0] == ' ' || text[0] == '\t':
			if !inBlock {
				return nil, &ParseError{Line: n, Msg: "unexpected indented line"}
			}
			l.kind = kCont
		default:
			if !inSection {
				return nil, &ParseError{Line: n, Msg: "key before any section header"}
			}
			eq := strings.IndexByte(text, '=')
			if eq <= 0 {
				return nil, &ParseError{Line: n, Msg: "expected key = value"}
			}
			l.kind = kKey
			l.key = strings.TrimRight(text[:eq], " \t")
			j := eq + 1
			for j < len(text) && (text[j] == ' ' || text[j] == '\t') {
				j++
			}
			l.prefix = text[:j]
			inBlock = true
		}
		d.lines = append(d.lines, l)
	}
	return d, nil
}

// Bytes serialises the document.
func (d *Doc) Bytes() []byte {
	var b strings.Builder
	b.WriteString(d.bom)
	for _, l := range d.lines {
		b.WriteString(l.raw)
		b.WriteString(l.eol)
	}
	return []byte(b.String())
}

// Sections returns normalized header names in file order (duplicates once).
func (d *Doc) Sections() []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range d.lines {
		if l.kind == kHeader && !seen[l.name] {
			seen[l.name] = true
			out = append(out, l.name)
		}
	}
	return out
}

func (d *Doc) Has(section string) bool { return d.SectionLine(section) > 0 }

// SectionLine returns the 1-based header line of the first section with that name, 0 if missing.
func (d *Doc) SectionLine(section string) int {
	section = normalizeName(section)
	for i, l := range d.lines {
		if l.kind == kHeader && l.name == section {
			return i + 1
		}
	}
	return 0
}

type span struct{ h, e int } // header index, end index (exclusive)

func (d *Doc) spans(section string) []span {
	section = normalizeName(section)
	var out []span
	for i, l := range d.lines {
		if l.kind == kHeader && l.name == section {
			e := i + 1
			for e < len(d.lines) && d.lines[e].kind != kHeader {
				e++
			}
			out = append(out, span{i, e})
		}
	}
	return out
}

// lastKeyIdx returns the index of the last key/continuation line of the span, or the header index.
func (d *Doc) lastKeyIdx(sp span) int {
	last := sp.h
	for i := sp.h + 1; i < sp.e; i++ {
		if k := d.lines[i].kind; k == kKey || k == kCont {
			last = i
		}
	}
	return last
}

func valueOf(l line) string { return strings.TrimSpace(strings.TrimPrefix(l.raw, l.prefix)) }

// Get returns the first occurrence of key (ASCII case-insensitive) in the section.
func (d *Doc) Get(section, key string) (string, bool) {
	for _, sp := range d.spans(section) {
		for i := sp.h + 1; i < sp.e; i++ {
			if l := d.lines[i]; l.kind == kKey && strings.EqualFold(l.key, key) {
				return valueOf(l), true
			}
		}
	}
	return "", false
}

// Keys returns the top-level keys of the section in order (nested blocks have Value "").
func (d *Doc) Keys(section string) []KV {
	var out []KV
	for _, sp := range d.spans(section) {
		for i := sp.h + 1; i < sp.e; i++ {
			if l := d.lines[i]; l.kind == kKey {
				out = append(out, KV{Key: l.key, Value: valueOf(l)})
			}
		}
	}
	return out
}

func (d *Doc) insertLines(at int, ls []line) {
	// keep a missing final newline missing: if inserting after the last line that lacks an EOL, move the gap.
	if at > 0 && at == len(d.lines) && d.lines[at-1].eol == "" {
		d.lines[at-1].eol = d.nl
		ls[len(ls)-1].eol = ""
	}
	out := make([]line, 0, len(d.lines)+len(ls))
	out = append(out, d.lines[:at]...)
	out = append(out, ls...)
	out = append(out, d.lines[at:]...)
	d.lines = out
}

func (d *Doc) batchLines(existing []line, kvs []KV) []line {
	// alignment rule: new keys follow the column of the neighbouring keys
	c, aligned := -1, false
	if len(existing) >= 2 {
		aligned = true
		multi := false
		for _, l := range existing {
			eq := strings.IndexByte(l.raw, '=')
			col := utf8.RuneCountInString(l.raw[:eq])
			if c == -1 {
				c = col
			} else if c != col {
				aligned = false
			}
			t := strings.TrimRight(l.raw[:eq], " \t")
			if len(l.raw[:eq])-len(t) >= 2 {
				multi = true
			}
		}
		aligned = aligned && multi
	}
	w := 0
	for _, kv := range kvs {
		if n := utf8.RuneCountInString(kv.Key); n > w {
			w = n
		}
	}
	out := make([]line, 0, len(kvs))
	for _, kv := range kvs {
		n := utf8.RuneCountInString(kv.Key)
		var prefix string
		switch {
		case aligned && w+1 <= c:
			prefix = kv.Key + strings.Repeat(" ", c-n) + "= "
		case len(kvs) >= 2:
			prefix = kv.Key + strings.Repeat(" ", w-n) + " = "
		default:
			prefix = kv.Key + " = "
		}
		out = append(out, line{kind: kKey, raw: prefix + kv.Value, eol: d.nl, key: kv.Key, prefix: prefix})
	}
	return out
}

// Set replaces existing keys' values in place and inserts the missing keys as one batch.
// It returns false when the section does not exist or an input is unsafe (see Err).
func (d *Doc) Set(section string, kvs []KV, pos InsertPos) bool {
	sps := d.spans(section)
	if len(sps) == 0 {
		return false
	}
	if !d.checkKVs(kvs) {
		return false
	}
	var missing []KV
	for _, kv := range kvs {
		found := false
	outer:
		for _, sp := range sps {
			for i := sp.h + 1; i < sp.e; i++ {
				if l := d.lines[i]; l.kind == kKey && strings.EqualFold(l.key, kv.Key) {
					d.lines[i].raw = l.prefix + kv.Value
					found = true
					break outer
				}
			}
		}
		if !found {
			missing = append(missing, kv)
		}
	}
	if len(missing) == 0 {
		return true
	}
	sp := sps[0]
	var existing []line
	for i := sp.h + 1; i < sp.e; i++ {
		if d.lines[i].kind == kKey {
			existing = append(existing, d.lines[i])
		}
	}
	batch := d.batchLines(existing, missing)
	at := sp.h + 1
	if pos == InsertBottom {
		at = d.lastKeyIdx(sp) + 1
	}
	d.insertLines(at, batch)
	return true
}

// DeleteKeys removes the key lines (and their continuation lines); it returns the number of keys removed.
func (d *Doc) DeleteKeys(section string, keys ...string) int {
	count := 0
	// spans are recomputed after each deletion
	for _, key := range keys {
		for {
			sps := d.spans(section)
			idx := -1
			var sp span
			for _, s := range sps {
				for i := s.h + 1; i < s.e; i++ {
					if l := d.lines[i]; l.kind == kKey && strings.EqualFold(l.key, key) {
						idx, sp = i, s
						break
					}
				}
				if idx >= 0 {
					break
				}
			}
			if idx < 0 {
				break
			}
			end := idx + 1
			for end < sp.e && d.lines[end].kind == kCont {
				end++
			}
			d.lines = append(d.lines[:idx], d.lines[end:]...)
			count++
		}
	}
	return count
}

// AddSection appends a section at the end of the file. Unsafe input adds nothing (see Err).
func (d *Doc) AddSection(section string, kvs []KV) {
	if unsafeSection(section) {
		d.refuse("section")
		return
	}
	if !d.checkKVs(kvs) {
		return
	}
	var add []line
	if n := len(d.lines); n > 0 {
		if d.lines[n-1].eol == "" {
			d.lines[n-1].eol = d.nl
		}
		if d.lines[n-1].kind != kBlank {
			add = append(add, line{kind: kBlank, raw: "", eol: d.nl})
		}
	}
	name := normalizeName(section)
	add = append(add, line{kind: kHeader, raw: "[" + name + "]", eol: d.nl, name: name})
	add = append(add, d.batchLines(nil, kvs)...)
	d.lines = append(d.lines, add...)
}

// DeleteSection removes every section with that name (header through last key line).
func (d *Doc) DeleteSection(section string) bool {
	removed := false
	for {
		sps := d.spans(section)
		if len(sps) == 0 {
			return removed
		}
		sp := sps[0]
		last := d.lastKeyIdx(sp)
		d.lines = append(d.lines[:sp.h], d.lines[last+1:]...)
		for sp.h < len(d.lines) && d.lines[sp.h].kind == kBlank && (sp.h == 0 || d.lines[sp.h-1].kind == kBlank) {
			d.lines = append(d.lines[:sp.h], d.lines[sp.h+1:]...)
		}
		removed = true
	}
}

// RenameSection renames every header with the old name; text after the closing bracket is kept. An unsafe new
// name renames nothing (see Err).
func (d *Doc) RenameSection(oldName, newName string) bool {
	if unsafeSection(newName) {
		d.refuse("section")
		return false
	}
	oldName = normalizeName(oldName)
	newName = normalizeName(newName)
	done := false
	for i, l := range d.lines {
		if l.kind == kHeader && l.name == oldName {
			o := strings.IndexByte(l.raw, '[')
			c := strings.IndexByte(l.raw, ']')
			d.lines[i].raw = l.raw[:o+1] + newName + l.raw[c:]
			d.lines[i].name = newName
			done = true
		}
	}
	return done
}
