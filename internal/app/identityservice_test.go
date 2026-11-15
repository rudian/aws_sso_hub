package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/projects"
)

// The service is a thin layer; this checks the wiring against a real container in the sandbox layout:
// the global file comes from internal/paths, the backup lands in the app's backup store, the credential
// sections of the same file stay byte-identical, and nothing outside the temp folder is read.
func TestIdentityServiceGlobalWriteAndUndo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell probe")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	// the probe starts a shell: keep it away from any real profile
	zdot := t.TempDir()
	t.Setenv("ZDOTDIR", zdot)
	t.Setenv("SHELL", "/bin/zsh")
	for _, n := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "EMAIL"} {
		t.Setenv(n, "")
		os.Unsetenv(n)
	}
	w := newWorld(t)
	reg, err := projects.Open(filepath.Join(filepath.Dir(w.p.SettingsFile), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewIdentityService(w.c, reg)

	ov := ok(svc.GetOverview(bg))
	if ov.Global.Set || ov.Global.Path != w.p.GitGlobalFile || ov.Projects == nil || ov.Ungrouped == nil || ov.Env.Vars == nil {
		t.Fatalf("%+v", ov)
	}
	a := api.Author{Name: "Alex", Email: "alex@acme.example.com"}
	pv := ok(svc.PreviewGlobal(bg, a))
	if bad := fails(svc.PreviewGlobal(bg, api.Author{Name: "Alex", Email: "nope"})); bad.Code != api.ErrInvalidInput {
		t.Fatalf("%+v", bad)
	}
	before := w.read(w.p.GitGlobalFile)
	res := ok(svc.ApplyGlobal(bg, a, pv.Preview.BaseHash))
	after := w.read(w.p.GitGlobalFile)
	if !strings.Contains(after, "alex@acme.example.com") || !strings.Contains(after, "credential") {
		t.Fatalf("%s", after)
	}
	if w.rt.count(api.EventIdentityChanged) == 0 {
		t.Error("identity:changed must be emitted")
	}
	ents := ok(w.c.Backup.List())
	if len(ents) == 0 || ents[0].Reason != api.ReasonIdentity || ents[0].Kind != api.BackupGitGlobal {
		t.Fatalf("%+v", ents)
	}
	ov = ok(svc.GetOverview(bg))
	if !ov.Global.Set || ov.Global.Email != "alex@acme.example.com" {
		t.Fatalf("%+v", ov.Global)
	}
	ok(svc.Undo(bg, res.UndoID))
	if got := w.read(w.p.GitGlobalFile); got != before {
		t.Fatalf("undo:\n%s", got)
	}
}

// First-time flow of design 5.12: no global config, save, then Undo from the toast. The undo removes the
// file the app created; the watcher must not report that as an outside change.
func TestIdentityUndoOfCreatedGlobalIsNotAnExternalChange(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell probe")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("ZDOTDIR", t.TempDir())
	w := newWorld(t)
	reg, err := projects.Open(filepath.Join(filepath.Dir(w.p.SettingsFile), "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewIdentityService(w.c, reg)
	_ = os.Remove(w.p.GitGlobalFile) // the first-time flow starts without a global config
	w.c.files.prime()
	a := api.Author{Name: "Alex", Email: "alex@acme.example.com"}
	pv := ok(svc.PreviewGlobal(bg, a))
	res := ok(svc.ApplyGlobal(bg, a, pv.Preview.BaseHash))
	ok(svc.Undo(bg, res.UndoID))
	if _, err := os.Stat(w.p.GitGlobalFile); !os.IsNotExist(err) {
		t.Fatal("the created file must be gone")
	}
	if ch := w.c.files.poll(); len(ch) != 0 {
		t.Fatalf("own undo reported as external: %+v", ch)
	}
	// a forged ID is refused through the service too
	if bad := fails(svc.Undo(bg, "../../x")); bad.Code != api.ErrInvalidInput {
		t.Fatalf("%+v", bad)
	}
}
