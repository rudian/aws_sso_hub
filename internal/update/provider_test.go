package update

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

func checkReq() updater.CheckRequest {
	return updater.CheckRequest{CurrentVersion: "1.0.0-rc.1", Platform: "darwin", Arch: "arm64"}
}

func newProv(t *testing.T, f *fakeAPI, k testKeys, highest func() (string, error)) *Provider {
	t.Helper()
	p, err := NewProvider(ProviderConfig{Channel: f.channel(k, ""), Running: "1.0.0-rc.1", Highest: highest})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProviderGood(t *testing.T) {
	k := newTestKeys(t)
	for name, key := range map[string][]byte{"current key": k.cur, "second key (rotation)": k.next} {
		f := newFakeAPI(t)
		f.publish(relOpts{tag: "v1.0.0-rc.2", manifestVersion: "1.0.0-rc.2", signWith: key, notes: "Signed notes."})
		p := newProv(t, f, k, nil)
		rel, err := p.Check(context.Background(), checkReq())
		if err != nil || rel == nil {
			t.Fatalf("%s: %v", name, err)
		}
		if rel.Version != "1.0.0-rc.2" || rel.Notes != "Signed notes." {
			t.Errorf("%s: release %+v", name, rel)
		}
		if strings.Contains(rel.Notes, "UNSIGNED") {
			t.Errorf("%s: the API body leaked into the notes", name)
		}
		if len(rel.Verification.Digest) != 32 || len(rel.Verification.Signature) != 0 {
			t.Errorf("%s: digest not pinned from the manifest", name)
		}
		var buf bytes.Buffer
		if err := p.Download(context.Background(), rel, &buf, nil); err != nil || buf.String() != "archive bytes" {
			t.Errorf("%s: download: %v %q", name, err, buf.String())
		}
	}
}

func TestProviderRefusals(t *testing.T) {
	k := newTestKeys(t)
	rc2 := relOpts{tag: "v1.0.0-rc.2", manifestVersion: "1.0.0-rc.2", signWith: k.cur}
	cases := []struct {
		name    string
		opts    func(o *relOpts)
		highest func() (string, error)
		want    string
		code    string
	}{
		{"unpinned key", func(o *relOpts) { o.signWith = k.evil }, nil, "signature does not match", CodeManifestSignature},
		{"signature missing", func(o *relOpts) { o.dropSig = true }, nil, "no signed manifest", CodeManifestMissing},
		{"manifest missing", func(o *relOpts) { o.dropMan = true }, nil, "no signed manifest", CodeManifestMissing},
		{"garbage signature", func(o *relOpts) { o.rawSig = "AAAA" }, nil, "malformed signature", CodeManifestSignature},
		{"old version under a new tag", func(o *relOpts) { o.tag = "v1.0.0-rc.3" }, nil, "does not equal release tag", CodeVersionTag},
		{"version below the stored highest", nil, func() (string, error) { return "1.0.0-rc.5", nil }, "below the highest installed", CodeBelowHighest},
		{"stored highest unreadable", nil, func() (string, error) { return "", ErrHighestUnreadable }, "cannot be read", CodeHighestUnreadable},
		{"stored highest not a version", nil, func() (string, error) { return "garbage", nil }, "not semver", CodeHighestUnreadable},
		{"wrong platform", func(o *relOpts) { o.mutate = func(m *Manifest) { m.OS = "windows" } }, nil, "not darwin", CodePlatform},
		{"unknown manifest field", func(o *relOpts) {
			o.rawMan = func(b []byte) []byte {
				return []byte(strings.Replace(string(b), `"notes"`, `"url":"http://evil","notes"`, 1))
			}
		}, nil, "manifest", CodeManifestInvalid},
		{"zero size", func(o *relOpts) { o.mutate = func(m *Manifest) { m.Size = 0 } }, nil, "size", CodeManifestInvalid},
	}
	for _, c := range cases {
		f := newFakeAPI(t)
		o := rc2
		if c.opts != nil {
			c.opts(&o)
		}
		f.publish(o)
		p := newProv(t, f, k, c.highest)
		rel, err := p.Check(context.Background(), checkReq())
		if err == nil || rel != nil {
			t.Errorf("%s: accepted (%v)", c.name, rel)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: reason %q lacks %q", c.name, err, c.want)
		}
		if !IsRefusal(err) || p.Refusal() == nil {
			t.Errorf("%s: not reported as a security refusal: %v", c.name, err)
		}
		if got := RefusalCode(err); got != c.code {
			t.Errorf("%s: code %q, want %q", c.name, got, c.code)
		}
	}
}

func TestProviderAPIErrorIsNotARefusal(t *testing.T) {
	k := newTestKeys(t)
	f := newFakeAPI(t)
	f.publish(relOpts{tag: "v1.0.0-rc.2", manifestVersion: "1.0.0-rc.2", signWith: k.cur})
	f.down = true
	p := newProv(t, f, k, nil)
	_, err := p.Check(context.Background(), checkReq())
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("API 500 must be an error: %v", err)
	}
	if IsRefusal(err) {
		t.Error("a server error is not a security refusal")
	}
}

func TestProviderOldReleaseIsUpToDate(t *testing.T) {
	k := newTestKeys(t)
	f := newFakeAPI(t)
	f.publish(relOpts{tag: "v1.0.0-rc.1", manifestVersion: "1.0.0-rc.1", signWith: k.cur})
	rel, err := newProv(t, f, k, nil).Check(context.Background(), checkReq())
	if rel != nil || err != nil {
		t.Fatalf("the same version is up to date: %v %v", rel, err)
	}
}

func TestProviderRcTenSortsAboveRcTwo(t *testing.T) {
	k := newTestKeys(t)
	f := newFakeAPI(t)
	f.publish(relOpts{tag: "v1.0.0-rc.10", manifestVersion: "1.0.0-rc.10", signWith: k.cur})
	p, _ := NewProvider(ProviderConfig{Channel: f.channel(k, ""), Running: "1.0.0-rc.2", Highest: func() (string, error) { return "1.0.0-rc.2", nil }})
	req := checkReq()
	req.CurrentVersion = "1.0.0-rc.2"
	if rel, err := p.Check(context.Background(), req); err != nil || rel == nil {
		t.Fatalf("rc.10 must be newer than rc.2: %v", err)
	}
}

func TestProviderDownloadSizeAndDigest(t *testing.T) {
	k := newTestKeys(t)
	f := newFakeAPI(t)
	// the archive served is longer than the signed size
	f.publish(relOpts{tag: "v1.0.0-rc.2", manifestVersion: "1.0.0-rc.2", signWith: k.cur, archive: []byte("0123456789"),
		mutate: func(m *Manifest) { m.Size = 4 }})
	p := newProv(t, f, k, nil)
	rel, err := p.Check(context.Background(), checkReq())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	err = p.Download(context.Background(), rel, &buf, nil)
	if err == nil || !IsRefusal(err) || p.Refusal() == nil {
		t.Fatalf("an over-long archive must be refused: %v", err)
	}
	// shorter than signed
	f2 := newFakeAPI(t)
	f2.publish(relOpts{tag: "v1.0.0-rc.2", manifestVersion: "1.0.0-rc.2", signWith: k.cur, archive: []byte("0123"),
		mutate: func(m *Manifest) { m.Size = 10 }})
	p2 := newProv(t, f2, k, nil)
	rel2, err := p2.Check(context.Background(), checkReq())
	if err != nil {
		t.Fatal(err)
	}
	if err := p2.Download(context.Background(), rel2, &bytes.Buffer{}, nil); err == nil || !IsRefusal(err) {
		t.Fatalf("a short archive must be refused: %v", err)
	}
}

func TestProviderAssetOutsideAPIIsRefused(t *testing.T) {
	k := newTestKeys(t)
	f := newFakeAPI(t)
	f.publish(relOpts{tag: "v1.0.0-rc.2", manifestVersion: "1.0.0-rc.2", signWith: k.cur})
	p := newProv(t, f, k, nil)
	if _, err := p.fetch(context.Background(), "https://evil.example.com/x", 100); !IsRefusal(err) {
		t.Fatalf("foreign URL: %v", err)
	}
	if p.inAPI(f.srv.URL + "/repos/other/repo/releases/assets/x") {
		t.Error("another repository's asset URL accepted")
	}
}

func TestTokenIsSentToTheAPIOnlyAndNotToTheCDNHop(t *testing.T) {
	k := newTestKeys(t)
	f := newFakeAPI(t)
	f.publish(relOpts{tag: "v1.0.0-rc.2", manifestVersion: "1.0.0-rc.2", signWith: k.cur})
	tok := "github_pat_" + strings.Repeat("a", 30)
	p, err := NewProvider(ProviderConfig{Channel: f.channel(k, tok), Running: "1.0.0-rc.1"})
	if err != nil {
		t.Fatal(err)
	}
	rel, err := p.Check(context.Background(), checkReq())
	if err != nil {
		t.Fatal(err)
	}
	_ = p.Download(context.Background(), rel, &bytes.Buffer{}, nil)
	sent := 0
	for _, a := range f.auth {
		if strings.Contains(a, tok) {
			sent++
			if strings.HasPrefix(a, "/cdn/") {
				t.Errorf("token reached the CDN path: %s", a[:20])
			}
		}
	}
	if sent == 0 {
		t.Error("token never sent to the API")
	}
}

func TestProductionChannelIsPublic(t *testing.T) {
	k := newTestKeys(t)
	_, err := NewProvider(ProviderConfig{Channel: Channel{Name: "production", Repository: ProductionRepository, Keys: k.pubs, Token: "x"}})
	if err == nil {
		t.Error("the production channel must not take a token")
	}
	if _, err := NewProvider(ProviderConfig{Channel: Channel{Name: "test", Repository: "a/b"}}); err == nil {
		t.Error("a provider without a pinned key must not be built")
	}
	// until the key ceremony runs, the production channel refuses to exist
	if _, err := ProductionChannel(); err == nil && pinnedKeyHex[0] == "" {
		t.Error("production channel built without keys")
	}
	if ch, err := ProductionChannel(); err == nil && ch.Repository != "rudian/codepass" {
		t.Errorf("repository %q", ch.Repository)
	}
}

func TestParseKeys(t *testing.T) {
	if _, err := ParseKeys([]string{"zz"}); err == nil {
		t.Error("bad hex accepted")
	}
	if _, err := ParseKeys([]string{"abcd"}); err == nil {
		t.Error("short key accepted")
	}
	if _, err := ParseKeys(nil); err == nil {
		t.Error("no keys accepted")
	}
}

// A private repository the token cannot see answers 404, which the Wails provider reads as "no release".
func TestProviderWrongRepoOrTokenIsAnErrorNotUpToDate(t *testing.T) {
	k := newTestKeys(t)
	f := newFakeAPI(t)
	f.notFound = true
	p, err := NewProvider(ProviderConfig{Channel: f.channel(k, "github_pat_"+strings.Repeat("a", 30)), Running: "1.0.0-rc.1"})
	if err != nil {
		t.Fatal(err)
	}
	rel, err := p.Check(context.Background(), checkReq())
	if rel != nil || err == nil || !strings.Contains(err.Error(), testRepo) {
		t.Fatalf("a 404 with a token must be an error naming the repository: %v %v", rel, err)
	}
	if IsRefusal(err) {
		t.Error("not a security refusal")
	}
	// a test channel with no token is not "up to date" either
	if rel, err := newProv(t, f, k, nil).Check(context.Background(), checkReq()); rel != nil || err == nil {
		t.Errorf("test channel without a token: %v %v", rel, err)
	}
	// the public production channel needs no token: the same answer is just "no release"
	pub := f.channel(k, "")
	pub.Name = "production"
	pp, err := NewProvider(ProviderConfig{Channel: pub, Running: "1.0.0-rc.1"})
	if err != nil {
		t.Fatal(err)
	}
	if rel, err := pp.Check(context.Background(), checkReq()); rel != nil || err != nil {
		t.Errorf("public channel: %v %v", rel, err)
	}
	// and when the repository is reachable, an up-to-date app stays up to date
	f.notFound = false
	f.publish(relOpts{tag: "v1.0.0-rc.1", manifestVersion: "1.0.0-rc.1", signWith: k.cur})
	if rel, err := p.Check(context.Background(), checkReq()); rel != nil || err != nil {
		t.Errorf("reachable and current: %v %v", rel, err)
	}
}

func TestProviderUsesTheConfiguredProxy(t *testing.T) {
	k := newTestKeys(t)
	f := newFakeAPI(t)
	f.publish(relOpts{tag: "v1.0.0-rc.2", manifestVersion: "1.0.0-rc.2", signWith: k.cur})
	var calls atomic.Int32
	p, err := NewProvider(ProviderConfig{Channel: f.channel(k, ""), Running: "1.0.0-rc.1",
		Proxy: func(*http.Request) (*url.URL, error) { calls.Add(1); return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if rel, err := p.Check(context.Background(), checkReq()); err != nil || rel == nil {
		t.Fatalf("check: %v", err)
	}
	if calls.Load() == 0 {
		t.Error("the configured proxy function was never asked")
	}
}

// A peer that stays connected but stops sending ends the download with a plain network error.
func TestProviderDownloadIdleTimeout(t *testing.T) {
	k := newTestKeys(t)
	f := newFakeAPI(t)
	archive := bytes.Repeat([]byte("x"), 1<<16)
	f.publish(relOpts{tag: "v1.0.0-rc.2", manifestVersion: "1.0.0-rc.2", signWith: k.cur, archive: archive})
	p, err := NewProvider(ProviderConfig{Channel: f.channel(k, "tok"), Running: "1.0.0-rc.1", IdleTimeout: 300 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	rel, err := p.Check(context.Background(), checkReq())
	if err != nil || rel == nil {
		t.Fatal(err)
	}
	orig := f.cdn.Config.Handler
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	f.cdn.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ".zip") {
			orig.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(archive)))
		_, _ = w.Write(archive[:1000])
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	start := time.Now()
	err = p.Download(context.Background(), rel, &bytes.Buffer{}, nil)
	if err == nil || IsRefusal(err) || !strings.Contains(err.Error(), "no data received") {
		t.Fatalf("want a plain idle error, got %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("took %v", time.Since(start))
	}
	// the caller's own cancellation is not reported as an idle timeout
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	p2, _ := NewProvider(ProviderConfig{Channel: f.channel(k, "tok"), Running: "1.0.0-rc.1", IdleTimeout: time.Minute})
	if err := p2.Download(ctx, rel, &bytes.Buffer{}, nil); err == nil || strings.Contains(err.Error(), "no data received") {
		t.Errorf("cancel: %v", err)
	}
}

// GitHub returns the canonical case of owner and repository in asset URLs.
func TestProviderAssetURLRepositoryCaseInsensitive(t *testing.T) {
	k := newTestKeys(t)
	f := newFakeAPI(t)
	p := newProv(t, f, k, nil)
	base := f.srv.URL + "/repos/"
	if !p.inAPI(base + strings.ToUpper(testRepo) + "/releases/assets/1") {
		t.Error("canonical-case URL refused")
	}
	for _, bad := range []string{base + "acme/other/releases/assets/1", "https://evil.example/repos/" + testRepo + "/x", base + testRepo} {
		if p.inAPI(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}
