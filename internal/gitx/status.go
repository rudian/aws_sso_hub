package gitx

import (
	"context"
	"strconv"
	"strings"

	"github.com/rudian/codepass/internal/execx"
)

// Status is the parsed result of `git status --porcelain=v2 --branch -z`.
type Status struct {
	OID      string // commit id of HEAD, "" for an unborn branch
	Head     string // branch name, "" when detached
	Detached bool
	Upstream string // "" when none
	HasAB    bool   // Ahead and Behind are known (an upstream exists and is not gone)
	Ahead    int
	Behind   int

	Staged     int // entries with a change in the index
	Unstaged   int // entries with a change in the working tree
	Untracked  int
	Conflicted int
	Ignored    int
	Entries    []StatusEntry

	// Unverified lists paths whose work-tree change could not be confirmed: the repository configures a
	// filter driver (for example git-lfs) for them, the runner blanks drivers, and a clean filter that is
	// not the identity makes stale stat data look like a modification. They are not in Unstaged, Entries or
	// Changes(). A genuinely edited filtered file is also listed here until plain git refreshes the index.
	Unverified []string

	// RenamesSkipped is true when status had to run without rename detection: in a partial clone
	// (`--filter=blob:none`) detecting a rename needs file contents that are not on disk, the background
	// reader may not fetch them, and the first attempt failed. Renamed files then show as a deletion plus a
	// new file (FU-04b).
	RenamesSkipped bool
}

// StatusEntry is one changed path.
type StatusEntry struct {
	Kind     byte   // '1' ordinary, '2' renamed or copied, 'u' unmerged, '?' untracked, '!' ignored
	XY       string // two-letter state, "" for untracked and ignored
	Path     string
	OrigPath string // source path for kind '2'
}

// Dirty reports whether anything differs from HEAD or is untracked.
func (s Status) Dirty() bool { return s.Staged+s.Unstaged+s.Untracked+s.Conflicted > 0 }

// Changes is the count shown next to a repository: the number of changed or untracked files. A path changed
// in both the index and the work tree counts once; ignored paths are not counted.
func (s Status) Changes() int { return len(s.Entries) - s.Ignored }

// Status runs the read-only status of dir. When it fails, it is tried once more without rename detection: in a
// partial clone detecting a rename needs a blob that is not on disk, and GIT_NO_LAZY_FETCH stops the reader
// from fetching it. The result then has RenamesSkipped set.
func (r *Reader) Status(ctx context.Context, dir string) (Status, error) {
	names := r.filterNames(ctx, dir)
	args := []string{"--porcelain=v2", "--branch", "-z", "--untracked-files=normal"}
	res, err := r.runWith(ctx, dir, names, "status", args...)
	renamesSkipped := false
	if res, err = r.okResult(ctx, "status", res, err); err != nil {
		if ctx.Err() != nil {
			return Status{}, err
		}
		res2, err2 := r.statusNoRenames(ctx, dir, names, args)
		if res2, err2 = r.okResult(ctx, "status", res2, err2); err2 != nil {
			return Status{}, err // the first failure is the informative one
		}
		res, renamesSkipped = res2, true
	}
	st := ParseStatus(res.Stdout)
	st.RenamesSkipped = renamesSkipped
	if len(names) > 0 {
		r.markUnverified(ctx, dir, &st, names)
	}
	return st, nil
}

// statusNoRenames is the same read with rename detection turned off. It is built here, not through Run: the
// read forms of Run take no -c, and these two settings are the only difference from Run's own command line.
func (r *Reader) statusNoRenames(ctx context.Context, dir string, names []string, args []string) (execx.Result, error) {
	full := append([]string(nil), readOnlyFlags...)
	full = append(full, "-c", "status.renames=false", "-c", "diff.renames=false", "status", "--ignore-submodules=dirty")
	full = append(full, args...)
	env := []string{"GIT_OPTIONAL_LOCKS=0", "GIT_NO_LAZY_FETCH=1"}
	env = append(env, r.filterOverrides(names)...)
	return r.g.WithEnv(env...).Run(ctx, dir, nil, full...)
}

// markUnverified moves work-tree-modified entries whose filter attribute names a blanked driver out of the
// change counts (see Status.Unverified).
func (r *Reader) markUnverified(ctx context.Context, dir string, st *Status, names []string) {
	drivers := map[string]bool{}
	for _, n := range names {
		drivers[n] = true
	}
	var cand []string
	for _, e := range st.Entries {
		if (e.Kind == '1' || e.Kind == '2') && len(e.XY) == 2 && e.XY[1] == 'M' {
			cand = append(cand, e.Path)
		}
	}
	filtered := map[string]bool{}
	for i := 0; i < len(cand); {
		// A batch closes at 200 paths or ~8000 bytes (Windows limits a command line to 32767 characters).
		end, size := i, 0
		for end < len(cand) && end-i < 200 && (end == i || size+len(cand[end])+1 <= 8000) {
			size += len(cand[end]) + 1
			end++
		}
		args := append([]string{"-z", "filter", "--"}, cand[i:end]...)
		i = end
		res, err := r.Run(ctx, dir, "check-attr", args...)
		if err != nil || res.ExitCode != 0 {
			continue
		}
		// path NUL attr NUL value NUL
		f := strings.Split(string(res.Stdout), "\x00")
		for j := 0; j+2 < len(f); j += 3 {
			if drivers[f[j+2]] {
				filtered[f[j]] = true
			}
		}
	}
	if len(filtered) == 0 {
		return
	}
	kept := st.Entries[:0:0]
	for _, e := range st.Entries {
		if (e.Kind == '1' || e.Kind == '2') && len(e.XY) == 2 && e.XY[1] == 'M' && filtered[e.Path] {
			st.Unstaged--
			st.Unverified = append(st.Unverified, e.Path)
			if e.XY[0] == '.' {
				continue
			}
			e.XY = e.XY[:1] + "."
		}
		kept = append(kept, e)
	}
	st.Entries = kept
}

// ParseStatus parses porcelain v2 output produced with -z (NUL separated records). Unknown record types are
// skipped so a newer git does not break the parser.
func ParseStatus(b []byte) Status {
	var s Status
	recs := strings.Split(string(b), "\x00")
	for i := 0; i < len(recs); i++ {
		rec := recs[i]
		if rec == "" {
			continue
		}
		switch {
		case strings.HasPrefix(rec, "# "):
			parseHeader(&s, rec[2:])
		case rec[0] == '1':
			// 1 XY sub mH mI mW hH hI path   (8 fields before the path; the path may hold spaces)
			f := strings.SplitN(rec, " ", 9)
			if len(f) == 9 {
				s.add(StatusEntry{Kind: '1', XY: f[1], Path: f[8]})
			}
		case rec[0] == '2':
			// 2 XY sub mH mI mW hH hI Xscore path NUL origPath
			f := strings.SplitN(rec, " ", 10)
			if len(f) == 10 {
				e := StatusEntry{Kind: '2', XY: f[1], Path: f[9]}
				if i+1 < len(recs) {
					i++
					e.OrigPath = recs[i]
				}
				s.add(e)
			}
		case rec[0] == 'u':
			// u XY sub m1 m2 m3 mW h1 h2 h3 path
			f := strings.SplitN(rec, " ", 11)
			if len(f) == 11 {
				s.add(StatusEntry{Kind: 'u', XY: f[1], Path: f[10]})
			}
		case rec[0] == '?' && len(rec) > 2:
			s.add(StatusEntry{Kind: '?', Path: rec[2:]})
		case rec[0] == '!' && len(rec) > 2:
			s.add(StatusEntry{Kind: '!', Path: rec[2:]})
		}
	}
	return s
}

func (s *Status) add(e StatusEntry) {
	switch e.Kind {
	case '?':
		s.Untracked++
	case '!':
		s.Ignored++
	case 'u':
		s.Conflicted++
	default:
		if len(e.XY) == 2 {
			if e.XY[0] != '.' {
				s.Staged++
			}
			if e.XY[1] != '.' {
				s.Unstaged++
			}
		}
	}
	s.Entries = append(s.Entries, e)
}

func parseHeader(s *Status, h string) {
	k, v, _ := strings.Cut(h, " ")
	switch k {
	case "branch.oid":
		if v != "(initial)" {
			s.OID = v
		}
	case "branch.head":
		if v == "(detached)" {
			s.Detached = true
		} else {
			s.Head = v
		}
	case "branch.upstream":
		s.Upstream = v
	case "branch.ab":
		a, b, ok := strings.Cut(v, " ")
		if !ok {
			return
		}
		ai, err1 := strconv.Atoi(strings.TrimPrefix(a, "+"))
		bi, err2 := strconv.Atoi(strings.TrimPrefix(b, "-"))
		if err1 == nil && err2 == nil {
			s.HasAB, s.Ahead, s.Behind = true, ai, bi
		}
	}
}

// ParseNumStat parses `diff --numstat -z --no-renames`: "added TAB deleted TAB path NUL" per file.
func ParseNumStat(b []byte) []NumStat {
	var out []NumStat
	for _, rec := range strings.Split(string(b), "\x00") {
		f := strings.SplitN(rec, "\t", 3)
		if len(f) != 3 {
			continue
		}
		n := NumStat{Path: f[2]}
		if f[0] == "-" {
			n.Added, n.Deleted = -1, -1
		} else {
			n.Added, _ = strconv.Atoi(f[0])
			n.Deleted, _ = strconv.Atoi(f[1])
		}
		out = append(out, n)
	}
	return out
}
