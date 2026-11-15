package toolinstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/netx"
)

const (
	// ReleaseAPIURL names the latest Homebrew/brew release. Size and sha256 of Homebrew.pkg come from it.
	ReleaseAPIURL = "https://api.github.com/repos/Homebrew/brew/releases/latest"
	// FallbackURL is used when the API is rate limited: no size and no digest, so the signature check carries it.
	FallbackURL = "https://github.com/Homebrew/brew/releases/latest/download/Homebrew.pkg"
	pkgAsset    = "Homebrew.pkg"
)

// Net is what the downloads need from internal/netx: the one HTTP client (with the system proxy) and the proxy
// setting for child processes. *netx.Client implements it.
type Net interface {
	Do(req *http.Request) (*http.Response, error)
	Download() *http.Client
	Proxy() netx.Config
}

// Asset is the package to download. Size and Sha256 are zero values when the API could not be read.
type Asset struct {
	Tag    string
	URL    string
	Size   int64
	Sha256 string
}

type ghRelease struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name   string `json:"name"`
		Size   int64  `json:"size"`
		Digest string `json:"digest"`
		URL    string `json:"browser_download_url"`
	} `json:"assets"`
}

// allowedURL accepts https only; plain http is accepted for a loopback address, which is how the tests run a
// local server. The real asset URL comes from GitHub's API over TLS, and the pinned Team ID is the real defence.
func allowedURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("bad download URL")
	}
	if u.Scheme == "https" {
		return nil
	}
	if h := u.Hostname(); u.Scheme == "http" && (h == "127.0.0.1" || h == "localhost" || h == "::1") {
		return nil
	}
	return errors.New("the download URL is not https")
}

// networkFailure turns a Go transport error or an unexpected status into a Failure.
func networkFailure(err error, status int) *Failure {
	if status == 407 {
		return fail(api.CauseProxyAuth, "HTTP 407")
	}
	if status != 0 {
		return fail(api.CauseNetwork, "HTTP %d", status)
	}
	c := GoNet(err.Error())
	if c == api.CauseUnknown {
		c = api.CauseNetwork
	}
	return fail(c, "%v", scrubURLError(err))
}

// scrubURLError drops the URL (which could hold proxy credentials) from a *url.Error.
func scrubURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s: %v", ue.Op, ue.Err)
	}
	return err
}

// FetchAsset reads the latest release from the API. On a rate limit (403 with no quota left, or 429) it returns
// the fallback URL with unknown size and digest. Any other failure is a network failure.
func FetchAsset(ctx context.Context, n Net, apiURL, fallbackURL string) (Asset, *Failure) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := allowedURL(apiURL); err != nil {
		return Asset{}, fail(api.CauseInstallerFailed, "%v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return Asset{}, fail(api.CauseInstallerFailed, "%v", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := n.Do(req)
	if err != nil {
		return Asset{}, networkFailure(err, 0)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0") {
		return Asset{URL: fallbackURL}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return Asset{}, networkFailure(nil, resp.StatusCode)
	}
	var rel ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return Asset{}, fail(api.CauseNetwork, "the release answer is not JSON (a proxy page?)")
	}
	for _, a := range rel.Assets {
		if a.Name != pkgAsset {
			continue
		}
		if err := allowedURL(a.URL); err != nil {
			return Asset{}, fail(api.CauseInstallerFailed, "%v", err)
		}
		as := Asset{Tag: rel.Tag, URL: a.URL, Size: a.Size}
		if h, ok := strings.CutPrefix(a.Digest, "sha256:"); ok && len(h) == 64 {
			as.Sha256 = strings.ToLower(h)
		}
		if as.Size <= 0 || as.Size > MaxPkgBytes {
			return Asset{}, fail(api.CauseDamagedDownload, "the release lists an unusable size (%d)", as.Size)
		}
		return as, nil
	}
	return Asset{}, fail(api.CauseInstallerFailed, "the latest release has no %s", pkgAsset)
}

// Progress reports bytes done and the expected total (0 when unknown).
type Progress func(done, total int64)

// stallTimeout is how long a download may go without a single byte.
var stallTimeout = 45 * time.Second

// Download saves the asset to dir/Homebrew.pkg (mode 0600, replacing an earlier file of that name) and returns
// the path. The folder must be the app's own private folder. A response that is not 200, is a web page, is
// longer than the cap, or differs from the expected size or digest is a failure; the partial file is removed.
func Download(ctx context.Context, n Net, a Asset, dir string, progress Progress) (string, *Failure) {
	if err := allowedURL(a.URL); err != nil {
		return "", fail(api.CauseInstallerFailed, "%v", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fail(causeOfFS(err), "cannot create the download folder: %v", err)
	}
	if st, err := os.Lstat(dir); err != nil || !st.IsDir() {
		return "", fail(api.CauseInstallerFailed, "the download folder is not a folder")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return "", fail(api.CauseInstallerFailed, "%v", err)
	}
	resp, err := n.Download().Do(req)
	if err != nil {
		return "", networkFailure(err, 0)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", networkFailure(nil, resp.StatusCode)
	}
	if ct := strings.ToLower(resp.Header.Get("Content-Type")); strings.HasPrefix(ct, "text/html") {
		return "", fail(api.CauseDamagedDownload, "the server sent a web page, not the package")
	}
	total := a.Size
	if total <= 0 && resp.ContentLength > 0 {
		total = resp.ContentLength
	}
	if total > MaxPkgBytes {
		return "", fail(api.CauseDamagedDownload, "the package is larger than %d bytes", MaxPkgBytes)
	}
	f, err := os.CreateTemp(dir, "Homebrew-*.pkg.part")
	if err != nil {
		return "", fail(causeOfFS(err), "cannot create the download file: %v", err)
	}
	part := f.Name()
	keep := false
	defer func() {
		f.Close()
		if !keep {
			os.Remove(part) // our own file, by its exact name
		}
	}()
	// A watchdog stops a download that has gone quiet; the context is also the caller's cancel.
	timeout := stallTimeout
	var stalled atomic.Bool
	beat := make(chan struct{}, 1)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		t := time.NewTimer(timeout)
		defer t.Stop()
		for {
			select {
			case <-beat:
				if !t.Stop() {
					select {
					case <-t.C:
					default:
					}
				}
				t.Reset(timeout)
			case <-t.C:
				stalled.Store(true)
				cancel()
				return
			case <-stop:
				return
			}
		}
	}()
	h := sha256.New()
	var done int64
	buf := make([]byte, 256<<10)
	lr := io.LimitReader(resp.Body, MaxPkgBytes+1)
	for {
		nr, rerr := lr.Read(buf)
		if nr > 0 {
			if _, werr := f.Write(buf[:nr]); werr != nil {
				return "", fail(causeOfFS(werr), "cannot write the download: %v", werr)
			}
			h.Write(buf[:nr])
			done += int64(nr)
			select {
			case beat <- struct{}{}:
			default:
			}
			if progress != nil {
				progress(done, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			if stalled.Load() {
				return "", fail(api.CauseNetwork, "no data for %s", timeout)
			}
			return "", networkFailure(rerr, 0)
		}
	}
	if err := f.Close(); err != nil {
		return "", fail(causeOfFS(err), "cannot write the download: %v", err)
	}
	if done > MaxPkgBytes {
		return "", fail(api.CauseDamagedDownload, "the download is larger than %d bytes", MaxPkgBytes)
	}
	if a.Size > 0 && done != a.Size {
		return "", fail(api.CauseDamagedDownload, "size %d, expected %d", done, a.Size)
	}
	if got := hex.EncodeToString(h.Sum(nil)); a.Sha256 != "" && got != a.Sha256 {
		return "", fail(api.CauseDamagedDownload, "digest %s, expected %s", got, a.Sha256)
	}
	final := filepath.Join(dir, pkgAsset)
	if err := os.Chmod(part, 0o600); err != nil {
		return "", fail(api.CauseInstallerFailed, "%v", err)
	}
	if err := os.Rename(part, final); err != nil {
		return "", fail(causeOfFS(err), "cannot save the download: %v", err)
	}
	keep = true
	return final, nil
}

func causeOfFS(err error) api.FailureCause {
	if strings.Contains(strings.ToLower(err.Error()), "no space left") {
		return api.CauseDiskFull
	}
	return api.CauseInstallerFailed
}
