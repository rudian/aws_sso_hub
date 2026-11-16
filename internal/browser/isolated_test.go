package browser

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestOpenIsolatedDarwin(t *testing.T) {
	https := func(id string) string { return plist(`{"LSHandlerURLScheme":"https","LSHandlerRoleAll":"` + id + `"}`) }
	dir := t.TempDir()
	tests := []struct {
		name string
		pl   string
		args []string // nil: not isolated, nothing launched
	}{
		{"chrome", https("com.google.chrome"), []string{"-n", "-b", "com.google.chrome", "--args", "--user-data-dir=" + dir,
			"--no-first-run", "--no-default-browser-check", "--disable-search-engine-choice-screen", "--incognito", testURL}},
		{"edge", https("com.microsoft.edgemac"), []string{"-n", "-b", "com.microsoft.edgemac", "--args", "--user-data-dir=" + dir,
			"--no-first-run", "--no-default-browser-check", "--disable-search-engine-choice-screen", "--inprivate", testURL}},
		{"brave", https("com.brave.Browser"), []string{"-n", "-b", "com.brave.Browser", "--args", "--user-data-dir=" + dir,
			"--no-first-run", "--no-default-browser-check", "--disable-search-engine-choice-screen", "--incognito", testURL}},
		{"vivaldi has its own first run", https("com.vivaldi.Vivaldi"), nil},
		{"opera", https("com.operasoftware.Opera"), nil},
		{"firefox", https("org.mozilla.firefox"), nil},
		{"safari", https("com.apple.Safari"), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fake{outputs: map[string]string{"-convert": tc.pl}}
			w, err := f.opener("darwin").OpenIsolated(context.Background(), testURL, dir)
			if err != nil {
				t.Fatal(err)
			}
			if tc.args == nil {
				if w != nil || len(f.started) != 0 {
					t.Fatalf("must not launch: %v", f.started)
				}
				return
			}
			if w == nil || w.Dir() != dir || len(f.started) != 1 || !reflect.DeepEqual(f.started[0].args, tc.args) {
				t.Fatalf("%v %+v", w, f.started)
			}
		})
	}
}

func TestOpenIsolatedRespectsPolicyAndGuards(t *testing.T) {
	dir := t.TempDir()
	f := &fake{outputs: map[string]string{
		"-convert": plist(`{"LSHandlerURLScheme":"https","LSHandlerRoleAll":"com.google.Chrome"}`),
		"-extract": "1", // IncognitoModeAvailability = 1: private browsing turned off
	}}
	if w, err := f.opener("darwin").OpenIsolated(context.Background(), testURL, dir); w != nil || err != nil || len(f.started) != 0 {
		t.Fatalf("%v %v %v", w, err, f.started)
	}
	o := (&fake{}).opener("darwin")
	if _, err := o.OpenIsolated(context.Background(), "http://evil.example/", dir); !errors.Is(err, ErrInvalidURL) {
		t.Fatal(err)
	}
	if _, err := o.OpenIsolated(context.Background(), testURL, "relative/dir"); err == nil {
		t.Fatal("relative profile directory accepted")
	}
	if w, err := (&fake{}).opener("linux").OpenIsolated(context.Background(), testURL, dir); w != nil || err != nil {
		t.Fatal("unsupported system")
	}
}

func TestOpenIsolatedWindows(t *testing.T) {
	dir := `C:\Users\me\AppData\Roaming\AWS SSO Hub\signin-browser\s-1`
	var got []string
	o := winOpener(`"C:\Program Files\Google\Chrome\Application\chrome.exe" --single-argument %1`, nil, nil)
	o.StartPID = func(_ context.Context, name string, args ...string) (int, error) {
		got = append([]string{name}, args...)
		return 4242, nil
	}
	w, err := o.OpenIsolated(context.Background(), testURL, dir)
	want := []string{`C:\Program Files\Google\Chrome\Application\chrome.exe`, "--user-data-dir=" + dir, "--no-first-run",
		"--no-default-browser-check", "--disable-search-engine-choice-screen", "--incognito", testURL}
	if err != nil || w == nil || w.pid != 4242 || !reflect.DeepEqual(got, want) {
		t.Fatalf("%v %v %v", w, err, got)
	}
	// firefox keeps the shared private window
	o2 := winOpener(`"C:\Program Files\Mozilla Firefox\firefox.exe" -osint -url "%1"`, nil, nil)
	o2.StartPID = func(context.Context, string, ...string) (int, error) { t.Fatal("launched"); return 0, nil }
	if w, err := o2.OpenIsolated(context.Background(), testURL, dir); w != nil || err != nil {
		t.Fatal(w, err)
	}
}

func TestParsePS(t *testing.T) {
	dir := "/Users/me/Library/Application Support/AWS SSO Hub/signin-browser/s-1"
	out := strings.Join([]string{
		"  101   501 /Applications/Google Chrome.app/Contents/MacOS/Google Chrome --user-data-dir=" + dir + " --no-first-run --incognito https://x",
		"  102   501 /Applications/Google Chrome.app/Contents/Frameworks/Helper --type=renderer --user-data-dir=" + dir,
		"  103   502 /Applications/Google Chrome.app/Contents/MacOS/Google Chrome --user-data-dir=" + dir, // another user
		"  104   501 /Applications/Google Chrome.app/Contents/MacOS/Google Chrome --user-data-dir=" + dir + "-other",
		"  105   501 /Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"garbage",
		"",
	}, "\n")
	if got := parsePS(out, 501, dir); !reflect.DeepEqual(got, []int{101, 102}) {
		t.Fatalf("%v", got)
	}
}

func TestIsolatedCloseDarwin(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "s-1")
	os.MkdirAll(filepath.Join(dir, "Default"), 0o700)
	uid := strconv.Itoa(os.Getuid())
	alive := map[int]bool{301: true, 302: true}
	var sent []string
	o := &Opener{GOOS: "darwin",
		Output: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != psPath {
				return nil, errors.New("unexpected " + name)
			}
			var b strings.Builder
			for pid := range alive {
				b.WriteString(strconv.Itoa(pid) + " " + uid + " /Applications/Google Chrome.app/Contents/MacOS/Google Chrome --user-data-dir=" + dir + "\n")
			}
			return []byte(b.String()), nil
		},
		Signal: func(pid int, force bool) error {
			sent = append(sent, strconv.Itoa(pid)+map[bool]string{true: ":kill", false: ":term"}[force])
			delete(alive, pid) // quits on SIGTERM
			return nil
		},
	}
	w := &Isolated{o: o, goos: "darwin", dir: dir}
	if err := w.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 2 || !strings.HasSuffix(sent[0], ":term") || !strings.HasSuffix(sent[1], ":term") {
		t.Fatalf("%v", sent)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("profile directory must be deleted")
	}
	if err := w.Close(context.Background()); err != nil || len(sent) != 2 {
		t.Fatal("second Close must do nothing")
	}
}

func TestIsolatedCloseWindows(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "s-1")
	os.MkdirAll(dir, 0o700)
	var got []string
	o := &Opener{GOOS: "windows", Output: func(_ context.Context, name string, args ...string) ([]byte, error) {
		got = append([]string{name}, args...)
		return nil, nil
	}}
	w := &Isolated{o: o, goos: "windows", dir: dir, pid: 77}
	if err := w.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 || !strings.HasSuffix(got[0], `\System32\taskkill.exe`) || !reflect.DeepEqual(got[1:], []string{"/PID", "77", "/T", "/F"}) {
		t.Fatalf("%v", got)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("profile directory must be deleted")
	}
}

func TestRemoveStale(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"s-old", "s-keep"} {
		os.MkdirAll(filepath.Join(root, n, "Default"), 0o700)
	}
	os.WriteFile(filepath.Join(root, "note.txt"), []byte("x"), 0o600)
	o := &Opener{GOOS: "darwin", Output: func(context.Context, string, ...string) ([]byte, error) { return nil, nil }}
	o.RemoveStale(context.Background(), root, func(dir string) bool { return filepath.Base(dir) == "s-keep" })
	if _, err := os.Stat(filepath.Join(root, "s-old")); !os.IsNotExist(err) {
		t.Fatal("stale profile kept")
	}
	if _, err := os.Stat(filepath.Join(root, "s-keep")); err != nil {
		t.Fatal("claimed profile removed")
	}
	if _, err := os.Stat(filepath.Join(root, "note.txt")); err != nil {
		t.Fatal("only directories are removed")
	}
}
