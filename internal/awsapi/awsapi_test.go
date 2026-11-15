package awsapi_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/awsapi"
	"github.com/rudian/codepass/internal/awsapi/awsapitest"
	"github.com/rudian/codepass/internal/awsconfig"
)

const (
	oldKeyID = "AKIAIOSFODNN7EXAMPLE"
	ssoKeyID = "ASIAIOSFODNN7EXAMPLE"
)

var oldKey = awsconfig.StaticKey{AccessKeyID: oldKeyID, SecretAccessKey: "fakeSecretValueDoNotUse0000000000000000"}

func ssoProvider() aws.CredentialsProvider {
	return awsapi.StaticProvider(awsconfig.StaticKey{AccessKeyID: ssoKeyID, SecretAccessKey: "s", SessionToken: "tok"})
}

func newFake(t *testing.T) (*awsapitest.Fake, awsapi.Endpoints) {
	f := awsapitest.New(t)
	f.Identities[oldKeyID] = awsapitest.Identity{Account: "111122223333", Arn: "arn:aws:iam::111122223333:user/acme-dev", UserID: "AIDA1"}
	f.Identities[ssoKeyID] = awsapitest.Identity{Account: "111122223333", Arn: "arn:aws:sts::111122223333:assumed-role/AWSReservedSSO_AdministratorAccess_abc/alex", UserID: "AROA1:alex"}
	f.Keys["acme-dev"] = []awsapitest.Key{{ID: "AKIAOTHERKEY0EXAMPLE", Status: "Active"}, {ID: oldKeyID, Status: "Active"}}
	return f, awsapi.Endpoints{STS: f.URL, IAM: f.URL}
}

func TestGetCallerIdentityParsesARN(t *testing.T) {
	f, ep := newFake(t)
	f.Identities["AKIAROOTKEY00EXAMPLE"] = awsapitest.Identity{Account: "999900001111", Arn: "arn:aws:iam::999900001111:root", UserID: "999900001111"}
	ctx := context.Background()

	id, err := awsapi.GetCallerIdentity(ctx, awsapi.StaticProvider(oldKey), ep)
	if err != nil || id.AccountID != "111122223333" || id.IAMUser != "acme-dev" || id.IsRoot || id.IsAssumedRole {
		t.Fatalf("%+v %v", id, err)
	}
	id, err = awsapi.GetCallerIdentity(ctx, ssoProvider(), ep)
	if err != nil || !id.IsAssumedRole || id.IAMUser != "" || !strings.Contains(id.Arn, "AWSReservedSSO_AdministratorAccess_") {
		t.Fatalf("%+v %v", id, err)
	}
	id, err = awsapi.GetCallerIdentity(ctx, awsapi.StaticProvider(awsconfig.StaticKey{AccessKeyID: "AKIAROOTKEY00EXAMPLE", SecretAccessKey: "x"}), ep)
	if err != nil || !id.IsRoot || id.AccountID != "999900001111" {
		t.Fatalf("%+v %v", id, err)
	}
	_, err = awsapi.GetCallerIdentity(ctx, awsapi.StaticProvider(awsconfig.StaticKey{AccessKeyID: "AKIAUNKNOWN00EXAMPLE", SecretAccessKey: "x"}), ep)
	if awsapi.ErrorCode(err) != "InvalidClientTokenId" {
		t.Fatalf("code %q err %v", awsapi.ErrorCode(err), err)
	}
	if ae := awsapi.ToAppError(err); ae.Code != api.ErrAWSInvalidKey || ae.Detail != "InvalidClientTokenId" {
		t.Fatalf("%+v", ae)
	}
	a, u, root, assumed := awsapi.ParseARN("arn:aws:iam::123456789012:user/division/team/bob")
	if a != "123456789012" || u != "bob" || root || assumed {
		t.Fatal(a, u)
	}
	if a, _, _, _ := awsapi.ParseARN("garbage"); a != "" {
		t.Fatal("garbage ARN")
	}
}

func TestToAppErrorMapping(t *testing.T) {
	f, ep := newFake(t)
	f.Deny["*|ListAccessKeys"] = true
	_, errDenied := awsapi.DeactivateWithRole(context.Background(), ssoProvider(), "acme-dev", oldKeyID, ep)
	if ae := api.AsAppError(errDenied); ae.Code != api.ErrAWSAccessDenied || ae.Detail != "iam:ListAccessKeys" {
		t.Fatalf("%+v", ae)
	}
	cases := []struct {
		err    error
		code   api.ErrorCode
		detail string
	}{
		{api.NewError(api.ErrAWSAPI, "x"), api.ErrAWSAPI, ""},
		{context.Canceled, api.ErrCancelled, ""},
		{errors.New("plain"), api.ErrInternal, "plain"},
	}
	for _, c := range cases {
		if got := awsapi.ToAppError(c.err); got.Code != c.code || (c.detail != "" && got.Detail != c.detail) {
			t.Errorf("%v -> %+v", c.err, got)
		}
	}
	if awsapi.ToAppError(nil) != nil {
		t.Fatal("nil")
	}
	for _, code := range []string{"InvalidClientTokenId", "SignatureDoesNotMatch", "ExpiredToken", "UnrecognizedClientException"} {
		if !awsapi.IsInvalidKeyCode(code) {
			t.Errorf("%s", code)
		}
	}
	if awsapi.IsInvalidKeyCode("AccessDenied") {
		t.Fatal("access denied is not an invalid key")
	}
}

func TestToAppErrorNetwork(t *testing.T) {
	ep := awsapi.Endpoints{STS: "http://127.0.0.1:1"} // nothing listens here
	_, err := awsapi.GetCallerIdentity(context.Background(), awsapi.StaticProvider(oldKey), ep)
	if err == nil {
		t.Fatal("expected error")
	}
	if ae := awsapi.ToAppError(err); ae.Code != api.ErrNetwork {
		t.Fatalf("%+v", ae)
	}
}

func TestDeactivateWithRole(t *testing.T) {
	f, ep := newFake(t)
	ctx := context.Background()
	st, err := awsapi.DeactivateWithRole(ctx, ssoProvider(), "acme-dev", oldKeyID, ep)
	if err != nil || st != api.DeactivationDone || f.KeyStatus("acme-dev", oldKeyID) != "Inactive" {
		t.Fatalf("%v %v", st, err)
	}
	if f.KeyStatus("acme-dev", "AKIAOTHERKEY0EXAMPLE") != "Active" {
		t.Fatal("other key must stay Active")
	}
	if st, err := awsapi.DeactivateWithRole(ctx, ssoProvider(), "acme-dev", oldKeyID, ep); err != nil || st != api.DeactivationAlready {
		t.Fatalf("%v %v", st, err)
	}
	if st, err := awsapi.DeactivateWithRole(ctx, ssoProvider(), "acme-dev", "AKIAGONEKEY00EXAMPLE", ep); err != nil || st != api.DeactivationKeyNotFound {
		t.Fatalf("%v %v", st, err)
	}
	f.Keys["acme-dev"][1].Status = "Active"
	f.Deny[ssoKeyID+"|UpdateAccessKey"] = true
	_, err = awsapi.DeactivateWithRole(ctx, ssoProvider(), "acme-dev", oldKeyID, ep)
	if ae := api.AsAppError(err); ae.Code != api.ErrAWSAccessDenied || ae.Detail != "iam:UpdateAccessKey" {
		t.Fatalf("%+v", ae)
	}
}

func TestDeactivateSelfOmitsUserName(t *testing.T) {
	f, ep := newFake(t)
	st, err := awsapi.DeactivateSelf(context.Background(), oldKey, ep)
	if err != nil || st != api.DeactivationDone {
		t.Fatalf("%v %v", st, err)
	}
	reqs := f.Requests()
	last := reqs[len(reqs)-1]
	if last.Action != "UpdateAccessKey" || last.Form.Has("UserName") || last.Form.Get("AccessKeyId") != oldKeyID || last.Form.Get("Status") != "Inactive" {
		t.Fatalf("%+v", last)
	}
	if last.AccessKeyID != oldKeyID {
		t.Fatal("must be signed with the old key")
	}
	if f.KeyStatus("acme-dev", oldKeyID) != "Inactive" {
		t.Fatal("key not deactivated")
	}
	f.Keys["acme-dev"][1].Status = "Active"
	f.Deny[oldKeyID+"|UpdateAccessKey"] = true
	if _, err := awsapi.DeactivateSelf(context.Background(), oldKey, ep); api.AsAppError(err).Code != api.ErrAWSAccessDenied {
		t.Fatal(err)
	}
}

func TestStaticProviderSigns(t *testing.T) {
	f, ep := newFake(t)
	if _, err := awsapi.GetCallerIdentity(context.Background(), awsapi.StaticProvider(oldKey), ep); err != nil {
		t.Fatal(err)
	}
	r := f.Requests()[0]
	if !strings.Contains(r.Authorization, "Credential=AKIAIOSFODNN7EXAMPLE/") || r.Action != "GetCallerIdentity" {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(r.Authorization, "/us-east-1/") {
		t.Fatalf("STS must sign for us-east-1: %s", r.Authorization)
	}
}

func TestIAMUserDestination(t *testing.T) {
	if got := awsapi.IAMUserDestination("acme-dev"); got != "https://console.aws.amazon.com/iam/home#/users/details/acme-dev?section=security_credentials" {
		t.Fatal(got)
	}
	if got := awsapi.IAMUserDestination("a b+c"); !strings.Contains(got, "details/a%20b+c?") {
		t.Fatal(got)
	}
}

func TestStaticKeyStringHidesSecret(t *testing.T) {
	s := oldKey.String()
	if strings.Contains(s, "fakeSecret") || s != "StaticKey(AKIA••••••••••••MPLE)" {
		t.Fatal(s)
	}
}

// Credentials must always be explicit. The SDK default chain would consult credential_process, environment
// keys and the instance metadata service, so nothing under internal/ may load it.
func TestNoDefaultCredentialChain(t *testing.T) {
	needle := "LoadDefault" + "Config"
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		if strings.Contains(string(b), needle) {
			t.Errorf("%s uses the SDK default configuration chain", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
