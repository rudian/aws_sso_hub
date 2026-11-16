package app

import (
	"context"
	"testing"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/awsguard"
)

// FU-22f: a Container built without the fake Endpoints must fail with an error, never reach AWS.
func TestContainerWithoutEndpointsNeverReachesAWS(t *testing.T) {
	w := newWorld(t, func(o *ContainerOptions) { o.Endpoints = Endpoints{} })
	before := len(awsguard.Refused())
	_, err := w.acc.Login(context.Background(), "initech", api.LoginModeBrowser)
	if err == nil {
		t.Fatal("sign-in without endpoints succeeded; it must be refused")
	}
	got := awsguard.Refused()
	if len(got) <= before {
		t.Fatalf("the guard refused no host (err=%v)", err)
	}
	for _, h := range got[before:] {
		if awsguard.Loopback(h) {
			t.Errorf("a loopback host %q was reported as refused", h)
		}
	}
}
