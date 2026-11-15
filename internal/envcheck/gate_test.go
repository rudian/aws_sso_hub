package envcheck

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/execx"
	"github.com/rudian/codepass/internal/netx"
)

// world is a fake machine: a bin folder with the tools the test wants and a Homebrew prefix.
type world struct {
	t      *testing.T
	bin    string
	prefix string
	marker string // a file that every fake tool appends to when it runs
}

func newWorld(t *testing.T) *world {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake tools are shell scripts")
	}
	root, _ := filepath.EvalSymlinks(t.TempDir())
	w := &world{t: t, bin: filepath.Join(root, "bin"), prefix: filepath.Join(root, "opt-homebrew"), marker: filepath.Join(root, "ran")}
	if err := os.MkdirAll(w.bin, 0o755); err != nil {
		t.Fatal(err)
	}
	// Isolate from the real machine: nothing from the process PATH, and /usr/bin/git (the real stub) is held
	// back by a fake xcode-select that says there are no developer tools.
	t.Setenv("PATH", filepath.Join(root, "nowhere"))
	xs := w.tool(filepath.Join(root, "sys"), "xcode-select", "xcode-select: error: no developer directory", 2)
	t.Cleanup(execx.SetDevToolStubRoot("/usr/bin", xs, "darwin"))
	execx.Configure(execx.Options{PreferredDirs: []string{w.bin}, DisableLoginShell: true})
	t.Cleanup(func() { execx.Configure(execx.Options{}) })
	return w
}

func (w *world) tool(dir, name, out string, exit int) string {
	w.t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		w.t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	body := "#!/bin/sh\necho " + name + " >> '" + w.marker + "'\necho '" + out + "'\nexit " + string(rune('0'+exit)) + "\n"
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		w.t.Fatal(err)
	}
	return p
}

func (w *world) opts() GateOptions {
	return GateOptions{
		GOOS:      "darwin",
		BrewPaths: []string{filepath.Join(w.prefix, "bin", "brew")},
		PrefixDir: w.prefix,
		SpaceDir:  w.bin,
		Run: func(ctx context.Context, name string, args ...string) (string, int, error) {
			switch name + " " + strings.Join(args, " ") {
			case "/usr/bin/id -Gn":
				return "staff everyone admin _developer\n", 0, nil
			case "/usr/bin/profiles status -type enrollment":
				return "Enrolled via DEP: No\nMDM enrollment: No\n", 0, nil
			}
			return "", 1, errors.New("unexpected probe " + name)
		},
		Proxy: func(context.Context) netx.Config { return netx.Config{Source: "none"} },
		Free:  func(string) (int64, error) { return 50_000_000_000, nil },
	}
}

func row(st api.SetupState, id api.SetupToolID) api.SetupTool {
	for _, r := range st.Tools {
		if r.ID == id {
			return r
		}
	}
	return api.SetupTool{}
}

func TestGateAllReadyOffline(t *testing.T) {
	w := newWorld(t)
	w.tool(filepath.Join(w.prefix, "bin"), "brew", "Homebrew 5.1.2", 0)
	w.tool(w.bin, "git", "git version 2.54.0", 0)
	w.tool(w.bin, "aws", "aws-cli/2.40.3 Python/3.15 Darwin/27 source/arm64", 0)

	// Detection must not touch the network: every default HTTP path is replaced by one that fails and counts.
	var calls atomic.Int32
	failing := roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("network is unreachable")
	})
	oldT, oldC := http.DefaultTransport, http.DefaultClient.Transport
	http.DefaultTransport, http.DefaultClient.Transport = failing, failing
	t.Cleanup(func() { http.DefaultTransport, http.DefaultClient.Transport = oldT, oldC })

	st := Gate(context.Background(), w.opts())
	if calls.Load() != 0 {
		t.Fatalf("detection made %d network calls", calls.Load())
	}
	if st.Blocked || st.Ready != 3 || st.Total != 3 || st.Platform != "darwin" {
		t.Fatalf("state: %+v", st)
	}
	if h := row(st, api.SetupHomebrew); h.Status != api.ToolOK || h.Version != "5.1.2" || h.Fix != api.FixNone {
		t.Errorf("brew: %+v", h)
	}
	pf := st.Preflight
	if pf.Admin != api.TriYes || pf.Managed != api.TriNo || !pf.EnoughSpace || pf.NeedBytes != NeedBytes || pf.PrefixExists != true {
		t.Errorf("preflight: %+v", pf)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGateNothingInstalled(t *testing.T) {
	w := newWorld(t)
	st := Gate(context.Background(), w.opts())
	if !st.Blocked || st.Ready != 0 {
		t.Fatalf("state: %+v", st)
	}
	for _, id := range []api.SetupToolID{api.SetupHomebrew, api.SetupGit, api.SetupAWSCLI} {
		if r := row(st, id); r.Status != api.ToolMissing || !r.Blocking || r.Fix != api.FixInstall {
			t.Errorf("%s: %+v", id, r)
		}
	}
	if st.Preflight.PrefixExists {
		t.Error("no prefix yet")
	}
	// An existing prefix without a working brew is reported, with its own fix, and never touched.
	if err := os.MkdirAll(w.prefix, 0o755); err != nil {
		t.Fatal(err)
	}
	st = Gate(context.Background(), w.opts())
	if h := row(st, api.SetupHomebrew); h.Fix != api.FixRepairPrefix || !h.Blocking {
		t.Errorf("partial prefix: %+v", h)
	}
	if !st.Preflight.PrefixExists {
		t.Error("prefix not reported")
	}
}

func TestGateGitBelowMinimumNamesVersionAndFix(t *testing.T) {
	w := newWorld(t)
	w.tool(filepath.Join(w.prefix, "bin"), "brew", "Homebrew 5.1.2", 0)
	w.tool(w.bin, "git", "git version 2.43.0 (Apple Git-146)", 0)
	w.tool(w.bin, "aws", "aws-cli/2.40.3 Python/3.15", 0)
	st := Gate(context.Background(), w.opts())
	g := row(st, api.SetupGit)
	if !st.Blocked || g.Status != api.ToolTooOld || g.Version != "2.43.0" || g.Min != "2.44.0" || g.Fix != api.FixUpgrade || !g.Elsewhere || !g.Blocking {
		t.Fatalf("git row: %+v", g)
	}
	if st.Ready != 2 {
		t.Errorf("ready = %d", st.Ready)
	}
	// 2.44.0 is the first accepted version.
	w.tool(w.bin, "git", "git version 2.44.0", 0)
	if g := row(Gate(context.Background(), w.opts()), api.SetupGit); g.Status != api.ToolOK || g.Blocking {
		t.Errorf("2.44.0: %+v", g)
	}
}

func TestGateAWSRules(t *testing.T) {
	w := newWorld(t)
	w.tool(filepath.Join(w.prefix, "bin"), "brew", "Homebrew 5.1.2", 0)
	w.tool(w.bin, "git", "git version 2.54.0", 0)
	cases := []struct {
		out      string
		status   api.ToolStatus
		blocking bool
	}{
		{"aws-cli/1.32.0 Python/3.11", api.ToolV1, true},
		{"aws-cli/2.8.9 Python/3.11", api.ToolTooOld, true},
		{"aws-cli/2.21.9 Python/3.12", api.ToolOutdated, false}, // older than recommended never blocks
		{"aws-cli/2.40.3 Python/3.15", api.ToolOK, false},
	}
	for _, c := range cases {
		w.tool(w.bin, "aws", c.out, 0)
		a := row(Gate(context.Background(), w.opts()), api.SetupAWSCLI)
		if a.Status != c.status || a.Blocking != c.blocking {
			t.Errorf("%s: %+v", c.out, a)
		}
	}
}

func TestGateUnreadableVersionDoesNotBlock(t *testing.T) {
	w := newWorld(t)
	w.tool(filepath.Join(w.prefix, "bin"), "brew", "boom", 1)
	w.tool(w.bin, "git", "git version 2.54.0", 0)
	w.tool(w.bin, "aws", "no version here", 0)
	st := Gate(context.Background(), w.opts())
	if st.Blocked {
		t.Errorf("an unreadable version must not close the gate: %+v", st.Tools)
	}
	if row(st, api.SetupHomebrew).Status != api.ToolError || row(st, api.SetupAWSCLI).Status != api.ToolError {
		t.Errorf("rows: %+v", st.Tools)
	}
}

func TestGateRunsNoStubWhileXcodeSelectFails(t *testing.T) {
	w := newWorld(t)
	stubs, _ := filepath.EvalSymlinks(t.TempDir())
	for _, n := range []string{"git", "make", "clang", "python3", "xcrun"} {
		w.tool(stubs, n, "git version 2.99.0", 0)
	}
	xs := w.tool(t.TempDir(), "xcode-select", "xcode-select: error: unable to get active developer directory", 2)
	restore := execx.SetDevToolStubRoot(stubs, xs, "darwin")
	t.Cleanup(restore)
	execx.Configure(execx.Options{PreferredDirs: []string{stubs}, DisableLoginShell: true})
	st := Gate(context.Background(), w.opts())
	if g := row(st, api.SetupGit); g.Status != api.ToolMissing || !g.Blocking {
		t.Errorf("git behind a stub must count as missing: %+v", g)
	}
	b, _ := os.ReadFile(w.marker)
	if strings.Contains(string(b), "git") || strings.Contains(string(b), "make") || strings.Contains(string(b), "clang") {
		t.Fatalf("a stub ran while xcode-select -p failed: %q", b)
	}
	// With the tools installed the same git is accepted.
	tools := t.TempDir()
	xs = w.tool(t.TempDir(), "xcode-select", tools, 0)
	restore2 := execx.SetDevToolStubRoot(stubs, xs, "darwin")
	t.Cleanup(restore2)
	if g := row(Gate(context.Background(), w.opts()), api.SetupGit); g.Status != api.ToolOK {
		t.Errorf("with developer tools: %+v", g)
	}
}

func TestPreflightFacts(t *testing.T) {
	w := newWorld(t)
	o := w.opts()
	o.Run = func(ctx context.Context, name string, args ...string) (string, int, error) {
		if name == "/usr/bin/id" {
			return "staff everyone localaccounts\n", 0, nil
		}
		return "Enrolled via DEP: Yes\nMDM enrollment: Yes (User Approved)\n", 0, nil
	}
	o.Free = func(string) (int64, error) { return 900_000_000, nil }
	o.Proxy = func(context.Context) netx.Config {
		return netx.Config{HTTPS: "http://user:secret@proxy.example.com:8080", Source: "system"}
	}
	pf := Gate(context.Background(), o).Preflight
	if pf.Admin != api.TriNo || pf.Managed != api.TriYes || pf.EnoughSpace || pf.FreeBytes != 900_000_000 {
		t.Errorf("preflight: %+v", pf)
	}
	if pf.Proxy.HostPort != "proxy.example.com:8080" || strings.Contains(pf.Proxy.HostPort, "secret") {
		t.Errorf("proxy: %+v", pf.Proxy)
	}
	// A probe that cannot run leaves the fact unknown, never "no".
	o.Run = func(context.Context, string, ...string) (string, int, error) {
		return "", -1, errors.New("no such program")
	}
	o.Free = func(string) (int64, error) { return 0, errors.New("statfs") }
	pf = Gate(context.Background(), o).Preflight
	if pf.Admin != api.TriUnknown || pf.Managed != api.TriUnknown || pf.FreeBytes != -1 || !pf.EnoughSpace {
		t.Errorf("unknown facts: %+v", pf)
	}
}

func TestGateWindowsRows(t *testing.T) {
	w := newWorld(t)
	winget := w.tool(filepath.Join(w.bin, "wa"), "winget", "v1.9.25200", 0)
	o := w.opts()
	o.GOOS = "windows"
	o.WingetPaths = []string{winget}
	w.tool(w.bin, "git", "git version 2.47.1.windows.1", 0)
	w.tool(w.bin, "aws", "aws-cli/2.17.0 Python/3.11 Windows/10", 0)
	st := Gate(context.Background(), o)
	if m := row(st, api.SetupWinget); m.Status != api.ToolOK || m.Version != "1.9.25200" {
		t.Errorf("winget: %+v", m)
	}
	if row(st, api.SetupGit).Status != api.ToolOK || st.Preflight.Admin != api.TriUnknown || st.Preflight.PrefixExists {
		t.Errorf("windows state: %+v", st)
	}
	o.WingetPaths = []string{filepath.Join(w.bin, "none", "winget.exe")}
	st = Gate(context.Background(), o)
	if m := row(st, api.SetupWinget); m.Status != api.ToolMissing || m.Fix != api.FixOpenStore || !st.Blocked {
		t.Errorf("no winget: %+v", m)
	}
}

// Only /opt/homebrew/bin/brew counts on macOS: the installer runs nothing else, and /usr/local is Intel/Rosetta.
func TestGateDefaultBrewPathIsTheArm64PrefixOnly(t *testing.T) {
	o := GateOptions{GOOS: "darwin"}
	o.defaults()
	if len(o.BrewPaths) != 1 || o.BrewPaths[0] != "/opt/homebrew/bin/brew" {
		t.Errorf("BrewPaths = %v", o.BrewPaths)
	}
}

// brew prints ">=" before the number when it cannot ask git for its version.
func TestGateAcceptsBrewVersionWithoutGit(t *testing.T) {
	w := newWorld(t)
	w.tool(filepath.Join(w.prefix, "bin"), "brew", "Homebrew >=4.3.0 (shallow or no git repository)", 0)
	st := Gate(context.Background(), w.opts())
	if b := row(st, api.SetupHomebrew); b.Status != api.ToolOK || b.Version != "4.3.0" {
		t.Errorf("%+v", b)
	}
}

// A tool path chosen in Settings that is too old: the row says so (Override), and when automatic detection finds
// an acceptable tool the fix is use-detected, not an install.
func TestGateChosenPathTooOld(t *testing.T) {
	w := newWorld(t)
	w.tool(filepath.Join(w.prefix, "bin"), "brew", "Homebrew 4.3.0", 0)
	old := w.tool(filepath.Join(w.bin, "old"), "git", "git version 2.43.0", 0)
	o := w.opts()
	o.GitPath = old
	// nothing else on the machine: no acceptable git to fall back on
	st := Gate(context.Background(), o)
	g := row(st, api.SetupGit)
	if !st.Blocked || !g.Blocking || !g.Override || g.Status != api.ToolTooOld || g.Fix != api.FixUpgrade || g.Version != "2.43.0" {
		t.Fatalf("%+v", g)
	}
	// a 2.56.1 git shows up on the PATH: the chosen path is the problem, and use-detected says so
	w.tool(w.bin, "git", "git version 2.56.1", 0)
	st = Gate(context.Background(), o)
	g = row(st, api.SetupGit)
	if !st.Blocked || g.Fix != api.FixUseDetected || !g.Override || g.Elsewhere {
		t.Fatalf("%+v", g)
	}
	// without a chosen path the same machine is ready and the row is no override
	o.GitPath = ""
	g = row(Gate(context.Background(), o), api.SetupGit)
	if g.Blocking || g.Override || g.Status != api.ToolOK {
		t.Fatalf("%+v", g)
	}
}
