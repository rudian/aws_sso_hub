package app

import (
	"runtime"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const wantMenuEn = `Codepass
  About Codepass <about>
  Check for updates... (check-updates)
  ---
  Settings... [CmdOrCtrl+,] (settings)
  ---
  Hide Codepass [Cmd+H] <hide>
  Hide others [Cmd+Option+H] <hideothers>
  ---
  Quit Codepass [Cmd+Q] <quit>
File
  New AI task [CmdOrCtrl+T] (new-task)
  Quick open [CmdOrCtrl+P] (quick-open)
  Save [CmdOrCtrl+S] (save)
  ---
  Close tab [CmdOrCtrl+W] (close-tab)
  Close window [CmdOrCtrl+Shift+W] (close-window)
Edit
  Undo [Cmd+Z] <undo>
  Redo [Cmd+Shift+Z] <redo>
  ---
  Cut [Cmd+X] <cut>
  Copy [Cmd+C] <copy>
  Paste [Cmd+V] <paste>
  Select all [Cmd+A] <selectall>
  ---
  Find [CmdOrCtrl+F] (find)
  Find in files [CmdOrCtrl+Shift+F] (find-in-files)
View
  Projects [CmdOrCtrl+1] (nav-projects)
  AI tasks [CmdOrCtrl+2] (nav-tasks)
  Pull requests [CmdOrCtrl+3] (nav-pulls)
  Accounts [CmdOrCtrl+4] (nav-accounts)
  ---
  Switch session [CmdOrCtrl+J] (switch-session)
  Toggle terminal [Ctrl+BACKTICK] (toggle-terminal)
  ---
  Actual size [Cmd+0] <resetzoom>
  Zoom in [Cmd++] <zoomin>
  Zoom out [Cmd+-] <zoomout>
  ---
  Enter full screen [Cmd+Ctrl+F] <fullscreen>
Window
  Minimize [Cmd+M] <minimise>
  Zoom <zoom>
  ---
  Bring all to front <front>
Help
  Tutorial (tutorial)
  Illustrated guide (guide)
  Report a problem (report-problem)
`

// realRoleAccel builds the real Wails menu (no Run needed) and returns the accelerator of each role item. A
// role item made with SetRole alone has none: Cmd+C/V/Z and Cmd+Q then do nothing in a WKWebView app.
func realRoleAccel(t *testing.T, lang string) func(string) string {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("the application menu exists on macOS only")
	}
	a := application.New(application.Options{Name: "menutest"})
	menu := buildMenu(a, menuModel(lang), func(string) {})
	return func(role string) string {
		r, ok := roleOf(role)
		if !ok {
			t.Fatalf("unknown role %s", role)
		}
		mi := menu.FindByRole(r)
		if mi == nil {
			t.Fatalf("role %s missing from the built menu", role)
		}
		return mi.GetAccelerator()
	}
}

func TestMenuSnapshotEnglish(t *testing.T) {
	want := strings.Replace(wantMenuEn, "BACKTICK", "`", 1)
	if got := renderMenu(menuModel("en"), realRoleAccel(t, "en")); got != want {
		t.Fatalf("menu changed:\n%s", got)
	}
}

func TestMenuRulesOfDesign44(t *testing.T) {
	for _, lang := range []string{"en", "zh-CN"} {
		m := menuModel(lang)
		text := renderMenu(m, nil)
		for _, banned := range []string{"<reload>", "<forcereload>", "<opendevtools>", "<closewindow>", "<help>"} {
			if strings.Contains(text, banned) {
				t.Errorf("%s: %s must not be in the menu", lang, banned)
			}
		}
		// Cmd+R is not in the menu at all: the frontend routes it; Cmd+W goes to the frontend, not to the window.
		if strings.Contains(text, "CmdOrCtrl+R") {
			t.Errorf("%s: Cmd+R must not be a menu accelerator", lang)
		}
		for _, s := range m {
			for _, it := range s.Items {
				if it.Accel == "CmdOrCtrl+W" && it.ID != CmdCloseTab {
					t.Errorf("%s: Cmd+W must be Close tab, got %q", lang, it.ID)
				}
				if it.Role != "" {
					if _, ok := roleOf(it.Role); !ok {
						t.Errorf("unknown role %q", it.Role)
					}
				}
				if it.Label == "" && !it.Sep {
					t.Errorf("%s: empty label", lang)
				}
			}
		}
	}
	// same structure in both languages
	strip := func(s string) int { return strings.Count(s, "\n") }
	if strip(renderMenu(menuModel("en"), nil)) != strip(renderMenu(menuModel("zh-CN"), nil)) {
		t.Error("zh and en menus differ in shape")
	}
}

func TestRoleItemsHaveKeyboardShortcuts(t *testing.T) {
	acc := realRoleAccel(t, "zh-CN")
	for role, want := range map[string]string{
		"undo": "Cmd+Z", "redo": "Cmd+Shift+Z", "cut": "Cmd+X", "copy": "Cmd+C",
		"paste": "Cmd+V", "selectall": "Cmd+A", "quit": "Cmd+Q", "hide": "Cmd+H",
		"hideothers": "Cmd+Option+H", "minimise": "Cmd+M",
		"resetzoom": "Cmd+0", "zoomin": "Cmd++", "zoomout": "Cmd+-",
	} {
		if got := acc(role); got != want {
			t.Errorf("%s: accelerator %q, want %q", role, got, want)
		}
	}
}

func TestBackendCommandsAreKnown(t *testing.T) {
	for _, id := range []string{CmdCloseWindow, CmdGuide, CmdReport} {
		if !isBackendCommand(id) {
			t.Errorf("%s", id)
		}
	}
	if isBackendCommand(CmdCloseTab) || isBackendCommand(CmdSave) {
		t.Error("tab and save commands belong to the frontend")
	}
}
