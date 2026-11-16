package codecommit

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/rudian/codepass/internal/awsguard"
)

// FU-22f: the production factory without an endpoint sits behind the loopback guard.
func TestFactoryWithoutEndpointIsGuarded(t *testing.T) {
	api := NewFactory("")(credentials.NewStaticCredentialsProvider("AKIAEXAMPLE", "secret", ""), "us-east-1")
	_, err := api.ListRepositories(context.Background())
	if !errors.Is(err, awsguard.ErrRefused) {
		t.Fatalf("the request was not refused by the guard: %v", err)
	}
}
