// Package awsguard keeps tests away from the real AWS. In a test binary every AWS client built by the app
// sends its requests through a transport that refuses any host that is not loopback, so a test that forgets
// the fake endpoints fails with an error before a connection is opened. In a production binary nothing
// changes: Client returns nil and Transport returns nil, and the clients use their defaults.
package awsguard

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// ErrRefused is wrapped by the error a refused request returns.
var ErrRefused = errors.New("awsguard: refusing a request to a non-loopback host in a test binary")

var (
	mu      sync.Mutex
	refused []string
)

// Refused lists the hosts that were refused since the process started (for tests).
func Refused() []string {
	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), refused...)
}

// Loopback reports whether host (with or without a port) is localhost or a loopback address.
func Loopback(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type guard struct{ next http.RoundTripper }

func (g guard) RoundTrip(r *http.Request) (*http.Response, error) {
	if !Loopback(r.URL.Host) {
		mu.Lock()
		refused = append(refused, r.URL.Host)
		mu.Unlock()
		if r.Body != nil {
			r.Body.Close()
		}
		return nil, fmt.Errorf("%w: %q", ErrRefused, r.URL.Host)
	}
	next := g.next
	if next == nil {
		next = http.DefaultTransport
	}
	return next.RoundTrip(r)
}

// Transport wraps next with the guard in a test binary, and returns next unchanged (possibly nil, meaning
// the default transport) otherwise.
func Transport(next http.RoundTripper) http.RoundTripper {
	if !testing.Testing() {
		return next
	}
	return guard{next: next}
}

// Client returns the HTTP client for an AWS SDK client's HTTPClient option: a guarded client in a test
// binary, nil otherwise (callers set the option only when it is not nil).
func Client() *http.Client {
	if !testing.Testing() {
		return nil
	}
	return &http.Client{Transport: guard{}}
}
