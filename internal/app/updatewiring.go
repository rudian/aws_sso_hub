package app

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/rudian/codepass/internal/update"
	"github.com/rudian/codepass/internal/version"
)

// EventUpdateState carries an api.UpdateState on every change of the update flow (registered in events.go).
const EventUpdateState = "update:state"

// UpdateOptions are what main passes to StartUpdates.
type UpdateOptions struct {
	// Quit ends the app at once with no quit dialog (the person already chose "Reopen"). main passes a function that
	// approves the quit in the Lifecycle and then quits.
	Quit func()
	// Proxy picks the proxy per request (the shared netx client's resolver).
	Proxy func(*http.Request) (*url.URL, error)
}

// StartUpdates builds the update controller (production gate on every OS, the public channel with its pinned
// keys), records the running version as seen, starts the periodic check and closes the controller on shutdown.
// The controller is kept in Container.Update for the update service. In sandbox mode nothing checks the network.
func (c *Container) StartUpdates(o UpdateOptions) *update.Controller {
	dir := filepath.Join(c.Paths.AppDataDir, "update")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		c.Log.Warn("cannot create the update folder", "err", err)
	}
	hi := update.Highest{Path: filepath.Join(dir, "highest")}
	if err := hi.Record(version.Version); err != nil {
		c.Log.Warn("cannot record the running version", "err", err)
	}
	var ctl *update.Controller
	auto := &autoChecker{on: func() bool { return readUpdatePrefs(updatePrefsFile(c)).AutoCheck }, now: time.Now,
		state: func() update.State { return ctl.State() }}
	ctl = update.NewController(update.ControllerConfig{
		Quit:         o.Quit,
		Running:      version.Version,
		Gate:         update.ProductionGate(),
		Highest:      hi,
		Channel:      update.ProductionChannel,
		Proxy:        o.Proxy,
		HelperLog:    filepath.Join(dir, "restarts"),
		StagedRecord: filepath.Join(dir, "staged"),
		OnState:      func(s update.State) { c.emit(EventUpdateState, toAPIUpdateState(s)) },
		AutoCheck:    auto.due,
		Logf:         func(f string, a ...any) { c.Log.Info(fmt.Sprintf(f, a...)) },
	})
	c.Update = ctl
	c.OnShutdown(ctl.Close)
	if !c.Paths.Sandbox {
		c.spawn(func() { ctl.Run(c.ctx, updateFirstCheck, updateCheckEvery) })
	}
	return ctl
}
