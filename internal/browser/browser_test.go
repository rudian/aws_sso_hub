package browser

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const testURL = "https://device.sso.us-east-1.amazonaws.com/?user_code=ABCD-EFGH"

type call struct {
	name string
	args []string
}

type fake struct {
	outputs map[string]string // key: substring of the joined args
	outErr  error
	startE  error
	started []call
}

func (f *fake) opener(goos string) *Opener {
	return &Opener{
		GOOS: goos, Home: "/Users/me",
		Output: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if f.outErr != nil {
				return nil, f.outErr
			}
			if name == "/usr/bin/open" {
				f.started = append(f.started, call{name, args})
				return nil, f.startE
			}
			joined := name + " " + strings.Join(args, " ")
			for k, v := range f.outputs {
				if strings.Contains(joined, k) {
					return []byte(v), nil
				}
			}
			return nil, errors.New("unexpected command: " + joined)
		},
		Start: func(_ context.Context, name string, args ...string) error {
			f.started = append(f.started, call{name, args})
			return f.startE
		},
	}
}

func plist(handlers string) string { return `{"LSHandlers":[` + handlers + `]}` }

func TestOpenPrivateDarwin(t *testing.T) {
	https := func(id string) string {
		return `{"LSHandlerURLScheme":"https","LSHandlerRoleAll":"` + id + `"}`
	}
	other := `{"LSHandlerURLScheme":"mailto","LSHandlerRoleAll":"com.apple.mail"},{"LSHandlerContentType":"x","LSHandlerRoleAll":"com.foo"}`
	tests := []struct {
		name    string
		plist   string
		want    bool
		wantErr bool
		args    []string
	}{
		{"chrome https", plist(other + "," + https("com.google.chrome")), true, false,
			[]string{"-n", "-b", "com.google.chrome", "--args", "--incognito", testURL}},
		{"upper-case ids", plist(https("COM.GOOGLE.Chrome")), true, false,
			[]string{"-n", "-b", "COM.GOOGLE.Chrome", "--args", "--incognito", testURL}},
		{"chrome canary", plist(https("com.google.Chrome.canary")), true, false,
			[]string{"-n", "-b", "com.google.Chrome.canary", "--args", "--incognito", testURL}},
		{"edge", plist(https("com.microsoft.edgemac")), true, false,
			[]string{"-n", "-b", "com.microsoft.edgemac", "--args", "--inprivate", testURL}},
		{"brave", plist(https("com.brave.Browser")), true, false,
			[]string{"-n", "-b", "com.brave.Browser", "--args", "--incognito", testURL}},
		{"vivaldi", plist(https("com.vivaldi.Vivaldi")), true, false,
			[]string{"-n", "-b", "com.vivaldi.Vivaldi", "--args", "--incognito", testURL}},
		{"opera", plist(https("com.operasoftware.Opera")), true, false,
			[]string{"-n", "-b", "com.operasoftware.Opera", "--args", "--private", testURL}},
		{"firefox is not opened privately on macOS", plist(https("org.mozilla.firefox")), false, false, nil},
		{"http handler only", plist(`{"LSHandlerURLScheme":"http","LSHandlerRoleAll":"com.microsoft.edgemac"}`), true, false,
			[]string{"-n", "-b", "com.microsoft.edgemac", "--args", "--inprivate", testURL}},
		{"https wins over http", plist(`{"LSHandlerURLScheme":"http","LSHandlerRoleAll":"com.apple.safari"},` + https("com.brave.browser")), true, false,
			[]string{"-n", "-b", "com.brave.browser", "--args", "--incognito", testURL}},
		{"no handler means safari", plist(other), false, false, nil},
		{"safari", plist(https("com.apple.Safari")), false, false, nil},
		{"arc", plist(https("company.thebrowser.Browser")), false, false, nil},
		{"bad json", "not json", false, true, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fake{outputs: map[string]string{"-convert": tc.plist}}
			got, err := f.opener("darwin").OpenPrivate(context.Background(), testURL)
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("got (%v,%v) want (%v, err=%v)", got, err, tc.want, tc.wantErr)
			}
			if tc.args == nil {
				if len(f.started) != 0 {
					t.Fatalf("must not launch: %v", f.started)
				}
				return
			}
			if len(f.started) != 1 || f.started[0].name != "/usr/bin/open" || !reflect.DeepEqual(f.started[0].args, tc.args) {
				t.Fatalf("started %+v want args %v", f.started, tc.args)
			}
		})
	}
}

func TestDarwinPlutilArgs(t *testing.T) {
	var gotName string
	var gotArgs []string
	o := &Opener{GOOS: "darwin", Home: "/Users/me",
		Output: func(_ context.Context, name string, args ...string) ([]byte, error) {
			gotName, gotArgs = name, args
			return []byte(plist("")), nil
		}}
	if _, err := o.OpenPrivate(context.Background(), testURL); err != nil {
		t.Fatal(err)
	}
	want := []string{"-convert", "json", "-o", "-", "/Users/me/Library/Preferences/com.apple.LaunchServices/com.apple.launchservices.secure.plist"}
	if gotName != "/usr/bin/plutil" || !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("%s %v", gotName, gotArgs)
	}
}

func winOpener(cmd string, pol map[string]uint32, start func(string, ...string) error) *Opener {
	return &Opener{GOOS: "windows",
		WinDefaultCommand: func() (string, error) { return cmd, nil },
		WinPolicy: func(sub, name string) (uint32, bool) {
			v, ok := pol[sub+"|"+name]
			return v, ok
		},
		Start: func(_ context.Context, n string, a ...string) error { return start(n, a...) }}
}

func TestOpenPrivateWindows(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		want    bool
		wantExe string
		flag    string
	}{
		{"chrome quoted with args", `"C:\Program Files\Google\Chrome\Application\chrome.exe" --single-argument %1`, true,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`, "--incognito"},
		{"edge", `"C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe" --single-argument %1`, true,
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`, "--inprivate"},
		{"brave", `"C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe" -- "%1"`, true,
			`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`, "--incognito"},
		{"firefox", `"C:\Program Files\Mozilla Firefox\firefox.exe" -osint -url "%1"`, true,
			`C:\Program Files\Mozilla Firefox\firefox.exe`, "--private-window"},
		{"vivaldi unquoted", `C:\Users\me\AppData\Local\Vivaldi\Application\vivaldi.exe -- %1`, true,
			`C:\Users\me\AppData\Local\Vivaldi\Application\vivaldi.exe`, "--incognito"},
		{"non-ASCII profile path", `"C:\Users\张三\AppData\Local\Google\Chrome\Application\chrome.exe" %1`, true,
			`C:\Users\张三\AppData\Local\Google\Chrome\Application\chrome.exe`, "--incognito"},
		{"opera launcher", `"C:\Users\me\AppData\Local\Programs\Opera\launcher.exe" -- "%1"`, true,
			`C:\Users\me\AppData\Local\Programs\Opera\launcher.exe`, "--private"},
		{"relative program path", `chrome.exe --single-argument %1`, false, "", ""},
		{"relative quoted path", `"Chrome\Application\chrome.exe" %1`, false, "", ""},
		{"unknown launcher", `"C:\Tools\launcher.exe" "%1"`, false, "", ""},
		{"unknown browser", `"C:\Program Files\Arc\arc.exe" "%1"`, false, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var started []call
			o := winOpener(tc.cmd, nil, func(n string, a ...string) error { started = append(started, call{n, a}); return nil })
			got, err := o.OpenPrivate(context.Background(), testURL)
			if err != nil || got != tc.want {
				t.Fatalf("got (%v,%v)", got, err)
			}
			if !tc.want {
				if len(started) != 0 {
					t.Fatalf("must not launch: %v", started)
				}
				return
			}
			if len(started) != 1 || started[0].name != tc.wantExe || !reflect.DeepEqual(started[0].args, []string{tc.flag, testURL}) {
				t.Fatalf("started %+v", started)
			}
		})
	}
}

func TestWindowsPrivateBrowsingPolicy(t *testing.T) {
	chromeCmd := `"C:\x\chrome.exe" %1`
	edgeCmd := `"C:\x\msedge.exe" %1`
	ffCmd := `"C:\x\firefox.exe" %1`
	tests := []struct {
		name string
		cmd  string
		pol  map[string]uint32
		want bool
	}{
		{"chrome disabled", chromeCmd, map[string]uint32{`SOFTWARE\Policies\Google\Chrome|IncognitoModeAvailability`: 1}, false},
		{"chrome forced is fine", chromeCmd, map[string]uint32{`SOFTWARE\Policies\Google\Chrome|IncognitoModeAvailability`: 2}, true},
		{"chrome enabled", chromeCmd, map[string]uint32{`SOFTWARE\Policies\Google\Chrome|IncognitoModeAvailability`: 0}, true},
		{"edge disabled", edgeCmd, map[string]uint32{`SOFTWARE\Policies\Microsoft\Edge|InPrivateModeAvailability`: 1}, false},
		{"firefox disabled", ffCmd, map[string]uint32{`SOFTWARE\Policies\Mozilla\Firefox|DisablePrivateBrowsing`: 1}, false},
		{"other browser's policy ignored", edgeCmd, map[string]uint32{`SOFTWARE\Policies\Google\Chrome|IncognitoModeAvailability`: 1}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := 0
			o := winOpener(tc.cmd, tc.pol, func(string, ...string) error { n++; return nil })
			got, err := o.OpenPrivate(context.Background(), testURL)
			if err != nil || got != tc.want || (n == 1) != tc.want {
				t.Fatalf("got (%v,%v) launches=%d", got, err, n)
			}
		})
	}
}

func TestDarwinPrivateBrowsingPolicy(t *testing.T) {
	handler := plist(`{"LSHandlerURLScheme":"https","LSHandlerRoleAll":"com.google.chrome"}`)
	for _, tc := range []struct {
		name, extract string
		want          bool
	}{{"disabled", "1", false}, {"enabled", "0", true}, {"unset", "", true}} {
		t.Run(tc.name, func(t *testing.T) {
			var opened int
			o := &Opener{GOOS: "darwin", Home: "/Users/me",
				Output: func(_ context.Context, name string, args ...string) ([]byte, error) {
					if name == "/usr/bin/open" {
						opened++
						return nil, nil
					}
					if len(args) > 0 && args[0] == "-extract" {
						if tc.extract == "" {
							return nil, errors.New("no such key")
						}
						return []byte(tc.extract + "\n"), nil
					}
					return []byte(handler), nil
				}}
			got, err := o.OpenPrivate(context.Background(), testURL)
			if err != nil || got != tc.want || (opened == 1) != tc.want {
				t.Fatalf("got (%v,%v) opened=%d", got, err, opened)
			}
		})
	}
}

func TestOpenPrivateErrorsAndGuards(t *testing.T) {
	boom := errors.New("boom")
	f0 := &fake{outErr: boom}
	if got, err := f0.opener("darwin").OpenPrivate(context.Background(), testURL); got || !errors.Is(err, boom) || len(f0.started) != 0 {
		t.Fatalf("darwin detection error: (%v,%v)", got, err)
	}
	wo := &Opener{GOOS: "windows", WinDefaultCommand: func() (string, error) { return "", boom }}
	if got, err := wo.OpenPrivate(context.Background(), testURL); got || !errors.Is(err, boom) {
		t.Fatalf("windows detection error: (%v,%v)", got, err)
	}
	// launch error (open exits non-zero)
	f := &fake{outputs: map[string]string{"-convert": plist(`{"LSHandlerURLScheme":"https","LSHandlerRoleAll":"com.google.chrome"}`)}, startE: boom}
	if got, err := f.opener("darwin").OpenPrivate(context.Background(), testURL); got || !errors.Is(err, boom) {
		t.Fatalf("launch error: (%v,%v)", got, err)
	}
	// other systems
	f = &fake{}
	if got, err := f.opener("linux").OpenPrivate(context.Background(), testURL); got || err != nil || len(f.started) != 0 {
		t.Fatalf("linux: (%v,%v)", got, err)
	}
	// URL checks run before anything else
	for _, bad := range []string{"", "http://example.com", "ftp://x", "https://", "/relative", "javascript:alert(1)", "https://a.com/\nx", "-n"} {
		f := &fake{}
		got, err := f.opener("darwin").OpenPrivate(context.Background(), bad)
		if got || !errors.Is(err, ErrInvalidURL) || len(f.started) != 0 {
			t.Fatalf("%q: (%v,%v)", bad, got, err)
		}
	}
}

func TestCommandHelpers(t *testing.T) {
	if got := exeFromCommand(`C:\Program Files\X\x.exe --a %1`); got != `C:\Program Files\X\x.exe` {
		t.Fatal(got)
	}
	if exeFromCommand(`"unterminated`) != "" || exeFromCommand("nothing") != "" {
		t.Fatal("expected empty")
	}
}

func TestWinAbs(t *testing.T) {
	for p, want := range map[string]bool{
		`C:\a\chrome.exe`: true, `c:/a/chrome.exe`: true, `\\server\share\chrome.exe`: true,
		`chrome.exe`: false, `.\chrome.exe`: false, `\chrome.exe`: false, `C:chrome.exe`: false, ``: false,
	} {
		if winAbs(p) != want {
			t.Errorf("winAbs(%q) = %v", p, !want)
		}
	}
}

func TestPickProgIDPrefersUserChoiceLatest(t *testing.T) {
	latest, plain := winChoiceKeys[0], winChoiceKeys[1]
	if !strings.HasSuffix(latest, `UserChoiceLatest`) || !strings.HasSuffix(plain, `UserChoice`) {
		t.Fatalf("keys %v", winChoiceKeys)
	}
	notFound := errors.New("not found")
	mk := func(m map[string]string) func(string) (string, error) {
		return func(k string) (string, error) {
			if v, ok := m[k]; ok {
				return v, nil
			}
			return "", notFound
		}
	}
	for _, tc := range []struct {
		name, want string
		keys       map[string]string
		wantErr    bool
	}{
		{"both: latest wins", "ChromeHTML", map[string]string{latest: "ChromeHTML", plain: "MSEdgeHTM"}, false},
		{"only UserChoice", "MSEdgeHTM", map[string]string{plain: "MSEdgeHTM"}, false},
		{"only UserChoiceLatest", "FirefoxURL-308046B0AF4A39CB", map[string]string{latest: "FirefoxURL-308046B0AF4A39CB"}, false},
		{"bad latest falls back", "MSEdgeHTM", map[string]string{latest: `a\b`, plain: "MSEdgeHTM"}, false},
		{"none", "", map[string]string{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pickProgID(mk(tc.keys))
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("got (%q,%v)", got, err)
			}
		})
	}
}

func TestDarwinPolicyDomain(t *testing.T) {
	for _, tc := range []struct{ bundle, domain string }{
		{"com.google.chrome", "com.google.Chrome"},
		{"com.microsoft.edgemac", "com.microsoft.Edge"},
		{"com.brave.browser", "com.brave.Browser"},
	} {
		t.Run(tc.bundle, func(t *testing.T) {
			handler := plist(`{"LSHandlerURLScheme":"https","LSHandlerRoleAll":"` + tc.bundle + `"}`)
			var plists []string
			o := &Opener{GOOS: "darwin", Home: "/Users/me",
				Output: func(_ context.Context, name string, args ...string) ([]byte, error) {
					if len(args) > 0 && args[0] == "-extract" {
						plists = append(plists, args[len(args)-1])
						return nil, errors.New("no such key")
					}
					if name == "/usr/bin/open" {
						return nil, nil
					}
					return []byte(handler), nil
				}}
			if got, err := o.OpenPrivate(context.Background(), testURL); err != nil || !got {
				t.Fatalf("got (%v,%v)", got, err)
			}
			want := []string{"/Library/Managed Preferences/" + tc.domain + ".plist", "/Library/Managed Preferences/me/" + tc.domain + ".plist"}
			if !reflect.DeepEqual(plists, want) {
				t.Fatalf("read %v want %v", plists, want)
			}
		})
	}
}
