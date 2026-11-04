package redact

import (
	"strings"
	"testing"
)

func TestMaskKeyID(t *testing.T) {
	if got, want := MaskKeyID("AKIAIOSFODNN7EXAMPLE"), "AKIA••••••••••••MPLE"; got != want {
		t.Errorf("MaskKeyID = %q, want %q", got, want)
	}
	if got := MaskKeyID("abc"); got != "" {
		t.Errorf("MaskKeyID(abc) = %q, want empty", got)
	}
	if got := MaskKeyID("1234567"); got != "" {
		t.Errorf("MaskKeyID(7 chars) = %q, want empty", got)
	}
}

func TestStringRedacts(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"ini secret", "aws_secret_access_key = abc", "aws_secret_access_key = " + Mask},
		{"ini secret no spaces", "aws_secret_access_key=abc", "aws_secret_access_key=" + Mask},
		{"ini session token", "aws_session_token = tok123", "aws_session_token = " + Mask},
		{"json access token", `"accessToken": "xyz"`, `"accessToken": "` + Mask + `"`},
		{"json client secret", `"clientSecret":"s"`, `"clientSecret":"` + Mask + `"`},
		{"json refresh token", `{"refreshToken":"r1","x":1}`, `{"refreshToken":"` + Mask + `","x":1}`},
		{"json secret access key", `"SecretAccessKey": "wJalrXUtn"`, `"SecretAccessKey": "` + Mask + `"`},
		{"case insensitive", "AWS_SECRET_ACCESS_KEY=abc", "AWS_SECRET_ACCESS_KEY=" + Mask},
		{"url userinfo", "https://u:p@h/x", "https://u:" + Mask + "@h/x"},
		{"key id", "key AKIAIOSFODNN7EXAMPLE here", "key AKIA••••••••••••MPLE here"},
		{"asia key id", "ASIAIOSFODNN7EXAMPLE", "ASIA••••••••••••MPLE"},
		{"password", "password=hunter2", "password=" + Mask},
		{"plain text", "hello world", "hello world"},
		{"empty", "", ""},
		{"empty value kept", "aws_session_token =", "aws_session_token ="},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := String(tc.in); got != tc.want {
				t.Errorf("String(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if got := String(String(tc.in)); got != tc.want {
				t.Errorf("String is not idempotent: %q", got)
			}
		})
	}
}

func TestStringRedactsCredentialHelperOutput(t *testing.T) {
	in := "username=ASIAIOSFODNN7EXAMPLE%SESSIONTOK\npassword=sig"
	got := String(in)
	if strings.Contains(got, "SESSIONTOK") {
		t.Errorf("session token leaked: %q", got)
	}
	if strings.Contains(got, "ASIAIOSFODNN7EXAMPLE") {
		t.Errorf("key id not masked: %q", got)
	}
	if !strings.Contains(got, "ASIA••••••••••••MPLE") {
		t.Errorf("masked key id missing: %q", got)
	}
	if strings.Contains(got, "sig") {
		t.Errorf("password leaked: %q", got)
	}
}

func TestURL(t *testing.T) {
	if got, want := URL("https://user:pass@h/x"), "https://user:"+Mask+"@h/x"; got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
	if got := URL("https://git-codecommit.eu-west-1.amazonaws.com/v1/repos/a"); got != "https://git-codecommit.eu-west-1.amazonaws.com/v1/repos/a" {
		t.Errorf("URL changed a clean URL: %q", got)
	}
}

func TestTail(t *testing.T) {
	in := "one\ntwo\r\nthree aws_secret_access_key=zzz\nfour\n"
	if got, want := Tail(in, 2), "three aws_secret_access_key="+Mask+"\nfour"; got != want {
		t.Errorf("Tail(2) = %q, want %q", got, want)
	}
	if got := Tail(in, 10); !strings.HasPrefix(got, "one\ntwo\n") {
		t.Errorf("Tail(10) = %q", got)
	}
	if got := Tail(in, 0); got != "" {
		t.Errorf("Tail(0) = %q", got)
	}
	if got := Tail("", 3); got != "" {
		t.Errorf("Tail(empty) = %q", got)
	}
}

func TestStringMasksGitCredentialsInText(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"token user", "url = https://ghp_0123456789abcdef0123@github.com/acme/x", "url = https://" + Mask + "@github.com/acme/x"},
		{"gitlab token user", "https://glpat-abcdef@gitlab.example.com/x.git", "https://" + Mask + "@gitlab.example.com/x.git"},
		{"slack style prefix", "https://xoxb-123@hooks.example.com/x", "https://" + Mask + "@hooks.example.com/x"},
		{"fine grained prefix", "https://github_pat_11AAA@github.com/x", "https://" + Mask + "@github.com/x"},
		{"long user", "https://abcdefghijklmnopqrstuvwx@host.example.com/x", "https://" + Mask + "@host.example.com/x"},
		{"extraheader spaced", "extraheader = AUTHORIZATION: bearer abc.def", "extraheader = " + Mask},
		{"extraheader dotted", "http.extraheader=Authorization: Basic dXNlcjpwdw==", "http.extraheader=" + Mask},
		{"extraheader other header", "[http]\n\textraHeader = X-Api-Key: 12345\n", "[http]\n\textraHeader = " + Mask + "\n"},
		{"authorization bearer", "Authorization: Bearer abc.def.ghi", "Authorization: Bearer " + Mask},
		{"authorization basic", "authorization:basic dXNlcjpwdw==", "authorization:basic " + Mask},
		{"authorization token", "Authorization: token ghp_abc", "Authorization: token " + Mask},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := String(tc.in); got != tc.want {
				t.Errorf("String(%q)\n got  %q\n want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStringKeepsOrdinaryUsers(t *testing.T) {
	for _, in := range []string{
		"https://user-at-111122223333@git-codecommit.us-east-1.amazonaws.com/v1/repos/x",
		"https://alex@github.com/acme/x",
		"ssh://git@github.com/acme/x",
		"https://git-codecommit.us-east-1.amazonaws.com/v1/repos/x",
		"remote: Authorization header missing",
		"http.extra settings",
	} {
		if got := String(in); got != in {
			t.Errorf("String(%q) = %q, must be unchanged", in, got)
		}
	}
}
