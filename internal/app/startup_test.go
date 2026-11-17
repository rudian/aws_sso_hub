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
	if !strings.Contains(told, matches[0]) || !strings.Contains(told, "坏了") || !strings.Contains(told, "damaged") {
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

func TestNewerMessageNamesTheReleasesPageInBothLanguages(t *testing.T) {
	m := newerVersionMessage(nil)
	if releasesAddress() != "github.com/rudian/codepass/releases" {
		t.Fatalf("releasesAddress = %q", releasesAddress())
	}
	zh, en, ok := strings.Cut(m, "\n\n")
	if !ok {
		t.Fatalf("both languages are expected, zh first: %q", m)
	}
	for _, c := range []struct{ lang, text, want string }{
		{"zh", zh, "这个版本不会打开它"},
		{"zh", zh, "github.com/rudian/codepass/releases"},
		{"en", en, "This copy will not open it"},
		{"en", en, "github.com/rudian/codepass/releases"},
	} {
		if !strings.Contains(c.text, c.want) {
			t.Errorf("%s message lacks %q: %q", c.lang, c.want, c.text)
		}
	}
	for _, bad := range []string{"https://", "/latest", "这个版本没有打开"} {
		if strings.Contains(m, bad) {
			t.Errorf("message still contains %q: %q", bad, m)
		}
	}
}

func TestDamagedMessageSaysTheRepositoriesAreUntouched(t *testing.T) {
	m := damagedMessage("/tmp/projects.json.corrupt-1")
	zh, en, ok := strings.Cut(m, "\n\n")
	if !ok {
		t.Fatalf("both languages are expected, zh first: %q", m)
	}
	if want := "项目分组的数据坏了，已另存为 /tmp/projects.json.corrupt-1。仓库本身没有动，分组要重新建。"; zh != want {
		t.Errorf("zh = %q, want %q", zh, want)
	}
	if want := "Your project list was damaged and was kept as /tmp/projects.json.corrupt-1. Your repositories are untouched; the projects start empty."; en != want {
		t.Errorf("en = %q, want %q", en, want)
	}
	if strings.Contains(m, "从空白开始") {
		t.Errorf("old wording is still there: %q", m)
	}
}
