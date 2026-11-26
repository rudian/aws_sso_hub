package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/browser"
)

// The console through a portal's HTTP proxy, for organizations that allow AWS access only from a fixed egress
// IP. A portal with a console proxy opens the console in a Google Chrome instance of its own (macOS only),
// whose proxy auto-config sends the AWS traffic through the proxy.

// consoleBrowser opens a page in Google Chrome through a proxy (internal/browser; faked in tests).
type consoleBrowser interface {
	// ChromeApp reports browser.ErrUnsupported on a system other than macOS and browser.ErrNoChrome when
	// Google Chrome is not installed.
	ChromeApp() (string, error)
	OpenChromeWithProxy(ctx context.Context, rawURL, dir, proxy string) error
}

// consoleProxy returns the HTTP proxy of the profile's portal; "" = the console opens in the default browser.
func (c *Container) consoleProxy(profile string) string {
	portal, ok := c.Accounts.ProfilePortal(profile)
	if !ok {
		return ""
	}
	return c.Settings.ConsoleProxy(portal)
}

// checkConsoleBrowser tells, before a console sign-in link is made, whether the console can open through a
// proxy: not in sandbox mode, on macOS only, and with Google Chrome installed.
func (c *Container) checkConsoleBrowser() error {
	if c.console == nil || c.Paths.Sandbox {
		return api.NewError(api.ErrSandbox, "the console does not open through a proxy in sandbox mode")
	}
	_, err := c.console.ChromeApp()
	return consoleBrowserError(err)
}

// consoleProfileDir is the Chrome profile of the console through a proxy. There is one per proxy: Chrome reads
// its proxy settings only when an instance starts, and a console opened again goes to the instance already
// running on the profile, so a changed proxy needs a profile of its own to take effect at once.
func (c *Container) consoleProfileDir(proxy string) string {
	return filepath.Join(c.Paths.AppDataDir, "console-browser", strings.ReplaceAll(proxy, ":", "_"))
}

// openConsoleThroughProxy opens the console sign-in link, a secret, in Google Chrome with the proxy.
func (c *Container) openConsoleThroughProxy(ctx context.Context, signIn, proxy string) error {
	dir := c.consoleProfileDir(proxy)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return api.NewError(api.ErrFileIO, "cannot create the console browser folder").WithParam("file", c.Paths.Display(dir))
	}
	return consoleBrowserError(c.console.OpenChromeWithProxy(ctx, signIn, dir, proxy))
}

// consoleBrowserError turns an error of internal/browser into an AppError. A failed launch is reported without
// its text, which could quote the sign-in link.
func consoleBrowserError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, browser.ErrUnsupported):
		return api.NewError(api.ErrUnsupportedOS, "the console opens through a proxy on macOS only")
	case errors.Is(err, browser.ErrNoChrome):
		return api.NewError(api.ErrChromeMissing, "Google Chrome is not installed")
	case errors.Is(err, browser.ErrInvalidProxy):
		return invalidField("consoleProxies")
	}
	return api.NewError(api.ErrInternal, "cannot open Google Chrome")
}

// moveConsoleProxy keeps a console proxy with its portal: it follows a renamed portal to its new name, and goes
// with a removed portal (to == "").
func (c *Container) moveConsoleProxy(from, to string) {
	proxy := c.Settings.ConsoleProxy(from)
	if proxy == "" {
		return
	}
	patch := map[string]string{from: ""}
	if to != "" {
		patch[to] = proxy
	}
	prev := c.Settings.API()
	next, err := c.Settings.Update(api.SettingsPatch{ConsoleProxies: patch})
	if err != nil {
		c.Log.Warn("cannot move the console proxy of a portal", "error", err.Error())
		return
	}
	next = nn(next)
	c.emit(api.EventSettingsChanged, next)
	c.notifySettings(prev, next)
}
