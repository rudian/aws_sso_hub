package netx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// Options tunes the client. Zero values give safe defaults.
type Options struct {
	Proxy     Config        // proxy to use; zero means none
	UserAgent string        // default "Codepass"
	Timeout   time.Duration // overall request timeout, default 30s; 0 keeps the default, negative disables (downloads)
	MaxBody   int64         // body cap for GetCached, default 8 MiB
	CacheSize int           // ETag cache entries, default 128
	Transport http.RoundTripper
}

// Client is the app's one HTTP client.
type Client struct {
	HTTP      *http.Client
	userAgent string
	maxBody   int64

	proxy atomic.Pointer[proxyState]

	mu    sync.Mutex
	cache map[string]*entry
	order []string
	limit int
}

type entry struct {
	etag   string
	body   []byte
	ct     string
	header http.Header
}

type proxyState struct {
	cfg Config
	f   func(*http.Request) (*url.URL, error)
}

// New builds a client with dial, TLS and response-header timeouts.
func New(o Options) *Client {
	if o.UserAgent == "" {
		o.UserAgent = "Codepass"
	}
	if o.Timeout == 0 {
		o.Timeout = 30 * time.Second
	}
	if o.MaxBody <= 0 {
		o.MaxBody = 8 << 20
	}
	if o.CacheSize <= 0 {
		o.CacheSize = 128
	}
	c := &Client{userAgent: o.UserAgent, maxBody: o.MaxBody, cache: map[string]*entry{}, limit: o.CacheSize}
	c.SetProxy(o.Proxy)
	rt := o.Transport
	if rt == nil {
		rt = &http.Transport{
			// Read per request, so SetProxy takes effect on the next call.
			Proxy:                 func(r *http.Request) (*url.URL, error) { return c.proxy.Load().f(r) },
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 20 * time.Second,
			IdleConnTimeout:       60 * time.Second,
			MaxIdleConns:          10,
		}
	}
	hc := &http.Client{Transport: rt}
	if o.Timeout > 0 {
		hc.Timeout = o.Timeout
	}
	c.HTTP = hc
	return c
}

// SetProxy switches the proxy used from now on (after a retry re-ran Detect)
// and drops idle connections made under the old setting. It has no effect on
// a custom Options.Transport, which owns its own proxy choice.
func (c *Client) SetProxy(cfg Config) {
	c.proxy.Store(&proxyState{cfg: cfg, f: cfg.ProxyFunc()})
	if c.HTTP != nil {
		c.HTTP.CloseIdleConnections()
	}
}

// Download returns an HTTP client for large bodies (installers, updates): it
// shares this client's transport, and so its proxy and SetProxy, but has no
// overall timeout. Callers bound it with a context and a progress watchdog.
func (c *Client) Download() *http.Client {
	return &http.Client{Transport: c.HTTP.Transport}
}

// Proxy returns the proxy setup in use, for ChildEnv and the UI.
func (c *Client) Proxy() Config { return c.proxy.Load().cfg }

// Result is a GetCached answer.
type Result struct {
	Body        []byte
	ContentType string
	FromCache   bool // server answered 304 and the cached body was used
	// Header holds the response headers. After a 304 it is the cached 200's
	// headers overlaid with the fresh 304 ones, so rate-limit values are
	// current and Link/pagination stays available.
	Header http.Header
}

// ErrStatus is returned for any status other than 200 and 304. It carries no
// URL, so credentials in a URL never reach a message.
type ErrStatus struct {
	Code   int
	Header http.Header // response headers (rate limit, scopes, Retry-After)
	// Body is up to 64 KiB of the error response, for the provider's own
	// message. Error() omits it, so it never reaches a log line.
	Body []byte
}

func (e *ErrStatus) Error() string { return fmt.Sprintf("unexpected HTTP status %d", e.Code) }

// ErrTooLarge means the body passed the cap.
var ErrTooLarge = errors.New("response body too large")

// Do sends a request with the client's User-Agent.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	return c.HTTP.Do(req)
}

// GetCached fetches url with If-None-Match when an ETag is known. A 304 serves
// the stored body. Only 200 and 304 succeed.
func (c *Client) GetCached(ctx context.Context, url string, header http.Header) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{}, err
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	// The key holds the URL, the Accept form and a hash of the credentials,
	// so identities and representations of one URL keep separate entries.
	key := cacheKey(url, req.Header)
	c.mu.Lock()
	prev := c.cache[key]
	c.mu.Unlock()
	if prev != nil && prev.etag != "" {
		req.Header.Set("If-None-Match", prev.etag)
	}
	resp, err := c.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotModified:
		if prev == nil {
			return Result{}, &ErrStatus{Code: resp.StatusCode, Header: resp.Header.Clone()}
		}
		h := prev.header.Clone()
		for k, vs := range resp.Header {
			h[k] = vs
		}
		return Result{Body: bytes.Clone(prev.body), ContentType: prev.ct, FromCache: true, Header: h}, nil
	case http.StatusOK:
	default:
		eb, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return Result{}, &ErrStatus{Code: resp.StatusCode, Header: resp.Header.Clone(), Body: eb}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
	if err != nil {
		return Result{}, err
	}
	if int64(len(body)) > c.maxBody {
		return Result{}, ErrTooLarge
	}
	ct := resp.Header.Get("Content-Type")
	if et := resp.Header.Get("ETag"); et != "" {
		c.store(key, &entry{etag: et, body: body, ct: ct, header: resp.Header.Clone()})
	}
	return Result{Body: bytes.Clone(body), ContentType: ct, Header: resp.Header.Clone()}, nil
}

func (c *Client) store(key string, e *entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.cache[key]; !ok {
		c.order = append(c.order, key)
		if len(c.order) > c.limit {
			delete(c.cache, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.cache[key] = e
}

func cacheKey(rawURL string, h http.Header) string {
	sum := sha256.Sum256([]byte(h.Get("Authorization")))
	return rawURL + "\x00" + h.Get("Accept") + "\x00" + hex.EncodeToString(sum[:])
}
