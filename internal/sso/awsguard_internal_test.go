package sso

import (
	"errors"
	"net/http"
	"testing"

	"github.com/rudian/codepass/internal/awsguard"
)

// FU-22f: the OIDC and portal clients built without endpoints sit behind the loopback guard.
func TestClientsWithoutEndpointsAreGuarded(t *testing.T) {
	m := NewManager(Config{})
	s := Session{Region: "us-east-1"}
	for name, hc := range map[string]any{
		"oidc":   m.oidcClient(s).Options().HTTPClient,
		"portal": m.portalClient(s).Options().HTTPClient,
	} {
		c, ok := hc.(*http.Client)
		if !ok {
			t.Fatalf("%s: no guarded HTTP client", name)
		}
		if _, err := c.Get("https://example.com/"); !errors.Is(err, awsguard.ErrRefused) {
			t.Errorf("%s: a non-loopback request was not refused: %v", name, err)
		}
	}
}
