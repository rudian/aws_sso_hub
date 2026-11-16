package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/paths"
	"github.com/rudian/codepass/internal/store"
)

func startupPaths(t *testing.T) paths.Paths {
	t.Helper()
	root := t.TempDir()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	p := paths.ForRoot(root)
	if err := p.EnsureAppDirs(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestOpenContainerNewerProjectsFileQuitsWithoutWriting(t *testing.T) {
	p := startupPaths(t)
	f := projectsFile(p)
	body := []byte(`{"schemaVersion": 999}`)
	if err := os.WriteFile(f, body, 0o600); err != nil {
		t.Fatal(err)
	}
	var told string
	_, err := OpenContainer(p, &fakeRuntime{}, ContainerOptions{DisableLoginShell: true}, func(m string) { told = m })
	if !errors.Is(err, ErrStartupStopped) {
		t.Fatalf("err = %v, want ErrStartupStopped", err)
	}
	if !strings.Contains(told, "newer Codepass") || !strings.Contains(told, "更新版本") {
		t.Fatalf("message = %q", told)
	}
	got, _ := os.ReadFile(f)
	if string(got) != string(body) {
		t.Fatalf("projects.json was changed: %q", got)
	}
}

func TestOpenContainerCorruptProjectsFileIsQuarantined(t *testing.T) {
	p := startupPaths(t)
	f := projectsFile(p)
	if err := os.WriteFile(f, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	var told string
	c, err := OpenContainer(p, &fakeRuntime{}, ContainerOptions{DisableLoginShell: true}, func(m string) { told = m })
	if err != nil {
		t.Fatal(err)
	}
	defer c.Shutdown()
	if c.Projects == nil {
		t.Fatal("Projects is not exposed")
	}
	matches, _ := filepath.Glob(f + ".corrupt-*")
	if len(matches) != 1 {
		t.Fatalf("quarantined files = %v", matches)
	}
	if !strings.Contains(told, matches[0]) || !strings.Contains(told, "已损坏") || !strings.Contains(told, "damaged") {
		t.Fatalf("the person was not told where the old file went: %q", told)
	}
}

func TestNewerMessageHasNoSchemaNumbers(t *testing.T) {
	m := newerVersionMessage(&store.NewerVersionError{FileVersion: 2, AppVersion: 1})
	if strings.Contains(m, "2 > 1") || strings.Contains(m, "(2") {
		t.Fatalf("schema numbers leaked into the dialog: %q", m)
	}
}

func TestStoreFileErrorUnwraps(t *testing.T) {
	e := &storeFileError{File: "x", Err: store.ErrCorrupt}
	if !errors.Is(e, store.ErrCorrupt) {
		t.Fatal("storeFileError must unwrap")
	}
}
