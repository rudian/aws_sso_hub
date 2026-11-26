package browser

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A console sign-in link: a secret that must never appear in an error.
const consoleURL = "https://signin.aws.amazon.com/federation?Action=login&Issuer=x&Destination=y&SigninToken=SECRET-TOKEN-VALUE"

func TestCleanProxy(t *testing.T) {
	good := map[string]string{
		"192.0.2.10:3128":                         "192.0.2.10:3128",
		" 198.51.100.7:8080 ":                     "198.51.100.7:8080",
		"203.0.113.5:03128":                       "203.0.113.5:3128",
		"0.0.0.0:1":                               "0.0.0.0:1",
		"255.255.255.255:65535":                   "255.255.255.255:65535",
		"proxy.example.com:3128":                  "proxy.example.com:3128",
		"Proxy.Example.COM:3128":                  "proxy.example.com:3128",
		"localhost:3128":                          "localhost:3128",
		"egress-1.corp.example.net:80":            "egress-1.corp.example.net:80",
		"a1.example:3128":                         "a1.example:3128",
		strings.Repeat("a", 63) + ".example:3128": strings.Repeat("a", 63) + ".example:3128",
	}
	for in, want := range good {
		if got, ok := CleanProxy(in); !ok || got != want {
			t.Errorf("CleanProxy(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	bad := []string{
		"", "   ", "192.0.2.10", "192.0.2.10:", ":3128", "192.0.2.10:0", "192.0.2.10:65536", "192.0.2.10:123456",
		"192.0.2.10:+3128", "192.0.2.10:-1", "192.0.2.10:31 28", "192.0.2.10:3128/", "192.0.2.10:3128/path",
		"http://192.0.2.10:3128", "https://proxy.example.com:3128", "socks5://192.0.2.10:1080", "user@proxy.example.com:3128",
		"user:pass@proxy.example.com:3128", "proxy example.com:3128", "192.0.2.10 :3128", "192.0.2.10: 3128", "proxy\t.example.com:3128",
		"[2001:db8::1]:3128", "2001:db8::1:3128", "256.0.2.1:3128", "192.0.2:3128", "192.0.2.1.5:3128", "192.0.02.1:3128",
		"-proxy.example.com:3128", "proxy-.example.com:3128", "proxy..example.com:3128", "proxy.example.com.:3128", ".example.com:3128",
		"prøxy.example:3128", `proxy".example:3128`, "proxy_1.example:3128", "proxy.example.com:3128;DIRECT",
		strings.Repeat("a", 64) + ".example:3128", strings.Repeat("a.", 127) + "example:3128",
	}
	for _, in := range bad {
		if got, ok := CleanProxy(in); ok {
			t.Errorf("CleanProxy(%q) = %q, want refused", in, got)
		}
	}
}

// chromeOpener is a macOS Opener whose Chrome lives in a temporary Applications folder; open is recorded.
func chromeOpener(t *testing.T) (*Opener, *[]call, string) {
	t.Helper()
	root := t.TempDir()
	apps := filepath.Join(root, "Applications")
	chrome := filepath.Join(apps, chromeBundle)
	if err := os.MkdirAll(chrome, 0o755); err != nil {
		t.Fatal(err)
	}
	var calls []call
	o := &Opener{GOOS: "darwin", Home: filepath.Join(root, "home"), Applications: apps,
		Output: func(_ context.Context, name string, args ...string) ([]byte, error) {
			calls = append(calls, call{name, args})
			return nil, nil
		}}
	return o, &calls, chrome
}

func TestChromeApp(t *testing.T) {
	for _, goos := range []string{"windows", "linux"} {
		if _, err := (&Opener{GOOS: goos}).ChromeApp(); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%s: %v", goos, err)
		}
	}
	root := t.TempDir()
	home, apps := filepath.Join(root, "home"), filepath.Join(root, "Applications")
	o := &Opener{GOOS: "darwin", Home: home, Applications: apps}
	if _, err := o.ChromeApp(); !errors.Is(err, ErrNoChrome) {
		t.Fatalf("no Chrome: %v", err)
	}
	// a file of that name is not an app
	if err := os.MkdirAll(apps, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apps, chromeBundle), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := o.ChromeApp(); !errors.Is(err, ErrNoChrome) {
		t.Fatalf("a file is not Chrome: %v", err)
	}
	user := filepath.Join(home, "Applications", chromeBundle)
	if err := os.MkdirAll(user, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := o.ChromeApp(); err != nil || got != user {
		t.Fatalf("~/Applications: %q %v", got, err)
	}
	if err := os.Remove(filepath.Join(apps, chromeBundle)); err != nil {
		t.Fatal(err)
	}
	system := filepath.Join(apps, chromeBundle)
	if err := os.MkdirAll(system, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := o.ChromeApp(); err != nil || got != system {
		t.Fatalf("/Applications comes first: %q %v", got, err)
	}
}

// wantPAC is the proxy auto-config script for 203.0.113.5:3128, exactly as the console needs it.
const wantPAC = `function FindProxyForURL(url, host) {
  if (url.substring(0, 6) != "https:" && url.substring(0, 4) != "wss:") return "DIRECT";
  host = host.toLowerCase();
  var aws = ["amazonaws.com", "aws.amazon.com", "awsapps.com", "app.aws", "api.aws"];
  for (var i = 0; i < aws.length; i++) {
    if (host == aws[i] || dnsDomainIs(host, "." + aws[i])) return "PROXY 203.0.113.5:3128";
  }
  return "DIRECT";
}
`

func TestOpenChromeWithProxy(t *testing.T) {
	o, calls, chrome := chromeOpener(t)
	dir := filepath.Join(t.TempDir(), "console-browser", "203.0.113.5_3128")
	if err := o.OpenChromeWithProxy(context.Background(), consoleURL, dir, " 203.0.113.5:03128 "); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0].name != "/usr/bin/open" {
		t.Fatalf("calls %+v", *calls)
	}
	args := (*calls)[0].args
	if len(args) != 9 {
		t.Fatalf("args %q", args)
	}
	const prefix = "--proxy-pac-url=data:application/x-ns-proxy-autoconfig;base64,"
	pacArg := args[5]
	if !strings.HasPrefix(pacArg, prefix) {
		t.Fatalf("PAC argument %q", pacArg)
	}
	want := []string{"-n", "-a", chrome, "--args", "--user-data-dir=" + dir, pacArg, "--no-first-run", "--no-default-browser-check", consoleURL}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args\n got %q\nwant %q", args, want)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(pacArg, prefix))
	if err != nil {
		t.Fatalf("PAC is not base64: %v", err)
	}
	pac := string(raw)
	if pac != wantPAC {
		t.Fatalf("PAC\n%s", pac)
	}
	// The properties the console depends on, checked on their own as well.
	for _, d := range []string{`"amazonaws.com"`, `"aws.amazon.com"`, `"awsapps.com"`, `"app.aws"`, `"api.aws"`} {
		if !strings.Contains(pac, d) {
			t.Errorf("PAC lacks %s", d)
		}
	}
	if !strings.Contains(pac, `if (url.substring(0, 6) != "https:" && url.substring(0, 4) != "wss:") return "DIRECT";`) {
		t.Error("only https and wss may go through the proxy")
	}
	if strings.Count(pac, "PROXY") != 1 || !strings.Contains(pac, `return "PROXY 203.0.113.5:3128";`) {
		t.Error("AWS hosts must go through the proxy, with no DIRECT fallback")
	}
	if strings.Contains(pac, "; DIRECT") || strings.Contains(pac, "SOCKS") {
		t.Error("no fallback after the proxy")
	}
}

func TestOpenChromeWithProxyGuards(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "profile")
	check := func(name string, err error, want error, calls []call) {
		t.Helper()
		if want != nil && !errors.Is(err, want) || want == nil && err == nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(err.Error(), "SECRET-TOKEN-VALUE") || strings.Contains(err.Error(), "federation") {
			t.Fatalf("%s: the error quotes the link: %v", name, err)
		}
		if name != "launch fails" && len(calls) != 0 {
			t.Fatalf("%s: launched %+v", name, calls)
		}
	}

	for _, goos := range []string{"windows", "linux"} {
		o, calls, _ := chromeOpener(t)
		o.GOOS = goos
		check(goos, o.OpenChromeWithProxy(ctx, consoleURL, dir, "192.0.2.10:3128"), ErrUnsupported, *calls)
	}

	o, calls, chrome := chromeOpener(t)
	if err := os.RemoveAll(chrome); err != nil {
		t.Fatal(err)
	}
	check("no Chrome", o.OpenChromeWithProxy(ctx, consoleURL, dir, "192.0.2.10:3128"), ErrNoChrome, *calls)

	o, calls, _ = chromeOpener(t)
	check("http link", o.OpenChromeWithProxy(ctx, "http://signin.aws.amazon.com/federation?SigninToken=SECRET-TOKEN-VALUE", dir, "192.0.2.10:3128"), ErrInvalidURL, *calls)
	check("bad proxy", o.OpenChromeWithProxy(ctx, consoleURL, dir, "http://192.0.2.10:3128"), ErrInvalidProxy, *calls)
	check("relative profile", o.OpenChromeWithProxy(ctx, consoleURL, "relative/dir", "192.0.2.10:3128"), nil, *calls)

	o, calls, _ = chromeOpener(t)
	o.Output = func(_ context.Context, name string, args ...string) ([]byte, error) {
		*calls = append(*calls, call{name, args})
		return nil, errors.New("open exited with code 1")
	}
	check("launch fails", o.OpenChromeWithProxy(ctx, consoleURL, dir, "192.0.2.10:3128"), nil, *calls)
	if len(*calls) != 1 {
		t.Fatalf("launch attempts %d", len(*calls))
	}
}
