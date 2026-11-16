package awsguard

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoopback(t *testing.T) {
	for h, want := range map[string]bool{
		"127.0.0.1:8080": true, "localhost:1": true, "[::1]:9": true, "127.0.0.1": true,
		"oidc.us-east-1.amazonaws.com": false, "sts.amazonaws.com:443": false, "10.0.0.1": false, "": false,
	} {
		if got := Loopback(h); got != want {
			t.Errorf("Loopback(%q)=%v", h, got)
		}
	}
}

func TestClientRefusesRemoteAndAllowsLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	c := Client()
	if c == nil {
		t.Fatal("no guarded client in a test binary")
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	req, _ = http.NewRequestWithContext(context.Background(), http.MethodGet, "https://portal.sso.us-east-1.amazonaws.com/x", nil)
	if _, err = c.Do(req); !errors.Is(err, ErrRefused) {
		t.Fatalf("remote host not refused: %v", err)
	}
}
