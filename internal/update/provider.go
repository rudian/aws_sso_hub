package update

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
	ghprov "github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// ProviderConfig configures the manifest provider.
type ProviderConfig struct {
	Channel Channel
	Running string                 // running version
	Highest func() (string, error) // stored highest installed version; an error refuses the update
	HTTP    *http.Client           // nil: NewHTTPClient(Proxy)
	Proxy   func(*http.Request) (*url.URL, error)
	// IdleTimeout ends a download that receives no data for this long; 0 means 60 s.
	IdleTimeout time.Duration
}

// Provider wraps the Wails GitHub provider. The GitHub provider is used for exactly two things: finding the newest
// release, and streaming an asset. It is never trusted for the version, the digest, the file name or the notes.
type Provider struct {
	cfg   ProviderConfig
	inner *ghprov.Provider
	http  *http.Client
	base  string

	mu      sync.Mutex
	assets  []ghprov.ReleaseAsset // captured by the matcher during inner.Check
	refusal error                 // the last security refusal, for the controller (Wails flattens errors to text)
}

// NewProvider builds a provider for one channel. Build a fresh one per check.
func NewProvider(cfg ProviderConfig) (*Provider, error) {
	ch := cfg.Channel
	if len(ch.Keys) == 0 {
		return nil, errors.New("update: at least one pinned key is required")
	}
	if ch.Name == "production" && ch.Token != "" {
		return nil, errors.New("update: the production channel is public and takes no token")
	}
	p := &Provider{cfg: cfg, http: cfg.HTTP, base: strings.TrimRight(ch.BaseURL, "/")}
	if p.base == "" {
		p.base = "https://api.github.com"
	}
	if p.http == nil {
		p.http = NewHTTPClient(cfg.Proxy)
	}
	inner, err := ghprov.New(ghprov.Config{
		Repository: ch.Repository,
		BaseURL:    ch.BaseURL,
		Token:      ch.Token,
		Prerelease: ch.Prerelease,
		HTTPClient: p.http,
		// Matcher: remember every asset of the one release the inner provider
		// fetched (no second API call, so no race), and hand back an index so
		// Check does not fail. The returned pick is discarded by our Check.
		AssetMatcher: func(_ updater.CheckRequest, assets []ghprov.ReleaseAsset) int {
			p.mu.Lock()
			p.assets = append([]ghprov.ReleaseAsset(nil), assets...)
			p.mu.Unlock()
			if len(assets) == 0 {
				return -1
			}
			return 0
		},
	})
	if err != nil {
		return nil, err
	}
	p.inner = inner
	return p, nil
}

// Refusal returns the security refusal of the last Check, or nil.
func (p *Provider) Refusal() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.refusal
}

func (p *Provider) Name() string { return "codepass-manifest" }

func manifestName(os, arch string) string { return "manifest-" + os + "-" + arch + ".json" }

// Check returns a release only when the manifest verifies and every rule holds. Any problem is an error, never
// "no update", so a refusal is visible.
func (p *Provider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	p.mu.Lock()
	p.refusal, p.assets = nil, nil
	p.mu.Unlock()
	rel, err := p.check(ctx, req)
	if err != nil && IsRefusal(err) {
		p.mu.Lock()
		p.refusal = err
		p.mu.Unlock()
	}
	return rel, err
}

func (p *Provider) check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	// A stored floor that cannot be read stops everything, before any request is made.
	highest := ""
	if p.cfg.Highest != nil {
		h, err := p.cfg.Highest()
		if err != nil {
			return nil, refuseAs(CodeHighestUnreadable, "%v", err)
		}
		highest = h
	}
	rel, err := p.inner.Check(ctx, req)
	if err != nil {
		return nil, err
	}
	if rel == nil {
		// GitHub answers 404 for a private repository the token cannot see, and the Wails provider reads that as
		// "no release". With a token, confirm the repository is reachable so a wrong token or name is not "up to date".
		if p.cfg.Channel.Token != "" || p.cfg.Channel.Name != "production" {
			if err := p.checkRepoAccess(ctx); err != nil {
				return nil, err
			}
		}
		return nil, nil // newest release is not newer than running (inner semver)
	}
	tag, _ := rel.Metadata["github.release.tag"].(string)

	p.mu.Lock()
	assets := p.assets
	p.mu.Unlock()
	byName := map[string]ghprov.ReleaseAsset{}
	for _, a := range assets {
		byName[a.Name] = a
	}
	mAsset, ok1 := byName[manifestName(req.Platform, req.Arch)]
	sAsset, ok2 := byName[manifestName(req.Platform, req.Arch)+".sig"]
	if !ok1 || !ok2 {
		return nil, refuseAs(CodeManifestMissing, "release %s has no signed manifest for %s/%s", tag, req.Platform, req.Arch)
	}
	raw, err := p.fetch(ctx, mAsset.URL, maxManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}
	sig, err := p.fetch(ctx, sAsset.URL, 1024)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest signature: %w", err)
	}
	m, err := VerifyManifest(raw, string(sig), p.cfg.Channel.Keys)
	if err != nil {
		return nil, refuseErr(err)
	}
	if err := m.Check(Rules{Tag: tag, Running: req.CurrentVersion, Highest: highest, OS: req.Platform, Arch: req.Arch}); err != nil {
		return nil, refuseErr(err)
	}
	arc, ok := byName[m.File]
	if !ok {
		return nil, refuseAs(CodeAsset, "release %s has no asset %q named by the manifest", tag, m.File)
	}
	if !p.inAPI(arc.URL) {
		return nil, refuseAs(CodeAsset, "asset URL %q is outside the release API", arc.URL)
	}
	digest, _ := hex.DecodeString(m.SHA256) // length checked in wellFormed

	// Rebuild the release from signed data only.
	out := &updater.Release{
		Version: m.Version,
		Channel: rel.Channel,
		Notes:   m.Notes, // only from the signed manifest
		Artifact: updater.Artifact{
			Filename: m.File, Filetype: "zip", Size: m.Size, Platform: m.OS, Arch: m.Arch,
		},
		Verification: &updater.Verification{DigestAlgo: "sha256", Digest: digest},
		Metadata: map[string]any{
			"github.asset.url":   arc.URL,
			"github.release.tag": tag,
		},
	}
	return out, nil
}

// Download streams the archive and refuses anything but exactly the signed size.
func (p *Provider) Download(ctx context.Context, rel *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	want := rel.Artifact.Size
	idle := p.cfg.IdleTimeout
	if idle <= 0 {
		idle = 60 * time.Second
	}
	ictx, cancel := context.WithCancel(ctx)
	defer cancel()
	var idled atomic.Bool
	timer := time.AfterFunc(idle, func() { idled.Store(true); cancel() })
	defer timer.Stop()
	lw := &limitWriter{w: dst, max: want, onWrite: func() { timer.Reset(idle) }}
	err := p.inner.Download(ictx, rel, lw, onProgress)
	if idled.Load() && ctx.Err() == nil && (err != nil || lw.n != want) {
		err = fmt.Errorf("no data received for %v", idle)
	}
	if err == nil && lw.n != want {
		err = refuseAs(CodeSize, "download is %d bytes, signed manifest says %d", lw.n, want)
	}
	if err != nil && IsRefusal(err) {
		p.mu.Lock()
		p.refusal = err
		p.mu.Unlock()
	}
	return err
}

// inAPI keeps every unsigned URL from the API response on the API host and repository; the CDN hop is made by the
// HTTP client's redirect, not by us.
func (p *Provider) inAPI(url string) bool {
	// GitHub answers with the canonical case of owner and repository whatever case was requested.
	prefix := p.base + "/repos/" + p.cfg.Channel.Repository + "/"
	return len(url) >= len(prefix) && strings.EqualFold(url[:len(prefix)], prefix)
}

type limitWriter struct {
	w       io.Writer
	max     int64
	n       int64
	onWrite func()
}

func (l *limitWriter) Write(b []byte) (int, error) {
	if l.onWrite != nil {
		l.onWrite()
	}
	if l.n+int64(len(b)) > l.max {
		return 0, refuseAs(CodeSize, "download exceeds signed size %d", l.max)
	}
	n, err := l.w.Write(b)
	l.n += int64(n)
	return n, err
}

// fetch reads one small asset (the manifest or its signature) with a bounded body and its own deadline. The token,
// when there is one, goes only to the API host; the client drops it on a redirect to another host.
func (p *Provider) fetch(ctx context.Context, url string, max int64) ([]byte, error) {
	if !p.inAPI(url) {
		return nil, refuseAs(CodeAsset, "asset URL %q is outside the release API", url)
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	if t := p.cfg.Channel.Token; t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	client := *p.http
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) > 0 && !strings.EqualFold(via[len(via)-1].URL.Host, r.URL.Host) {
			r.Header.Del("Authorization")
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, refuseAs(CodeManifestInvalid, "response too large")
	}
	return b, nil
}

// checkRepoAccess asks GET {base}/repos/{repo} and wants 200.
func (p *Provider) checkRepoAccess(ctx context.Context) error {
	repo := p.cfg.Channel.Repository
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+"/repos/"+repo, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if p.cfg.Channel.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.Channel.Token)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("cannot confirm access to repository %s: %w", repo, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the test channel repository %s is not reachable with this token (HTTP %d): check the repository name and the token", repo, resp.StatusCode)
	}
	return nil
}
