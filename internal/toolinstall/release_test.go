package toolinstall

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/netx"
)

func serveJSON(t *testing.T, status int, body string, hdr map[string]string) string {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range hdr {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s.URL
}

func TestFetchAsset(t *testing.T) {
	n := netx.New(netx.Options{})
	good := `{"tag_name":"7.0.13","assets":[{"name":"Homebrew.pkg","size":154292112,"digest":"sha256:f5dae55ec63149b50f9945fd3431a92e4ebd5614961d88e462563f2a1b7314e1","browser_download_url":"https://github.com/Homebrew/brew/releases/download/7.0.13/Homebrew.pkg"}]}`
	a, f := FetchAsset(context.Background(), n, serveJSON(t, 200, good, nil), "https://x.example.com/fallback")
	if f != nil || a.Size != 154292112 || a.Tag != "7.0.13" || a.Sha256 != "f5dae55ec63149b50f9945fd3431a92e4ebd5614961d88e462563f2a1b7314e1" || !strings.HasSuffix(a.URL, "/Homebrew.pkg") {
		t.Fatalf("%+v %v", a, f)
	}
	// rate limited: the fallback, with nothing known
	for name, c := range map[string]struct {
		status int
		hdr    map[string]string
	}{"403 with no quota": {403, map[string]string{"X-RateLimit-Remaining": "0"}}, "429": {429, nil}} {
		a, f := FetchAsset(context.Background(), n, serveJSON(t, c.status, "{}", c.hdr), "https://x.example.com/fallback")
		if f != nil || a.URL != "https://x.example.com/fallback" || a.Size != 0 || a.Sha256 != "" {
			t.Errorf("%s: %+v %v", name, a, f)
		}
	}
	// everything else is a failure with its own cause
	bad := map[string]struct {
		status int
		body   string
		want   api.FailureCause
	}{
		"403 for another reason":     {403, "{}", api.CauseNetwork},
		"407 from a proxy":           {407, "", api.CauseProxyAuth},
		"500":                        {500, "", api.CauseNetwork},
		"a web page instead of JSON": {200, "<html>captive portal</html>", api.CauseNetwork},
		"no Homebrew.pkg":            {200, `{"tag_name":"x","assets":[]}`, api.CauseInstallerFailed},
		"an unusable size":           {200, `{"assets":[{"name":"Homebrew.pkg","size":0,"browser_download_url":"https://x.example.com/p"}]}`, api.CauseDamagedDownload},
		"a size over the cap":        {200, `{"assets":[{"name":"Homebrew.pkg","size":5000000000,"browser_download_url":"https://x.example.com/p"}]}`, api.CauseDamagedDownload},
		"a plain http download URL":  {200, `{"assets":[{"name":"Homebrew.pkg","size":10,"browser_download_url":"http://x.example.com/p"}]}`, api.CauseInstallerFailed},
		"a file URL as the download": {200, `{"assets":[{"name":"Homebrew.pkg","size":10,"browser_download_url":"file:///etc/passwd"}]}`, api.CauseInstallerFailed},
	}
	for name, c := range bad {
		_, f := FetchAsset(context.Background(), n, serveJSON(t, c.status, c.body, nil), "https://x.example.com/fallback")
		if f == nil || f.Cause != c.want {
			t.Errorf("%s: %v", name, f)
		}
	}
	// a digest that is not 64 hex digits is ignored, not trusted
	odd := `{"assets":[{"name":"Homebrew.pkg","size":10,"digest":"md5:abc","browser_download_url":"https://x.example.com/p"}]}`
	if a, f := FetchAsset(context.Background(), n, serveJSON(t, 200, odd, nil), ""); f != nil || a.Sha256 != "" {
		t.Errorf("odd digest: %+v %v", a, f)
	}
	// the API URL itself must be https
	if _, f := FetchAsset(context.Background(), n, "http://api.example.com/x", ""); f == nil {
		t.Error("plain http API URL accepted")
	}
}

type failingNet struct{ err error }

func (f failingNet) Do(*http.Request) (*http.Response, error) { return nil, f.err }
func (f failingNet) Download() *http.Client {
	return &http.Client{Transport: roundTripFn(func(*http.Request) (*http.Response, error) { return nil, f.err })}
}
func (f failingNet) Proxy() netx.Config { return netx.Config{} }

type roundTripFn func(*http.Request) (*http.Response, error)

func (f roundTripFn) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNetworkErrorsAreClassifiedAndHideTheURL(t *testing.T) {
	for in, want := range map[string]api.FailureCause{
		"proxyconnect tcp: dial tcp 127.0.0.1:1: connect: connection refused":              api.CauseNetwork,
		"dial tcp: lookup x.invalid: no such host":                                         api.CauseNetwork,
		"tls: failed to verify certificate: x509: certificate signed by unknown authority": api.CauseNetwork,
		"Proxy Authentication Required":                                                    api.CauseProxyAuth,
	} {
		uerr := &url.Error{Op: "Get", URL: "https://user:secret@proxy.example.com/x", Err: errors.New(in)}
		_, f := FetchAsset(context.Background(), failingNet{uerr}, "https://api.example.com/x", "")
		if f == nil || f.Cause != want {
			t.Errorf("%q: %v", in, f)
			continue
		}
		if strings.Contains(f.Detail, "secret") || strings.Contains(f.Detail, "proxy.example.com/x") {
			t.Errorf("the URL leaked into %q", f.Detail)
		}
	}
	if _, f := Download(context.Background(), failingNet{errors.New("dial tcp: i/o timeout")}, Asset{URL: "https://x.example.com/p", Size: 5}, t.TempDir(), nil); f == nil || f.Cause != api.CauseNetwork {
		t.Errorf("download: %v", f)
	}
}

func TestDownloadRefusals(t *testing.T) {
	n := netx.New(netx.Options{})
	serve := func(status int, ct string, body string) string {
		return serveJSON(t, status, body, map[string]string{"Content-Type": ct})
	}
	cases := map[string]struct {
		url  string
		a    Asset
		want api.FailureCause
	}{
		"404":                   {serve(404, "text/plain", "no"), Asset{Size: 2}, api.CauseNetwork},
		"407":                   {serve(407, "text/plain", ""), Asset{Size: 2}, api.CauseProxyAuth},
		"web page with 200":     {serve(200, "text/html; charset=utf-8", "<html></html>"), Asset{Size: 13}, api.CauseDamagedDownload},
		"shorter than the API":  {serve(200, "application/octet-stream", "abc"), Asset{Size: 10}, api.CauseDamagedDownload},
		"longer than the API":   {serve(200, "application/octet-stream", "abcdefghijkl"), Asset{Size: 10}, api.CauseDamagedDownload},
		"digest differs":        {serve(200, "application/octet-stream", "abc"), Asset{Size: 3, Sha256: strings.Repeat("0", 64)}, api.CauseDamagedDownload},
		"unknown size over cap": {"", Asset{Size: MaxPkgBytes + 1}, api.CauseDamagedDownload},
	}
	for name, c := range cases {
		dir := t.TempDir()
		c.a.URL = c.url
		if name == "unknown size over cap" {
			c.a.URL = serve(200, "application/octet-stream", "x")
		}
		_, f := Download(context.Background(), n, c.a, dir, nil)
		if f == nil || f.Cause != c.want {
			t.Errorf("%s: %v", name, f)
		}
		if ents, _ := os.ReadDir(dir); len(ents) != 0 {
			t.Errorf("%s: leftovers %v", name, ents)
		}
	}
	// a good one is saved 0600 under its fixed name
	dir := filepath.Join(t.TempDir(), "downloads")
	p, f := Download(context.Background(), n, Asset{URL: serve(200, "application/octet-stream", "abc"), Size: 3, Sha256: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"}, dir, nil)
	if f != nil || filepath.Base(p) != "Homebrew.pkg" {
		t.Fatalf("%q %v", p, f)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode())
	}
	if st, _ := os.Stat(dir); st.Mode().Perm() != 0o700 {
		t.Errorf("folder mode %v", st.Mode())
	}
}

func TestPreCheckWithRecordedOutputs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script")
	}
	r := newRig(t)
	pk := r.pkgutil()
	write := func(name, body string) string {
		p := filepath.Join(r.dir, name)
		os.WriteFile(p, []byte(body), 0o600)
		return p
	}
	body := "FIXTURE:good\n" + strings.Repeat("z", 3000)
	good := write("good.pkg", body)
	sha := fileSHA256(good)
	size := int64(len(body))
	ctx := context.Background()
	if f := PreCheck(ctx, pk, PreCheckInput{Path: good, Team: HomebrewTeamID, Size: size, Sha256: sha}); f != nil {
		t.Fatalf("good: %v", f)
	}
	if f := PreCheck(ctx, pk, PreCheckInput{Path: good, Team: HomebrewTeamID}); f != nil {
		t.Fatalf("nothing known: %v", f)
	}
	cases := []struct {
		name string
		in   PreCheckInput
		want api.FailureCause
	}{
		{"wrong size", PreCheckInput{Path: good, Team: HomebrewTeamID, Size: size - 1}, api.CauseDamagedDownload},
		{"wrong digest", PreCheckInput{Path: good, Team: HomebrewTeamID, Sha256: strings.Repeat("0", 64)}, api.CauseDamagedDownload},
		{"wrong team", PreCheckInput{Path: good, Team: "ABCDEFGHIJ"}, api.CauseUnknownSigner},
		{"flipped byte", PreCheckInput{Path: write("f.pkg", "FIXTURE:invalid\n"+strings.Repeat("z", 3000)), Team: HomebrewTeamID}, api.CauseBadSignature},
		{"unsigned", PreCheckInput{Path: write("u.pkg", "FIXTURE:unsigned\n"), Team: HomebrewTeamID}, api.CauseBadSignature},
		{"web page", PreCheckInput{Path: write("h.pkg", "<html>407</html>"), Team: HomebrewTeamID}, api.CauseBadSignature},
		{"web page, size known", PreCheckInput{Path: write("h2.pkg", "<html>407</html>"), Team: HomebrewTeamID, Size: size}, api.CauseDamagedDownload},
		{"missing", PreCheckInput{Path: filepath.Join(r.dir, "none.pkg"), Team: HomebrewTeamID}, api.CauseDamagedDownload},
		{"a folder", PreCheckInput{Path: r.dir, Team: HomebrewTeamID}, api.CauseDamagedDownload},
	}
	link := filepath.Join(r.dir, "link.pkg")
	os.Symlink(good, link)
	cases = append(cases, struct {
		name string
		in   PreCheckInput
		want api.FailureCause
	}{"a symbolic link", PreCheckInput{Path: link, Team: HomebrewTeamID}, api.CauseDamagedDownload})
	for _, c := range cases {
		if f := PreCheck(ctx, pk, c.in); f == nil || f.Cause != c.want {
			t.Errorf("%s: %v", c.name, f)
		}
	}
}

// Real pkgutil on files made here: an unsigned re-wrap and a web page, as in the S0-04 test. These run on a Mac only.
func TestPreCheckWithRealPkgutil(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello"), 0o644)
	unsigned := filepath.Join(dir, "unsigned.pkg")
	if out, err := exec.Command("/usr/bin/pkgbuild", "--root", root, "--identifier", "com.example.test", "--version", "1", unsigned).CombinedOutput(); err != nil {
		t.Skipf("pkgbuild: %v %s", err, out)
	}
	html := filepath.Join(dir, "proxy.pkg")
	os.WriteFile(html, []byte("<html><body>407 Proxy Authentication Required</body></html>"), 0o600)
	for name, p := range map[string]string{"unsigned re-wrap": unsigned, "web page": html} {
		f := PreCheck(context.Background(), "", PreCheckInput{Path: p, Team: HomebrewTeamID})
		if f == nil || f.Cause != api.CauseBadSignature {
			t.Errorf("%s: %v", name, f)
		}
	}
}

// The real package, when the tester provides one (CODEPASS_TEST_HOMEBREW_PKG=/path/Homebrew.pkg). The file is
// only read; nothing is installed.
func TestPreCheckWithTheRealHomebrewPkg(t *testing.T) {
	p := os.Getenv("CODEPASS_TEST_HOMEBREW_PKG")
	if p == "" {
		t.Skip("set CODEPASS_TEST_HOMEBREW_PKG to the path of a downloaded Homebrew.pkg")
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if f := PreCheck(context.Background(), "", PreCheckInput{Path: p, Team: HomebrewTeamID, Size: st.Size()}); f != nil {
		t.Fatalf("the real package: %v", f)
	}
	if f := PreCheck(context.Background(), "", PreCheckInput{Path: p, Team: "ABCDEFGHIJ"}); f == nil || f.Cause != api.CauseUnknownSigner {
		t.Errorf("another team: %v", f)
	}
	if f := PreCheck(context.Background(), "", PreCheckInput{Path: p, Team: HomebrewTeamID, Size: st.Size() - 1}); f == nil || f.Cause != api.CauseDamagedDownload {
		t.Errorf("wrong size: %v", f)
	}
}
