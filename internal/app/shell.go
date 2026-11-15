package app

import (
	"os"
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

	keepRunning atomic.Bool
}

// NewShell creates the shell. Call Install once the main window exists.
func NewShell(c *Container, rt *wailsRuntime, app *application.App, win *application.WebviewWindow) *Shell {
	sh := &Shell{c: c, rt: rt, app: app, win: win}
	sh.tray = newTrayController(sh)
	return sh
}

func (sh *Shell) showWindow() { sh.rt.ShowWindow() }

// Install registers the window hook, applies the saved tray settings and subscribes to later changes.
func (sh *Shell) Install() {
	sh.rt.SetWindow(sh.win)

	// Closing the window quits, unless the tray is on and "keep running" is set: then it only hides.
	sh.win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if sh.keepRunning.Load() {
			sh.win.Hide()
			e.Cancel()
		}
	})

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
	if t.Enabled {
		sh.tray.Enable()
	} else {
		sh.tray.Disable()
	}
}
