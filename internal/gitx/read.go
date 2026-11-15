package gitx

import (
	"context"
	"strconv"
	"strings"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/execx"
	"github.com/rudian/codepass/internal/redact"
)

// Reader is the one runner for automatic (background) git reads: project rows, tree colours, change counts.
// Every call is `git [-C dir] --no-optional-locks -c core.fsmonitor=false <subcommand> ...` with
// GIT_OPTIONAL_LOCKS=0, and diff-family subcommands also get --no-ext-diff --no-textconv. Together these keep
// a read from taking index.lock (an AI's `git add` in the same folder must not fail at random) and from
// running programs named in the repository's own config (fsmonitor hook, diff drivers, textconv).
// Because the index is never refreshed and filter drivers are blanked, answers can differ from plain git:
// see Status (Unverified) and DiffNumStat.
// Subcommands that can change the repository, the index or config are refused before any process starts.
type Reader struct{ g *Git }

// NewReader returns a read-only runner on top of g.
func NewReader(g *Git) *Reader { return &Reader{g: g} }

// readOnlyFlags are placed before the subcommand on every automatic read.
var readOnlyFlags = []string{
	"--no-optional-locks", "-c", "core.fsmonitor=false",
	// diff would otherwise refresh (rewrite) a stale index and take index.lock.
	"-c", "diff.autoRefreshIndex=false",
	// Signature programs named in the repository's config must not run for log/show/for-each-ref.
	"-c", "log.showSignature=false", "-c", "gpg.program=", "-c", "gpg.ssh.program=", "-c", "gpg.x509.program=",
	// diff.submodule=diff makes git start a child `git diff` inside the submodule, which would run the
	// submodule's own diff drivers. Short form prints only "Subproject commit" lines.
	"-c", "diff.submodule=short",
	// -m / --first-parent output must never come from log.diffMerges=remerge in the repository's config
	// (remerge runs custom merge drivers and filters).
	"-c", "log.diffMerges=separate",
}

// configFlags is all a `config` read gets: it touches no index and runs no program, and -c overrides would
// show up in its output as scope "command" entries and change `--get` answers.
var configFlags = []string{"--no-optional-locks"}

// diffFamily lists the subcommands that accept (and need) --no-ext-diff --no-textconv.
var diffFamily = map[string]bool{"diff": true, "log": true, "show": true}

// allowed lists the subcommands an automatic read may run. config, remote, branch and symbolic-ref are
// further restricted to their read forms by checkReadForm.
var allowed = map[string]bool{
	"status": true, "rev-parse": true, "rev-list": true, "for-each-ref": true, "ls-files": true,
	"diff": true, "log": true, "show": true, "cat-file": true, "var": true, "config": true,
	"remote": true, "branch": true, "symbolic-ref": true, "check-attr": true,
}

// hasPrefixAny reports whether a begins with one of the prefixes.
func hasPrefixAny(a string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(a, p) {
			return true
		}
	}
	return false
}

func oneOf(a string, set ...string) bool {
	for _, s := range set {
		if a == s {
			return true
		}
	}
	return false
}

// noDash reports whether a is a plain positional (a name, never an option).
func noDash(a string) bool { return a != "" && !strings.HasPrefix(a, "-") }

// checkReadForm rejects argument shapes of an allowed subcommand that write or run programs. The
// subcommands that have write forms (config, remote, branch, symbolic-ref) and status are allow-lists: every
// argument starting with '-' must be a whole token from the subcommand's set (no combined short options, no
// abbreviations). The others refuse options that write files, re-enable drivers or redirect the repository.
func checkReadForm(sub string, args []string) bool {
	switch sub {
	case "config":
		// One read action first, then its positionals. A positional before the action (`config k v --get`)
		// is a write.
		action, pos := "", 0
		for _, a := range args {
			switch {
			case oneOf(a, "--list", "-l", "--get", "--get-all", "--get-regexp", "--get-urlmatch"):
				if action != "" || pos > 0 {
					return false
				}
				action = a
			case oneOf(a, "-z", "--null", "--show-origin", "--show-scope", "--includes", "--no-includes", "--local") || strings.HasPrefix(a, "--type="):
			case noDash(a):
				if action == "" {
					return false
				}
				pos++
			default:
				return false
			}
		}
		switch action {
		case "--list", "-l":
			return pos == 0
		case "--get", "--get-all", "--get-regexp":
			return pos >= 1 && pos <= 2
		case "--get-urlmatch":
			return pos == 2
		}
		return false
	case "remote":
		switch {
		case len(args) == 0:
			return true
		case len(args) == 1:
			return oneOf(args[0], "-v", "--verbose")
		case args[0] == "get-url":
			rest := args[1:]
			if len(rest) > 0 && oneOf(rest[0], "--push", "--all") {
				rest = rest[1:]
			}
			if len(rest) == 2 && rest[0] == "--" {
				return rest[1] != ""
			}
			return len(rest) == 1 && noDash(rest[0])
		case args[0] == "show":
			return len(args) == 3 && args[1] == "-n" && noDash(args[2])
		}
		return false
	case "branch":
		if len(args) == 1 && args[0] == "--show-current" {
			return true
		}
		list, pos := false, 0
		for _, a := range args {
			switch {
			case oneOf(a, "--list", "-l"):
				list = true
			case oneOf(a, "-a", "-r", "--all", "--remotes", "--contains", "--merged", "--no-merged"),
				hasPrefixAny(a, "--format=", "--sort=", "--contains=", "--merged=", "--no-merged="):
			case noDash(a):
				pos++
			default:
				return false
			}
		}
		return pos == 0 || list
	case "symbolic-ref":
		pos := 0
		for _, a := range args {
			switch {
			case oneOf(a, "--short", "-q", "--quiet"):
			case noDash(a):
				pos++
			default:
				return false
			}
		}
		return pos == 1
	case "status":
		for _, a := range args {
			switch {
			case a == "--":
				return true // paths follow
			case oneOf(a, "--porcelain", "--porcelain=v1", "--porcelain=v2", "--short", "-s", "--branch", "-b", "-z", "--ignored"),
				oneOf(a, "--ignore-submodules=dirty", "--ignore-submodules=all"),
				hasPrefixAny(a, "--untracked-files=", "--ignored="):
			default:
				return false
			}
		}
		return true
	case "check-attr":
		// check-attr [-z] filter -- paths...: only reads attributes.
		for i, a := range args {
			switch {
			case a == "--":
				return i > 0
			case a == "-z":
			case noDash(a):
			default:
				return false
			}
		}
		return false
	case "cat-file":
		// Allow-list: git accepts any unique prefix of a long option, so an abbreviation such as --tex
		// (--textconv) or --fi (--filters) would otherwise slip past a prefix refusal.
		for _, a := range args {
			if a == "--" {
				return true
			}
			if !strings.HasPrefix(a, "-") {
				continue
			}
			name, _, _ := strings.Cut(a, "=")
			switch {
			case oneOf(a, "-t", "-s", "-e", "-p", "-z", "-Z", "--batch-all-objects", "--buffer", "--unordered",
				"--follow-symlinks", "--allow-unknown-type"):
			case oneOf(name, "--batch", "--batch-check"):
			default:
				return false
			}
		}
		return true
	case "rev-parse", "rev-list", "for-each-ref", "ls-files", "diff", "log", "show", "var":
		for _, a := range args {
			// Options that write output files, re-enable drivers or redirect the repository. --ou is the
			// shortest unambiguous abbreviation of --output.
			if hasPrefixAny(a, "--ou", "--ext-diff", "--textconv", "--filters", "--exec-path", "--ext", "--text") {
				return false
			}
			// --alternate-refs runs core.alternateRefsCommand (--alt is its shortest unambiguous form).
			if hasPrefixAny(a, "--alt") {
				return false
			}
			if !diffMergesOK(a) {
				return false
			}
			if diffFamily[sub] && !submoduleOptionsOK(a) {
				return false
			}
		}
		return true
	}
	return false
}

// diffMergesOK refuses --remerge-diff (and abbreviations) and every --diff-merges form except the stuck
// non-remerge values: remerge runs the repository's merge drivers and filters. A bare --diff-merges takes
// its value from the next argument, so it is refused too.
func diffMergesOK(a string) bool {
	if !strings.HasPrefix(a, "--") || a == "--" {
		return true
	}
	name, val, hasVal := strings.Cut(a, "=")
	if len(name) >= len("--rem") && strings.HasPrefix("--remerge-diff", name) {
		return false
	}
	if len(name) >= len("--diff-m") && strings.HasPrefix("--diff-merges", name) {
		if name != "--diff-merges" || !hasVal {
			return false
		}
		switch val {
		case "off", "none", "on", "m", "first-parent", "1", "separate", "combined", "c", "dense-combined", "cc":
			return true
		}
		return false
	}
	return true
}

// submoduleOptionsOK refuses --submodule=diff (child `git diff` in the submodule runs its diff drivers) and
// every --ignore-submodules value except dirty and all (none/untracked make git run `git status` in each
// submodule, where the submodule's own filter drivers run). Abbreviations of both options are refused too.
func submoduleOptionsOK(a string) bool {
	if !strings.HasPrefix(a, "--") {
		return true
	}
	name, val, hasVal := strings.Cut(a, "=")
	if len(name) >= len("--sub") && strings.HasPrefix("--submodule", name) {
		return name == "--submodule" && (!hasVal || val == "short" || val == "log")
	}
	if len(name) >= len("--ignore-su") && strings.HasPrefix("--ignore-submodules", name) {
		return name == "--ignore-submodules" && hasVal && (val == "dirty" || val == "all")
	}
	return true
}

// Run runs one allowed read. dir "" runs outside any repository (global/system config only). It returns an
// E_INVALID_INPUT error without starting a process when the subcommand or its form is not a read.
func (r *Reader) Run(ctx context.Context, dir, sub string, args ...string) (execx.Result, error) {
	return r.run(ctx, dir, nil, false, sub, args...)
}

// runWith is Run with the filter driver names already looked up (see Status), so one call costs one lookup.
func (r *Reader) runWith(ctx context.Context, dir string, names []string, sub string, args ...string) (execx.Result, error) {
	return r.run(ctx, dir, names, true, sub, args...)
}

func (r *Reader) run(ctx context.Context, dir string, names []string, haveNames bool, sub string, args ...string) (execx.Result, error) {
	if !allowed[sub] || !checkReadForm(sub, args) {
		return execx.Result{}, api.NewError(api.ErrInvalidInput, "git %s is not an automatic read", sub).WithParam("field", "subcommand")
	}
	flags := readOnlyFlags
	if sub == "config" {
		flags = configFlags
	}
	full := append([]string(nil), flags...)
	full = append(full, sub)
	if diffFamily[sub] {
		full = append(full, "--no-ext-diff", "--no-textconv")
	}
	if sub == "status" || sub == "diff" {
		// A submodule's own config could name filter drivers; never descend into dirty submodules.
		full = append(full, "--ignore-submodules=dirty")
	}
	full = append(full, args...)
	// GIT_NO_LAZY_FETCH keeps a partial clone from fetching missing objects, which would run the
	// repository's uploadpack/ssh/credential programs. (An env var: git 2.39 lacks the --no-lazy-fetch flag
	// and ignores the variable.)
	env := []string{"GIT_OPTIONAL_LOCKS=0", "GIT_NO_LAZY_FETCH=1"}
	if dir != "" && (sub == "status" || sub == "diff" || sub == "ls-files") {
		if !haveNames {
			names = r.filterNames(ctx, dir)
		}
		env = append(env, r.filterOverrides(names)...)
	}
	return r.g.WithEnv(env...).Run(ctx, dir, nil, full...)
}

// filterNames lists the clean/smudge/process filter drivers the repository configures.
func (r *Reader) filterNames(ctx context.Context, dir string) []string {
	full := append([]string(nil), configFlags...)
	full = append(full, "config", "-z", "--get-regexp", `^filter\..+\.(clean|smudge|process)$`)
	res, err := r.g.WithEnv("GIT_OPTIONAL_LOCKS=0", "GIT_NO_LAZY_FETCH=1").Run(ctx, dir, nil, full...)
	if err != nil || res.ExitCode != 0 {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	for _, rec := range strings.Split(string(res.Stdout), "\x00") {
		key, _, _ := strings.Cut(rec, "\n")
		if !strings.HasPrefix(key, "filter.") {
			continue
		}
		i := strings.LastIndexByte(key, '.')
		if i <= len("filter.") {
			continue
		}
		name := key[len("filter."):i]
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// filterOverrides returns GIT_CONFIG_COUNT/KEY/VALUE entries that blank every named filter driver, so
// refreshing stale stat data never runs a repository-defined program. The env form avoids -c splitting
// driver names at '='.
func (r *Reader) filterOverrides(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	n := 0
	for _, kv := range r.g.Env {
		if v, ok := strings.CutPrefix(kv, "GIT_CONFIG_COUNT="); ok {
			if c, err := strconv.Atoi(v); err == nil && c >= 0 {
				n = c
			}
		}
	}
	var out []string
	add := func(k, v string) {
		out = append(out, "GIT_CONFIG_KEY_"+strconv.Itoa(n)+"="+k, "GIT_CONFIG_VALUE_"+strconv.Itoa(n)+"="+v)
		n++
	}
	for _, name := range names {
		add("filter."+name+".clean", "")
		add("filter."+name+".smudge", "")
		add("filter."+name+".process", "")
		add("filter."+name+".required", "false")
	}
	return append(out, "GIT_CONFIG_COUNT="+strconv.Itoa(n))
}

// runOK is Run with unexpected exit codes mapped to E_GIT_FAILED.
func (r *Reader) runOK(ctx context.Context, dir, sub string, args ...string) (execx.Result, error) {
	res, err := r.Run(ctx, dir, sub, args...)
	return r.okResult(ctx, sub, res, err)
}

func (r *Reader) okResult(ctx context.Context, sub string, res execx.Result, err error) (execx.Result, error) {
	if err != nil {
		if ae, ok := err.(*api.AppError); ok {
			return res, ae
		}
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		return res, api.NewError(api.ErrGitFailed, "git could not run").WithDetail(err.Error())
	}
	if res.ExitCode != 0 {
		return res, api.NewError(api.ErrGitFailed, "git %s failed (exit %d)", sub, res.ExitCode).
			WithDetail(redact.Tail(string(res.Stderr), 3))
	}
	return res, nil
}

// ConfigEntry is one setting from `git config --show-origin --show-scope`.
type ConfigEntry struct {
	Scope  string // system, global, local, worktree, command
	Origin string // file:/path, command line, ...
	Key    string
	Value  string
}

// ConfigList returns every effective setting in dir (or the global view for dir "") with its origin. Config
// reads run without the runner's -c overrides, so values are the user's own.
func (r *Reader) ConfigList(ctx context.Context, dir string) ([]ConfigEntry, error) {
	res, err := r.runOK(ctx, dir, "config", "--show-origin", "--show-scope", "-z", "--list")
	if err != nil {
		return nil, err
	}
	return ParseConfigList(res.Stdout), nil
}

// ParseConfigList parses `config --show-origin --show-scope -z --list` output:
// scope NUL origin NUL key NL value NUL, repeated. A key without value has no NL.
func ParseConfigList(b []byte) []ConfigEntry {
	parts := strings.Split(string(b), "\x00")
	var out []ConfigEntry
	for i := 0; i+2 < len(parts); i += 3 {
		e := ConfigEntry{Scope: parts[i], Origin: parts[i+1]}
		kv := parts[i+2]
		if j := strings.IndexByte(kv, '\n'); j >= 0 {
			e.Key, e.Value = kv[:j], kv[j+1:]
		} else {
			e.Key = kv
		}
		out = append(out, e)
	}
	return out
}

// Var returns `git var <name>` (for example GIT_AUTHOR_IDENT, GIT_EDITOR), trimmed.
func (r *Reader) Var(ctx context.Context, dir, name string) (string, error) {
	if name == "" || strings.HasPrefix(name, "-") {
		return "", api.NewError(api.ErrInvalidInput, "invalid git variable").WithParam("field", "name")
	}
	res, err := r.runOK(ctx, dir, "var", name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(res.Stdout)), nil
}

// Branch returns the current branch name, or "" when HEAD is detached or unborn-and-unnamed.
func (r *Reader) Branch(ctx context.Context, dir string) (string, error) {
	res, err := r.runOK(ctx, dir, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(res.Stdout)), nil
}

// NumStat is the per-file line change count of a diff.
type NumStat struct {
	Path    string
	Added   int // -1 for a binary file
	Deleted int
}

// DiffNumStat returns line counts for the working tree against the index (staged false) or the index
// against HEAD (staged true). Filter drivers are blanked and the index is never refreshed, so a file with a
// non-identity clean filter and stale stat data can show a false change here; use Status (which reports
// those as Unverified) to learn what changed. diff name-only/--quiet/--raw forms report timestamp-only
// changes for the same reason (diff.autoRefreshIndex=false).
func (r *Reader) DiffNumStat(ctx context.Context, dir string, staged bool) ([]NumStat, error) {
	args := []string{"--numstat", "-z", "--no-renames"}
	if staged {
		args = append(args, "--cached")
	}
	res, err := r.runOK(ctx, dir, "diff", args...)
	if err != nil {
		return nil, err
	}
	return ParseNumStat(res.Stdout), nil
}
