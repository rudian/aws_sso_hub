//go:build darwin

package update

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// toolStub replaces codesign and spctl (the signature logic is covered end to end by integration/run.sh with real
// signed bundles) and records every call. plutil runs for real on a real Info.plist.
type toolStub struct {
	mu         sync.Mutex
	calls      [][]string
	codesignOK []bool // result per codesign call; the last one repeats
	spctlOK    bool
}

func (s *toolStub) install(t *testing.T) {
	t.Helper()
	orig := runTool
	t.Cleanup(func() { runTool = orig })
	runTool = func(d time.Duration, name string, args ...string) ([]byte, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.calls = append(s.calls, append([]string{name}, args...))
		switch name {
		case codesignPath:
			n := 0
			for _, c := range s.calls {
				if c[0] == codesignPath {
					n++
				}
			}
			ok := true
			if len(s.codesignOK) > 0 {
				ok = s.codesignOK[min(n, len(s.codesignOK))-1]
			}
			if !ok {
				return []byte("code failed to satisfy specified code requirement(s)"), errFake
			}
			return nil, nil
		case spctlPath:
			if !s.spctlOK {
				return []byte("rejected"), errFake
			}
			return nil, nil
		}
		return orig(d, name, args...)
	}
}

func (s *toolStub) names() []string {
	var out []string
	for _, c := range s.calls {
		out = append(out, filepath.Base(c[0]))
	}
	return out
}

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

const errFake = fakeErr("exit status 3")

func plist(id, ver string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>CFBundleIdentifier</key><string>` + id + `</string><key>CFBundleShortVersionString</key><string>` + ver + `</string></dict></plist>`
}

func appZip(t *testing.T, id, ver string, extra map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"Codepass.app/Contents/Info.plist":     plist(id, ver),
		"Codepass.app/Contents/MacOS/codepass": "binary",
	}
	for k, v := range extra {
		files[k] = v
	}
	for name, body := range files {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(body))
	}
	_ = zw.Close()
	return buf.Bytes()
}

type ctlEnv struct {
	f     *fakeAPI
	k     testKeys
	c     *Controller
	hi    Highest
	stub  *toolStub
	dir   string // where Wails stages: the folder of the running test binary
	state []State
	quit  int
}

func newCtlEnv(t *testing.T, running string) *ctlEnv {
	t.Helper()
	e := &ctlEnv{f: newFakeAPI(t), k: newTestKeys(t), stub: &toolStub{spctlOK: true}}
	e.stub.install(t)
	e.hi = Highest{Path: filepath.Join(t.TempDir(), "highest")}
	exe, err := realExecutable()
	if err != nil {
		t.Fatal(err)
	}
	e.dir = filepath.Dir(exe)
	e.c = NewController(ControllerConfig{
		Quit:         func() { e.quit++ },
		Running:      running,
		Gate:         Gate{BundleID: BundleID, Requirement: DesignatedRequirement(BundleID, TeamID), RequireGatekeeper: true},
		Highest:      e.hi,
		HelperLog:    filepath.Join(t.TempDir(), "helperlog"),
		StagedRecord: filepath.Join(t.TempDir(), "staged"),
		Channel:      func() (Channel, error) { return e.f.channel(e.k, ""), nil },
		OnState:      func(s State) { e.state = append(e.state, s) },
	})
	t.Cleanup(func() { // never leave a staging folder in the shared go-build folder
		ents, _ := os.ReadDir(e.dir)
		for _, en := range ents {
			if strings.HasPrefix(en.Name(), "wails-update-") {
				_ = os.RemoveAll(filepath.Join(e.dir, en.Name()))
			}
		}
	})
	return e
}

func (e *ctlEnv) strays() []string {
	var out []string
	ents, _ := os.ReadDir(e.dir)
	for _, en := range ents {
		if strings.HasPrefix(en.Name(), "wails-update-") {
			out = append(out, en.Name())
		}
	}
	return out
}

func (e *ctlEnv) publish(archive []byte, mver string) {
	e.f.publish(relOpts{tag: "v" + mver, manifestVersion: mver, signWith: e.k.cur, archive: archive, notes: "Signed notes."})
}

func TestControllerGoodUpdateReachesReady(t *testing.T) {
	e := newCtlEnv(t, "1.0.0-rc.1")
	e.publish(appZip(t, BundleID, "1.0.0-rc.2", nil), "1.0.0-rc.2")
	st, err := e.c.Check(context.Background())
	if err != nil || st.Phase != PhaseAvailable || st.Version != "1.0.0-rc.2" || st.Notes != "Signed notes." {
		t.Fatalf("check: %+v %v", st, err)
	}
	if len(e.strays()) != 0 {
		t.Fatal("a check must not download anything")
	}
	if err := e.c.Download(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.c.State().Phase != PhaseReady {
		t.Fatalf("state %+v", e.c.State())
	}
	var phases []Phase
	for _, s := range e.state {
		if len(phases) == 0 || phases[len(phases)-1] != s.Phase {
			phases = append(phases, s.Phase)
		}
	}
	want := []Phase{PhaseChecking, PhaseAvailable, PhaseDownloading, PhaseVerifying, PhaseReady}
	if strings.Join(phasesToStrings(phases), ",") != strings.Join(phasesToStrings(want), ",") {
		t.Errorf("phases %v, want %v", phases, want)
	}
	// the gate: plutil x2, then codesign with the full requirement, then spctl, in that order, absolute paths
	names := e.stub.names()
	cs := -1
	for i, n := range names {
		if n == "codesign" {
			cs = i
		}
	}
	if cs < 0 || names[len(names)-1] != "spctl" {
		t.Fatalf("calls %v", names)
	}
	argv := e.stub.calls[cs]
	if argv[0] != "/usr/bin/codesign" || argv[len(argv)-2] != "-R="+DesignatedRequirement(BundleID, TeamID) {
		t.Errorf("codesign argv %v", argv)
	}
	if e.quit != 0 {
		t.Error("the host was asked to quit before Restart")
	}
	// a newer check discards the staged copy
	if _, err := e.c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.strays()) != 0 {
		t.Errorf("staging left behind by a new check: %v", e.strays())
	}
}

func phasesToStrings(p []Phase) []string {
	var s []string
	for _, x := range p {
		s = append(s, string(x))
	}
	return s
}

func TestControllerRefusalsRemoveStaging(t *testing.T) {
	cases := []struct {
		name   string
		zip    func(t *testing.T) []byte
		cs     []bool
		spctl  bool
		want   string
		code   string
		noCS   bool // codesign must not even run
		noSPCT bool // spctl must not run
	}{
		{"wrong bundle id", func(t *testing.T) []byte { return appZip(t, "com.example.other", "1.0.0-rc.2", nil) }, nil, true, "bundle identifier", CodeBundleID, true, true},
		{"bundle version below the manifest's", func(t *testing.T) []byte { return appZip(t, BundleID, "1.0.0-rc.1", nil) }, nil, true, "does not equal manifest version", CodeBundleVersion, true, true},
		{"AppleDouble entry", func(t *testing.T) []byte {
			return appZip(t, BundleID, "1.0.0-rc.2", map[string]string{"Codepass.app/Contents/MacOS/._codepass": "x"})
		}, nil, true, "AppleDouble", CodeAppleDouble, true, true},
		{"ad-hoc / development signature, or a changed sealed resource (the latter is run.sh R7)", func(t *testing.T) []byte { return appZip(t, BundleID, "1.0.0-rc.2", nil) }, []bool{false}, true, "codesign requirement failed", CodeCodesign, false, true},
		{"Gatekeeper refuses", func(t *testing.T) []byte { return appZip(t, BundleID, "1.0.0-rc.2", nil) }, []bool{true}, false, "gatekeeper assessment failed", CodeGatekeeper, false, false},
	}
	for _, c := range cases {
		e := newCtlEnv(t, "1.0.0-rc.1")
		e.stub.codesignOK, e.stub.spctlOK = c.cs, c.spctl
		e.publish(c.zip(t), "1.0.0-rc.2")
		if _, err := e.c.Check(context.Background()); err != nil {
			t.Fatalf("%s: check %v", c.name, err)
		}
		err := e.c.Download(context.Background())
		if err == nil || !IsRefusal(err) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v", c.name, err)
		}
		if s := e.c.State(); s.Code != c.code || s.Step != StepVerify {
			t.Errorf("%s: code %q step %q, want %q / verify", c.name, s.Code, s.Step, c.code)
		}
		if s := e.c.State(); s.Phase != PhaseRefused || !strings.Contains(s.Error, c.want) {
			t.Errorf("%s: state %+v", c.name, s)
		}
		if len(e.strays()) != 0 {
			t.Errorf("%s: staging left behind: %v", c.name, e.strays())
		}
		names := e.stub.names()
		has := func(n string) bool {
			for _, x := range names {
				if x == n {
					return true
				}
			}
			return false
		}
		if c.noCS && has("codesign") {
			t.Errorf("%s: codesign ran before the cheap checks", c.name)
		}
		if c.noSPCT && has("spctl") {
			t.Errorf("%s: spctl ran after codesign refused: it must never decide alone", c.name)
		}
		if err := e.c.Restart(context.Background()); err == nil {
			t.Errorf("%s: Restart accepted after a refusal", c.name)
		}
		if e.quit != 0 {
			t.Errorf("%s: host quit", c.name)
		}
	}
}

func TestControllerGateRunsAgainInsideRestart(t *testing.T) {
	e := newCtlEnv(t, "1.0.0-rc.1")
	e.publish(appZip(t, BundleID, "1.0.0-rc.2", nil), "1.0.0-rc.2")
	e.stub.codesignOK = []bool{true, false} // passes at staging, fails at Restart (the staged copy was swapped)
	if _, err := e.c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.c.Download(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := e.c.Restart(context.Background())
	if err == nil || !strings.Contains(err.Error(), "refused at restart") || !IsRefusal(err) {
		t.Fatalf("restart: %v", err)
	}
	if len(e.strays()) != 0 || e.quit != 0 {
		t.Errorf("strays %v quit %d", e.strays(), e.quit)
	}
}

func TestControllerDigestMismatchIsRefused(t *testing.T) {
	e := newCtlEnv(t, "1.0.0-rc.1")
	good := appZip(t, BundleID, "1.0.0-rc.2", nil)
	e.f.publish(relOpts{tag: "v1.0.0-rc.2", manifestVersion: "1.0.0-rc.2", signWith: e.k.cur, archive: good,
		mutate: func(m *Manifest) { m.SHA256 = strings.Repeat("ab", 32) }})
	if _, err := e.c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := e.c.Download(context.Background())
	if err == nil || e.c.State().Phase != PhaseRefused || len(e.strays()) != 0 {
		t.Fatalf("%v %+v %v", err, e.c.State(), e.strays())
	}
	if st := e.c.State(); st.Code != CodeDigest || st.Step != StepDownload {
		t.Errorf("code %q step %q", st.Code, st.Step)
	}
}

func TestControllerCheckStates(t *testing.T) {
	// refusal: re-published old archive under a new tag
	e := newCtlEnv(t, "1.0.0-rc.2")
	e.f.publish(relOpts{tag: "v1.0.0-rc.3", manifestVersion: "1.0.0-rc.1", signWith: e.k.cur})
	if _, err := e.c.Check(context.Background()); err == nil || e.c.State().Phase != PhaseRefused {
		t.Fatalf("old archive: %v %+v", err, e.c.State())
	}
	// the next check starts from a fresh updater: a later good release is found
	e.publish(appZip(t, BundleID, "1.0.0-rc.4", nil), "1.0.0-rc.4")
	if st, err := e.c.Check(context.Background()); err != nil || st.Phase != PhaseAvailable || st.Version != "1.0.0-rc.4" {
		t.Fatalf("after a refusal the next check must still work: %+v %v", st, err)
	}
	// API 500 is an error, not a refusal
	e2 := newCtlEnv(t, "1.0.0-rc.1")
	e2.publish(appZip(t, BundleID, "1.0.0-rc.2", nil), "1.0.0-rc.2")
	e2.f.down = true
	if _, err := e2.c.Check(context.Background()); err == nil || e2.c.State().Phase != PhaseError || !strings.Contains(e2.c.State().Error, "500") {
		t.Fatalf("API 500: %v %+v", err, e2.c.State())
	}
	// up to date
	e3 := newCtlEnv(t, "1.0.0-rc.2")
	e3.publish(appZip(t, BundleID, "1.0.0-rc.2", nil), "1.0.0-rc.2")
	if _, err := e3.c.Check(context.Background()); err != ErrUpToDate || e3.c.State().Phase != PhaseUpToDate {
		t.Fatalf("up to date: %v %+v", err, e3.c.State())
	}
	// an unreadable stored highest version blocks the update with a visible message
	e4 := newCtlEnv(t, "1.0.0-rc.1")
	e4.publish(appZip(t, BundleID, "1.0.0-rc.2", nil), "1.0.0-rc.2")
	_ = os.WriteFile(e4.hi.Path, []byte("junk"), 0o600)
	if _, err := e4.c.Check(context.Background()); err == nil || e4.c.State().Phase != PhaseRefused || !strings.Contains(e4.c.State().Error, "cannot be read") {
		t.Fatalf("unreadable highest: %v %+v", err, e4.c.State())
	}
}

func TestControllerBlockedWhenTranslocated(t *testing.T) {
	e := newCtlEnv(t, "1.0.0-rc.1")
	e.c.cfg.Executable = func() (string, error) {
		return "/private/var/folders/xx/T/AppTranslocation/ID/d/Codepass.app/Contents/MacOS/codepass", nil
	}
	e.publish(appZip(t, BundleID, "1.0.0-rc.2", nil), "1.0.0-rc.2")
	_, err := e.c.Check(context.Background())
	s := e.c.State()
	if err == nil || s.Phase != PhaseBlocked || s.Block != BlockTranslocated || s.Folder == "" {
		t.Fatalf("%v %+v", err, s)
	}
	if len(e.f.auth) != 0 {
		t.Error("the network was used before the preflight said no")
	}
}

func TestControllerRunStartsFreshChecks(t *testing.T) {
	e := newCtlEnv(t, "1.0.0-rc.1")
	e.publish(appZip(t, BundleID, "1.0.0-rc.2", nil), "1.0.0-rc.2")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.c.Run(ctx, time.Millisecond, 20*time.Millisecond); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for e.c.State().Phase != PhaseAvailable && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if e.c.State().Phase != PhaseAvailable {
		t.Fatalf("%+v", e.c.State())
	}
}

func (e *ctlEnv) stage(t *testing.T) {
	t.Helper()
	e.publish(appZip(t, BundleID, "1.0.0-rc.2", nil), "1.0.0-rc.2")
	if _, err := e.c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.c.Download(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.strays()) != 1 {
		t.Fatalf("expected one staging folder, got %v", e.strays())
	}
}

func TestControllerCloseRemovesAnUnusedStagedUpdate(t *testing.T) {
	e := newCtlEnv(t, "1.0.0-rc.1")
	e.stage(t)
	e.c.Close()
	if len(e.strays()) != 0 {
		t.Errorf("staging left behind by Close: %v", e.strays())
	}
	if _, err := os.Stat(e.c.cfg.StagedRecord); err == nil {
		t.Error("the staged record was not cleared")
	}
}

func TestRunRemovesTheStagingFolderAnEarlierRunAbandoned(t *testing.T) {
	e := newCtlEnv(t, "1.0.0-rc.1")
	e.stage(t)
	// the app quit without Restart and without Close: a new controller (a new process) starts
	c2 := NewController(e.c.cfg)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c2.Run(ctx, time.Hour, time.Hour)
	if len(e.strays()) != 0 {
		t.Errorf("abandoned staging not removed: %v", e.strays())
	}
	if _, err := os.Stat(e.c.cfg.StagedRecord); err == nil {
		t.Error("the staged record was not cleared")
	}
}

func TestRunLeavesARecordedFolderThatIsNotBesideTheApp(t *testing.T) {
	e := newCtlEnv(t, "1.0.0-rc.1")
	other := filepath.Join(t.TempDir(), "wails-update-1")
	if err := os.MkdirAll(filepath.Join(other, "Codepass.app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeStagedRecord(e.c.cfg.StagedRecord, filepath.Join(other, "Codepass.app")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e.c.Run(ctx, time.Hour, time.Hour)
	if _, err := os.Stat(other); err != nil {
		t.Errorf("a folder outside the install dir was removed: %v", err)
	}
}

func TestHandedOffStagingSurvivesCheckAndDownload(t *testing.T) {
	t.Setenv("CODEPASS_TEST_HELPER_WAITS", "1")
	e := newCtlEnv(t, "1.0.0-rc.1")
	e.stage(t)
	if err := e.c.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.quit != 1 || e.c.State().Phase != PhaseRestarting {
		t.Fatalf("quit %d state %+v", e.quit, e.c.State())
	}
	if _, err := e.c.Check(context.Background()); !errors.Is(err, errRestartPending) {
		t.Errorf("Check: %v", err)
	}
	if err := e.c.Download(context.Background()); !errors.Is(err, errRestartPending) {
		t.Errorf("Download: %v", err)
	}
	if err := e.c.Restart(context.Background()); !errors.Is(err, errRestartPending) {
		t.Errorf("second Restart: %v", err)
	}
	e.c.Close()
	if e.c.State().Phase != PhaseRestarting {
		t.Errorf("state changed: %+v", e.c.State())
	}
	if len(e.strays()) != 1 {
		t.Errorf("the handed-off staging folder was removed: %v", e.strays())
	}
	if _, err := os.Stat(e.c.cfg.StagedRecord); err == nil {
		t.Error("the staged record must be cleared at hand-off")
	}
}

func TestRefusedReleaseIsNotOfferedAgain(t *testing.T) {
	e := newCtlEnv(t, "1.0.0-rc.1")
	e.stub.codesignOK = []bool{false}
	e.publish(appZip(t, BundleID, "1.0.0-rc.2", nil), "1.0.0-rc.2")
	if _, err := e.c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.c.Download(context.Background()); err == nil {
		t.Fatal("expected a refusal")
	}
	want := e.c.State()
	if want.Phase != PhaseRefused {
		t.Fatalf("%+v", want)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.c.Run(ctx, time.Millisecond, time.Hour); close(done) }()
	time.Sleep(300 * time.Millisecond) // one tick
	cancel()
	<-done
	if got := e.c.State(); got.Phase != PhaseRefused || got.Code != want.Code || got.Step != want.Step {
		t.Fatalf("after a tick: %+v, want %+v", got, want)
	}
	if err := e.c.Download(context.Background()); err == nil {
		t.Error("Download accepted a refused release")
	}
	// the user's Retry offers it again; a different release also clears the memory
	if st, err := e.c.Retry(context.Background()); err != nil || st.Phase != PhaseAvailable {
		t.Fatalf("retry: %+v %v", st, err)
	}
	e.stub.codesignOK = []bool{true}
	e.publish(appZip(t, BundleID, "1.0.0-rc.3", nil), "1.0.0-rc.3")
	if st, err := e.c.Check(context.Background()); err != nil || st.Phase != PhaseAvailable || st.Version != "1.0.0-rc.3" {
		t.Fatalf("newer release: %+v %v", st, err)
	}
}

// Wails' Restart quits the host before it returns, and the shutdown hook calls Close from inside Quit: Close must
// not delete the staging folder the helper is waiting for.
func TestCloseInsideQuitKeepsTheStagedUpdate(t *testing.T) {
	t.Setenv("CODEPASS_TEST_HELPER_WAITS", "1")
	e := newCtlEnv(t, "1.0.0-rc.1")
	during := -1
	e.c.cfg.Quit = func() {
		e.quit++
		e.c.Close()
		during = len(e.strays())
	}
	e.publish(appZip(t, BundleID, "1.0.0-rc.2", nil), "1.0.0-rc.2")
	if _, err := e.c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.c.Download(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.c.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.quit != 1 || during != 1 {
		t.Fatalf("quit=%d staging folders inside Close=%d, want 1 and 1", e.quit, during)
	}
	if e.c.State().Phase != PhaseRestarting {
		t.Errorf("phase %s", e.c.State().Phase)
	}
	if _, err := os.Stat(e.c.cfg.StagedRecord); err == nil {
		t.Error("the staged record must be gone once the helper owns the folder")
	}
}

// stallDownloads makes the fake CDN send half of the archive and then stop, with the connection kept open.
func (e *ctlEnv) stallDownloads(t *testing.T, archive []byte) {
	orig := e.f.cdn.Config.Handler
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	e.f.cdn.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ".zip") {
			orig.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(archive)))
		_, _ = w.Write(archive[:len(archive)/2])
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
}

func (e *ctlEnv) waitStaging(t *testing.T) {
	deadline := time.Now().Add(5 * time.Second)
	for len(e.strays()) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if len(e.strays()) == 0 {
		t.Fatal("the download never started")
	}
}

// Quitting during a download is a normal path: Close cancels it and Wails removes its own staging folder.
func TestCloseDuringDownloadRemovesTheStagingFolder(t *testing.T) {
	e := newCtlEnv(t, "1.0.0-rc.1")
	archive := appZip(t, BundleID, "1.0.0-rc.2", map[string]string{"Codepass.app/Contents/Resources/big": strings.Repeat("x", 1<<20)})
	e.publish(archive, "1.0.0-rc.2")
	e.stallDownloads(t, archive)
	if _, err := e.c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- e.c.Download(context.Background()) }()
	e.waitStaging(t)
	e.c.Close()
	if n := e.strays(); len(n) != 0 {
		t.Errorf("staging folders left after Close: %v", n)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Error("a cancelled download reported success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Download did not return after Close")
	}
}

// A stalled download ends with a network error the UI can retry, instead of staying in "downloading" for ever.
func TestStalledDownloadEndsWithANetworkError(t *testing.T) {
	e := newCtlEnv(t, "1.0.0-rc.1")
	e.c.cfg.IdleTimeout = 300 * time.Millisecond
	archive := appZip(t, BundleID, "1.0.0-rc.2", map[string]string{"Codepass.app/Contents/Resources/big": strings.Repeat("x", 1<<20)})
	e.publish(archive, "1.0.0-rc.2")
	e.stallDownloads(t, archive)
	if _, err := e.c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.c.Download(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
	st := e.c.State()
	if st.Phase != PhaseError || st.Code != CodeNetwork || st.Step != StepDownload {
		t.Errorf("state %+v", st)
	}
	if n := e.strays(); len(n) != 0 {
		t.Errorf("staging folders left: %v", n)
	}
	if _, err := e.c.Check(context.Background()); errors.Is(err, errBusy) {
		t.Error("the controller stayed busy")
	}
}
