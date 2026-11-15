package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeAPI is a tiny GitHub releases API: the list, asset URLs that redirect to a "CDN" path, a hostile release body.
type fakeAPI struct {
	t        *testing.T
	srv, cdn *httptest.Server
	repo     string
	mu       sync.Mutex
	releases []*fakeRelease
	down     bool
	notFound bool     // a private repository the token cannot see: GitHub answers 404 everywhere
	auth     []string // Authorization headers seen on asset and list requests, in order
}

type fakeRelease struct {
	Tag    string
	Body   string
	Assets map[string][]byte
}

const testRepo = "acme/codepass-test"

func newFakeAPI(t *testing.T) *fakeAPI {
	f := &fakeAPI{t: t, repo: testRepo}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/"+f.repo, func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		f.mu.Lock()
		nf := f.notFound
		f.mu.Unlock()
		if nf {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"full_name":"` + f.repo + `"}`))
	})
	mux.HandleFunc("/repos/"+f.repo+"/releases", f.list)
	mux.HandleFunc("/repos/"+f.repo+"/releases/latest", f.latest)
	prefix := "/repos/" + f.repo + "/releases/assets/"
	mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		http.Redirect(w, r, f.cdn.URL+"/cdn/"+strings.TrimPrefix(r.URL.Path, prefix), http.StatusFound)
	})
	cdn := http.NewServeMux()
	cdn.HandleFunc("/cdn/", func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/cdn/"), "/", 2)
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, rel := range f.releases {
			if rel.Tag == parts[0] && len(parts) == 2 {
				if b, ok := rel.Assets[parts[1]]; ok {
					w.Header().Set("Content-Type", "application/octet-stream")
					_, _ = w.Write(b)
					return
				}
			}
		}
		http.NotFound(w, r)
	})
	f.srv = httptest.NewServer(mux)
	f.cdn = httptest.NewServer(cdn) // another host:port, like the real CDN hop
	t.Cleanup(f.srv.Close)
	t.Cleanup(f.cdn.Close)
	return f
}

func (f *fakeAPI) record(r *http.Request) {
	f.mu.Lock()
	f.auth = append(f.auth, r.URL.Path+" "+r.Header.Get("Authorization"))
	f.mu.Unlock()
}

func (f *fakeAPI) json() []map[string]any {
	var out []map[string]any
	for _, rel := range f.releases {
		var assets []map[string]any
		for name, b := range rel.Assets {
			assets = append(assets, map[string]any{"id": 1, "name": name, "size": len(b),
				"url": f.srv.URL + "/repos/" + f.repo + "/releases/assets/" + rel.Tag + "/" + name})
		}
		out = append(out, map[string]any{"tag_name": rel.Tag, "name": rel.Tag, "body": rel.Body, "prerelease": true, "draft": false, "assets": assets})
	}
	return out
}

func (f *fakeAPI) list(w http.ResponseWriter, r *http.Request) {
	f.record(r)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.notFound {
		http.NotFound(w, r)
		return
	}
	if f.down {
		http.Error(w, "boom", 500)
		return
	}
	_ = json.NewEncoder(w).Encode(f.json())
}

func (f *fakeAPI) latest(w http.ResponseWriter, r *http.Request) {
	f.record(r)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down || len(f.releases) == 0 {
		http.Error(w, "boom", 500)
		return
	}
	_ = json.NewEncoder(w).Encode(f.json()[0])
}

func (f *fakeAPI) add(rel *fakeRelease) {
	f.mu.Lock()
	f.releases = append([]*fakeRelease{rel}, f.releases...)
	f.mu.Unlock()
}

// release builds a signed release for darwin/arm64 around archive.
type relOpts struct {
	tag, manifestVersion string
	archive              []byte
	notes                string
	signWith             ed25519.PrivateKey
	mutate               func(*Manifest)
	dropSig, dropMan     bool
	rawSig               string
	rawMan               func([]byte) []byte // rewrites the manifest bytes before they are signed
}

func (f *fakeAPI) publish(o relOpts) {
	f.t.Helper()
	if o.archive == nil {
		o.archive = []byte("archive bytes")
	}
	sum := sha256.Sum256(o.archive)
	m := Manifest{Version: o.manifestVersion, OS: "darwin", Arch: "arm64", File: "Codepass-darwin-arm64.zip",
		Size: int64(len(o.archive)), SHA256: hex.EncodeToString(sum[:]), Notes: o.notes}
	if o.mutate != nil {
		o.mutate(&m)
	}
	raw, _ := json.Marshal(m)
	if o.rawMan != nil {
		raw = o.rawMan(raw)
	}
	assets := map[string][]byte{m.File: o.archive}
	if !o.dropMan {
		assets["manifest-darwin-arm64.json"] = raw
	}
	if !o.dropSig {
		sig := SignManifest(o.signWith, raw)
		if o.rawSig != "" {
			sig = o.rawSig
		}
		assets["manifest-darwin-arm64.json.sig"] = []byte(sig)
	}
	f.add(&fakeRelease{Tag: o.tag, Body: "UNSIGNED NOTES FROM THE API", Assets: assets})
}

type testKeys struct {
	cur, next, evil ed25519.PrivateKey
	pubs            []ed25519.PublicKey
}

func newTestKeys(t *testing.T) testKeys {
	t.Helper()
	p1, k1, _ := ed25519.GenerateKey(rand.Reader)
	p2, k2, _ := ed25519.GenerateKey(rand.Reader)
	_, k3, _ := ed25519.GenerateKey(rand.Reader)
	return testKeys{cur: k1, next: k2, evil: k3, pubs: []ed25519.PublicKey{p1, p2}}
}

func (f *fakeAPI) channel(k testKeys, token string) Channel {
	return Channel{Name: "test", Repository: f.repo, Prerelease: true, Keys: k.pubs, Token: token, BaseURL: f.srv.URL}
}
