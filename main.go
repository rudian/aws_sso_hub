package main

import (
	"embed"
	"fmt"
	"log"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/app"
	"github.com/rudian/codepass/internal/logx"
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
	c, err := app.NewContainer(p, rt, app.ContainerOptions{Endpoints: app.EndpointsFromEnv(p), Logger: logger})
	if err != nil {
		logger.Error("startup failed", "err", err)
		log.Fatal(fmt.Errorf("startup failed: %w", err))
	}
	logger.Info("starting", "version", version.Version, "sandbox", p.Sandbox)

	var mainWindow *application.WebviewWindow

	wapp := application.New(application.Options{
		Name:        version.ProductName,
		Description: "AWS IAM Identity Center and CodeCommit helper",
		Icon:        appIcon,
		Logger:      logger,
		Services: []application.Service{
			application.NewService(app.NewAccountService(c)),
			application.NewService(app.NewMigrationService(c)),
			application.NewService(app.NewRepoService(c)),
			application.NewService(app.NewGitGlobalService(c)),
			application.NewService(app.NewSystemService(c)),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: application.Middleware(app.RuntimeGuard),
		},
		Mac: application.MacOptions{
			// Closing the window quits; the shell hides it instead when the tray and "keep running" are on.
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: false,
			WebviewUserDataPath:           p.WebViewDir, // stable even if the exe is renamed
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

	app.NewShell(c, rt, wapp, mainWindow).Install()

	if err := wapp.Run(); err != nil {
		logger.Error("run failed", "err", err)
		log.Fatal(err)
	}
}
