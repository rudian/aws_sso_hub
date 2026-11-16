package awsapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/awsapi"
	"github.com/rudian/aws_sso_hub/internal/awsapi/awsapitest"
)

const (
	roleKeyID  = "ASIAROLEKEY00EXAMPLE"
	roleSecret = "roleSecretValueDoNotUse0000000000000000"
	roleToken  = "IQoJb3JpZ2luX2VjEFakeRoleSessionToken+/abc=="
	issuer     = "https://d-1234567890.awsapps.com/start"
)

func consoleInput() awsapi.ConsoleSignIn {
	return awsapi.ConsoleSignIn{
		Credentials: aws.Credentials{AccessKeyID: roleKeyID, SecretAccessKey: roleSecret, SessionToken: roleToken, CanExpire: true, Expires: time.Now().Add(time.Hour)},
		SSORegion:   "us-west-2", Region: "eu-west-1", Issuer: issuer,
	}
}

// noConsoleSecrets fails when the JSON of the error carries a credential or a sign-in token.
func noConsoleSecrets(t *testing.T, err error) *api.AppError {
	t.Helper()
	ae := api.AsAppError(err)
	b, _ := json.Marshal(ae)
	for _, s := range []string{roleKeyID, roleSecret, roleToken, "SIGNIN-"} {
		if strings.Contains(string(b), s) {
			t.Errorf("%q in %s", s, b)
		}
	}
	return ae
}

func TestConsoleSignInURL(t *testing.T) {
	f := awsapitest.New(t)
	ep := awsapi.Endpoints{SignIn: f.URL}
	got, err := awsapi.ConsoleSignInURL(context.Background(), consoleInput(), ep)
	if err != nil {
		t.Fatal(err)
	}
	// one POST with the credentials in the form body: nothing in the URL, no SessionDuration
	calls := f.SignIns()
	if len(calls) != 1 {
		t.Fatalf("%d federation calls", len(calls))
	}
	c := calls[0]
	if c.Method != http.MethodPost || c.ContentType != "application/x-www-form-urlencoded" || c.RawQuery != "" {
		t.Fatalf("%+v", c)
	}
	if len(c.Form) != 2 || c.Form.Get("Action") != "getSigninToken" || c.Form.Has("SessionDuration") {
		t.Fatalf("form %v", c.Form)
	}
	var session map[string]string
	if err := json.Unmarshal([]byte(c.Form.Get("Session")), &session); err != nil || len(session) != 3 {
		t.Fatalf("session %v %v", session, err)
	}
	if c.SessionID != roleKeyID || c.SessionKey != roleSecret || c.SessionToken != roleToken {
		t.Fatalf("session %+v", c)
	}
	// the login URL is on the same federation endpoint
	u, perr := url.Parse(got)
	if perr != nil || u.Scheme+"://"+u.Host != f.URL || u.Path != "/federation" {
		t.Fatalf("%q %v", got, perr)
	}
	want := url.Values{"Action": {"login"}, "Issuer": {issuer}, "SigninToken": {"SIGNIN-" + roleKeyID},
		"Destination": {"https://console.aws.amazon.com/console/home?region=eu-west-1"}}
	if u.Query().Encode() != want.Encode() {
		t.Fatalf("login query\n got %v\nwant %v", u.Query(), want)
	}

	in := consoleInput()
	in.Issuer = ""
	got, err = awsapi.ConsoleSignInURL(context.Background(), in, ep)
	if u, perr := url.Parse(got); err != nil || perr != nil || u.Query().Has("Issuer") || u.Query().Get("Action") != "login" {
		t.Fatalf("without issuer: %q %v", got, err)
	}
}

func TestConsoleSignInURLFailures(t *testing.T) {
	ctx := context.Background()
	f := awsapitest.New(t)
	ep := awsapi.Endpoints{SignIn: f.URL}

	f.FailSignIn(http.StatusBadRequest)
	_, err := awsapi.ConsoleSignInURL(ctx, consoleInput(), ep)
	if ae := noConsoleSecrets(t, err); ae == nil || ae.Code != api.ErrAWSAPI || ae.Detail != "getSigninToken: HTTP 400" {
		t.Fatalf("HTTP 400: %+v", ae)
	}
	f.FailSignIn(http.StatusOK) // an HTML page with status 200 is not an answer either
	_, err = awsapi.ConsoleSignInURL(ctx, consoleInput(), ep)
	if ae := noConsoleSecrets(t, err); ae == nil || ae.Code != api.ErrAWSAPI || ae.Detail != "getSigninToken: bad response" {
		t.Fatalf("HTML 200: %+v", ae)
	}
	f.FailSignIn(0)

	_, err = awsapi.ConsoleSignInURL(ctx, consoleInput(), awsapi.Endpoints{SignIn: "http://127.0.0.1:1"}) // nothing listens here
	if ae := noConsoleSecrets(t, err); ae == nil || ae.Code != api.ErrNetwork || ae.Detail != "127.0.0.1:1" {
		t.Fatalf("transport: %+v", ae)
	}

	before := len(f.SignIns())
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = awsapi.ConsoleSignInURL(cctx, consoleInput(), ep)
	if ae := noConsoleSecrets(t, err); ae == nil || ae.Code != api.ErrCancelled {
		t.Fatalf("cancelled: %+v", ae)
	}

	in := consoleInput()
	in.SSORegion = "us-iso-east-1"
	if got, err := awsapi.ConsoleSignInURL(ctx, in, ep); !errors.Is(err, awsapi.ErrConsoleUnsupported) || got != "" {
		t.Fatalf("ISO partition: %q %v", got, err)
	}
	if n := len(f.SignIns()); n != before {
		t.Fatalf("no request may be sent: %d -> %d", before, n)
	}
}

// Answers the federation endpoint should never give: the body never reaches the error, and a redirect is not
// followed with the credentials.
func TestConsoleSignInURLOddAnswers(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { elsewhere.Add(1) }))
	t.Cleanup(other.Close)
	cases := []struct {
		name, body, detail string
		status             int
	}{
		{"html page", "<html>SIGNIN-ECHO " + roleSecret + "</html>", "getSigninToken: HTTP 400", http.StatusBadRequest},
		{"server error", `{"SigninToken":"SIGNIN-5XX"}`, "getSigninToken: HTTP 500", http.StatusInternalServerError},
		{"no token", `{"Other":"x"}`, "getSigninToken: bad response", http.StatusOK},
		{"empty token", `{"SigninToken":""}`, "getSigninToken: bad response", http.StatusOK},
		{"too large", `{"SigninToken":"SIGNIN-` + strings.Repeat("A", 70<<10) + `"}`, "getSigninToken: bad response", http.StatusOK},
		{"redirect", "", "getSigninToken: HTTP 307", http.StatusTemporaryRedirect},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c.status == http.StatusTemporaryRedirect {
				w.Header().Set("Location", other.URL+"/federation")
			}
			w.WriteHeader(c.status)
			_, _ = io.WriteString(w, c.body)
		}))
		_, err := awsapi.ConsoleSignInURL(context.Background(), consoleInput(), awsapi.Endpoints{SignIn: srv.URL})
		srv.Close()
		if ae := noConsoleSecrets(t, err); ae == nil || ae.Code != api.ErrAWSAPI || ae.Detail != c.detail {
			t.Errorf("%s: %+v", c.name, ae)
		}
	}
	if n := elsewhere.Load(); n != 0 {
		t.Fatalf("the credentials followed a redirect (%d requests)", n)
	}
}
