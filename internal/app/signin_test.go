package app

import (
	"context"
	"errors"
	"testing"

	"github.com/rudian/aws_sso_hub/internal/api"
)

type fakePrivate struct {
	private bool
	err     error
	urls    []string
}

func (f *fakePrivate) OpenPrivate(_ context.Context, u string) (bool, error) {
	f.urls = append(f.urls, u)
	return f.private, f.err
}

const signInURL = "https://device.sso.us-east-1.amazonaws.com/?user_code=ABCD-EFGH"

func TestOpenSignInPage(t *testing.T) {
	off := false
	cases := []struct {
		name        string
		setting     *bool // nil = default (on)
		fake        *fakePrivate
		wantPrivate bool
		wantPrivCal int
		wantNormal  int
	}{
		{"private window", nil, &fakePrivate{private: true}, true, 1, 0},
		{"default browser has no private switch", nil, &fakePrivate{private: false}, false, 1, 1},
		{"detection error falls back", nil, &fakePrivate{err: errors.New("boom")}, false, 1, 1},
		{"setting off", &off, &fakePrivate{private: true}, false, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld(t)
			w.c.Paths.Sandbox = false // pretend to be a real run; the fake runtime never launches anything
			w.c.private = tc.fake
			if tc.setting != nil {
				if _, err := w.c.Settings.Update(api.SettingsPatch{PrivateSignIn: tc.setting}); err != nil {
					t.Fatal(err)
				}
			}
			rt := w.c.RT.(*fakeRuntime)
			got, err := NewAccountService(w.c).OpenSignInPage(signInURL)
			if err != nil || got != tc.wantPrivate {
				t.Fatalf("got (%v,%v)", got, err)
			}
			if len(tc.fake.urls) != tc.wantPrivCal || len(rt.urls) != tc.wantNormal {
				t.Fatalf("private calls %v, normal opens %v", tc.fake.urls, rt.urls)
			}
		})
	}
}

func TestOpenSignInPageRejectsOtherURLs(t *testing.T) {
	w := newWorld(t)
	w.c.Paths.Sandbox = false
	fake := &fakePrivate{private: true}
	w.c.private = fake
	svc := NewAccountService(w.c)
	for _, u := range []string{
		"", "http://device.sso.us-east-1.amazonaws.com/", "https://evil.example.com/",
		"https://amazonaws.com.evil.com/", "https://notamazonaws.com/", "https://my-bucket.s3.amazonaws.com/x", "https://oidc.s3.amazonaws.com/", "https://oidc.s3-accelerate.amazonaws.com/", "https://amazonaws.com/", "https://app.aws.evil.com/", "ftp://x.awsapps.com/",
		"https://user@d-123.awsapps.com/start", "http://127.0.0.1:9/x", "javascript:alert(1)",
	} {
		if _, err := svc.OpenSignInPage(u); err == nil || api.AsAppError(err).Code != api.ErrInvalidInput {
			t.Fatalf("%q: %v", u, err)
		}
	}
	for _, u := range []string{
		"https://device.sso.us-east-1.amazonaws.com/?user_code=X", "https://d-123.awsapps.com/start/#/device",
		"https://oidc.cn-north-1.amazonaws.com.cn/x", "https://corp.awsapps.cn/start", "https://DEVICE.sso.eu-west-1.amazonaws.com/",
		"https://d-123.portal.us-east-1.app.aws/", "https://corp.app.aws/start", "https://x.app.amazonwebservices.com.cn/start",
	} {
		if err := checkSignInURL(u, false); err != nil {
			t.Fatalf("%q: %v", u, err)
		}
	}
	if len(fake.urls) != 0 {
		t.Fatalf("rejected URLs must not be opened: %v", fake.urls)
	}
}

func TestOpenSignInPageSandbox(t *testing.T) {
	w := newWorld(t) // sandbox: loopback http allowed, never a private launch
	fake := &fakePrivate{private: true}
	w.c.private = fake
	got, err := NewAccountService(w.c).OpenSignInPage("http://127.0.0.1:5555/device")
	if err != nil || got || len(fake.urls) != 0 {
		t.Fatalf("(%v,%v) %v", got, err, fake.urls)
	}
	if rt := w.c.RT.(*fakeRuntime); len(rt.urls) != 1 {
		t.Fatalf("%v", rt.urls)
	}
}
