package textdiff

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/api"
)

func numbered(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("line%d", i+1)
	}
	return out
}

func TestDiffNoChange(t *testing.T) {
	in := []byte("a\nb\nc\n")
	lines, added, removed := Diff(in, in, DefaultContext)
	if len(lines) != 0 || added != 0 || removed != 0 {
		t.Errorf("got %v +%d -%d, want empty", lines, added, removed)
	}
	if lines == nil {
		t.Error("lines must be a non-nil empty slice")
	}
}

func TestDiffFolds(t *testing.T) {
	before := numbered(25)
	after := append([]string{}, before[:1]...)
	after = append(after, "new1", "new2", "new3")
	after = append(after, before[1:]...)
	lines, added, removed := Diff([]byte(strings.Join(before, "\n")+"\n"), []byte(strings.Join(after, "\n")+"\n"), 2)
	want := []api.DiffLine{
		{Kind: api.DiffContext, Text: "line1"},
		{Kind: api.DiffAdded, Text: "new1"},
		{Kind: api.DiffAdded, Text: "new2"},
		{Kind: api.DiffAdded, Text: "new3"},
		{Kind: api.DiffContext, Text: "line2"},
		{Kind: api.DiffContext, Text: "line3"},
		{Kind: api.DiffFolded, Count: 22},
	}
	if fmt.Sprint(lines) != fmt.Sprint(want) {
		t.Errorf("lines =\n%v\nwant\n%v", lines, want)
	}
	if added != 3 || removed != 0 {
		t.Errorf("counts +%d -%d, want +3 -0", added, removed)
	}
}

func TestDiffFoldsMiddleAndLeading(t *testing.T) {
	before := numbered(30)
	after := append([]string{}, before...)
	after[2] = "changed3"
	after[26] = "changed27"
	lines, added, removed := Diff([]byte(strings.Join(before, "\n")), []byte(strings.Join(after, "\n")), 2)
	if added != 2 || removed != 2 {
		t.Fatalf("counts +%d -%d, want +2 -2", added, removed)
	}
	var kinds []string
	for _, l := range lines {
		k := string(l.Kind)
		if l.Kind == api.DiffFolded {
			k = fmt.Sprintf("fold%d", l.Count)
		}
		kinds = append(kinds, k)
	}
	// ctx ctx del add ctx ctx fold19 ctx ctx del add ctx ctx fold1... verify the middle fold only.
	got := strings.Join(kinds, " ")
	if !strings.HasPrefix(got, "ctx ctx del add ctx ctx fold") {
		t.Errorf("unexpected shape: %s", got)
	}
	if lines[0].Text != "line1" || lines[2].Text != "line3" || lines[3].Text != "changed3" {
		t.Errorf("unexpected lines: %v", lines)
	}
}

func TestDiffCounts(t *testing.T) {
	before := []byte("a\nb\nc\nd\n")
	after := []byte("a\nB\nc\nd\ne\n")
	lines, added, removed := Diff(before, after, 2)
	if added != 2 || removed != 1 {
		t.Errorf("counts +%d -%d, want +2 -1", added, removed)
	}
	var del, add []string
	for _, l := range lines {
		switch l.Kind {
		case api.DiffRemoved:
			del = append(del, l.Text)
		case api.DiffAdded:
			add = append(add, l.Text)
		}
	}
	if fmt.Sprint(del) != "[b]" || fmt.Sprint(add) != "[B e]" {
		t.Errorf("del=%v add=%v", del, add)
	}
}

func TestDiffStripsLineEndings(t *testing.T) {
	lines, added, removed := Diff([]byte("a\r\nb\r\n"), []byte("a\r\nc\r\n"), 2)
	if added != 1 || removed != 1 {
		t.Fatalf("counts +%d -%d", added, removed)
	}
	for _, l := range lines {
		if strings.ContainsAny(l.Text, "\r\n") {
			t.Errorf("line ending kept in %q", l.Text)
		}
	}
}

func TestDiffFromEmpty(t *testing.T) {
	lines, added, removed := Diff(nil, []byte("a\nb\n"), 2)
	if added != 2 || removed != 0 || len(lines) != 2 || lines[0].Kind != api.DiffAdded {
		t.Errorf("got %v +%d -%d", lines, added, removed)
	}
}

func TestDiffHugeFallsBack(t *testing.T) {
	n := 2100
	before := make([]string, n)
	after := make([]string, n)
	for i := range before {
		before[i] = fmt.Sprintf("b%d", i)
		after[i] = fmt.Sprintf("a%d", i)
	}
	lines, added, removed := Diff([]byte(strings.Join(before, "\n")), []byte(strings.Join(after, "\n")), 2)
	if added != n || removed != n || len(lines) != 2*n {
		t.Errorf("fallback: +%d -%d len %d", added, removed, len(lines))
	}
	if lines[0].Kind != api.DiffRemoved || lines[n].Kind != api.DiffAdded {
		t.Errorf("fallback order wrong")
	}
}
