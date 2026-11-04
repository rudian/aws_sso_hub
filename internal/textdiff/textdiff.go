// Package textdiff produces the line diffs shown in write previews.
package textdiff

import (
	"strings"

	"github.com/rudian/aws_sso_hub/internal/api"
)

const DefaultContext = 2

// maxPairs is the largest n*m the LCS table may have before the diff degrades to "all removed + all added".
const maxPairs = 4_000_000

// Diff returns a line diff (LCS; inputs over 4,000,000 line pairs fall back to "all removed + all added").
// Unchanged runs farther than ctx lines from a change collapse into one api.DiffFolded line with Count.
// No change → empty slice. Line endings are stripped from Text.
func Diff(before, after []byte, ctx int) (lines []api.DiffLine, added, removed int) {
	lines = make([]api.DiffLine, 0)
	if ctx < 0 {
		ctx = 0
	}
	a, b := splitLines(before), splitLines(after)

	// Common prefix and suffix are context; only the middle needs the LCS.
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	if pre == len(a) && pre == len(b) {
		return lines, 0, 0
	}
	midA, midB := a[pre:len(a)-suf], b[pre:len(b)-suf]

	ops := make([]api.DiffLine, 0, len(a)+len(b))
	for _, t := range a[:pre] {
		ops = append(ops, api.DiffLine{Kind: api.DiffContext, Text: t})
	}
	ops = append(ops, diffMiddle(midA, midB)...)
	for _, t := range a[len(a)-suf:] {
		ops = append(ops, api.DiffLine{Kind: api.DiffContext, Text: t})
	}
	for _, op := range ops {
		switch op.Kind {
		case api.DiffAdded:
			added++
		case api.DiffRemoved:
			removed++
		}
	}
	return fold(ops, ctx), added, removed
}

func diffMiddle(a, b []string) []api.DiffLine {
	out := make([]api.DiffLine, 0, len(a)+len(b))
	n, m := len(a), len(b)
	if n == 0 || m == 0 || (n+1)*(m+1) > maxPairs {
		for _, t := range a {
			out = append(out, api.DiffLine{Kind: api.DiffRemoved, Text: t})
		}
		for _, t := range b {
			out = append(out, api.DiffLine{Kind: api.DiffAdded, Text: t})
		}
		return out
	}
	// lcs[i][j] = LCS length of a[i:] and b[j:].
	w := m + 1
	lcs := make([]int32, (n+1)*w)
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i*w+j] = lcs[(i+1)*w+j+1] + 1
			} else if lcs[(i+1)*w+j] >= lcs[i*w+j+1] {
				lcs[i*w+j] = lcs[(i+1)*w+j]
			} else {
				lcs[i*w+j] = lcs[i*w+j+1]
			}
		}
	}
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, api.DiffLine{Kind: api.DiffContext, Text: a[i]})
			i++
			j++
		case lcs[(i+1)*w+j] >= lcs[i*w+j+1]:
			out = append(out, api.DiffLine{Kind: api.DiffRemoved, Text: a[i]})
			i++
		default:
			out = append(out, api.DiffLine{Kind: api.DiffAdded, Text: b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, api.DiffLine{Kind: api.DiffRemoved, Text: a[i]})
	}
	for ; j < m; j++ {
		out = append(out, api.DiffLine{Kind: api.DiffAdded, Text: b[j]})
	}
	return out
}

// fold collapses context runs that are farther than ctx lines from any change.
func fold(ops []api.DiffLine, ctx int) []api.DiffLine {
	out := make([]api.DiffLine, 0, len(ops))
	for i := 0; i < len(ops); {
		if ops[i].Kind != api.DiffContext {
			out = append(out, ops[i])
			i++
			continue
		}
		j := i
		for j < len(ops) && ops[j].Kind == api.DiffContext {
			j++
		}
		run := ops[i:j]
		keepHead, keepTail := ctx, ctx
		if i == 0 {
			keepHead = 0 // nothing changed above the run
		}
		if j == len(ops) {
			keepTail = 0 // nothing changed below the run
		}
		if len(run) > keepHead+keepTail {
			out = append(out, run[:keepHead]...)
			out = append(out, api.DiffLine{Kind: api.DiffFolded, Count: len(run) - keepHead - keepTail})
			out = append(out, run[len(run)-keepTail:]...)
		} else {
			out = append(out, run...)
		}
		i = j
	}
	return out
}

func splitLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	parts := strings.Split(s, "\n")
	for i, p := range parts {
		parts[i] = strings.TrimSuffix(p, "\r")
	}
	return parts
}
