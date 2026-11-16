package gitx_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/gitx/gitxtest"
)

func TestParseStatusTable(t *testing.T) {
	h := "# branch.oid abc123\x00# branch.head main\x00# branch.upstream origin/main\x00# branch.ab +2 -3\x00"
	cases := []struct {
		name string
		in   string
		want gitx.Status
	}{
		{"clean", h, gitx.Status{OID: "abc123", Head: "main", Upstream: "origin/main", HasAB: true, Ahead: 2, Behind: 3}},
		{"unborn", "# branch.oid (initial)\x00# branch.head main\x00? new.txt\x00",
			gitx.Status{Head: "main", Untracked: 1, Entries: []gitx.StatusEntry{{Kind: '?', Path: "new.txt"}}}},
		{"detached no upstream", "# branch.oid abc\x00# branch.head (detached)\x00",
			gitx.Status{OID: "abc", Detached: true}},
		{"mixed", h +
			"1 .M N... 100644 100644 100644 aaa bbb dir/a b.txt\x00" +
			"1 M. N... 100644 100644 100644 aaa bbb staged.txt\x00" +
			"1 MM N... 100644 100644 100644 aaa bbb both.txt\x00" +
			"2 R. N... 100644 100644 100644 aaa bbb R100 new name.txt\x00old name.txt\x00" +
			"u UU N... 100644 100644 100644 100644 a b c conflict.txt\x00" +
			"? u1.txt\x00? u2.txt\x00! ignored.log\x00",
			gitx.Status{OID: "abc123", Head: "main", Upstream: "origin/main", HasAB: true, Ahead: 2, Behind: 3,
				Staged: 3, Unstaged: 2, Untracked: 2, Conflicted: 1, Ignored: 1,
				Entries: []gitx.StatusEntry{
					{Kind: '1', XY: ".M", Path: "dir/a b.txt"},
					{Kind: '1', XY: "M.", Path: "staged.txt"},
					{Kind: '1', XY: "MM", Path: "both.txt"},
					{Kind: '2', XY: "R.", Path: "new name.txt", OrigPath: "old name.txt"},
					{Kind: 'u', XY: "UU", Path: "conflict.txt"},
					{Kind: '?', Path: "u1.txt"}, {Kind: '?', Path: "u2.txt"}, {Kind: '!', Path: "ignored.log"},
				}}},
		{"unknown record skipped", h + "9 whatever\x00", gitx.Status{OID: "abc123", Head: "main", Upstream: "origin/main", HasAB: true, Ahead: 2, Behind: 3}},
		{"empty", "", gitx.Status{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := gitx.ParseStatus([]byte(c.in))
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got  %+v\nwant %+v", got, c.want)
			}
		})
	}
	m := gitx.ParseStatus([]byte("? a\x001 .M N... 1 1 1 a b c\x00"))
	if !m.Dirty() || m.Changes() != 2 {
		t.Fatalf("dirty/changes: %+v", m)
	}
	// 7 changed files in the table (both.txt counts once, ignored.log not at all).
	if got := gitx.ParseStatus([]byte(cases[3].in)).Changes(); got != 7 {
		t.Fatalf("mixed Changes() = %d, want 7", got)
	}
	if got := gitx.ParseStatus([]byte("1 MM N... 100644 100644 100644 aaa bbb x.txt\x00")).Changes(); got != 1 {
		t.Fatalf("MM Changes() = %d, want 1", got)
	}
}

func TestParseConfigAndNumStat(t *testing.T) {
	cfg := gitx.ParseConfigList([]byte("global\x00file:/h/.gitconfig\x00user.name\nAlex\x00local\x00file:.git/config\x00core.bare\nfalse\x00local\x00file:.git/config\x00flag.only\x00"))
	want := []gitx.ConfigEntry{
		{"global", "file:/h/.gitconfig", "user.name", "Alex"},
		{"local", "file:.git/config", "core.bare", "false"},
		{"local", "file:.git/config", "flag.only", ""},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("%+v", cfg)
	}
	ns := gitx.ParseNumStat([]byte("3\t1\ta.txt\x00-\t-\timg.png\x00"))
	if len(ns) != 2 || ns[0] != (gitx.NumStat{Path: "a.txt", Added: 3, Deleted: 1}) || ns[1].Added != -1 {
		t.Fatalf("%+v", ns)
	}
}

func TestReaderRefusesMutations(t *testing.T) {
	sb := gitxtest.New(t)
	repo := sb.InitRepo(t, "r", nil)
	r := gitx.NewReader(sb.Git)
	ctx := context.Background()
	bad := [][]string{
		{"add", "-A"}, {"commit", "-m", "x"}, {"checkout", "main"}, {"fetch"}, {"gc"}, {"reset", "--hard"},
		{"push"}, {"clean", "-fd"}, {"stash"}, {"-c", "core.fsmonitor=x", "status"}, {"--git-dir=x", "status"}, {""},
		{"config", "user.name", "x"}, {"config", "--unset", "user.name"}, {"config", "--global", "--list"},
		{"remote", "add", "o", "u"}, {"remote", "remove", "o"}, {"branch", "-D", "main"}, {"branch", "new"},
		{"symbolic-ref", "HEAD", "refs/heads/x"}, {"symbolic-ref", "-d", "HEAD"},
		{"diff", "--output=/tmp/x"}, {"diff", "--ext-diff"}, {"show", "--textconv"},
		{"remote", "-v", "add", "o", "u"}, {"remote", "-v", "set-url", "origin", "u"}, {"remote", "-v", "rename", "a", "b"},
		{"remote", "show", "origin"}, {"status", "-v"}, {"status", "--verbose"},
		{"branch", "-r", "-dr", "origin/main"}, {"branch", "-a", "-Mr", "main", "renamed"}, {"branch", "-r", "-rc"},
		{"branch", "-r", "-uorigin/main"}, {"branch", "--list", "--delete", "x"}, {"branch", "--contains", "HEAD", "new"},
		{"symbolic-ref", "--delet", "HEAD"}, {"symbolic-ref", "--short", "HEAD", "refs/heads/x"},
		{"config", "user.name", "Mallory", "--get"}, {"config", "set", "--", "user.email", "--get"},
		{"config", "--get", "--", "user.name"}, {"config", "--get", "--add", "a", "b"}, {"config", "--list", "x"},
		{"diff", "--outp=/tmp/x"},
		{"cat-file", "--tex", "HEAD:a.txt"}, {"cat-file", "--t", "HEAD:a.txt"}, {"cat-file", "--te", "HEAD:a.txt"},
		{"cat-file", "--fi", "HEAD:a.txt"}, {"cat-file", "--filters", "HEAD:a.txt"}, {"cat-file", "--textconv", "HEAD:a.txt"},
		{"status", "--ignore-submodules=none"}, {"status", "--ignore-submodules=untracked"}, {"status", "--ignore-submodules"},
		{"diff", "--ignore-submodules=none"}, {"diff", "--ignore-submodules"}, {"diff", "--ignore-submodules=untracked"},
		{"log", "--ignore-submodules=none"}, {"show", "--ignore-submod=none"},
		{"diff", "--submodule=diff"}, {"show", "--submodule=diff"}, {"log", "-p", "--submodule=diff"}, {"diff", "--submod=diff"},
		{"check-attr", "filter", "a.txt"}, {"check-attr", "--stdin", "filter", "--"},
		{"rev-list", "--alternate-refs", "--count", "HEAD"}, {"log", "--alternate-refs", "--oneline"}, {"log", "--alt", "--oneline"},
		{"show", "--remerge-diff", "HEAD"}, {"log", "-p", "--diff-merges=r", "-1"}, {"log", "-p", "--diff-merges=remerge", "-1"},
		{"log", "-p", "--diff-merges", "remerge", "-1"}, {"log", "--diff-merges", "-p"}, {"log", "--remerge", "-p"},
		{"show", "--diff-merge=remerge", "HEAD"}, {"show", "--diff-merges=bogus", "HEAD"},
	}
	for _, a := range bad {
		_, err := r.Run(ctx, repo, a[0], a[1:]...)
		var ae *api.AppError
		if err == nil || !asApp(err, &ae) || ae.Code != api.ErrInvalidInput {
			t.Errorf("%v: want refusal, got %v", a, err)
		}
	}
	good := [][]string{{"status"}, {"rev-parse", "--git-dir"}, {"config", "--list"}, {"remote", "-v"}, {"branch", "--show-current"}, {"symbolic-ref", "HEAD"}, {"log", "-1"},
		{"log", "-p", "-m", "-1"}, {"show", "--diff-merges=first-parent", "HEAD"}, {"show", "--diff-merges=cc", "HEAD"},
		{"remote", "get-url", "--push", "--", "origin"}, {"remote", "show", "-n", "origin"}, {"remote", "get-url", "origin"},
		{"branch", "--list", "-a", "ma*"}, {"branch", "-r"}, {"config", "--get", "core.bare"}, {"symbolic-ref", "--short", "-q", "HEAD"},
		{"status", "--porcelain=v2", "--branch", "-z", "--untracked-files=normal", "--", "a.txt"},
		{"status", "--ignore-submodules=all"}, {"diff", "--ignore-submodules=dirty"}, {"diff", "--submodule=log"}, {"diff", "--submodule"},
		{"cat-file", "-p", "HEAD:a.txt"}, {"cat-file", "-t", "HEAD"}, {"cat-file", "--batch-check=%(objectname)"}, {"check-attr", "-z", "filter", "--", "a.txt"}}
	for _, a := range good {
		if _, err := r.Run(ctx, repo, a[0], a[1:]...); err != nil {
			t.Errorf("%v refused: %v", a, err)
		}
	}
	if _, err := r.Var(ctx, repo, "--list"); err == nil {
		t.Error("var with a flag must be refused")
	}
}

func asApp(err error, out **api.AppError) bool {
	ae, ok := err.(*api.AppError)
	*out = ae
	return ok
}

func TestReaderStatusBranchConfigVar(t *testing.T) {
	sb := gitxtest.New(t)
	sb.WriteGlobal(t, "[user]\n\tname = Alex\n\temail = alex@acme.example.com\n")
	bare := sb.InitBare(t, "origin.git")
	repo := sb.InitRepo(t, "work", map[string]string{"origin": bare})
	ctx := context.Background()
	run := func(args ...string) {
		t.Helper()
		if res, err := sb.Git.Run(ctx, repo, nil, args...); err != nil || res.ExitCode != 0 {
			t.Fatalf("git %v: %v %s", args, err, res.Stderr)
		}
	}
	run("fetch", "-q", "origin")
	run("reset", "-q", "--hard", "origin/main")
	run("branch", "-q", "--set-upstream-to=origin/main", "main")
	sb.CommitFile(t, repo, "a.txt", "one\n")
	sb.CommitFile(t, repo, "b.txt", "two\n")
	must(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\nmore\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(repo, "u.txt"), []byte("x"), 0o644))
	r := gitx.NewReader(sb.Git)
	st, err := r.Status(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if st.Head != "main" || st.Upstream != "origin/main" || !st.HasAB || st.Ahead != 2 || st.Behind != 0 || st.Unstaged != 1 || st.Untracked != 1 || st.Staged != 0 || st.OID == "" {
		t.Fatalf("%+v", st)
	}
	if b, err := r.Branch(ctx, repo); err != nil || b != "main" {
		t.Fatalf("branch %q %v", b, err)
	}
	ns, err := r.DiffNumStat(ctx, repo, false)
	if err != nil || len(ns) != 1 || ns[0].Path != "a.txt" || ns[0].Added != 1 {
		t.Fatalf("%+v %v", ns, err)
	}
	ent, err := r.ConfigList(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	var sawGlobal, sawLocal bool
	for _, e := range ent {
		if e.Key == "user.name" && e.Scope == "global" && e.Value == "Alex" && strings.HasPrefix(e.Origin, "file:") {
			sawGlobal = true
		}
		if e.Key == "remote.origin.url" && e.Scope == "local" {
			sawLocal = true
		}
	}
	if !sawGlobal || !sawLocal {
		t.Fatalf("config entries: %+v", ent)
	}
	id, err := r.Var(ctx, repo, "GIT_AUTHOR_IDENT")
	if err != nil || !regexp.MustCompile(`^.+ <.+@.+> \d+ [+-]\d{4}$`).MatchString(id) {
		t.Fatalf("%q %v", id, err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// gitEnv is the sandbox environment for plain git commands used as the control experiment.
func plainGit(t *testing.T, sb *gitxtest.Sandbox, dir string, args ...string) {
	t.Helper()
	plainGitOut(t, sb, dir, args...)
}

func plainGitOut(t *testing.T, sb *gitxtest.Sandbox, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(sb.Git.Bin, append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), sb.Env...)
	var out, stdout bytes.Buffer
	cmd.Stderr = &out
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out.String())
	}
	return stdout.String()
}

func TestStatusTakesNoIndexLock(t *testing.T) {
	sb := gitxtest.New(t)
	repo := sb.InitRepo(t, "r", nil)
	sb.CommitFile(t, repo, "a.txt", "one\n")
	ctx := context.Background()
	file := filepath.Join(repo, "a.txt")
	index := filepath.Join(repo, ".git", "index")
	lock := index + ".lock"
	r := gitx.NewReader(sb.Git)

	// Make the index stale: same content, new mtime. A plain `git status` refreshes (rewrites) the index.
	stale := func() {
		must(t, os.WriteFile(file, []byte("one\n"), 0o644))
		old := mtimeOf(t, index)
		must(t, os.Chtimes(file, old.Add(5*time.Second), old.Add(5*time.Second)))
	}
	snapshot := func() ([]byte, int64) {
		b, err := os.ReadFile(index)
		must(t, err)
		return b, mtimeOf(t, index).UnixNano()
	}

	// Control: plain status rewrites the stale index (so the assertion below can fail).
	stale()
	before, _ := snapshot()
	plainGit(t, sb, repo, "status", "--porcelain")
	after, _ := snapshot()
	if bytes.Equal(before, after) {
		t.Skip("this git does not refresh the index on plain status; control experiment not meaningful")
	}

	// Through the runner: another process holds index.lock; the read succeeds and the index stays untouched.
	stale()
	before, mt0 := snapshot()
	must(t, os.WriteFile(lock, []byte("held by another process"), 0o644))
	st, err := r.Status(ctx, repo)
	if err != nil {
		t.Fatalf("status while the index is locked: %v", err)
	}
	if st.Changes() != 0 {
		t.Fatalf("unexpected changes: %+v", st)
	}
	after, mt1 := snapshot()
	if !bytes.Equal(before, after) || mt0 != mt1 {
		t.Fatal("the runner rewrote the index")
	}
	if b, _ := os.ReadFile(lock); string(b) != "held by another process" {
		t.Fatal("the lock file was touched")
	}
	must(t, os.Remove(lock))

	// And with no lock held, the runner leaves no lock and no index change either.
	if _, err := r.Status(ctx, repo); err != nil {
		t.Fatal(err)
	}
	after, _ = snapshot()
	if !bytes.Equal(before, after) {
		t.Fatal("the runner rewrote the index")
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatal("index.lock left behind")
	}
}

func mtimeOf(t *testing.T, p string) (mt time.Time) {
	t.Helper()
	st, err := os.Stat(p)
	must(t, err)
	return st.ModTime()
}

func TestReadsNeverRunFsmonitorOrDiffDrivers(t *testing.T) {
	sb := gitxtest.New(t)
	repo := sb.InitRepo(t, "r", nil)
	sb.CommitFile(t, repo, "a.txt", "one\n")
	ctx := context.Background()
	// One marker per program, so the control proves each one is detectable.
	mk := func(name string) (script, marker string) {
		marker = filepath.Join(sb.Root, name+".marker")
		script = filepath.Join(sb.Root, "bin", name)
		must(t, os.WriteFile(script, []byte("#!/bin/sh\necho \"ran $0 $*\" >> '"+marker+"'\nexit 0\n"), 0o755))
		return
	}
	fsScript, fsMarker := mk("fsmonitor")
	cmdScript, cmdMarker := mk("diffcmd")
	tcScript, tcMarker := mk("textconv")
	set := func(k, v string) {
		t.Helper()
		if res, err := sb.Git.Run(ctx, repo, nil, "config", "--local", k, v); err != nil || res.ExitCode != 0 {
			t.Fatalf("config %s: %v %s", k, err, res.Stderr)
		}
	}
	set("core.fsmonitor", fsScript)
	set("diff.evil.command", cmdScript)
	set("diff.evil.textconv", tcScript)
	must(t, os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("*.txt diff=evil\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\nchanged\n"), 0o644))
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }

	r := gitx.NewReader(sb.Git)
	if _, err := r.Status(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if _, err := r.DiffNumStat(ctx, repo, false); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, repo, "diff", "-p"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, repo, "log", "-p", "-1"); err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{fsMarker, cmdMarker, tcMarker} {
		if exists(m) {
			b, _ := os.ReadFile(m)
			t.Fatalf("a repository program ran during automatic reads: %s", b)
		}
	}

	// Controls: plain git runs each program, so each marker really detects execution.
	plainGit(t, sb, repo, "status", "--porcelain")
	if !exists(fsMarker) {
		t.Fatal("control failed: plain status did not run fsmonitor")
	}
	plainGit(t, sb, repo, "diff")
	if !exists(cmdMarker) {
		t.Fatal("control failed: plain diff did not run the diff command")
	}
	plainGit(t, sb, repo, "log", "-p", "-1")
	if !exists(tcMarker) {
		t.Fatal("control failed: plain log -p did not run textconv")
	}
}

// staleIndexRepo returns a repo whose index has stale stat data for a.txt (same content, newer mtime).
func staleIndexRepo(t *testing.T, sb *gitxtest.Sandbox) (repo, index string) {
	t.Helper()
	repo = sb.InitRepo(t, "r", nil)
	sb.CommitFile(t, repo, "a.txt", "one\n")
	index = filepath.Join(repo, ".git", "index")
	return repo, index
}

func makeStale(t *testing.T, repo, index string) {
	t.Helper()
	file := filepath.Join(repo, "a.txt")
	must(t, os.WriteFile(file, []byte("one\n"), 0o644))
	old := mtimeOf(t, index)
	must(t, os.Chtimes(file, old.Add(5*time.Second), old.Add(5*time.Second)))
}

func TestDiffTakesNoIndexRewrite(t *testing.T) {
	sb := gitxtest.New(t)
	repo, index := staleIndexRepo(t, sb)
	ctx := context.Background()
	r := gitx.NewReader(sb.Git)
	snap := func() ([]byte, int64) {
		b, err := os.ReadFile(index)
		must(t, err)
		return b, mtimeOf(t, index).UnixNano()
	}

	makeStale(t, repo, index)
	before, _ := snap()
	plainGit(t, sb, repo, "diff", "--numstat")
	after, _ := snap()
	if bytes.Equal(before, after) {
		t.Skip("this git does not refresh the index on plain diff; control experiment not meaningful")
	}

	makeStale(t, repo, index)
	before, mt0 := snap()
	if _, err := r.DiffNumStat(ctx, repo, false); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, repo, "diff"); err != nil {
		t.Fatal(err)
	}
	after, mt1 := snap()
	if !bytes.Equal(before, after) || mt0 != mt1 {
		t.Fatal("diff through the runner rewrote the index")
	}
}

func TestReadsNeverRunFilterDrivers(t *testing.T) {
	sb := gitxtest.New(t)
	repo, index := staleIndexRepo(t, sb)
	ctx := context.Background()
	marker := filepath.Join(sb.Root, "filter.log")
	script := filepath.Join(sb.Root, "bin", "evilfilter")
	must(t, os.WriteFile(script, []byte("#!/bin/sh\necho ran >> '"+marker+"'\ncat\n"), 0o755))
	for _, kv := range [][2]string{{"filter.evil.clean", script}, {"filter.evil.smudge", script}, {"filter.evil.required", "true"}, {"filter.we=ird.clean", script}} {
		if res, err := sb.Git.Run(ctx, repo, nil, "config", "--local", kv[0], kv[1]); err != nil || res.ExitCode != 0 {
			t.Fatalf("config %s: %v %s", kv[0], err, res.Stderr)
		}
	}
	must(t, os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("*.txt filter=evil\n"), 0o644))
	makeStale(t, repo, index)
	ran := func() bool { _, err := os.Stat(marker); return err == nil }

	r := gitx.NewReader(sb.Git)
	if _, err := r.Status(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if _, err := r.DiffNumStat(ctx, repo, false); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, repo, "ls-files", "-m"); err != nil {
		t.Fatal(err)
	}
	if ran() {
		t.Fatal("a repository filter driver ran during automatic reads")
	}
	// Control: plain git runs the filter on the stale file.
	plainGit(t, sb, repo, "status", "--porcelain")
	if !ran() {
		t.Fatal("control failed: plain git did not run the filter")
	}
}

func TestReadsNeverRunSignatureProgram(t *testing.T) {
	sb := gitxtest.New(t)
	repo := sb.InitRepo(t, "r", nil)
	sb.CommitFile(t, repo, "a.txt", "one\n")
	ctx := context.Background()
	marker := filepath.Join(sb.Root, "gpg.log")
	script := filepath.Join(sb.Root, "bin", "evilgpg")
	must(t, os.WriteFile(script, []byte("#!/bin/sh\necho ran >> '"+marker+"'\nexit 1\n"), 0o755))
	out := func(args ...string) string {
		t.Helper()
		res, err := sb.Git.Run(ctx, repo, nil, args...)
		if err != nil || res.ExitCode != 0 {
			t.Fatalf("git %v: %v %s", args, err, res.Stderr)
		}
		return strings.TrimSpace(string(res.Stdout))
	}
	tree := out("rev-parse", "HEAD^{tree}")
	parent := out("rev-parse", "HEAD")
	obj := "tree " + tree + "\nparent " + parent + "\nauthor Alex <alex@acme.example.com> 1700000000 +0000\ncommitter Alex <alex@acme.example.com> 1700000000 +0000\n" +
		"gpgsig -----BEGIN PGP SIGNATURE-----\n \n AAAA\n -----END PGP SIGNATURE-----\n\nsigned\n"
	res, err := sb.Git.Run(ctx, repo, []byte(obj), "hash-object", "-t", "commit", "-w", "--stdin")
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("hash-object: %v %s", err, res.Stderr)
	}
	out("update-ref", "HEAD", strings.TrimSpace(string(res.Stdout)))
	out("config", "--local", "log.showSignature", "true")
	out("config", "--local", "gpg.program", script)
	out("config", "--local", "gpg.ssh.program", script)
	out("config", "--local", "gpg.x509.program", script)
	ran := func() bool { _, err := os.Stat(marker); return err == nil }

	r := gitx.NewReader(sb.Git)
	for _, a := range [][]string{{"log", "-1"}, {"show", "HEAD"}, {"log", "-1", "--format=%G?"}, {"for-each-ref", "--format=%(signature:grade)"}} {
		if _, err := r.Run(ctx, repo, a[0], a[1:]...); err != nil {
			t.Fatalf("%v: %v", a, err)
		}
	}
	if ran() {
		t.Fatal("a signature program ran during automatic reads")
	}
	plainGit(t, sb, repo, "log", "-1")
	if !ran() {
		t.Fatal("control failed: plain git log did not run the signature program")
	}
}

func TestReadsNeverRunSubmoduleDiffPrograms(t *testing.T) {
	sb := gitxtest.New(t)
	ctx := context.Background()
	sub := sb.InitRepo(t, "sub", nil)
	sb.CommitFile(t, sub, "s.txt", "one\n")
	super := sb.InitRepo(t, "super", nil)
	sb.CommitFile(t, super, "a.txt", "x\n")
	git := func(dir string, args ...string) {
		t.Helper()
		plainGit(t, sb, dir, args...)
	}
	git(super, "-c", "protocol.file.allow=always", "submodule", "add", "-q", sub, "sm")
	git(super, "commit", "-q", "-m", "add sm")
	marker := filepath.Join(sb.Root, "submarker.log")
	script := filepath.Join(sb.Root, "bin", "evilsub")
	must(t, os.WriteFile(script, []byte("#!/bin/sh\necho \"ran $0\" >> '"+marker+"'\ncat \"$1\" 2>/dev/null\nexit 0\n"), 0o755))
	smDir := filepath.Join(super, "sm")
	git(super, "config", "--local", "diff.submodule", "diff")
	git(smDir, "config", "--local", "diff.external", script)
	git(smDir, "config", "--local", "diff.evil.textconv", script)
	git(smDir, "config", "--local", "diff.evil.command", script)
	git(smDir, "config", "--local", "filter.evil.clean", script)
	must(t, os.WriteFile(filepath.Join(smDir, ".gitattributes"), []byte("*.txt diff=evil filter=evil\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(smDir, "s.txt"), []byte("two\n"), 0o644))
	git(smDir, "add", "-A")
	git(smDir, "-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", "move")
	// A stale-stat file with a clean filter, for --ignore-submodules=none.
	must(t, os.WriteFile(filepath.Join(smDir, "s.txt"), []byte("two\n"), 0o644))
	old := time.Now().Add(time.Hour)
	must(t, os.Chtimes(filepath.Join(smDir, "s.txt"), old, old))
	ran := func() bool { _, err := os.Stat(marker); return err == nil }
	_ = os.Remove(marker)

	r := gitx.NewReader(sb.Git)
	for _, a := range [][]string{{"diff"}, {"log", "-p", "-1"}, {"show", "HEAD"}, {"diff", "HEAD~1", "HEAD"}, {"status", "--porcelain=v2"}} {
		if _, err := r.Run(ctx, super, a[0], a[1:]...); err != nil {
			t.Fatalf("%v: %v", a, err)
		}
	}
	if _, err := r.Status(ctx, super); err != nil {
		t.Fatal(err)
	}
	if ran() {
		b, _ := os.ReadFile(marker)
		t.Fatalf("a submodule program ran during automatic reads: %s", b)
	}
	// Control: plain git runs them.
	plainGit(t, sb, super, "show", "HEAD")
	if !ran() {
		t.Skip("control: this git did not run the submodule diff program with plain show")
	}
}

func TestConfigReadsShowUserValues(t *testing.T) {
	sb := gitxtest.New(t)
	sb.WriteGlobal(t, "[core]\n\tfsmonitor = true\n[gpg]\n\tprogram = /opt/x/gpg\n")
	repo := sb.InitRepo(t, "r", nil)
	ctx := context.Background()
	r := gitx.NewReader(sb.Git)
	list, err := r.ConfigList(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	vals := map[string]string{}
	for _, e := range list {
		if e.Scope == "command" {
			t.Errorf("runner override leaked into config list: %+v", e)
		}
		vals[e.Key] = e.Value
	}
	if vals["core.fsmonitor"] != "true" || vals["gpg.program"] != "/opt/x/gpg" {
		t.Fatalf("user values lost: %v", vals)
	}
	for k, want := range map[string]string{"core.fsmonitor": "true", "gpg.program": "/opt/x/gpg"} {
		res, err := r.Run(ctx, repo, "config", "--get", k)
		if err != nil || strings.TrimSpace(string(res.Stdout)) != want {
			t.Errorf("--get %s = %q, %v; want %q", k, res.Stdout, err, want)
		}
	}
}

func TestStatusMarksFilteredChangesUnverified(t *testing.T) {
	sb := gitxtest.New(t)
	repo := sb.InitRepo(t, "r", nil)
	ctx := context.Background()
	for _, kv := range [][2]string{{"filter.up.clean", "tr a-z A-Z"}, {"filter.up.smudge", "cat"}} {
		plainGit(t, sb, repo, "config", "--local", kv[0], kv[1])
	}
	must(t, os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("a.txt filter=up\n"), 0o644))
	// The index holds the cleaned (upper-case) blob; with the filter blanked the work file looks modified.
	sb.CommitFile(t, repo, "a.txt", "one\n")
	sb.CommitFile(t, repo, "b.txt", "plain\n")
	// Control: plain git sees a clean tree apart from the untracked .gitattributes.
	if out := plainGitOut(t, sb, repo, "status", "--porcelain"); out != "?? .gitattributes\n" {
		t.Fatalf("control: plain status = %q", out)
	}
	// Make a.txt's stat data stale on purpose (same content, newer mtime), so git must re-hash it with the
	// filter blanked instead of trusting the index; otherwise this test depends on the clock second.
	makeStale(t, repo, filepath.Join(repo, ".git", "index"))
	// A real change to an unfiltered file must still count.
	must(t, os.WriteFile(filepath.Join(repo, "b.txt"), []byte("plain\nedited\n"), 0o644))

	r := gitx.NewReader(sb.Git)
	st, err := r.Status(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(st.Unverified, []string{"a.txt"}) || st.Changes() != 2 || st.Unstaged != 1 || st.Untracked != 1 ||
		st.Entries[0].Path != "b.txt" {
		t.Fatalf("status = %+v", st)
	}
}

// writeMarker writes a script that logs its invocation to a marker file and execs the real command.
func writeMarker(t *testing.T, path, marker, execLine string) {
	t.Helper()
	must(t, os.WriteFile(path, []byte("#!/bin/sh\necho \"ran $0 $*\" >> '"+marker+"'\n"+execLine+"\n"), 0o755))
}

func countObjects(t *testing.T, sb *gitxtest.Sandbox, repo string) string {
	t.Helper()
	return plainGitOut(t, sb, repo, "count-objects", "-v")
}

func TestReadsNeverLazyFetch(t *testing.T) {
	sb := gitxtest.New(t)
	ctx := context.Background()
	work := sb.InitRepo(t, "up-work", nil)
	sb.CommitFile(t, work, "a.txt", "one\n")
	sb.CommitFile(t, work, "a.txt", "two\n")
	sb.CommitFile(t, work, "a.txt", "three\n")
	bare := filepath.Join(sb.Root, "up.git")
	plainGit(t, sb, sb.Root, "clone", "-q", "--bare", work, bare)
	plainGit(t, sb, bare, "config", "uploadpack.allowFilter", "true")
	plainGit(t, sb, bare, "config", "uploadpack.allowAnySHA1InWant", "true")
	repo := filepath.Join(sb.Root, "partial")
	plainGit(t, sb, sb.Root, "-c", "protocol.file.allow=always", "clone", "-q", "--filter=blob:none", "file://"+bare, repo)
	marker := filepath.Join(sb.Root, "uploadpack.marker")
	wrapper := filepath.Join(sb.Root, "bin", "uploadpack")
	exe := plainGitOut(t, sb, repo, "--exec-path")
	writeMarker(t, wrapper, marker, "exec '"+strings.TrimSpace(exe)+"/git-upload-pack' \"$@\"")
	plainGit(t, sb, repo, "config", "remote.origin.uploadpack", wrapper)
	plainGit(t, sb, repo, "config", "protocol.file.allow", "always")
	// Blobs of the old commits are absent from the partial clone (only HEAD's checkout was fetched).
	before := countObjects(t, sb, repo)

	r := gitx.NewReader(sb.Git)
	for _, a := range [][]string{
		{"log", "-p", "-3"}, {"show", "HEAD~2:a.txt"}, {"diff", "HEAD~2", "HEAD", "--numstat"},
		{"cat-file", "-p", "HEAD~2:a.txt"},
	} {
		if _, err := r.Run(ctx, repo, a[0], a[1:]...); err != nil {
			t.Fatalf("%v: %v", a, err)
		}
	}
	if _, err := os.Stat(marker); err == nil {
		b, _ := os.ReadFile(marker)
		t.Fatalf("a read ran the repository's uploadpack program: %s", b)
	}
	if after := countObjects(t, sb, repo); after != before {
		t.Fatalf("object store changed during reads:\nbefore %s\nafter %s", before, after)
	}
	// Control: plain git lazily fetches through the wrapper.
	plainGit(t, sb, repo, "log", "-p", "-3")
	if _, err := os.Stat(marker); err != nil {
		t.Skip("control: this git did not lazy-fetch on plain log -p")
	}
}

func TestReadsNeverRunMergeDrivers(t *testing.T) {
	sb := gitxtest.New(t)
	ctx := context.Background()
	repo := sb.InitRepo(t, "r", nil)
	sb.CommitFile(t, repo, "a.txt", "base\n")
	plainGit(t, sb, repo, "checkout", "-q", "-b", "side")
	sb.CommitFile(t, repo, "a.txt", "side\n")
	plainGit(t, sb, repo, "checkout", "-q", "main")
	sb.CommitFile(t, repo, "a.txt", "main\n")
	// Merge with plain git before the evil driver exists; the conflict is resolved by hand.
	cmd := exec.Command(sb.Git.Bin, "-C", repo, "merge", "-q", "side")
	cmd.Env = append(os.Environ(), sb.Env...)
	_ = cmd.Run()
	must(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("resolved\n"), 0o644))
	plainGit(t, sb, repo, "add", "a.txt")
	plainGit(t, sb, repo, "commit", "-q", "-m", "merge")

	marker := filepath.Join(sb.Root, "merge.marker")
	driver := filepath.Join(sb.Root, "bin", "mergedriver")
	writeMarker(t, driver, marker, "exit 1")
	must(t, os.WriteFile(filepath.Join(repo, ".git", "info", "attributes"), []byte("*.txt merge=evil\n"), 0o644))
	plainGit(t, sb, repo, "config", "merge.evil.driver", driver+" %O %A %B")
	plainGit(t, sb, repo, "config", "log.diffMerges", "remerge")
	ran := func() bool { _, err := os.Stat(marker); return err == nil }

	r := gitx.NewReader(sb.Git)
	for _, a := range [][]string{{"log", "-p", "-m", "-1"}, {"show", "-m", "HEAD"}, {"log", "-p", "-1"}, {"show", "HEAD"}} {
		if _, err := r.Run(ctx, repo, a[0], a[1:]...); err != nil {
			t.Fatalf("%v: %v", a, err)
		}
	}
	if ran() {
		b, _ := os.ReadFile(marker)
		t.Fatalf("a read ran the repository's merge driver: %s", b)
	}
	for _, a := range [][]string{{"show", "--remerge-diff", "HEAD"}, {"log", "-p", "--diff-merges=r", "-1"},
		{"log", "-p", "--diff-merges", "remerge", "-1"}} {
		if _, err := r.Run(ctx, repo, a[0], a[1:]...); err == nil {
			t.Errorf("%v must be refused", a)
		}
	}
	if ran() {
		t.Fatal("a refused remerge form still ran the merge driver")
	}
	// Control: plain git runs the driver.
	plainGit(t, sb, repo, "show", "--remerge-diff", "HEAD")
	if !ran() {
		t.Skip("control: this git did not run the merge driver for --remerge-diff")
	}
}

func TestReadsNeverRunAlternateRefsCommand(t *testing.T) {
	sb := gitxtest.New(t)
	ctx := context.Background()
	alt := sb.InitRepo(t, "alt", nil)
	sb.CommitFile(t, alt, "a.txt", "one\n")
	repo := sb.InitRepo(t, "r", nil)
	sb.CommitFile(t, repo, "b.txt", "two\n")
	must(t, os.WriteFile(filepath.Join(repo, ".git", "objects", "info", "alternates"), []byte(filepath.Join(alt, ".git", "objects")+"\n"), 0o644))
	marker := filepath.Join(sb.Root, "alt.marker")
	script := filepath.Join(sb.Root, "bin", "altrefs")
	writeMarker(t, script, marker, "exit 0")
	plainGit(t, sb, repo, "config", "core.alternateRefsCommand", script)
	r := gitx.NewReader(sb.Git)
	// the one read that could run the command is refused, and the command is not run
	_, err := r.Run(ctx, repo, "rev-list", "--alternate-refs", "--count", "HEAD")
	var ae *api.AppError
	if !errors.As(err, &ae) || ae.Code != api.ErrInvalidInput {
		t.Fatalf("want E_INVALID_INPUT, got %v", err)
	}
	if _, err := r.Run(ctx, repo, "rev-list", "--count", "HEAD"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a read ran core.alternateRefsCommand")
	}
	plainGit(t, sb, repo, "rev-list", "--alternate-refs", "--count", "HEAD")
	if _, err := os.Stat(marker); err != nil {
		t.Skip("control: this git did not run core.alternateRefsCommand")
	}
}
