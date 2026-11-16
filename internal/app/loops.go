package app

import (
	"context"
	"net"
	"time"

	"github.com/rudian/codepass/internal/gitx"
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

	fetchEvery := c.opts.FetchInterval
	if fetchEvery <= 0 {
		fetchEvery = 15 * time.Minute
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
	c.spawn(func() { c.fetchLoop(ctx, fetchEvery) }) // background fetch of bound or opened repositories (the engine picks the targets)
	c.spawn(func() { c.removeStaleSignIn(ctx) })     // sign-in browsers left behind by a crash
	c.spawn(func() {                                 // environment check, then scan and owner detection
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

// online reports whether the machine looks connected: a non-loopback network interface is up and has a routable
// address (not loopback, not link-local), and the system has a default route to the internet. It sends no
// traffic (a UDP connect to a documentation address writes no packet). Background fetch is skipped while it is
// false.
func (c *Container) online() bool {
	if c.opts.Online != nil {
		return c.opts.Online()
	}
	return interfaceUp()
}

// fetchLoop runs one background-fetch round every `every`, skipped while offline. One round is bounded by
// `every`, so a hung git process cannot stop the later rounds.
func (c *Container) fetchLoop(ctx context.Context, every time.Duration) {
	fetch := c.fetchRound
	if fetch == nil {
		fetch = c.Repos.BackgroundFetch
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if !c.online() {
				continue
			}
			rctx, cancel := context.WithTimeout(ctx, every)
			ok, failed := fetch(rctx)
			cancel()
			if ok+failed > 0 && c.Log != nil {
				c.Log.Info("background fetch", "ok", ok, "failed", failed)
			}
		}
	}
}

// usableAddr reports whether an interface address can carry traffic off the machine: not loopback, not link-local
// (macOS keeps utun, awdl and llw interfaces up with IPv6 link-local addresses whatever the Wi-Fi state).
func usableAddr(a net.Addr) bool {
	n, ok := a.(*net.IPNet)
	if !ok {
		return false
	}
	return !n.IP.IsLoopback() && !n.IP.IsLinkLocalUnicast() && !n.IP.IsUnspecified()
}

// hasUsableAddr is true when at least one address passes usableAddr.
func hasUsableAddr(addrs []net.Addr) bool {
	for _, a := range addrs {
		if usableAddr(a) {
			return true
		}
	}
	return false
}

// hasDefaultRoute asks the OS whether a route to the internet exists. A UDP connect sends no packet; it fails
// with "network is unreachable" when there is no route.
func hasDefaultRoute() bool {
	for _, t := range []struct{ network, addr string }{{"udp4", "192.0.2.1:9"}, {"udp6", "[2001:db8::1]:9"}} {
		conn, err := net.Dial(t.network, t.addr)
		if err == nil {
			_ = conn.Close()
			return true
		}
	}
	return false
}

func interfaceUp() bool {
	ifs, err := net.Interfaces()
	if err != nil {
		return true // cannot tell: let the fetch try and fail quietly
	}
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		if addrs, err := i.Addrs(); err == nil && hasUsableAddr(addrs) {
			return hasDefaultRoute()
		}
	}
	return false
}
