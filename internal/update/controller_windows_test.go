//go:build windows

package update

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests go through the controller on Windows (FU-20b); they run on the Windows VM (manual check B16, and
// B9 for a real release). Wails stages next to the running test binary, as it does next to the app.

// exeZip is a release archive that holds only a codepass.exe whose fixed file version is ver.
func exeZip(t *testing.T, ver [4]uint16) []byte {
	t.Helper()
	p := filepath.Join(t.TempDir(), "codepass.exe")
	buildExe(t, p, ver, nil)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("codepass.exe")
	_, _ = w.Write(b)
	_ = zw.Close()
	return buf.Bytes()
}

type winCtlEnv struct {
	f     *fakeAPI
	k     testKeys
	c     *Controller
	dir   string
	state []State
}

func newWinCtlEnv(t *testing.T, running string) *winCtlEnv {
	t.Helper()
	e := &winCtlEnv{f: newFakeAPI(t), k: newTestKeys(t)}
	exe, err := realExecutable()
	if err != nil {
		t.Fatal(err)
	}
	e.dir = filepath.Dir(exe)
	e.c = NewController(ControllerConfig{
		Quit:         func() {},
		Running:      running,
		Gate:         ProductionGate(),
		Highest:      Highest{Path: filepath.Join(t.TempDir(), "highest")},
		HelperLog:    filepath.Join(t.TempDir(), "helperlog"),
		StagedRecord: filepath.Join(t.TempDir(), "staged"),
		Channel:      func() (Channel, error) { return e.f.channel(e.k, ""), nil },
		OnState:      func(s State) { e.state = append(e.state, s) },
	})
	t.Cleanup(func() { // never leave a staging folder next to the test binary
		for _, s := range e.strays() {
			_ = os.RemoveAll(filepath.Join(e.dir, s))
		}
	})
	return e
}

func (e *winCtlEnv) strays() []string {
	var out []string
	ents, _ := os.ReadDir(e.dir)
	for _, en := range ents {
		if strings.HasPrefix(en.Name(), "wails-update-") {
			out = append(out, en.Name())
		}
	}
	return out
}

func (e *winCtlEnv) publish(archive []byte, mver string) {
	e.f.publish(relOpts{tag: "v" + mver, manifestVersion: mver, signWith: e.k.cur, archive: archive, notes: "Signed notes.",
		os: "windows", arch: "amd64"})
}

// A file version that is not the manifest's is refused at the verify step and leaves no staging folder; the
// matching one reaches Ready.
func TestControllerWindowsRefusalRemovesStagingAndMatchReachesReady(t *testing.T) {
	e := newWinCtlEnv(t, "1.0.0-rc.1")
	e.c.cfg.OS, e.c.cfg.Arch = "windows", "amd64"
	e.publish(exeZip(t, [4]uint16{1, 0, 0, 1}), "1.0.0-rc.2") // rc.1's exe under rc.2's manifest
	if _, err := e.c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := e.c.Download(context.Background())
	if err == nil || !IsRefusal(err) {
		t.Fatalf("expected a refusal, got %v", err)
	}
	if s := e.c.State(); s.Phase != PhaseRefused || s.Code != CodeBundleVersion || s.Step != StepVerify {
		t.Fatalf("state %+v", s)
	}
	if len(e.strays()) != 0 {
		t.Errorf("staging left behind: %v", e.strays())
	}

	e2 := newWinCtlEnv(t, "1.0.0-rc.1")
	e2.c.cfg.OS, e2.c.cfg.Arch = "windows", "amd64"
	e2.publish(exeZip(t, [4]uint16{1, 0, 0, 2}), "1.0.0-rc.2")
	if _, err := e2.c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e2.c.Download(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := e2.c.State(); s.Phase != PhaseReady {
		t.Fatalf("state %+v", s)
	}
	e2.c.Close()
	if len(e2.strays()) != 0 {
		t.Errorf("Close left %v", e2.strays())
	}
}
