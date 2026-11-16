package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/app"
	"github.com/rudian/codepass/internal/appguard"
	"github.com/rudian/codepass/internal/logx"
	"github.com/rudian/codepass/internal/netx"
	"github.com/rudian/codepass/internal/paths"
	"github.com/rudian/codepass/internal/version"
)

// Wails embeds the built frontend (frontend/dist) into the binary.
//
//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// Event registration lives in internal/app/events.go (init). Services are built by internal/app from one
// container that owns every engine (container.go).
func main() {
	// Must stay the first statement (plan 4.14 item 6; main_test.go checks it). In helper mode (this binary
	// re-executed by the updater to swap itself) nothing below may run: no paths, no log file, no services.
	updater.HandleHelperMode()

	p, err := paths.Resolve()
	if err != nil {
		log.Fatal(err)
	}
	if err := p.EnsureAppDirs(); err != nil {
		log.Fatal(err)
	}
	logger, logCloser, err := logx.New(p.LogDir, os.Getenv("AWSSSOHUB_DEBUG") != "")
	if err != nil {
		log.Fatal(err)
	}
	defer logCloser.Close()

	rt := app.NewWailsRuntime(p.Sandbox)
	// A projects.json from a newer Codepass stops the start with a native message and writes nothing; a damaged one
	// is moved aside and replaced by a fresh one (FU-18d). Both happen before the window opens.
	c, err := app.OpenContainer(p, rt, app.ContainerOptions{Endpoints: app.EndpointsFromEnv(p), Logger: logger}, appguard.SystemDeps().Prompter.Tell)
	if errors.Is(err, app.ErrStartupStopped) {
		logger.Info("startup stopped: projects.json was written by a newer version")
		return
	}
	if err != nil {
		logger.Error("startup failed", "err", err)
		log.Fatal(fmt.Errorf("startup failed: %w", err))
	}
	// The old AWS SSO Hub has the same bundle ID, so Wails' single-instance check would hand this launch to it and
	// exit without a word. Ask the user first; this must run before application.New.
	if !p.Sandbox && appguard.Run(oldAppDeps(c.Settings.UILanguage())) == appguard.Exit {
		logger.Info("old app is running and was not stopped; exiting")
		return
	}
	logger.Info("starting", "version", version.Version, "sandbox", p.Sandbox)

	// One HTTP client for the setup downloads and the update check; the system proxy is read once at start.
	detectCtx, cancelDetect := context.WithTimeout(context.Background(), 5*time.Second)
	nc := netx.New(netx.Options{Proxy: netx.Detect(detectCtx)})
	cancelDetect()

	var mainWindow *application.WebviewWindow
	life := app.NewLifecycle() // tasks-running and quit-decision hooks; A2-05 and A2-14 supply them

	wapp := application.New(application.Options{
		Name:        version.ProductName,
		Description: "AWS IAM Identity Center and CodeCommit helper",
		Icon:        appIcon,
		Logger:      logger,
		Services: []application.Service{
			// first: the system notification service (started and guarded by internal/notify) must be up, with
			// its delegate set, before the accounts ticker can send a notice or a click can arrive
			application.NewService(app.NewNotifyService(c)),
			application.NewService(app.NewAccountService(c)),
			application.NewService(app.NewMigrationService(c)),
			application.NewService(app.NewRepoService(c)),
			application.NewService(app.NewGitGlobalService(c)),
			application.NewService(app.NewSystemService(c)),
			application.NewService(app.NewToolService(c, nc)),
			application.NewService(app.NewIdentityService(c, c.Projects)),
			application.NewService(app.NewUpdateService(c)),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: application.Middleware(app.RuntimeGuard),
		},
		Mac: application.MacOptions{
			// Hiding or closing the last window never ends the app; the quit flow does (internal/app/window.go).
			ApplicationShouldTerminateAfterLastWindowClosed: app.TerminateAfterLastWindowClosed,
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: !app.TerminateAfterLastWindowClosed,
			WebviewUserDataPath:           p.WebViewDir, // stable even if the exe is renamed
			WndProcInterceptor:            app.WndProc,  // notes a logout, restart or shutdown (WM_QUERYENDSESSION)
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: version.BundleID,
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				rt.ShowWindow()
				rt.Emit(api.EventSecondInstance, append([]string{}, data.Args...))
			},
		},
		// Quitting stops tickers, servers and the tray only. Tokens, role caches and session records stay.
		OnShutdown: c.Shutdown,
		// Cmd+Q, the menu, the Dock and the tray ask the quit decision hook.
		ShouldQuit: life.ShouldQuit,
	})

	// The update controller: production gate and channel, quits without the quit dialog (the person chose Reopen).
	c.StartUpdates(app.UpdateOptions{
		Quit:  func() { _ = life.QuitApproved(func() error { wapp.Quit(); return nil }) },
		Proxy: func(r *http.Request) (*url.URL, error) { return nc.Proxy().ProxyFunc()(r) },
	})

	mainWindow = wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            version.ProductName,
		Width:            1200,
		Height:           780,
		MinWidth:         980,
		MinHeight:        660,
		URL:              "/",
		BackgroundColour: application.NewRGB(242, 244, 247), // matches --bg light; avoids a white flash
		Mac: application.MacWindow{
			TitleBar:                application.MacTitleBarHiddenInset,
			InvisibleTitleBarHeight: 40, // matches --titlebar-h
			Backdrop:                application.MacBackdropNormal,
		},
		Windows: application.WindowsWindow{
			Theme: application.SystemDefault,
		},
	})

	app.NewShell(c, rt, wapp, mainWindow, life).Install()

	if err := wapp.Run(); err != nil {
		logger.Error("run failed", "err", err)
		log.Fatal(err)
	}
}

// oldAppDeps are the production dependencies of the old-app handoff, with the prompts in the saved UI language.
func oldAppDeps(lang string) appguard.Deps {
	d := appguard.SystemDeps()
	d.Lang = lang
	return d
}
