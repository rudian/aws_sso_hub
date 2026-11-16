package awsapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/awsconfig"
)

// ErrConsoleUnsupported means the Identity Center region is in no partition with a known console sign-in
// endpoint (ISO partitions, unknown regions). Callers open the access portal link instead.
var ErrConsoleUnsupported = errors.New("awsapi: console sign-in is not supported in this partition")

// ConsoleSignIn is the input of ConsoleSignInURL.
type ConsoleSignIn struct {
	Credentials aws.Credentials // temporary role credentials (secret)
	SSORegion   string          // Identity Center region; picks the partition and the federation endpoint
	Region      string          // console region; used only when valid and in the same partition
	Issuer      string          // access portal URL; the console sends the user there when the session ends
}

// maxSignInBody bounds the getSigninToken response that is read.
const maxSignInBody = 64 << 10

// signInClient never follows redirects, so the credentials in the request body only reach the federation
// endpoint.
var signInClient = &http.Client{
	Timeout:       30 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// partition holds the console sign-in hosts of an AWS partition.
type partition struct {
	signIn  string // sign-in domain; regional endpoints are <region>.<signIn>
	console string // console host
	global  string // region served by https://<signIn> itself ("" = none)
}

func partitionOf(region string) (partition, bool) {
	switch {
	case !awsconfig.RegionRE.MatchString(region), strings.Contains(region, "-iso"):
		return partition{}, false
	case strings.HasPrefix(region, "us-gov-"):
		return partition{signIn: "signin.amazonaws-us-gov.com", console: "console.amazonaws-us-gov.com", global: "us-gov-west-1"}, true
	case strings.HasPrefix(region, "cn-"):
		return partition{signIn: "signin.amazonaws.cn", console: "console.amazonaws.cn"}, true
	}
	return partition{signIn: "signin.aws.amazon.com", console: "console.aws.amazon.com", global: "us-east-1"}, true
}

// ConsoleSupported reports whether ConsoleSignInURL knows the console sign-in endpoint for an Identity Center
// region. Callers check it before they fetch role credentials.
func ConsoleSupported(ssoRegion string) bool {
	_, ok := partitionOf(ssoRegion)
	return ok
}

// consoleEndpoints returns the federation endpoint for the Identity Center region and the console home page,
// in region when that is a valid region of the same partition.
func consoleEndpoints(ssoRegion, region string, ep Endpoints) (federation, destination string, err error) {
	p, ok := partitionOf(ssoRegion)
	if !ok {
		return "", "", ErrConsoleUnsupported
	}
	switch {
	case ep.SignIn != "":
		federation = strings.TrimRight(ep.SignIn, "/") + "/federation"
	case ssoRegion == p.global:
		federation = "https://" + p.signIn + "/federation"
	default:
		federation = "https://" + ssoRegion + "." + p.signIn + "/federation"
	}
	destination = "https://" + p.console + "/console/home"
	if rp, ok := partitionOf(region); ok && rp == p {
		destination += "?region=" + region
	}
	return federation, destination, nil
}

// ConsoleSignInURL returns a link that signs the browser in to the AWS console with the role credentials (the
// AWS custom identity broker flow): the federation endpoint of the Identity Center partition trades them for
// a sign-in token, which goes into an Action=login URL. The console session lasts as long as the credentials.
// The URL is a secret, valid for 15 minutes: open it, never log it or return it to the frontend. Unsupported
// partitions get ErrConsoleUnsupported before any request.
func ConsoleSignInURL(ctx context.Context, in ConsoleSignIn, ep Endpoints) (string, error) {
	federation, destination, err := consoleEndpoints(in.SSORegion, in.Region, ep)
	if err != nil {
		return "", err
	}
	token, err := signInToken(ctx, federation, in.Credentials)
	if err != nil {
		return "", err
	}
	q := url.Values{"Action": {"login"}, "Destination": {destination}, "SigninToken": {token}}
	if in.Issuer != "" {
		q.Set("Issuer", in.Issuer)
	}
	return federation + "?" + q.Encode(), nil
}

// signInToken calls Action=getSigninToken. The credentials go in the POST body, never in a URL. No
// SessionDuration is sent, so the console session follows the credentials.
func signInToken(ctx context.Context, federation string, c aws.Credentials) (string, error) {
	session, _ := json.Marshal(struct {
		ID    string `json:"sessionId"`
		Key   string `json:"sessionKey"`
		Token string `json:"sessionToken"`
	}{c.AccessKeyID, c.SecretAccessKey, c.SessionToken})
	form := url.Values{"Action": {"getSigninToken"}, "Session": {string(session)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, federation, strings.NewReader(form.Encode()))
	if err != nil {
		return "", api.NewError(api.ErrInternal, "invalid sign-in endpoint")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := signInClient.Do(req)
	if err != nil {
		return "", signInTransportError(ctx, err, req.URL.Host)
	}
	defer resp.Body.Close()
	// Failures are HTML pages (HTTP 400); only a 200 carries the JSON answer.
	if resp.StatusCode != http.StatusOK {
		return "", api.NewError(api.ErrAWSAPI, "AWS returned an error").WithDetail("getSigninToken: HTTP " + strconv.Itoa(resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSignInBody))
	if err != nil {
		return "", signInTransportError(ctx, err, req.URL.Host)
	}
	var out struct{ SigninToken string }
	if json.Unmarshal(body, &out) != nil || out.SigninToken == "" {
		return "", api.NewError(api.ErrAWSAPI, "AWS returned an error").WithDetail("getSigninToken: bad response")
	}
	return out.SigninToken, nil
}

// signInTransportError maps a failed request. The text of a net/http error names the request URL, so only the
// host is kept.
func signInTransportError(ctx context.Context, err error, host string) *api.AppError {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return api.NewError(api.ErrCancelled, "cancelled")
	}
	return api.NewError(api.ErrNetwork, "network error").WithDetail(host)
}
