package toolinstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/netx"
)

// rig is a fake machine and a fake GitHub for the installer: a local server for the release API and the
// package, fake pkgutil, osascript, brew and winget, and a recorder for every state the installer emits.
type rig struct {
	t       *testing.T
	dir     string
	cache   string
	prefix  string
	brew    string
	srv     *httptest.Server
	pkgBody []byte
	hits    struct{ api, pkg, fallback atomic.Int32 }

	mu     sync.Mutex
	events []api.InstallState
	mode   string        // "ok" | "ratelimit" | "digest-lie" | "html" | "truncate" | "hang" | "slowapi"
	hang   chan struct{} // closed to release a "hang" download
	osaOut string        // what the fake osascript prints on success (default "installed")
}

func newRig(t *testing.T) *rig {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake tools are shell scripts")
	}
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	r := &rig{t: t, dir: dir, cache: filepath.Join(dir, "downloads"), prefix: filepath.Join(dir, "opt", "homebrew"), mode: "ok", hang: make(chan struct{})}
	r.brew = filepath.Join(r.prefix, "bin", "brew")
	r.pkgBody = []byte("FIXTURE:good\n" + strings.Repeat("p", 70_000))
	mux := http.NewServeMux()
	mux.HandleFunc("/api/latest", func(w http.ResponseWriter, req *http.Request) {
		r.hits.api.Add(1)
		if r.mode == "slowapi" { // a slow proxy: the release lookup does not answer
			select {
			case <-r.hang:
			case <-req.Context().Done():
			}
			return
		}
		if r.mode == "ratelimit" {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		sum := sha256.Sum256(r.pkgBody)
		digest := "sha256:" + hex.EncodeToString(sum[:])
		if r.mode == "digest-lie" {
			digest = "sha256:" + strings.Repeat("0", 64)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "7.0.13",
			"assets": []map[string]any{
				{"name": "Homebrew.zip", "size": 1, "digest": "sha256:" + strings.Repeat("1", 64), "browser_download_url": r.srv.URL + "/other"},
				{"name": "Homebrew.pkg", "size": len(r.pkgBody), "digest": digest, "browser_download_url": r.srv.URL + "/Homebrew.pkg"},
			},
		})
	})
	serve := func(w http.ResponseWriter, req *http.Request) {
		switch r.mode {
		case "html":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html>407 Proxy Authentication Required</html>"))
		case "truncate":
			w.Write(r.pkgBody[:len(r.pkgBody)/2]) // no Content-Length: the answer just ends early
		case "hang":
			w.Write(r.pkgBody[:2000])
			w.(http.Flusher).Flush()
			select {
			case <-r.hang:
			case <-req.Context().Done():
			}
		default:
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(r.pkgBody)
		}
	}
	mux.HandleFunc("/Homebrew.pkg", func(w http.ResponseWriter, q *http.Request) { r.hits.pkg.Add(1); serve(w, q) })
	mux.HandleFunc("/fallback/Homebrew.pkg", func(w http.ResponseWriter, q *http.Request) { r.hits.fallback.Add(1); serve(w, q) })
	r.srv = httptest.NewServer(mux)
	t.Cleanup(r.srv.Close)
	t.Cleanup(func() {
		select {
		case <-r.hang:
		default:
			close(r.hang)
		}
	})
	return r
}

func (r *rig) script(name, body string) string {
	r.t.Helper()
	p := filepath.Join(r.dir, "bin", name)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		r.t.Fatal(err)
	}
	return p
}

func (r *rig) pkgutil() string {
	os.MkdirAll(filepath.Join(r.dir, "fix"), 0o755)
	for n, text := range sigFixtures {
		os.WriteFile(filepath.Join(r.dir, "fix", n+".txt"), []byte(text), 0o644)
	}
	return r.script("pkgutil", "first=$(/usr/bin/head -n1 \"$2\")\ncase \"$first\" in FIXTURE:*) ;; *) echo 'Could not open package'; exit 1;; esac\n/bin/cat '"+r.dir+"/fix/'\"${first#FIXTURE:}\".txt\n")
}

// brewBody is the text of a fake brew: it records its arguments and environment, optionally waits for a file,
// prints out and exits with code.
func (r *rig) brewBody(code int, out, wait string) string {
	w := ""
	if wait != "" {
		w = fmt.Sprintf("while [ ! -e '%s' ]; do /bin/sleep 0.05; done\n", wait)
	}
	return fmt.Sprintf("#!/bin/sh\necho \"$@\" >> '%s/brew-calls'\n/usr/bin/env > '%s/brew-env'\n%s/bin/cat <<'EOT'\n%s\nEOT\nexit %d\n", r.dir, r.dir, w, out, code)
}

func (r *rig) putBrew(body string) {
	r.t.Helper()
	os.MkdirAll(filepath.Dir(r.brew), 0o755)
	if err := os.WriteFile(r.brew, []byte(body), 0o755); err != nil {
		r.t.Fatal(err)
	}
}

// osa is the fake osascript. On success it "installs Homebrew": it writes brewBody to the brew path.
func (r *rig) osa(exit int, stderr, brewBody string) string {
	os.WriteFile(filepath.Join(r.dir, "brew-template"), []byte(brewBody), 0o755)
	return r.script("osascript", fmt.Sprintf(
		"echo \"$@\" > '%s/osa-args'\n/usr/bin/env > '%s/osa-env'\necho x >> '%s/osa-calls'\nif [ %d -eq 0 ]; then\n/bin/mkdir -p '%s'\n/bin/cp '%s/brew-template' '%s'\nprintf '%%s' '%s'\nelse\nprintf '%%s' '%s' >&2\nfi\nexit %d\n",
		r.dir, r.dir, r.dir, exit, filepath.Dir(r.brew), r.dir, r.brew, orDefault(r.osaOut, "installed"), stderr, exit))
}

func (r *rig) cfg() Config {
	return Config{
		GOOS: "darwin", Net: netx.New(netx.Options{}), CacheDir: r.cache,
		Emit: func(s api.InstallState) {
			r.mu.Lock()
			r.events = append(r.events, s)
			r.mu.Unlock()
		},
		Pkgutil:     r.pkgutil(),
		OSAScript:   r.osa(0, "", r.brewBody(0, "==> Pouring x", "")),
		APIURL:      r.srv.URL + "/api/latest",
		FallbackURL: r.srv.URL + "/fallback/Homebrew.pkg",
		PrefixDir:   r.prefix, BrewPath: r.brew,
		BaseEnv: func() []string {
			return []string{"HOME=/Users/example", "PATH=/usr/bin", "HTTPS_PROXY=http://old.example.com:1", "BASH_ENV=/tmp/evil"}
		},
		Lang: func() string { return "en" },
	}
}

func rowOf(id api.SetupToolID, st api.ToolStatus, fix api.SetupFix) api.SetupTool {
	return api.SetupTool{ID: id, Status: st, Blocking: st != api.ToolOK, Fix: fix}
}

func macSetup(brew, git, aws api.SetupTool) api.SetupState {
	return api.SetupState{Platform: "darwin", Tools: []api.SetupTool{brew, git, aws}, Total: 3,
		Preflight: api.Preflight{Admin: api.TriYes, Managed: api.TriNo, FreeBytes: 50e9, NeedBytes: 1.2e9, EnoughSpace: true,
			Proxy: api.ProxyInfo{HostPort: "proxy.example.com:8080", Source: "system"}}}
}

func allMissing() api.SetupState {
	return macSetup(rowOf(api.SetupHomebrew, api.ToolMissing, api.FixInstall), rowOf(api.SetupGit, api.ToolMissing, api.FixInstall), rowOf(api.SetupAWSCLI, api.ToolMissing, api.FixInstall))
}

func brewReady() api.SetupState {
	return macSetup(rowOf(api.SetupHomebrew, api.ToolOK, ""), rowOf(api.SetupGit, api.ToolMissing, api.FixInstall), rowOf(api.SetupAWSCLI, api.ToolMissing, api.FixInstall))
}

func (r *rig) runAll(cfg Config, s api.SetupState) api.InstallState {
	r.t.Helper()
	in := New(cfg)
	if err := in.Start(context.Background(), s); err != nil {
		r.t.Fatal(err)
	}
	in.Wait()
	return r.finalCheck(in)
}

func (r *rig) finalCheck(in *Installer) api.InstallState {
	st := in.State()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.events) == 0 || !sameState(r.events[len(r.events)-1], st) {
		r.t.Errorf("the last emitted state is not the final state")
	}
	return st
}

func sameState(a, b api.InstallState) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func (r *rig) read(name string) string {
	b, _ := os.ReadFile(filepath.Join(r.dir, name))
	return string(b)
}
func (r *rig) exists(name string) bool {
	_, err := os.Stat(filepath.Join(r.dir, name))
	return err == nil
}

func stepStates(st api.InstallState) string {
	var out []string
	for _, s := range st.Steps {
		out = append(out, s.ID+"="+string(s.State))
	}
	return strings.Join(out, " ")
}

func TestHappyPathMac(t *testing.T) {
	r := newRig(t)
	cfg := r.cfg()
	cfg.Net = netx.New(netx.Options{Proxy: netx.Config{HTTPS: "http://proxy.example.com:8080", NoProxy: "internal.example.com"}})
	st := r.runAll(cfg, allMissing())
	if st.Failure != nil || st.Cancelled || !st.Finished || st.Running {
		t.Fatalf("failure %+v, steps: %s", st.Failure, stepStates(st))
	}
	if got := stepStates(st); got != "homebrew:download=done homebrew:verify=done homebrew:install=done git:install=done awscli:install=done" {
		t.Errorf("steps: %s", got)
	}
	// brew ran as the user, with analytics and auto-update off and the proxy in both cases, no stale proxy.
	if calls := r.read("brew-calls"); calls != "install git\ninstall awscli\n" {
		t.Errorf("brew calls: %q", calls)
	}
	env := r.read("brew-env")
	for _, want := range []string{"HOMEBREW_NO_ANALYTICS=1", "HOMEBREW_NO_AUTO_UPDATE=1", "HTTPS_PROXY=http://proxy.example.com:8080", "https_proxy=http://proxy.example.com:8080", "NO_PROXY=localhost,127.0.0.1,::1,internal.example.com", "PATH=" + r.prefix + "/bin:" + r.prefix + "/sbin:/usr/bin:/bin:/usr/sbin:/sbin"} {
		if !strings.Contains(env, want) {
			t.Errorf("brew env lacks %q", want)
		}
	}
	if strings.Contains(env, "old.example.com") {
		t.Errorf("a stale proxy reached brew")
	}
	// The password window got the package, the pinned team and the size and digest from the API.
	args := r.read("osa-args")
	sum := sha256.Sum256(r.pkgBody)
	for _, want := range []string{filepath.Join(r.cache, "Homebrew.pkg"), "927JGANW46", fmt.Sprint(len(r.pkgBody)), hex.EncodeToString(sum[:]), "Codepass needs your password"} {
		if !strings.Contains(args, want) {
			t.Errorf("osascript args lack %q:\n%s", want, args)
		}
	}
	if osaEnv := r.read("osa-env"); strings.Contains(osaEnv, "PROXY") || strings.Contains(osaEnv, "BASH_ENV") || strings.Contains(osaEnv, "HOME=") {
		t.Errorf("osascript env: %s", osaEnv)
	}
	// The package is gone afterwards, and no partial file is left.
	if ents, _ := os.ReadDir(r.cache); len(ents) != 0 {
		t.Errorf("download folder: %v", ents)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	sawWait, sawDL := false, false
	for _, e := range r.events {
		for _, s := range e.Steps {
			if s.ID == "homebrew:install" && s.State == api.StepWaitingOS && s.Indeterminate {
				sawWait = true
			}
			if s.ID == "homebrew:download" && s.Done > 0 && s.Total == int64(len(r.pkgBody)) {
				sawDL = true
			}
		}
	}
	if !sawWait || !sawDL {
		t.Errorf("events: wait=%v download=%v", sawWait, sawDL)
	}
	if joined := strings.Join(st.Log, "\n"); !strings.Contains(joined, "==> Pouring x") {
		t.Errorf("log: %q", joined)
	}
}

func TestDamagedDownloadsStopBeforeThePasswordWindow(t *testing.T) {
	for name, mode := range map[string]string{"digest differs from the API": "digest-lie", "a proxy's web page": "html", "answer ends early": "truncate"} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.mode = mode
			st := r.runAll(r.cfg(), allMissing())
			if st.Failure == nil || st.Failure.Cause != api.CauseDamagedDownload || st.Failure.StepID != "homebrew:download" {
				t.Fatalf("failure: %+v", st.Failure)
			}
			if r.exists("osa-args") {
				t.Error("the password window was asked for")
			}
			if ents, _ := os.ReadDir(r.cache); len(ents) != 0 {
				t.Errorf("leftovers: %v", ents)
			}
			if got := stepStates(st); !strings.Contains(got, "git:install=skipped") || !strings.Contains(got, "homebrew:download=failed") {
				t.Errorf("steps: %s", got)
			}
		})
	}
}

func TestSignatureProblemsStopBeforeThePasswordWindow(t *testing.T) {
	for fixture, want := range map[string]api.FailureCause{
		"invalid": api.CauseBadSignature, "unsigned": api.CauseBadSignature, "wrongteam": api.CauseUnknownSigner, "notnotarized": api.CauseNotNotarized,
	} {
		t.Run(fixture, func(t *testing.T) {
			r := newRig(t)
			r.pkgBody = []byte("FIXTURE:" + fixture + "\n" + strings.Repeat("p", 5000))
			st := r.runAll(r.cfg(), allMissing())
			if st.Failure == nil || st.Failure.Cause != want || st.Failure.StepID != "homebrew:verify" {
				t.Fatalf("failure: %+v", st.Failure)
			}
			if r.exists("osa-args") {
				t.Error("the password window was asked for")
			}
			if _, err := os.Stat(filepath.Join(r.cache, "Homebrew.pkg")); want != api.CauseNotNotarized && err == nil {
				t.Error("a package with a wrong signature was kept")
			}
			if got := stepStates(st); !strings.Contains(got, "homebrew:download=done homebrew:verify=failed homebrew:install=skipped") {
				t.Errorf("steps: %s", got)
			}
		})
	}
}

func TestRateLimitedAPIFallsBackToTheLatestURL(t *testing.T) {
	r := newRig(t)
	r.mode = "ratelimit"
	// the fallback URL serves the same body, with no size and no digest to compare: the signature check carries it
	st := r.runAll(r.cfg(), allMissing())
	if st.Failure != nil {
		t.Fatalf("failure: %+v", st.Failure)
	}
	if r.hits.fallback.Load() != 1 || r.hits.pkg.Load() != 0 {
		t.Errorf("hits: fallback %d, pkg %d", r.hits.fallback.Load(), r.hits.pkg.Load())
	}
	if args := r.read("osa-args"); !strings.Contains(args, " - - install ") {
		t.Errorf("size and digest must be unknown (-): %s", args)
	}
}

func TestPasswordWindowOutcomes(t *testing.T) {
	cases := []struct {
		name   string
		exit   int
		stderr string
		admin  api.Tri
		want   api.FailureCause
	}{
		{"cancelled", 1, "6:22: execution error: User canceled. (-128)", api.TriYes, api.CauseCancelled},
		{"wrong password", 1, "execution error: The administrator user name or password was incorrect. (-60005)", api.TriYes, api.CauseAuthDenied},
		{"standard account, wrong credentials", 1, "execution error: The administrator user name or password was incorrect. (-60005)", api.TriNo, api.CauseNotAdmin},
		{"prefix appeared meanwhile", 1, "7:1: execution error: codepass-pkg: /opt/homebrew already exists (20)", api.TriYes, api.CausePrefixExists},
		{"wrong signer, caught as root", 1, "7:1: execution error: codepass-pkg: Team ID is ABCDEFGHIJ, expected 927JGANW46 (13)", api.TriYes, api.CauseUnknownSigner},
		{"disk full", 1, "7:1: execution error: codepass-pkg: disk full (11)", api.TriYes, api.CauseDiskFull},
		{"no window", 1, "execution error: No user interaction allowed. (-60007)", api.TriYes, api.CauseNoSession},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			cfg := r.cfg()
			cfg.OSAScript = r.osa(c.exit, c.stderr, "")
			s := allMissing()
			s.Preflight.Admin = c.admin
			st := r.runAll(cfg, s)
			if st.Failure == nil || st.Failure.Cause != c.want || st.Failure.StepID != "homebrew:install" {
				t.Fatalf("failure: %+v", st.Failure)
			}
			if st.Failure.ExitCode == 0 {
				t.Errorf("the script's exit code is missing: %+v", st.Failure)
			}
			if got := stepStates(st); !strings.HasSuffix(got, "homebrew:install=failed git:install=skipped awscli:install=skipped") {
				t.Errorf("steps: %s", got)
			}
			if len(st.Log) == 0 || st.Failure.Line == "" || strings.Contains(st.Failure.Line, "execution error") {
				t.Errorf("the failure text must reach the log and the banner: %v / %q", st.Log, st.Failure.Line)
			}
		})
	}
}

func TestPreflightAndPrefixFailuresRunNothing(t *testing.T) {
	t.Run("not enough disk space", func(t *testing.T) {
		r := newRig(t)
		s := allMissing()
		s.Preflight.EnoughSpace, s.Preflight.FreeBytes = false, 300_000_000
		st := r.runAll(r.cfg(), s)
		if st.Failure == nil || st.Failure.Cause != api.CauseDiskFull || r.hits.api.Load() != 0 {
			t.Fatalf("%+v hits=%d", st.Failure, r.hits.api.Load())
		}
	})
	t.Run("an unfinished Homebrew folder", func(t *testing.T) {
		r := newRig(t)
		s := allMissing()
		s.Tools[0].Fix = api.FixRepairPrefix
		st := r.runAll(r.cfg(), s)
		if st.Failure == nil || st.Failure.Cause != api.CausePrefixExists || st.Failure.StepID != "homebrew:install" {
			t.Fatalf("%+v", st.Failure)
		}
		if r.hits.api.Load() != 0 || r.exists("osa-args") {
			t.Error("nothing may run")
		}
		if got := stepStates(st); got != "homebrew:install=failed git:install=skipped awscli:install=skipped" {
			t.Errorf("steps: %s", got)
		}
	})
	t.Run("the folder appears after the check", func(t *testing.T) {
		r := newRig(t)
		os.MkdirAll(r.prefix, 0o755) // someone created it while the screen was open
		st := r.runAll(r.cfg(), allMissing())
		if st.Failure == nil || st.Failure.Cause != api.CausePrefixExists || r.exists("osa-args") {
			t.Fatalf("%+v", st.Failure)
		}
	})
	t.Run("nothing to do", func(t *testing.T) {
		r := newRig(t)
		ok := api.SetupState{Tools: []api.SetupTool{rowOf(api.SetupHomebrew, api.ToolOK, ""), rowOf(api.SetupGit, api.ToolOK, ""), rowOf(api.SetupAWSCLI, api.ToolOutdated, "")}}
		ok.Tools[2].Blocking = false
		ok.Preflight.EnoughSpace = true
		st := r.runAll(r.cfg(), ok)
		if st.Failure != nil || len(st.Steps) != 0 || !st.Finished {
			t.Fatalf("%+v", st)
		}
	})
}

func TestBrewFailuresAreClassified(t *testing.T) {
	cases := []struct {
		name string
		out  string
		code int
		want api.FailureCause
	}{
		{"dead proxy", "==> Fetching git\nError: Failure while executing; `/usr/bin/env /x/shims/shared/curl --disable --fail https://ghcr.io/v2/x` exited with 7.", 1, api.CauseNetwork},
		{"proxy wants a sign-in", "Error: Failure while executing; `curl --fail https://ghcr.io/x` exited with 22.\ncurl: (22) The requested URL returned error: 407", 1, api.CauseProxyAuth},
		{"disk full", "Error: No space left on device @ io_write - /opt/homebrew/Cellar/git", 1, api.CauseDiskFull},
		{"other", "Error: git: unexpected failure in post_install", 1, api.CauseInstallerFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			r.putBrew(r.brewBody(c.code, c.out, ""))
			cfg := r.cfg()
			st := r.runAll(cfg, brewReady())
			f := st.Failure
			if f == nil || f.Cause != c.want || f.StepID != "git:install" || f.ExitCode != c.code {
				t.Fatalf("failure: %+v", f)
			}
			if !strings.HasPrefix(f.Line, "Error:") {
				t.Errorf("line: %q", f.Line)
			}
			if (c.want == api.CauseNetwork || c.want == api.CauseProxyAuth) && f.Proxy != "proxy.example.com:8080" {
				t.Errorf("the system proxy must be named: %q", f.Proxy)
			}
			if f.Command != "brew install git" {
				t.Errorf("command: %q", f.Command)
			}
			if got := stepStates(st); got != "git:install=failed awscli:install=skipped" {
				t.Errorf("steps: %s", got)
			}
		})
	}
}

func TestBrewMissingBeforeBrewSteps(t *testing.T) {
	r := newRig(t)
	st := r.runAll(r.cfg(), brewReady()) // the gate said brew is fine, but the file is not there
	if st.Failure == nil || st.Failure.Cause != api.CauseInstallerFailed || st.Failure.StepID != "git:install" {
		t.Fatalf("%+v", st.Failure)
	}
}

func TestUpgradeVerbs(t *testing.T) {
	r := newRig(t)
	r.putBrew(r.brewBody(0, "", ""))
	git := rowOf(api.SetupGit, api.ToolTooOld, api.FixUpgrade)
	git.Elsewhere = false // installed by brew itself: upgrade
	aws := rowOf(api.SetupAWSCLI, api.ToolTooOld, api.FixUpgrade)
	aws.Elsewhere = true // found elsewhere: a newer one is installed next to it
	st := r.runAll(r.cfg(), macSetup(rowOf(api.SetupHomebrew, api.ToolOK, ""), git, aws))
	if st.Failure != nil {
		t.Fatalf("%+v", st.Failure)
	}
	if calls := r.read("brew-calls"); calls != "upgrade git\ninstall awscli\n" {
		t.Errorf("brew calls: %q", calls)
	}
}

func TestCancelAfterTheCurrentBrewStep(t *testing.T) {
	r := newRig(t)
	release := filepath.Join(r.dir, "release")
	r.putBrew(r.brewBody(0, "==> Pouring git", release))
	in := New(r.cfg())
	if err := in.Start(context.Background(), brewReady()); err != nil {
		t.Fatal(err)
	}
	if err := in.Start(context.Background(), brewReady()); err != ErrRunning {
		t.Errorf("second start: %v", err)
	}
	waitFor(t, func() bool { return strings.Contains(r.read("brew-calls"), "install git") })
	in.Cancel()
	if st := in.State(); !st.Cancelling || !st.Running {
		t.Errorf("the running step must go on: %+v", st)
	}
	os.WriteFile(release, nil, 0o600)
	in.Wait()
	st := r.finalCheck(in)
	if !st.Cancelled || st.Failure != nil || st.Running {
		t.Fatalf("%+v", st)
	}
	if got := stepStates(st); got != "git:install=done awscli:install=skipped" {
		t.Errorf("what is installed stays, the rest is skipped: %s", got)
	}
}

func TestCancelStopsTheDownloadAtOnce(t *testing.T) {
	r := newRig(t)
	r.mode = "hang"
	in := New(r.cfg())
	if err := in.Start(context.Background(), allMissing()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return r.hits.pkg.Load() == 1 })
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	in.Cancel()
	in.Wait()
	if time.Since(start) > 3*time.Second {
		t.Errorf("cancel took %s", time.Since(start))
	}
	st := r.finalCheck(in)
	if !st.Cancelled || st.Failure != nil {
		t.Fatalf("%+v", st)
	}
	if ents, _ := os.ReadDir(r.cache); len(ents) != 0 {
		t.Errorf("partial file left: %v", ents)
	}
	if r.exists("osa-args") {
		t.Error("no password window after cancel")
	}
}

func TestStalledDownloadIsANetworkFailure(t *testing.T) {
	r := newRig(t)
	r.mode = "hang"
	old := stallTimeout
	stallTimeout = 300 * time.Millisecond
	t.Cleanup(func() { stallTimeout = old })
	st := r.runAll(r.cfg(), allMissing())
	if st.Failure == nil || st.Failure.Cause != api.CauseNetwork || st.Failure.StepID != "homebrew:download" {
		t.Fatalf("%+v", st.Failure)
	}
	if st.Failure.Proxy != "proxy.example.com:8080" {
		t.Errorf("proxy: %q", st.Failure.Proxy)
	}
}

func TestTryAgainContinuesAndReusesAGoodDownload(t *testing.T) {
	r := newRig(t)
	// first run: the password window is cancelled
	cfg := r.cfg()
	cfg.OSAScript = r.osa(1, "execution error: User canceled. (-128)", "")
	st := r.runAll(cfg, allMissing())
	if st.Failure == nil || st.Failure.Cause != api.CauseCancelled {
		t.Fatalf("%+v", st.Failure)
	}
	if r.hits.pkg.Load() != 1 {
		t.Fatalf("pkg hits %d", r.hits.pkg.Load())
	}
	// the package stays (it was good); a second run reuses it without downloading again
	cfg = r.cfg()
	st = r.runAll(cfg, allMissing())
	if st.Failure != nil {
		t.Fatalf("%+v", st.Failure)
	}
	if r.hits.pkg.Load() != 1 {
		t.Errorf("downloaded again: %d", r.hits.pkg.Load())
	}
	// third run, brew is there: only the brew rows run
	os.RemoveAll(r.cache)
	st = r.runAll(r.cfg(), macSetup(rowOf(api.SetupHomebrew, api.ToolOK, ""), rowOf(api.SetupGit, api.ToolOK, ""), rowOf(api.SetupAWSCLI, api.ToolMissing, api.FixInstall)))
	if got := stepStates(st); got != "awscli:install=done" {
		t.Errorf("steps: %s", got)
	}
}

func TestEmittedStatesAreIndependentCopies(t *testing.T) {
	r := newRig(t)
	r.putBrew(r.brewBody(0, "line 1\nline 2", ""))
	r.runAll(r.cfg(), brewReady())
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.events) < 3 {
		t.Fatalf("events: %d", len(r.events))
	}
	first := r.events[0]
	if !first.Running || len(first.Steps) != 2 || first.Steps[0].State != api.StepRunning && first.Steps[0].State != api.StepQueued {
		t.Errorf("first event: %+v", first)
	}
	// the first snapshot was not changed by what happened later
	if len(first.Log) != 0 || first.Finished {
		t.Errorf("an emitted snapshot was mutated: %+v", first)
	}
}

func TestLogIsCappedAndCleaned(t *testing.T) {
	r := newRig(t)
	var out strings.Builder
	for i := 0; i < 500; i++ {
		fmt.Fprintf(&out, "\x1b[32m==>\x1b[0m line %d\n", i)
		out.WriteString("######## 100.0%\n")
	}
	out.WriteString("password=hunter2 https://user:pw@example.com/x\n")
	r.putBrew(r.brewBody(0, out.String(), ""))
	st := r.runAll(r.cfg(), brewReady())
	if len(st.Log) != logCap {
		t.Fatalf("log length %d", len(st.Log))
	}
	joined := strings.Join(st.Log, "\n")
	if strings.Contains(joined, "\x1b") || strings.Contains(joined, "####") || strings.Contains(joined, "hunter2") || strings.Contains(joined, ":pw@") {
		t.Errorf("log not cleaned: %q", joined[len(joined)-300:])
	}
}

func TestWindowsFlow(t *testing.T) {
	r := newRig(t)
	winget := func(code int) string {
		return r.script("winget", fmt.Sprintf("echo \"$@\" >> '%s/winget-calls'\n/usr/bin/env > '%s/winget-env'\necho 'Found Git'\nexit %d\n", r.dir, r.dir, code))
	}
	setup := func(git, aws api.SetupTool) api.SetupState {
		return api.SetupState{Platform: "windows", Preflight: api.Preflight{Admin: api.TriUnknown, EnoughSpace: true},
			Tools: []api.SetupTool{{ID: api.SetupWinget, Status: api.ToolOK}, git, aws}}
	}
	cfg := r.cfg()
	cfg.GOOS = "windows"
	cfg.Net = netx.New(netx.Options{Proxy: netx.Config{HTTPS: "http://proxy.example.com:8080"}})

	cfg.WingetPath = winget(0)
	st := r.runAll(cfg, setup(rowOf(api.SetupGit, api.ToolMissing, api.FixInstall), rowOf(api.SetupAWSCLI, api.ToolV1, api.FixUpgrade)))
	if st.Failure != nil {
		t.Fatalf("%+v", st.Failure)
	}
	calls := r.read("winget-calls")
	// `install` for both (it upgrades an installed copy); the AWS CLI is machine-wide until check B11
	want := "install --id Git.Git -e --accept-package-agreements --accept-source-agreements --silent\ninstall --id Amazon.AWSCLI -e --scope machine --accept-package-agreements --accept-source-agreements --silent\n"
	if calls != want {
		t.Errorf("winget calls:\n%s", calls)
	}
	if env := r.read("winget-env"); !strings.Contains(env, "HTTPS_PROXY=http://proxy.example.com:8080") || !strings.Contains(env, "https_proxy=") {
		t.Errorf("winget env lacks the proxy: %s", env)
	}
	if r.hits.api.Load() != 0 || r.exists("osa-args") {
		t.Error("no pkg flow on Windows")
	}
	// Windows exit codes are 32-bit; a shell cannot return them, so a fake runner does.
	for code, cause := range map[uint32]api.FailureCause{
		0x8A15010C: api.CauseCancelled, 0x8A150019: api.CauseNeedsAdmin, 0x8A150107: api.CauseNetwork,
		0x8A150105: api.CauseDiskFull, 0x8A15010F: api.CausePolicy, 0x8A150109: api.CauseReboot, 0x8A150049: api.CauseInstallerFailed,
	} {
		c2 := cfg
		c2.Runner = runnerFunc(func(ctx context.Context, c Cmd, onLine func(string)) (int, error) { return int(code), nil })
		c2.WingetPath = `C:\fake\winget.exe`
		st := r.runAll(c2, setup(rowOf(api.SetupGit, api.ToolMissing, api.FixInstall), rowOf(api.SetupAWSCLI, api.ToolOK, "")))
		if st.Failure == nil || st.Failure.Cause != cause || st.Failure.ExitCode != int(code) {
			t.Errorf("%#x: %+v", code, st.Failure)
		}
	}
	// "no applicable upgrade" and "already installed" are success
	for _, code := range []uint32{0x8A15002B, 0x8A15010D} {
		c2 := cfg
		c2.Runner = runnerFunc(func(ctx context.Context, c Cmd, onLine func(string)) (int, error) { return int(code), nil })
		c2.WingetPath = `C:\fake\winget.exe`
		if st := r.runAll(c2, setup(rowOf(api.SetupGit, api.ToolMissing, api.FixInstall), rowOf(api.SetupAWSCLI, api.ToolOK, ""))); st.Failure != nil {
			t.Errorf("%#x must be success: %+v", code, st.Failure)
		}
	}
	// winget gets a PATH of its own, never none
	if env := r.read("winget-env"); !strings.Contains(env, `PATH=C:\Windows\System32;C:\Windows;`) {
		t.Errorf("winget env: %s", env)
	}
	// a start failure
	cfg.WingetPath = filepath.Join(r.dir, "bin", "no-such-winget")
	st = r.runAll(cfg, setup(rowOf(api.SetupGit, api.ToolMissing, api.FixInstall), rowOf(api.SetupAWSCLI, api.ToolOK, "")))
	if st.Failure == nil || st.Failure.Cause != api.CauseWingetMissing {
		t.Fatalf("%+v", st.Failure)
	}
	// winget itself missing: the gate offers the Store, a run fails with its own cause and runs nothing
	st = r.runAll(cfg, api.SetupState{Platform: "windows", Preflight: api.Preflight{EnoughSpace: true},
		Tools: []api.SetupTool{rowOf(api.SetupWinget, api.ToolMissing, api.FixOpenStore), rowOf(api.SetupGit, api.ToolMissing, api.FixInstall), rowOf(api.SetupAWSCLI, api.ToolOK, "")}})
	if st.Failure == nil || st.Failure.Cause != api.CauseWingetMissing || st.Failure.StepID != "winget:install" {
		t.Fatalf("%+v", st.Failure)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("timed out waiting")
}

type runnerFunc func(ctx context.Context, c Cmd, onLine func(string)) (int, error)

func (f runnerFunc) Run(ctx context.Context, c Cmd, onLine func(string)) (int, error) {
	return f(ctx, c, onLine)
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// The real osascript separates the script's lines with \r. Every line must reach the log, the banner shows the
// last one, and a successful run logs the script's own answer (the Gatekeeper-off sentence).
func TestOSAScriptLinesAreSplitOnCR(t *testing.T) {
	r := newRig(t)
	cfg := r.cfg()
	cfg.OSAScript = r.osa(1, "7:1: execution error: codepass-pkg: installer failed (exit 1). Last lines:\rinstaller: line 1\rinstaller: line 2\rinstaller: No space left on device (30)\n", "")
	st := r.runAll(cfg, allMissing())
	if st.Failure == nil || st.Failure.ExitCode != 30 {
		t.Fatalf("%+v", st.Failure)
	}
	if st.Failure.Line != "installer: No space left on device" {
		t.Errorf("banner line: %q", st.Failure.Line)
	}
	log := strings.Join(st.Log, "\n")
	for _, want := range []string{"codepass-pkg: installer failed (exit 1). Last lines:", "installer: line 1", "installer: line 2"} {
		if !strings.Contains(log, want) {
			t.Errorf("log lacks %q:\n%s", want, log)
		}
	}
	for _, l := range st.Log {
		if strings.Contains(l, "execution error") || len([]rune(l)) > 100 {
			t.Errorf("a fused or unprefixed-line problem: %q", l)
		}
	}

	r2 := newRig(t)
	r2.osaOut = "verified: size=1 team=927JGANW46 sha256=ab gatekeeper=off\rinstalled\n"
	cfg2 := r2.cfg()
	cfg2.OSAScript = r2.osa(0, "", r2.brewBody(0, "", ""))
	st2 := r2.runAll(cfg2, allMissing())
	if st2.Failure != nil {
		t.Fatalf("%+v", st2.Failure)
	}
	if !strings.Contains(strings.Join(st2.Log, "\n"), "gatekeeper=off") {
		t.Errorf("the verified line must be logged: %v", st2.Log)
	}
}

// The real /usr/bin/osascript, with the administrator clause removed (no root, no dialog), really separates the
// lines with \r, and osaLines turns that into clean lines.
func TestOSAScriptRealOutputShape(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	if _, err := os.Stat(OSAScriptPath); err != nil {
		t.Skip("no osascript")
	}
	out, err := exec.Command(OSAScriptPath, "-e", `do shell script "echo 'a one'; echo 'b two' >&2; echo 'c three' >&2; exit 30"`).CombinedOutput()
	if err == nil {
		t.Fatal("the script exits 30")
	}
	got := osaLines(string(out))
	// stdout of a failed script is lost (documented); stderr arrives as two lines
	if len(got) != 2 || got[0] != "b two" || got[1] != "c three" {
		t.Errorf("%q -> %q", out, got)
	}
}

func TestCancelDuringTheReleaseLookupCancelsAtOnce(t *testing.T) {
	r := newRig(t)
	r.mode = "slowapi"
	in := New(r.cfg())
	if err := in.Start(context.Background(), allMissing()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return r.hits.api.Load() == 1 })
	start := time.Now()
	in.Cancel()
	in.Wait()
	if time.Since(start) > 3*time.Second {
		t.Errorf("cancel took %s", time.Since(start))
	}
	st := r.finalCheck(in)
	if !st.Cancelled || st.Failure != nil || r.hits.pkg.Load() != 0 {
		t.Fatalf("cancelled=%v failure=%+v pkgHits=%d", st.Cancelled, st.Failure, r.hits.pkg.Load())
	}
}

func TestRemoveStaleDownload(t *testing.T) {
	r := newRig(t)
	os.MkdirAll(r.cache, 0o755)
	p := filepath.Join(r.cache, pkgAsset)
	os.WriteFile(p, []byte("x"), 0o600)
	other := filepath.Join(r.cache, "keep.txt")
	os.WriteFile(other, []byte("x"), 0o600)
	New(r.cfg()).RemoveStaleDownload()
	if r.exists("downloads/"+pkgAsset) || !r.exists("downloads/keep.txt") {
		t.Errorf("only Homebrew.pkg goes")
	}
}

// FU-15e: the Health "Update" action runs the installer that manages the tool: brew upgrade on macOS (brew
// refreshes its list first), winget upgrade on Windows; progress and failure causes as for an install.
func TestStartUpdateMac(t *testing.T) {
	r := newRig(t)
	r.putBrew(r.brewBody(0, "==> Upgrading git", ""))
	in := New(r.cfg())
	if err := in.StartUpdate(context.Background(), brewReady(), api.SetupGit); err != nil {
		t.Fatal(err)
	}
	if st := in.State(); !st.Running || len(st.Steps) != 1 || st.Steps[0].ID != "git:update" || st.Steps[0].Command != "brew upgrade git" {
		t.Fatalf("%+v", st)
	}
	in.Wait()
	st := r.finalCheck(in)
	if st.Failure != nil || st.Cancelled || stepStates(st) != "git:update=done" {
		t.Fatalf("%+v %s", st.Failure, stepStates(st))
	}
	if calls := r.read("brew-calls"); calls != "upgrade git\n" {
		t.Errorf("brew calls: %q", calls)
	}
	if env := r.read("brew-env"); strings.Contains(env, "HOMEBREW_NO_AUTO_UPDATE") || !strings.Contains(env, "NONINTERACTIVE=1") {
		t.Errorf("an update lets brew refresh its list: %s", env)
	}
	if len(st.Log) == 0 || st.Log[len(st.Log)-1] != "==> Upgrading git" {
		t.Errorf("log: %v", st.Log)
	}
	// an install after an update is a normal run again
	if err := in.Start(context.Background(), brewReady()); err != nil {
		t.Fatal(err)
	}
	in.Wait()
	if env := r.read("brew-env"); !strings.Contains(env, "HOMEBREW_NO_AUTO_UPDATE=1") {
		t.Errorf("an install keeps the auto-update off: %s", env)
	}
}

func TestStartUpdateRefusesOtherToolsAndSecondRuns(t *testing.T) {
	r := newRig(t)
	r.putBrew(r.brewBody(0, "", ""))
	in := New(r.cfg())
	for _, id := range []api.SetupToolID{api.SetupHomebrew, api.SetupWinget, "nope"} {
		if err := in.StartUpdate(context.Background(), brewReady(), id); err != ErrNotUpdatable {
			t.Errorf("%s: %v", id, err)
		}
	}
	release := filepath.Join(r.dir, "release")
	r.putBrew(r.brewBody(0, "", release))
	if err := in.StartUpdate(context.Background(), brewReady(), api.SetupAWSCLI); err != nil {
		t.Fatal(err)
	}
	if err := in.StartUpdate(context.Background(), brewReady(), api.SetupGit); err != ErrRunning {
		t.Errorf("second start: %v", err)
	}
	os.WriteFile(release, nil, 0o600)
	in.Wait()
}

func TestStartUpdateFailureIsClassified(t *testing.T) {
	r := newRig(t)
	r.putBrew(r.brewBody(1, "Error: git is not installed", ""))
	st := r.runUpdate(r.cfg(), brewReady(), api.SetupGit)
	if st.Failure == nil || st.Failure.StepID != "git:update" || st.Failure.Tool != api.SetupGit || st.Failure.Command != "brew upgrade git" {
		t.Fatalf("%+v", st.Failure)
	}
	if st.Failure.Line != "Error: git is not installed" {
		t.Errorf("the line that explains: %q", st.Failure.Line)
	}
	// brew itself missing
	r2 := newRig(t)
	st = r2.runUpdate(r2.cfg(), brewReady(), api.SetupGit)
	if st.Failure == nil || st.Failure.Cause != api.CauseInstallerFailed {
		t.Fatalf("%+v", st.Failure)
	}
}

func (r *rig) runUpdate(cfg Config, s api.SetupState, tool api.SetupToolID) api.InstallState {
	r.t.Helper()
	in := New(cfg)
	if err := in.StartUpdate(context.Background(), s, tool); err != nil {
		r.t.Fatal(err)
	}
	in.Wait()
	return r.finalCheck(in)
}

// Cancel stops an update at once (the program is asked to stop); it is no failure.
func TestUpdateCancelStopsTheProgram(t *testing.T) {
	r := newRig(t)
	r.putBrew(r.brewBody(0, "==> Upgrading git", filepath.Join(r.dir, "never")))
	in := New(r.cfg())
	if err := in.StartUpdate(context.Background(), brewReady(), api.SetupGit); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return strings.Contains(r.read("brew-calls"), "upgrade git") })
	start := time.Now()
	in.Cancel()
	in.Wait()
	if time.Since(start) > 8*time.Second {
		t.Errorf("cancel took %s", time.Since(start))
	}
	st := r.finalCheck(in)
	if !st.Cancelled || st.Failure != nil || st.Running || stepStates(st) != "git:update=skipped" {
		t.Fatalf("%+v %s", st, stepStates(st))
	}
}

func TestStartUpdateWindows(t *testing.T) {
	r := newRig(t)
	winget := r.script("winget", fmt.Sprintf("echo \"$@\" >> '%s/winget-calls'\necho 'Found Git'\nexit 0\n", r.dir))
	cfg := r.cfg()
	cfg.GOOS, cfg.WingetPath = "windows", winget
	setup := api.SetupState{Platform: "windows", Preflight: api.Preflight{EnoughSpace: true},
		Tools: []api.SetupTool{{ID: api.SetupWinget, Status: api.ToolOK}, rowOf(api.SetupGit, api.ToolOK, ""), rowOf(api.SetupAWSCLI, api.ToolOK, "")}}
	st := r.runUpdate(cfg, setup, api.SetupAWSCLI)
	if st.Failure != nil || st.Steps[0].Command != "winget upgrade --id Amazon.AWSCLI -e" {
		t.Fatalf("%+v %+v", st.Failure, st.Steps)
	}
	want := "upgrade --id Amazon.AWSCLI -e --accept-package-agreements --accept-source-agreements --silent\n"
	if calls := r.read("winget-calls"); calls != want {
		t.Errorf("winget calls: %q", calls)
	}
	// "no applicable upgrade" (already the latest) is success; a refusal by policy is a cause
	for code, cause := range map[uint32]api.FailureCause{0x8A15002B: "", 0x8A15010F: api.CausePolicy, 0x8A150107: api.CauseNetwork} {
		c2 := cfg
		c2.Runner = runnerFunc(func(ctx context.Context, c Cmd, onLine func(string)) (int, error) { return int(code), nil })
		st := r.runUpdate(c2, setup, api.SetupGit)
		if cause == "" && st.Failure != nil || cause != "" && (st.Failure == nil || st.Failure.Cause != cause) {
			t.Errorf("%#x: %+v", code, st.Failure)
		}
	}
	// winget missing from the state: the run fails with its own cause
	c3 := cfg
	c3.WingetPath = ""
	st = r.runUpdate(c3, api.SetupState{Platform: "windows", Tools: []api.SetupTool{rowOf(api.SetupGit, api.ToolOK, "")}}, api.SetupGit)
	if st.Failure == nil || st.Failure.Cause != api.CauseWingetMissing {
		t.Fatalf("%+v", st.Failure)
	}
}

// FU-12c: a child that handles the polite stop and exits with 130 after the context ended is cancelled, not failed.
func TestAChildThatExitsAfterAStopIsCancelledNotFailed(t *testing.T) {
	r := newRig(t)
	r.putBrew(r.brewBody(0, "", ""))
	cfg := r.cfg()
	started := make(chan struct{})
	cfg.Runner = runnerFunc(func(ctx context.Context, c Cmd, onLine func(string)) (int, error) {
		close(started)
		<-ctx.Done()
		return 130, nil // exited on SIGINT with its own code, not killed
	})
	in := New(cfg)
	if err := in.StartUpdate(context.Background(), brewReady(), api.SetupGit); err != nil {
		t.Fatal(err)
	}
	<-started
	in.Cancel()
	in.Wait()
	st := r.finalCheck(in)
	if !st.Cancelled || st.Failure != nil {
		t.Fatalf("cancelled=%v failure=%+v", st.Cancelled, st.Failure)
	}
	// without a stop, 130 is an ordinary failure
	cfg.Runner = runnerFunc(func(ctx context.Context, c Cmd, onLine func(string)) (int, error) { return 130, nil })
	st = r.runUpdate(cfg, brewReady(), api.SetupGit)
	if st.Failure == nil || st.Cancelled {
		t.Fatalf("%+v", st)
	}
}
