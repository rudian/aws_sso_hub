package app

import (
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/rudian/codepass/internal/api"
)

// Shell connects the container to the running Wails application: window close behaviour, the optional tray and
// the sandbox self-test. main() creates it after the window exists.
type Shell struct {
	c    *Container
	rt   *wailsRuntime
	app  *application.App
	win  *application.WebviewWindow
	tray *trayController
	life *Lifecycle

	keepRunning atomic.Bool
	trayOn      atomic.Bool
}

// NewShell creates the shell. Call Install once the main window exists.
func NewShell(c *Container, rt *wailsRuntime, app *application.App, win *application.WebviewWindow, life *Lifecycle) *Shell {
	if life == nil {
		life = NewLifecycle()
	}
	sh := &Shell{c: c, rt: rt, app: app, win: win, life: life}
	sh.tray = newTrayController(sh)
	return sh
}

// hideWindow hides the window. On Windows a hidden window has no taskbar button, so with the tray setting off
// the window is minimised instead: the user can always get it back.
func (sh *Shell) hideWindow() {
	if runtime.GOOS == "windows" && !sh.trayOn.Load() {
		sh.win.Minimise()
		return
	}
	sh.win.Hide()
}

// showWindow brings the window back only when it is not on screen. It never calls Restore: Wails' Restore
// leaves full screen and un-zooms a zoomed window, which a menu command must not do.
func (sh *Shell) showWindow() {
	if !sh.win.IsVisible() {
		sh.win.Show()
	}
	if sh.win.IsMinimised() {
		sh.win.UnMinimise()
	}
	sh.win.Focus()
}

// Install registers the window hook, applies the saved tray settings and subscribes to later changes.
func (sh *Shell) Install() {
	sh.rt.SetWindow(sh.win)

	// Closing the window hides it while tasks run or "keep running" is set; otherwise it goes through the quit
	// flow. The event is always cancelled: the app ends only through Quit, never because the last window went.
	sh.win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		if sh.life.CloseAction(sh.keepRunning.Load()) == CloseHide {
			sh.hideWindow()
			return
		}
		go sh.app.Quit() // asks Lifecycle.ShouldQuit, which never blocks and may open the dialog
	})
	sh.life.SetQuit(sh.app.Quit)
	sh.life.SetOnBackground(sh.hideWindow)
	sh.installMenu(sh.c.Settings.UILanguage())
	sh.c.OnUILanguage(sh.installMenu)

	sh.apply(sh.c.Settings.Tray())
	sh.c.OnSettingsChanged(func(prev, next api.Settings) {
		if prev.Tray != next.Tray {
			sh.apply(next.Tray)
		}
	})
	sh.c.OnUILanguage(func(string) { sh.tray.Refresh() })
	sh.c.Subscribe(func(name string, _ any) {
		if name == api.EventAccountsChanged || name == api.EventSessionChanged {
			sh.tray.Refresh()
		}
	})
	sh.c.OnShutdown(sh.tray.Disable)

	if file := os.Getenv("AWSSSOHUB_SELFTEST"); file != "" && sh.c.Paths.Sandbox {
		sh.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
			go func() {
				time.Sleep(2 * time.Second)
				if err := RunSelfTest(sh.c, file); err != nil {
					sh.c.Log.Error("selftest failed", "err", err)
				}
				sh.app.Quit()
			}()
		})
	}
}

func (sh *Shell) apply(t api.TraySettings) {
	sh.keepRunning.Store(t.Enabled && t.KeepRunning)
	sh.trayOn.Store(t.Enabled)
	if t.Enabled {
		sh.tray.Enable()
	} else {
		sh.tray.Disable()
	}
}
