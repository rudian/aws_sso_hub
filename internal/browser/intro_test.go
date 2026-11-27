package browser

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const testIntroURL = "http://127.0.0.1:53682/start/0123456789abcdef0123456789abcdef"

func TestCheckIntroURL(t *testing.T) {
	if err := CheckIntroURL(testIntroURL); err != nil {
		t.Fatal(err)
	}
	hex := "0123456789abcdef0123456789abcdef"
	for _, bad := range []string{
		"",
		"https://127.0.0.1:53682/start/" + hex,         // not the intro page's scheme
		"http://localhost:53682/start/" + hex,          // a name can be pointed elsewhere
		"http://127.0.0.2:53682/start/" + hex,          // only 127.0.0.1
		"http://[::1]:53682/start/" + hex,              // only IPv4
		"http://127.0.0.1/start/" + hex,                // no port
		"http://127.0.0.1:0/start/" + hex,              // port 0
		"http://127.0.0.1:65536/start/" + hex,          // port out of range
		"http://127.0.0.1:053682/start/" + hex,         // leading zero
		"http://user@127.0.0.1:53682/start/" + hex,     // user info
		"http://127.0.0.1:53682/start/" + hex + "?x=1", // query
		"http://127.0.0.1:53682/start/" + hex + "#x",   // fragment
		"http://127.0.0.1:53682/start/" + hex + "/x",   // longer path
		"http://127.0.0.1:53682/oauth/callback?code=c", // the callback
		"http://127.0.0.1:53682/start/" + hex[:31],     // too short
		"http://127.0.0.1:53682/start/" + strings.ToUpper(hex),
		"http://127.0.0.1:53682/start/" + hex + "\n",
	} {
		if err := CheckIntroURL(bad); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("%q accepted", bad)
		}
	}
	if err := CheckURL(testIntroURL); err == nil {
		t.Fatal("CheckURL stays https only")
	}
}

func TestSignInWindowsOpenTheIntroPage(t *testing.T) {
	https := plist(`{"LSHandlerURLScheme":"https","LSHandlerRoleAll":"com.google.chrome"}`)
	dir := t.TempDir()
	f := &fake{outputs: map[string]string{"-convert": https}}
	w, err := f.opener("darwin").OpenIsolated(context.Background(), testIntroURL, dir)
	if err != nil || w == nil || len(f.started) != 1 {
		t.Fatalf("%v %v %+v", w, err, f.started)
	}
	if args := f.started[0].args; !reflect.DeepEqual(args[len(args)-1], testIntroURL) {
		t.Fatalf("%v", args)
	}
	for _, bad := range []string{"http://127.0.0.1:53682/oauth/callback", "http://example.com/"} {
		if _, err := f.opener("darwin").OpenIsolated(context.Background(), bad, dir); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("OpenIsolated %q: %v", bad, err)
		}
		if _, err := f.opener("darwin").OpenPrivate(context.Background(), bad); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("OpenPrivate %q: %v", bad, err)
		}
	}
}
