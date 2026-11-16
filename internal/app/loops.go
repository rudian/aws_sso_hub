package app

import (
	"context"
	"time"

	"github.com/rudian/aws_sso_hub/internal/gitx"
)

// StartLoops starts the background work: the 30 s accounts ticker, the 3 s file watcher, the initial
// environment check and, after a short delay, the repository scan followed by owner detection. It is idempotent.
// Nothing important relies on Shutdown being called.
func (c *Container) StartLoops() {
	c.lifeMu.Lock()
	if c.started || c.closed {
		c.lifeMu.Unlock()
		return
	}
	c.started = true
	ctx := c.ctx
	c.lifeMu.Unlock()

	tick := c.opts.TickInterval
	if tick <= 0 {
		tick = 30 * time.Second
	}
	watch := c.opts.WatchInterval
	if watch <= 0 {
		watch = 3 * time.Second
	}
	delay := c.opts.ScanDelay
	if delay <= 0 {
		delay = time.Second
	}

	c.files.prime()

	c.spawn(func() { // accounts ticker
		c.Accounts.Tick(ctx)
		t := time.NewTicker(tick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.Accounts.Tick(ctx)
			}
		}
	})
	c.spawn(func() { // file watcher
		t := time.NewTicker(watch)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.pollFiles(ctx)
			}
		}
	})
	c.spawn(func() { c.removeStaleSignIn(ctx) }) // sign-in browsers left behind by a crash
	c.spawn(func() {                             // environment check, then scan and owner detection
		c.CheckEnvironment(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		if _, err := c.Repos.Scan(ctx); err != nil {
			c.Log.Warn("initial scan failed", "err", err)
			return
		}
		if _, err := c.Repos.DetectOwners(ctx, nil); err != nil {
			c.Log.Warn("owner detection failed", "err", err)
		}
	})
}

// spawn runs f in a goroutine that Shutdown waits for (bounded).
func (c *Container) spawn(f func()) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				c.Log.Error("background task panicked", "panic", r)
			}
		}()
		f()
	}()
}

// Background runs f in a goroutine whose context Shutdown cancels (rescans after a settings change).
func (c *Container) Background(f func(ctx context.Context)) {
	c.spawn(func() { f(c.ctx) })
}

// Shutdown stops tickers, watchers and the tray. It never touches token files, role caches or session
// records: quitting the app does not sign anyone out. Safe to call more than once.
func (c *Container) Shutdown() {
	c.lifeMu.Lock()
	if c.closed {
		c.lifeMu.Unlock()
		return
	}
	c.closed = true
	cancel := c.cancel
	hooks := c.onClose
	c.onClose = nil
	c.lifeMu.Unlock()

	if cancel != nil {
		cancel()
	}
	for _, h := range hooks {
		h()
	}
	done := make(chan struct{})
	go func() { c.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
	gitx.RemoveNeutralDir()
}
