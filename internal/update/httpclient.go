package update

import (
	"net"
	"net/http"
	"net/url"
	"time"
)

// NewHTTPClient is the production client for the check and the download. It has dial, TLS and response-header
// timeouts but no overall timeout: the Wails default (30 s for the whole request, body included) cuts a download of
// a large archive off on a slow line. Callers bound a check with a context deadline instead.
//
// proxy chooses the proxy for each request. A copy of the app opened from Finder has no proxy variables, so the
// wiring passes the system-proxy resolver of internal/netx; nil falls back to the environment variables.
func NewHTTPClient(proxy func(*http.Request) (*url.URL, error)) *http.Client {
	if proxy == nil {
		proxy = http.ProxyFromEnvironment
	}
	tr := &http.Transport{
		Proxy:                 proxy,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       60 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{Transport: tr}
}
