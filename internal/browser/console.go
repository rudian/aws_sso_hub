package browser

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Opening a page through an HTTP proxy, in a Google Chrome instance of its own (macOS only). The AWS console
// uses it for organizations that allow AWS access only from a fixed egress IP that a proxy provides.

var (
	// ErrUnsupported is returned on a system other than macOS.
	ErrUnsupported = errors.New("browser: a browser with a proxy can be opened on macOS only")
	// ErrNoChrome is returned when Google Chrome is in neither /Applications nor ~/Applications.
	ErrNoChrome = errors.New("browser: Google Chrome is not installed")
	// ErrInvalidProxy is returned for a proxy that CleanProxy does not accept.
	ErrInvalidProxy = errors.New("browser: the proxy must be host:port")
)

const (
	chromeBundle = "Google Chrome.app"
	systemApps   = "/Applications"
	pacURLPrefix = "data:application/x-ns-proxy-autoconfig;base64,"
)

// pacTemplate is the proxy auto-config script; %s is the proxy (host:port). It sends https and wss requests
// for AWS hosts through the proxy and everything else direct. AWS hosts have no DIRECT fallback: where the
// proxy cannot be reached, AWS pages fail instead of quietly going direct.
const pacTemplate = `function FindProxyForURL(url, host) {
  if (url.substring(0, 6) != "https:" && url.substring(0, 4) != "wss:") return "DIRECT";
  host = host.toLowerCase();
  var aws = ["amazonaws.com", "aws.amazon.com", "awsapps.com", "app.aws", "api.aws"];
  for (var i = 0; i < aws.length; i++) {
    if (host == aws[i] || dnsDomainIs(host, "." + aws[i])) return "PROXY %s";
  }
  return "DIRECT";
}
`

// CleanProxy checks an HTTP proxy written as host:port and returns it in canonical form (host in lower case,
// port without leading zeros). The host is an IPv4 address or a DNS name and the port is 1 to 65535. A
// scheme, a path, user info, spaces and IPv6 addresses are refused; surrounding spaces are trimmed.
func CleanProxy(v string) (string, bool) {
	v = strings.TrimSpace(v)
	i := strings.LastIndexByte(v, ':')
	if i <= 0 {
		return "", false
	}
	host, port := strings.ToLower(v[:i]), v[i+1:]
	if port == "" || len(port) > 5 || strings.Trim(port, "0123456789") != "" {
		return "", false
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || !validProxyHost(host) {
		return "", false
	}
	return host + ":" + strconv.Itoa(n), true
}

// validProxyHost accepts an IPv4 address or a DNS name (lower case).
func validProxyHost(h string) bool {
	if h == "" || len(h) > 253 {
		return false
	}
	if strings.Trim(h, "0123456789.") == "" {
		// digits and dots only: an IPv4 address, never a name
		a, err := netip.ParseAddr(h)
		return err == nil && a.Is4()
	}
	for _, label := range strings.Split(h, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	return true
}

// pacURL returns the proxy auto-config script for proxy (already clean) as a data: URL.
func pacURL(proxy string) string {
	return pacURLPrefix + base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(pacTemplate, proxy)))
}

// ChromeApp returns the Google Chrome app bundle, in /Applications first, then in ~/Applications. It reports
// ErrUnsupported on a system other than macOS and ErrNoChrome when Chrome is not installed.
func (o *Opener) ChromeApp() (string, error) {
	if o.goos() != "darwin" {
		return "", ErrUnsupported
	}
	dirs := []string{systemApps}
	if o.Applications != "" {
		dirs[0] = o.Applications
	}
	if home := o.homeDir(); home != "" {
		dirs = append(dirs, filepath.Join(home, "Applications"))
	}
	for _, d := range dirs {
		p := filepath.Join(d, chromeBundle)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p, nil
		}
	}
	return "", ErrNoChrome
}

// OpenChromeWithProxy opens rawURL in a new Google Chrome instance that runs on the profile directory dir. A
// proxy auto-config script, given to Chrome as a data: URL, sends https and wss requests for AWS hosts
// through the HTTP proxy (host:port) and everything else direct. Chrome reads its proxy settings only when an
// instance starts: a URL sent to an instance already running on dir opens there, with that instance's proxy.
// The returned error never contains rawURL.
func (o *Opener) OpenChromeWithProxy(ctx context.Context, rawURL, dir, proxy string) error {
	chrome, err := o.ChromeApp()
	if err != nil {
		return err
	}
	if err := CheckURL(rawURL); err != nil {
		return err
	}
	p, ok := CleanProxy(proxy)
	if !ok {
		return ErrInvalidProxy
	}
	if !filepath.IsAbs(dir) || strings.ContainsAny(dir, "\x00\r\n") {
		return errors.New("browser: the profile directory must be an absolute path")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// -n starts a new instance; Chrome keeps it apart from the user's own because the profile directory differs.
	args := []string{"-n", "-a", chrome, "--args", userDataFlag + dir, "--proxy-pac-url=" + pacURL(p),
		"--no-first-run", "--no-default-browser-check", rawURL}
	// open returns once LaunchServices has started Chrome (or failed to); its error names no argument.
	if _, err := o.output(ctx, openPath, args...); err != nil {
		return fmt.Errorf("cannot start Google Chrome: %w", err)
	}
	return nil
}
