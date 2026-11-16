package awsapi

import (
	"errors"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/rudian/codepass/internal/awsguard"
)

func refuses(t *testing.T, hc *http.Client) {
	t.Helper()
	if hc == nil {
		t.Fatal("no HTTP client set: the guard is missing")
	}
	_, err := hc.Get("https://example.com/")
	if !errors.Is(err, awsguard.ErrRefused) {
		t.Fatalf("a non-loopback request was not refused: %v", err)
	}
}

// FU-22f: every client built without endpoints in a test binary sits behind the loopback guard.
func TestClientsWithoutEndpointsAreGuarded(t *testing.T) {
	p := credentials.NewStaticCredentialsProvider("AKIAEXAMPLE", "secret", "")
	refuses(t, stsClient(p, Endpoints{}).Options().HTTPClient.(*http.Client))
	refuses(t, iamClient(p, Endpoints{}).Options().HTTPClient.(*http.Client))
	refuses(t, signInClient)
}
