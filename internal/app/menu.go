package app

import (
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/codepass/internal/version"
)

// EventAppCommand carries a menu command id (a string) to the frontend, which owns routing, editor tabs and
// the palette. It is registered here (not in events.go); frontend/src/api/events.ts needs the matching entry.
const EventAppCommand = "app:command"

func init() { application.RegisterEvent[string](EventAppCommand) }

// Menu command ids sent to the frontend. The backend handles the ones in backendCommands itself.
const (
	CmdCheckUpdates = "check-updates"
	CmdSettings     = "settings"
	CmdNewTask      = "new-task"
	CmdQuickOpen    = "quick-open"
	CmdSave         = "save"
	CmdCloseTab     = "close-tab"
	CmdFind         = "find"
	CmdFindInFiles  = "find-in-files"
	CmdProjects     = "nav-projects"
	CmdTasks        = "nav-tasks"
	CmdPulls        = "nav-pulls"
	CmdAccounts     = "nav-accounts"
	CmdSwitchTask   = "switch-session"
	CmdTerminal     = "toggle-terminal"
	CmdTutorial     = "tutorial"
	// backend commands
	CmdCloseWindow = "close-window"
	CmdGuide       = "guide"
	CmdReport      = "report-problem"
)

// Help links (https only; opened through the runtime's OpenURL).
const (
	// Like REPO_URL in frontend/src/pages/settings/util.ts these stay on the old repository name until the
	// rename (R7-08 switches them to rudian/codepass); GitHub redirects the old URL after the rename.
	GuideURL   = "https://github.com/rudian/aws_sso_hub/blob/main/docs/guide.md"
	GuideURLZh = "https://github.com/rudian/aws_sso_hub/blob/main/docs/guide.zh-CN.md"
	ReportURL  = "https://github.com/rudian/aws_sso_hub/issues"
)

// menuEntry is one line of the menu model. Kind "" is an item; Role is an application.Role name for system items.
type menuEntry struct {
	Sep   bool
	ID    string // command id; "" for role items
	Role  string // "undo", "quit", ... mapped by roleOf
	Label string
	Accel string // Wails accelerator, "CmdOrCtrl+W"; empty = none
}

type menuSection struct {
	Title string
	Items []menuEntry
}

func sep() menuEntry { return menuEntry{Sep: true} }

// menuModel is the whole menu bar for a language ("zh-CN" or "en"). It is plain data so a test can snapshot it.
// macOS only: Windows has no menu bar (the same commands live in the palette and the shortcuts).
func menuModel(lang string) []menuSection {
	l := func(zhText, enText string) string { return pick(lang, zhText, enText) }
	name := version.ProductName
	return []menuSection{
		{name, []menuEntry{
			{Role: "about", Label: l("关于 "+name, "About "+name)},
			{ID: CmdCheckUpdates, Label: l("检查更新...", "Check for updates...")},
			sep(),
			{ID: CmdSettings, Label: l("设置...", "Settings..."), Accel: "CmdOrCtrl+,"},
			sep(),
			{Role: "hide", Label: l("隐藏 "+name, "Hide "+name)},
			{Role: "hideothers", Label: l("隐藏其他", "Hide others")},
			sep(),
			{Role: "quit", Label: l("退出 "+name, "Quit "+name)},
		}},
		{l("文件", "File"), []menuEntry{
			{ID: CmdNewTask, Label: l("新建 AI 任务", "New AI task"), Accel: "CmdOrCtrl+T"},
			{ID: CmdQuickOpen, Label: l("快速打开", "Quick open"), Accel: "CmdOrCtrl+P"},
			{ID: CmdSave, Label: l("保存", "Save"), Accel: "CmdOrCtrl+S"},
			sep(),
			{ID: CmdCloseTab, Label: l("关闭标签页", "Close tab"), Accel: "CmdOrCtrl+W"},
			{ID: CmdCloseWindow, Label: l("关闭窗口", "Close window"), Accel: "CmdOrCtrl+Shift+W"},
		}},
		{l("编辑", "Edit"), []menuEntry{
			{Role: "undo", Label: l("撤销", "Undo")},
			{Role: "redo", Label: l("重做", "Redo")},
			sep(),
			{Role: "cut", Label: l("剪切", "Cut")},
			{Role: "copy", Label: l("拷贝", "Copy")},
			{Role: "paste", Label: l("粘贴", "Paste")},
			{Role: "selectall", Label: l("全选", "Select all")},
			sep(),
			{ID: CmdFind, Label: l("查找", "Find"), Accel: "CmdOrCtrl+F"},
			{ID: CmdFindInFiles, Label: l("在文件中查找", "Find in files"), Accel: "CmdOrCtrl+Shift+F"},
		}},
		{l("显示", "View"), []menuEntry{
			{ID: CmdProjects, Label: l("项目", "Projects"), Accel: "CmdOrCtrl+1"},
			{ID: CmdTasks, Label: l("AI 任务", "AI tasks"), Accel: "CmdOrCtrl+2"},
			{ID: CmdPulls, Label: l("拉取请求", "Pull requests"), Accel: "CmdOrCtrl+3"},
			{ID: CmdAccounts, Label: l("账号", "Accounts"), Accel: "CmdOrCtrl+4"},
			sep(),
			{ID: CmdSwitchTask, Label: l("切换会话", "Switch session"), Accel: "CmdOrCtrl+J"},
			{ID: CmdTerminal, Label: l("显示或隐藏终端", "Toggle terminal"), Accel: "Ctrl+`"},
			sep(),
			{Role: "resetzoom", Label: l("实际大小", "Actual size")},
			{Role: "zoomin", Label: l("放大", "Zoom in")},
			{Role: "zoomout", Label: l("缩小", "Zoom out")},
			sep(),
			{Role: "fullscreen", Label: l("进入全屏", "Enter full screen")},
		}},
		{l("窗口", "Window"), []menuEntry{
			{Role: "minimise", Label: l("最小化", "Minimize")},
			{Role: "zoom", Label: l("缩放", "Zoom")},
			sep(),
			{Role: "front", Label: l("前置全部窗口", "Bring all to front")},
		}},
		{l("帮助", "Help"), []menuEntry{
			{ID: CmdTutorial, Label: l("教程", "Tutorial")},
			{ID: CmdGuide, Label: l("图文指南", "Illustrated guide")},
			{ID: CmdReport, Label: l("报告问题", "Report a problem")},
		}},
	}
}

// renderMenu is the text form the snapshot test compares: "Title" then "  label [accel] (id|role)" lines.
// roleAccel gives the accelerator the framework attaches to a role item ("" when none).
func renderMenu(m []menuSection, roleAccel func(role string) string) string {
	var b strings.Builder
	for _, s := range m {
		b.WriteString(s.Title + "\n")
		for _, it := range s.Items {
			if it.Sep {
				b.WriteString("  ---\n")
				continue
			}
			b.WriteString("  " + it.Label)
			accel := it.Accel
			if it.Role != "" && roleAccel != nil {
				accel = roleAccel(it.Role)
			}
			if accel != "" {
				b.WriteString(" [" + accel + "]")
			}
			if it.ID != "" {
				b.WriteString(" (" + it.ID + ")")
			} else {
				b.WriteString(" <" + it.Role + ">")
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func roleOf(name string) (application.Role, bool) {
	r, ok := map[string]application.Role{
		"about": application.About, "hide": application.Hide, "hideothers": application.HideOthers, "quit": application.Quit,
		"undo": application.Undo, "redo": application.Redo, "cut": application.Cut, "copy": application.Copy,
		"paste": application.Paste, "selectall": application.SelectAll,
		"resetzoom": application.ResetZoom, "zoomin": application.ZoomIn, "zoomout": application.ZoomOut,
		"fullscreen": application.ToggleFullscreen, "minimise": application.Minimise, "zoom": application.Zoom,
		"front": application.BringAllToFront,
	}[name]
	return r, ok
}

// buildMenu turns the model into a Wails menu. run receives the command id of a custom item.
func buildMenu(a *application.App, model []menuSection, run func(id string)) *application.Menu {
	menu := a.Menu.New()
	for _, sec := range model {
		sub := menu.AddSubmenu(sec.Title)
		for _, it := range sec.Items {
			switch {
			case it.Sep:
				sub.AddSeparator()
			case it.Role != "":
				role, ok := roleOf(it.Role)
				if !ok {
					continue
				}
				// AddRole takes the accelerator and the click handler from the framework (SetRole alone stores only
				// the role: no Cmd+C/V/Z/Q and no zoom action). Then the item gets the localised label.
				sub.AddRole(role)
				if mi := sub.FindByRole(role); mi != nil {
					mi.SetLabel(it.Label)
				}
			default:
				id := it.ID
				mi := sub.Add(it.Label)
				if it.Accel != "" {
					mi.SetAccelerator(it.Accel)
				}
				mi.OnClick(func(*application.Context) { run(id) })
			}
		}
	}
	return menu
}

func isBackendCommand(id string) bool {
	switch id {
	case CmdCloseWindow, CmdGuide, CmdReport:
		return true
	}
	return false
}

// installMenu (re)builds the application menu for lang. Windows has no menu bar: the same commands are reached
// through the palette and the shortcuts the frontend handles itself.
func (sh *Shell) installMenu(lang string) {
	if runtime.GOOS != "darwin" {
		return
	}
	sh.app.Menu.SetApplicationMenu(buildMenu(sh.app, menuModel(lang), sh.runCommand))
}

// runCommand runs a menu item: backend commands here, everything else goes to the frontend.
func (sh *Shell) runCommand(id string) {
	switch id {
	case CmdCloseWindow:
		sh.win.Close() // the WindowClosing hook decides: hide or quit flow
	case CmdGuide:
		url := GuideURL
		if sh.c.Settings.UILanguage() == "zh-CN" {
			url = GuideURLZh
		}
		_ = sh.rt.OpenURL(url)
	case CmdReport:
		_ = sh.rt.OpenURL(ReportURL)
	default:
		sh.showWindow()
		sh.rt.Emit(EventAppCommand, id)
	}
}
