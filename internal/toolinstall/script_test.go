package toolinstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
)

// lab is a throw-away machine for the privileged script: a work base that stands for /var/root, fake pkgutil,
// spctl and installer, and a prefix that stands for /opt/homebrew. The script text is the production constant;
// only the header lines (BASE, NEEDROOT, PREFIX, BREW and the tool paths) are replaced, line by line.
type lab struct {
	t        *testing.T
	dir      string
	base     string
	prefix   string
	brew     string
	installs string // the fake installer's call log
	subs     map[string]string
}

func newLab(t *testing.T) *lab {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("the script uses macOS tools (stat -f, /usr/bin/openssl)")
	}
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	l := &lab{t: t, dir: dir, base: filepath.Join(dir, "root"), prefix: filepath.Join(dir, "opt", "homebrew")}
	l.brew = filepath.Join(l.prefix, "bin", "brew")
	l.installs = filepath.Join(dir, "installer.log")
	for _, d := range []string{l.base, filepath.Join(dir, "bin"), filepath.Join(dir, "fix")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for n, text := range sigFixtures {
		l.write(filepath.Join(dir, "fix", n+".txt"), text, 0o600)
	}
	l.write(filepath.Join(dir, "bin", "pkgutil"), "#!/bin/sh\nfirst=$(/usr/bin/head -n1 \"$2\")\ncase \"$first\" in FIXTURE:*) ;; *) echo 'Could not open package at path: x'; exit 1;; esac\nname=${first#FIXTURE:}\nexec /bin/cat '"+dir+"/fix/'\"$name\".txt\n", 0o755)
	l.spctl("enabled", "/x: accepted\nsource=Notarized Developer ID\norigin=Developer ID Installer: Example Maintainer (927JGANW46)", 0)
	l.installer(0, "installer: Package name is Homebrew\ninstaller: The install was successful.", true)
	l.subs = map[string]string{
		"BASE": l.base, "NEEDROOT": "0", "STRICTPARENT": "0", "PREFIX": l.prefix, "BREW": l.brew,
		"PKGUTIL": filepath.Join(dir, "bin", "pkgutil"), "SPCTL": filepath.Join(dir, "bin", "spctl"),
		"INSTALLER": filepath.Join(dir, "bin", "installer"),
	}
	return l
}

func (l *lab) write(path, text string, mode os.FileMode) {
	l.t.Helper()
	if err := os.WriteFile(path, []byte(text), mode); err != nil {
		l.t.Fatal(err)
	}
}

func (l *lab) spctl(state, assess string, code int) {
	l.write(filepath.Join(l.dir, "bin", "spctl"), fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"--status\" ]; then echo 'assessments %s'; exit 0; fi\n/bin/cat <<'EOT'\n%s\nEOT\nexit %d\n", state, assess, code), 0o755)
}

func (l *lab) installer(code int, out string, makeBrew bool) {
	mk := ""
	if makeBrew {
		mk = fmt.Sprintf("/bin/mkdir -p '%s' && : > '%s' && /bin/chmod +x '%s'\n", filepath.Dir(l.brew), l.brew, l.brew)
	}
	l.write(filepath.Join(l.dir, "bin", "installer"), fmt.Sprintf("#!/bin/sh\necho \"$@\" >> '%s'\n/bin/cat <<'EOT'\n%s\nEOT\nif [ %d -eq 0 ]; then\n:\n%sfi\nexit %d\n", l.installs, out, code, mk, code), 0o755)
}

// pkg writes a fake download whose first line names the pkgutil fixture to answer with.
func (l *lab) pkg(name, fixture string, size int) string {
	l.t.Helper()
	body := "FIXTURE:" + fixture + "\n"
	if len(body) < size {
		body += strings.Repeat("x", size-len(body))
	}
	p := filepath.Join(l.dir, name)
	l.write(p, body, 0o600)
	return p
}

func sum(path string) string {
	b, _ := os.ReadFile(path)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func sizeOf(path string) string {
	st, _ := os.Stat(path)
	return strconv.FormatInt(st.Size(), 10)
}

// script returns pkgScript with the header lines of subs replaced. It fails when a header line is missing, so a
// renamed variable cannot silently make a test run against the real /var/root.
func (l *lab) script() string {
	s := pkgScript
	for k, v := range l.subs {
		re := regexp.MustCompile(`(?m)^` + k + `=.*$`)
		if !re.MatchString(s) {
			l.t.Fatalf("pkgScript has no header line %s=", k)
		}
		s = re.ReplaceAllLiteralString(s, k+"="+v)
	}
	return s
}

type scriptRun struct {
	exit           int
	stdout, stderr string
}

// run starts the script exactly like the app's AppleScript does: /usr/bin/env -i /bin/bash --noprofile --norc -c.
func (l *lab) run(args ...string) scriptRun { return l.runEnv(nil, args...) }

func (l *lab) runEnv(env []string, args ...string) scriptRun {
	l.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	full := append([]string{"-i", "/bin/bash", "--noprofile", "--norc", "-c", l.script(), "codepass-pkg"}, args...)
	cmd := exec.CommandContext(ctx, "/usr/bin/env", full...)
	cmd.Env = env
	var so, se strings.Builder
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	r := scriptRun{stdout: so.String(), stderr: se.String()}
	if ctx.Err() != nil {
		l.t.Fatalf("the script hung (a FIFO or device was read?): stdout=%q stderr=%q", r.stdout, r.stderr)
	}
	if ee, ok := err.(*exec.ExitError); ok {
		r.exit = ee.ExitCode()
	} else if err != nil {
		l.t.Fatal(err)
	}
	return r
}

func (l *lab) leftovers() []string {
	ents, _ := os.ReadDir(l.base)
	var out []string
	for _, e := range ents {
		out = append(out, e.Name())
	}
	return out
}

const team = HomebrewTeamID

func TestScriptVerifyCases(t *testing.T) {
	l := newLab(t)
	good := l.pkg("good.pkg", "good", 4096)
	gsize, gsha := sizeOf(good), sum(good)
	atoiSize, _ := strconv.Atoi(gsize)

	flipped := l.pkg("flip.pkg", "invalid", atoiSize)
	unsigned := l.pkg("unsigned.pkg", "unsigned", 3000)
	trunc := l.pkg("trunc.pkg", "invalid", atoiSize/3)
	html := filepath.Join(l.dir, "proxy.pkg")
	l.write(html, "<html><body>407 Proxy Authentication Required</body></html>", 0o600)
	notNotarized := l.pkg("nn.pkg", "notnotarized", atoiSize)
	otherLeaf := l.pkg("ol.pkg", "otherleaf", atoiSize)
	over := l.pkg("over.pkg", "good", atoiSize+500)

	cases := []struct {
		name string
		pkg  string
		team string
		size string
		sha  string
		want int
		in   string // text expected on stderr (or stdout for 0)
	}{
		{"good pkg", good, team, gsize, gsha, 0, "verified: size=" + gsize + " team=" + team + " sha256=" + gsha},
		{"good pkg, size and digest unknown (rate-limited fallback)", good, team, "-", "-", 0, "gatekeeper=on"},
		{"wrong expected size", good, team, strconv.Itoa(atoiSize - 1), "-", ExitSizeMismatch, "size"},
		{"wrong expected digest", good, team, gsize, strings.Repeat("0", 64), ExitSizeMismatch, "digest"},
		{"wrong expected Team ID", good, "ABCDEFGHIJ", gsize, "-", ExitWrongTeam, "Team ID is " + team + ", expected ABCDEFGHIJ"},
		{"signed by another team", l.pkg("wt.pkg", "wrongteam", atoiSize), team, "-", "-", ExitWrongTeam, "ABCDEFGHIJ"},
		{"flipped byte, digest known", flipped, team, gsize, gsha, ExitSizeMismatch, "digest"},
		{"flipped byte, nothing known", flipped, team, "-", "-", ExitNotSigned, "checksum did not verify"},
		{"unsigned re-wrap", unsigned, team, "-", "-", ExitNotSigned, "no signature"},
		{"truncated, size known", trunc, team, gsize, "-", ExitSizeMismatch, "size"},
		{"truncated, nothing known", trunc, team, "-", "-", ExitNotSigned, "checksum did not verify"},
		{"proxy web page, size known", html, team, gsize, "-", ExitSizeMismatch, "size"},
		{"proxy web page, nothing known", html, team, "-", "-", ExitNotSigned, "Could not open package"},
		{"not notarized", notNotarized, team, "-", "-", ExitNotNotarized, "not notarized"},
		{"leaf is not a Developer ID Installer", otherLeaf, team, "-", "-", ExitNotSigned, "no Developer ID Installer"},
		{"file longer than the expected size", over, team, gsize, "-", ExitSizeMismatch, "size"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := l.run(c.pkg, c.team, c.size, c.sha, "verify")
			if r.exit != c.want {
				t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", r.exit, c.want, r.stdout, r.stderr)
			}
			out := r.stderr
			if c.want == 0 {
				out = r.stdout
			}
			if !strings.Contains(out, c.in) {
				t.Errorf("output %q does not contain %q", out, c.in)
			}
			if got := l.leftovers(); len(got) != 0 {
				t.Errorf("the work folder was left behind: %v", got)
			}
		})
	}
}

// The Go pre-check and the script must give the same verdict for the same recorded output.
func TestSignatureVerdictsAgree(t *testing.T) {
	l := newLab(t)
	want := map[string]api.FailureCause{
		"good": "", "wrongteam": api.CauseUnknownSigner, "notnotarized": api.CauseNotNotarized, "invalid": api.CauseBadSignature,
		"unsigned": api.CauseBadSignature, "notapkg": api.CauseBadSignature, "otherleaf": api.CauseBadSignature,
	}
	for name, cause := range want {
		f := CheckSignatureOutput(sigFixtures[name], team)
		got := api.FailureCause("")
		if f != nil {
			got = f.Cause
		}
		if got != cause {
			t.Errorf("%s: Go says %q, want %q", name, got, cause)
		}
		r := l.run(l.pkg(name+".pkg", name, 100), team, "-", "-", "verify")
		if g := MacPrivileged(1, fmt.Sprintf("1:1: execution error: %s (%d)", r.stderr, r.exit)); r.exit == 0 && cause != "" || r.exit != 0 && g != cause {
			t.Errorf("%s: script exit %d maps to %q, want %q", name, r.exit, g, cause)
		}
	}
}

func TestScriptRefusesDevicesAndFifos(t *testing.T) {
	l := newLab(t)
	zero := filepath.Join(l.dir, "zero.pkg")
	if err := os.Symlink("/dev/zero", zero); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(l.dir, "fifo.pkg")
	if err := mkfifo(fifo); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(l.dir, "adir.pkg")
	os.Mkdir(dir, 0o700)
	realTarget := l.pkg("target.pkg", "good", 1000)
	link := filepath.Join(l.dir, "link.pkg")
	if err := os.Symlink(realTarget, link); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]string{"symlink to /dev/zero": zero, "FIFO": fifo, "directory": dir, "symlink to a good file": link, "device": "/dev/null"} {
		start := time.Now()
		r := l.run(p, team, "-", "-", "verify")
		if r.exit != ExitCopyFailed || !strings.Contains(r.stderr, "not a regular file") {
			t.Errorf("%s: exit %d stderr %q", name, r.exit, r.stderr)
		}
		if time.Since(start) > 5*time.Second {
			t.Errorf("%s took %s", name, time.Since(start))
		}
	}
	if got := l.leftovers(); len(got) != 0 {
		t.Errorf("left behind: %v", got)
	}
	// A missing file is a copy failure too, not a hang or a crash.
	if r := l.run(filepath.Join(l.dir, "none.pkg"), team, "-", "-", "verify"); r.exit != ExitCopyFailed {
		t.Errorf("missing file: %+v", r)
	}
}

func TestScriptCopyIsBounded(t *testing.T) {
	l := newLab(t)
	// A download of 5 MB when 1 KB was announced: at most 1 KB + 1 byte is ever copied, so the mismatch shows
	// without the copy growing with the file (the work folder is removed, so look at the stderr text).
	big := filepath.Join(l.dir, "big.pkg")
	l.write(big, "FIXTURE:good\n"+strings.Repeat("y", 5<<20), 0o600)
	r := l.run(big, team, "1024", "-", "verify")
	if r.exit != ExitSizeMismatch || !strings.Contains(r.stderr, "size 1025, expected 1024") {
		t.Errorf("exit %d stderr %q", r.exit, r.stderr)
	}
	// With nothing announced the hard cap applies (400 MiB): a file over it is refused, one at it is not read on.
	r = l.run(big, team, "999999999", "-", "verify")
	if r.exit != ExitSizeMismatch || !strings.Contains(r.stderr, "over the limit") {
		t.Errorf("huge announced size: exit %d stderr %q", r.exit, r.stderr)
	}
}

func TestScriptInstallMode(t *testing.T) {
	t.Run("installs the verified copy", func(t *testing.T) {
		l := newLab(t)
		p := l.pkg("good.pkg", "good", 2048)
		r := l.run(p, team, sizeOf(p), sum(p), "install")
		if r.exit != 0 || !strings.Contains(r.stdout, "installed") {
			t.Fatalf("%+v", r)
		}
		calls, _ := os.ReadFile(l.installs)
		// The installer was given the root-side copy, never the user's file.
		if !strings.Contains(string(calls), l.base+"/.codepass-pkg.") || strings.Contains(string(calls), p) || !strings.Contains(string(calls), "-target /") {
			t.Errorf("installer call: %q", calls)
		}
		if got := l.leftovers(); len(got) != 0 {
			t.Errorf("left behind: %v", got)
		}
	})
	t.Run("an existing prefix stops it before anything is copied", func(t *testing.T) {
		l := newLab(t)
		os.MkdirAll(l.prefix, 0o755)
		p := l.pkg("good.pkg", "good", 2048)
		r := l.run(p, team, "-", "-", "install")
		if r.exit != ExitPrefixExists || !strings.Contains(r.stderr, "already exists") {
			t.Fatalf("%+v", r)
		}
		if _, err := os.Stat(l.installs); err == nil {
			t.Error("the installer ran")
		}
		// a dangling link counts too
		os.RemoveAll(l.prefix)
		os.MkdirAll(filepath.Dir(l.prefix), 0o755)
		os.Symlink(filepath.Join(l.dir, "nowhere"), l.prefix)
		if r := l.run(p, team, "-", "-", "install"); r.exit != ExitPrefixExists {
			t.Errorf("dangling link: %+v", r)
		}
	})
	t.Run("the folder above the prefix must be root's and closed to others", func(t *testing.T) {
		l := newLab(t)
		l.subs["STRICTPARENT"] = "1"
		l.subs["PARENTUID"] = strconv.Itoa(os.Getuid())
		parent := filepath.Dir(l.prefix)
		p := l.pkg("good.pkg", "good", 2048)
		os.MkdirAll(parent, 0o755)
		os.Chmod(parent, 0o777)
		if r := l.run(p, team, "-", "-", "install"); r.exit == 0 || !strings.Contains(r.stderr, "writable by others") {
			t.Fatalf("a group/world writable parent must stop it: %+v", r)
		}
		os.Chmod(parent, 0o755)
		if r := l.run(p, team, "-", "-", "install"); r.exit != 0 {
			t.Fatalf("a closed parent is fine: %+v", r)
		}
		l.subs["PARENTUID"] = strconv.Itoa(os.Getuid() + 1)
		os.RemoveAll(l.prefix)
		if r := l.run(p, team, "-", "-", "install"); r.exit == 0 || !strings.Contains(r.stderr, "not owned by root") {
			t.Fatalf("a parent of another owner must stop it: %+v", r)
		}
	})
	t.Run("a prefix planted during the verification stops it before the installer", func(t *testing.T) {
		l := newLab(t)
		p := l.pkg("good.pkg", "good", 2048)
		// the fake spctl runs after the first prefix check: it plants the folder
		l.write(filepath.Join(l.dir, "bin", "spctl"), "#!/bin/sh\nif [ \"$1\" = \"--status\" ]; then /bin/mkdir -p '"+l.prefix+"'; echo 'assessments disabled'; exit 0; fi\nexit 1\n", 0o755)
		r := l.run(p, team, "-", "-", "install")
		if r.exit != ExitPrefixExists {
			t.Fatalf("%+v", r)
		}
		if _, err := os.Stat(l.installs); err == nil {
			t.Error("the installer ran")
		}
	})
	t.Run("verify mode ignores the prefix and never installs", func(t *testing.T) {
		l := newLab(t)
		os.MkdirAll(l.prefix, 0o755)
		p := l.pkg("good.pkg", "good", 2048)
		if r := l.run(p, team, "-", "-", "verify"); r.exit != 0 {
			t.Fatalf("%+v", r)
		}
		if _, err := os.Stat(l.installs); err == nil {
			t.Error("the installer ran")
		}
	})
	t.Run("the installer's last lines reach stderr", func(t *testing.T) {
		l := newLab(t)
		var lines []string
		for i := 1; i <= 30; i++ {
			lines = append(lines, "installer: line "+strconv.Itoa(i))
		}
		lines = append(lines, "installer: No space left on device")
		l.installer(1, strings.Join(lines, "\n"), false)
		p := l.pkg("good.pkg", "good", 2048)
		r := l.run(p, team, "-", "-", "install")
		if r.exit != ExitInstallFailed || !strings.Contains(r.stderr, "No space left on device") || !strings.Contains(r.stderr, "line 30") || strings.Contains(r.stderr, "line 5\n") {
			t.Fatalf("%+v", r)
		}
		if c := MacPrivileged(1, "1:1: execution error: "+r.stderr+" (30)"); c != api.CauseDiskFull {
			t.Errorf("classified %q", c)
		}
	})
	t.Run("no brew afterwards", func(t *testing.T) {
		l := newLab(t)
		l.installer(0, "installer: done", false)
		p := l.pkg("good.pkg", "good", 2048)
		if r := l.run(p, team, "-", "-", "install"); r.exit != ExitBrewMissing {
			t.Fatalf("%+v", r)
		}
	})
}

func TestScriptGatekeeperAnswers(t *testing.T) {
	l := newLab(t)
	p := l.pkg("good.pkg", "good", 2048)
	// Gatekeeper on and rejecting: exit 14.
	l.spctl("enabled", "/x: rejected\nsource=no usable signature", 3)
	if r := l.run(p, team, "-", "-", "verify"); r.exit != ExitNotNotarized || !strings.Contains(r.stderr, "spctl") {
		t.Errorf("rejected: %+v", r)
	}
	// Gatekeeper on, accepted, but the origin names another team: exit 14.
	l.spctl("enabled", "/x: accepted\nsource=Notarized Developer ID\norigin=Developer ID Installer: Another Maintainer (ABCDEFGHIJ)", 0)
	if r := l.run(p, team, "-", "-", "verify"); r.exit != ExitNotNotarized {
		t.Errorf("other origin: %+v", r)
	}
	// Gatekeeper off: spctl cannot judge. The defined answer is that pkgutil's checks stand, and it says so.
	l.spctl("disabled", "/x: accepted", 0)
	r := l.run(p, team, "-", "-", "verify")
	if r.exit != 0 || !strings.Contains(r.stdout, "gatekeeper=off") {
		t.Errorf("gatekeeper off: %+v", r)
	}
	// ... but a package that pkgutil rejects is rejected with Gatekeeper off as well.
	if r := l.run(l.pkg("wt.pkg", "wrongteam", 100), team, "-", "-", "verify"); r.exit != ExitWrongTeam {
		t.Errorf("gatekeeper off, wrong team: %+v", r)
	}
}

func TestScriptRefusesBadArgumentsAndNonRoot(t *testing.T) {
	l := newLab(t)
	p := l.pkg("good.pkg", "good", 100)
	for name, a := range map[string][]string{
		"empty team":    {p, "", "-", "-", "verify"},
		"short team":    {p, "ABC", "-", "-", "verify"},
		"team injected": {p, "ABCDEFGHIJ; /usr/bin/touch " + filepath.Join(l.dir, "pwned"), "-", "-", "verify"},
		"bad size":      {p, team, "12x", "-", "verify"},
		"bad digest":    {p, team, "-", "xyz", "verify"},
		"bad mode":      {p, team, "-", "-", "format"},
		"no path":       {"", team, "-", "-", "verify"},
	} {
		if r := l.run(a...); r.exit != ExitCopyFailed {
			t.Errorf("%s: %+v", name, r)
		}
	}
	if _, err := os.Stat(filepath.Join(l.dir, "pwned")); err == nil {
		t.Error("an argument was run as shell text")
	}
	// The production text itself, with no header changed, refuses to run as an ordinary user.
	if os.Getuid() != 0 {
		cmd := exec.Command("/usr/bin/env", "-i", "/bin/bash", "--noprofile", "--norc", "-c", pkgScript, "codepass-pkg", p, team, "-", "-", "verify")
		err := cmd.Run()
		if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != ExitNotRoot {
			t.Errorf("unmodified script as a normal user: %v", err)
		}
	}
}

// What the caller's environment holds must not change what the script runs.
func TestScriptIgnoresCallerEnvironment(t *testing.T) {
	l := newLab(t)
	p := l.pkg("good.pkg", "good", 2048)
	m := filepath.Join(l.dir, "marker")
	bashEnv := filepath.Join(l.dir, "bashenv.sh")
	l.write(bashEnv, "/usr/bin/touch '"+m+"-bashenv'\n", 0o644)
	hostile := []string{
		"BASH_ENV=" + bashEnv,
		"ENV=" + bashEnv,
		"BASH_FUNC_grep%%=() { /usr/bin/touch '" + m + "-grep'; command grep \"$@\"; }",
		"BASH_FUNC_head%%=() { /usr/bin/touch '" + m + "-head'; command head \"$@\"; }",
		"BASH_FUNC_cat%%=() { /usr/bin/touch '" + m + "-cat'; command cat \"$@\"; }",
		"SHELLOPTS=xtrace",
		"PS4=$(/usr/bin/touch '" + m + "-ps4')",
		"PATH=" + filepath.Join(l.dir, "evil"),
		"IFS=x",
	}
	evil := filepath.Join(l.dir, "evil")
	os.MkdirAll(evil, 0o755)
	for _, n := range []string{"grep", "head", "stat", "openssl", "mktemp", "tr", "cut", "rm", "chmod", "tail", "id"} {
		l.write(filepath.Join(evil, n), "#!/bin/sh\n/usr/bin/touch '"+m+"-path-"+n+"'\n", 0o755)
	}
	r := l.runEnv(hostile, p, team, sizeOf(p), sum(p), "install")
	if r.exit != 0 {
		t.Fatalf("%+v", r)
	}
	ents, _ := filepath.Glob(m + "-*")
	if len(ents) != 0 {
		t.Fatalf("the caller's environment changed what ran: %v", ents)
	}
	// Control: the same hostile environment DOES act on a shell without env -i, so the test can see it.
	cmd := exec.Command("/bin/bash", "--noprofile", "--norc", "-c", "grep -q x /dev/null; true")
	cmd.Env = hostile
	cmd.Run()
	if ents, _ := filepath.Glob(m + "-bashenv"); len(ents) != 1 {
		t.Fatal("control failed: BASH_ENV did not run in a bare bash; the test cannot tell")
	}
}

// The real package through the real pkgutil and spctl, in verify mode (nothing is installed, nothing needs root).
// Set CODEPASS_TEST_HOMEBREW_PKG to the path of a downloaded Homebrew.pkg; the cases of the S0-04 test follow.
func TestScriptVerifyWithTheRealHomebrewPkg(t *testing.T) {
	good := os.Getenv("CODEPASS_TEST_HOMEBREW_PKG")
	if good == "" {
		t.Skip("set CODEPASS_TEST_HOMEBREW_PKG to the path of a downloaded Homebrew.pkg")
	}
	l := newLab(t)
	for _, k := range []string{"PKGUTIL", "SPCTL", "INSTALLER"} {
		delete(l.subs, k) // the real tools
	}
	size, sha := sizeOf(good), sum(good)
	n, _ := strconv.Atoi(size)
	if r := l.run(good, team, size, sha, "verify"); r.exit != 0 || !strings.Contains(r.stdout, "verified: size="+size) {
		t.Fatalf("good pkg: %+v", r)
	}
	if r := l.run(good, team, strconv.Itoa(n-1), "-", "verify"); r.exit != ExitSizeMismatch {
		t.Errorf("wrong size: %+v", r)
	}
	if r := l.run(good, "ABCDEFGHIJ", size, "-", "verify"); r.exit != ExitWrongTeam {
		t.Errorf("wrong team: %+v", r)
	}
	// one byte flipped in the middle of the payload
	flip := filepath.Join(l.dir, "flip.pkg")
	b, _ := os.ReadFile(good)
	b[n/2] ^= 0xff
	os.WriteFile(flip, b, 0o600)
	if r := l.run(flip, team, size, "-", "verify"); r.exit != ExitNotSigned {
		t.Errorf("flipped byte: %+v", r)
	}
	// truncated to a third, and a proxy's web page
	os.WriteFile(filepath.Join(l.dir, "trunc.pkg"), b[:n/3], 0o600)
	if r := l.run(filepath.Join(l.dir, "trunc.pkg"), team, "-", "-", "verify"); r.exit != ExitNotSigned {
		t.Errorf("truncated: %+v", r)
	}
	html := filepath.Join(l.dir, "proxy.pkg")
	os.WriteFile(html, []byte("<html><body>407 Proxy Authentication Required</body></html>"), 0o600)
	if r := l.run(html, team, "-", "-", "verify"); r.exit != ExitNotSigned {
		t.Errorf("web page: %+v", r)
	}
}
