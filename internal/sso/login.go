package sso

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	oidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/backup"
)

// registration is the app's own OIDC client registration (never the CLI's file).
type registration struct {
	ClientID     string   `json:"clientId"`
	ClientSecret string   `json:"clientSecret"`
	ExpiresAt    string   `json:"expiresAt"`
	Scopes       []string `json:"scopes,omitempty"`
	GrantTypes   []string `json:"grantTypes,omitempty"`
}

func (m *Manager) scopesFor(s Session) []string {
	if len(s.Scopes) > 0 {
		return s.Scopes
	}
	return []string{defaultScope}
}

func (m *Manager) registrationPath(s Session, flow string) string {
	scopes := s.Scopes
	if scopes == nil {
		scopes = []string{}
	}
	b, _ := json.Marshal(map[string]any{"flow": flow, "region": s.Region, "scopes": scopes, "session": s.Name, "startUrl": s.StartURL})
	return filepath.Join(m.cfg.RegistrationsDir, sha1Hex(string(b))+".json")
}

func (m *Manager) loadRegistration(s Session, flow string) (*registration, bool) {
	b, err := os.ReadFile(m.registrationPath(s, flow))
	if err != nil {
		return nil, false
	}
	var r registration
	if json.Unmarshal(b, &r) != nil || r.ClientID == "" || r.ClientSecret == "" {
		return nil, false
	}
	exp, err := time.Parse(time.RFC3339, r.ExpiresAt)
	if err != nil || exp.Sub(m.now()) < refreshWindow {
		return nil, false
	}
	if flow == "pkce" {
		ok := false
		for _, g := range r.GrantTypes {
			if g == grantAuthCode {
				ok = true
			}
		}
		if !ok {
			return nil, false
		}
	}
	return &r, true
}

func (m *Manager) registerClient(ctx context.Context, s Session, flow string) (*registration, error) {
	if r, ok := m.loadRegistration(s, flow); ok {
		return r, nil
	}
	name := "aws-sso-hub-" + s.Name
	if s.Name == "" {
		name = "aws-sso-hub-" + strconv.FormatInt(m.now().Unix(), 10)
	}
	in := &ssooidc.RegisterClientInput{ClientName: aws.String(name), ClientType: aws.String(clientTypePublic)}
	if flow == "pkce" {
		in.GrantTypes = []string{grantAuthCode, grantRefresh}
		in.RedirectUris = []string{redirectNoPort}
		in.IssuerUrl = aws.String(s.StartURL)
		in.Scopes = m.scopesFor(s)
	} else if len(s.Scopes) > 0 {
		in.Scopes = s.Scopes
	}
	out, err := m.oidcClient(s).RegisterClient(ctx, in)
	if err != nil {
		return nil, err
	}
	r := &registration{
		ClientID: aws.ToString(out.ClientId), ClientSecret: aws.ToString(out.ClientSecret),
		ExpiresAt: time.Unix(out.ClientSecretExpiresAt, 0).UTC().Format(CLITimeLayout),
		Scopes:    in.Scopes, GrantTypes: in.GrantTypes,
	}
	b, _ := json.Marshal(r)
	if err := os.MkdirAll(m.cfg.RegistrationsDir, 0o700); err == nil {
		_ = backup.WriteFileAtomic(m.registrationPath(s, flow), b, 0o600)
	}
	return r, nil
}

func (m *Manager) dropRegistration(s Session, flow string) {
	_ = os.Remove(m.registrationPath(s, flow))
}

// regionRE mirrors awsconfig.RegionRE (this package does not import the config package).
var regionRE = regexp.MustCompile(`^[a-z]{2}(-gov|-iso[a-z]*)?-[a-z]+-[0-9]{1,2}$`)

func checkRegion(region string) error {
	if !regionRE.MatchString(region) {
		return api.NewError(api.ErrInvalidRegion, "invalid region")
	}
	return nil
}

func (m *Manager) authorizeBase(ctx context.Context, s Session) (string, error) {
	if err := checkRegion(s.Region); err != nil {
		return "", err
	}
	if m.cfg.Endpoints.OIDC != "" {
		return trimSlash(m.cfg.Endpoints.OIDC), nil
	}
	ep, err := ssooidc.NewDefaultEndpointResolverV2().ResolveEndpoint(ctx, ssooidc.EndpointParameters{Region: aws.String(s.Region)})
	if err != nil {
		return "", err
	}
	return trimSlash(ep.URI.String()), nil
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func (m *Manager) beginLogin(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.logging[key] {
		return false
	}
	m.logging[key] = true
	return true
}

func (m *Manager) endLogin(key string) {
	m.mu.Lock()
	delete(m.logging, key)
	m.mu.Unlock()
}

// Login runs the interactive sign-in and writes the CLI format token file.
func (m *Manager) Login(ctx context.Context, s Session, mode api.LoginMode, progress func(api.LoginProgress)) (*CachedToken, error) {
	key := keyOf(s)
	if !m.beginLogin(key) {
		return nil, ErrInProgress
	}
	defer m.endLogin(key)
	if progress == nil {
		progress = func(api.LoginProgress) {}
	}
	var tok *CachedToken
	var err error
	if mode == api.LoginModeDevice {
		tok, err = m.loginDevice(ctx, s, progress)
	} else {
		for attempt := 0; attempt < 2; attempt++ {
			var restart bool
			tok, restart, err = m.loginPKCE(ctx, s, progress, attempt == 0)
			if !restart {
				break
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if werr := WriteToken(m.tokenDir(), key, tok); werr != nil {
		return nil, api.NewError(api.ErrFileIO, "cannot write the SSO token cache").WithParam("file", key+".json")
	}
	m.mu.Lock()
	delete(m.failed, key)
	delete(m.netErr, key)
	m.mu.Unlock()
	return tok, nil
}

type callbackResult struct{ code, errParam string }

func (m *Manager) loginPKCE(ctx context.Context, s Session, progress func(api.LoginProgress), mayRestart bool) (*CachedToken, bool, error) {
	if err := checkRegion(s.Region); err != nil {
		return nil, false, err
	}
	progress(api.LoginProgress{Portal: s.Name, Phase: api.LoginPhaseStarting})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, false, ErrPort
	}
	port := ln.Addr().(*net.TCPAddr).Port
	wantHost := fmt.Sprintf("127.0.0.1:%d", port)
	redirect := "http://" + wantHost + redirectPath

	closeLn := true
	defer func() {
		if closeLn {
			ln.Close()
		}
	}()

	reg, err := m.registerClient(ctx, s, "pkce")
	if err != nil {
		return nil, false, err
	}
	verifier, challenge, err := NewPKCE()
	if err != nil {
		return nil, false, err
	}
	state := NewState()
	base, err := m.authorizeBase(ctx, s)
	if err != nil {
		return nil, false, err
	}
	authURL := AuthorizeURL(base, reg.ClientID, redirect, state, challenge, m.scopesFor(s))

	results := make(chan callbackResult, 1)
	lang := m.lang()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != redirectPath {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		q := r.URL.Query()
		// Any web page can make the browser call this loopback address. Only a request that carries the
		// unguessable state of this login, addressed to the exact listener, is a callback; everything else is
		// answered neutrally and ignored, so it can neither complete nor abort the sign-in.
		stateOK := subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) == 1
		if !stateOK || r.Host != wantHost {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		res := callbackResult{code: q.Get("code"), errParam: q.Get("error")}
		ok, errValue := res.errParam == "" && res.code != "", res.errParam
		if errValue == "" && !ok {
			errValue = "invalid_request"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(CallbackPage(lang, ok, errValue))
		if res.errParam != "" || res.code != "" {
			select {
			case results <- res:
			default:
			}
		}
	})
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       10 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}
	closeLn = false
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
		ln.Close()
	}()

	opened, private := false, false
	switch {
	case m.cfg.OpenSignIn != nil:
		p, err := m.cfg.OpenSignIn(authURL)
		opened = err == nil
		private = opened && p
	case m.cfg.OpenBrowser != nil:
		opened = m.cfg.OpenBrowser(authURL) == nil
	}
	timeout := m.cfg.CallbackTimeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	progress(api.LoginProgress{Portal: s.Name, Phase: api.LoginPhaseWaiting, AuthorizeURL: authURL, BrowserOpened: opened, PrivateWindow: private,
		ExpiresAt: m.now().UTC().Add(timeout).Format(time.RFC3339)})

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var res callbackResult
	select {
	case res = <-results:
	case <-timer.C:
		return nil, false, ErrTimeout
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
	if res.errParam != "" {
		switch res.errParam {
		case "access_denied":
			return nil, false, ErrDenied
		case "invalid_client", "unauthorized_client":
			if mayRestart {
				m.dropRegistration(s, "pkce")
				return nil, true, nil
			}
		}
		return nil, false, &AuthError{Value: res.errParam}
	}
	progress(api.LoginProgress{Portal: s.Name, Phase: api.LoginPhaseExchanging})
	out, err := m.oidcClient(s).CreateToken(ctx, &ssooidc.CreateTokenInput{
		GrantType: aws.String(grantAuthCode), ClientId: aws.String(reg.ClientID), ClientSecret: aws.String(reg.ClientSecret),
		RedirectUri: aws.String(redirect), CodeVerifier: aws.String(verifier), Code: aws.String(res.code),
	})
	if err != nil {
		var exp *oidctypes.ExpiredTokenException
		var ic *oidctypes.InvalidClientException
		switch {
		case errors.As(err, &exp):
			return nil, false, ErrTimeout
		case errors.As(err, &ic) && mayRestart:
			m.dropRegistration(s, "pkce")
			return nil, true, nil
		case errors.As(err, &ic):
			return nil, false, &AuthError{Value: "invalid_client"}
		}
		return nil, false, err
	}
	return m.buildToken(s, reg, out), false, nil
}

func (m *Manager) buildToken(s Session, reg *registration, out *ssooidc.CreateTokenOutput) *CachedToken {
	regExp, _ := time.Parse(time.RFC3339, reg.ExpiresAt)
	return &CachedToken{
		StartURL: s.StartURL, Region: s.Region,
		AccessToken:           aws.ToString(out.AccessToken),
		ExpiresAt:             m.now().UTC().Add(time.Duration(out.ExpiresIn) * time.Second).Truncate(time.Second),
		ClientID:              reg.ClientID,
		ClientSecret:          reg.ClientSecret,
		RegistrationExpiresAt: regExp.UTC(),
		RefreshToken:          aws.ToString(out.RefreshToken),
	}
}

func (m *Manager) loginDevice(ctx context.Context, s Session, progress func(api.LoginProgress)) (*CachedToken, error) {
	if err := checkRegion(s.Region); err != nil {
		return nil, err
	}
	progress(api.LoginProgress{Portal: s.Name, Phase: api.LoginPhaseStarting})
	reg, err := m.registerClient(ctx, s, "device")
	if err != nil {
		return nil, err
	}
	cli := m.oidcClient(s)
	da, err := cli.StartDeviceAuthorization(ctx, &ssooidc.StartDeviceAuthorizationInput{
		ClientId: aws.String(reg.ClientID), ClientSecret: aws.String(reg.ClientSecret), StartUrl: aws.String(s.StartURL)})
	if err != nil {
		var ic *oidctypes.InvalidClientException
		if errors.As(err, &ic) {
			m.dropRegistration(s, "device")
		}
		return nil, err
	}
	deadline := m.now().Add(time.Duration(da.ExpiresIn) * time.Second)
	interval := time.Duration(da.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	attempt := func() (*CachedToken, error) {
		out, err := cli.CreateToken(ctx, &ssooidc.CreateTokenInput{
			GrantType: aws.String(grantDeviceCode), ClientId: aws.String(reg.ClientID),
			ClientSecret: aws.String(reg.ClientSecret), DeviceCode: da.DeviceCode})
		if err == nil {
			return m.buildToken(s, reg, out), nil
		}
		var pending *oidctypes.AuthorizationPendingException
		var slow *oidctypes.SlowDownException
		var expired *oidctypes.ExpiredTokenException
		var denied *oidctypes.AccessDeniedException
		switch {
		case errors.As(err, &pending):
			return nil, nil
		case errors.As(err, &slow):
			interval += 5 * time.Second
			return nil, nil
		case errors.As(err, &expired):
			return nil, ErrDeviceExpired
		case errors.As(err, &denied):
			return nil, ErrDenied
		}
		return nil, err
	}
	if tok, err := attempt(); tok != nil || err != nil { // one immediate attempt, like the CLI
		return tok, err
	}
	progress(api.LoginProgress{
		Portal: s.Name, Phase: api.LoginPhaseDevice, UserCode: aws.ToString(da.UserCode),
		VerificationURI: aws.ToString(da.VerificationUri), VerificationURIComplete: aws.ToString(da.VerificationUriComplete),
		ExpiresAt: deadline.UTC().Format(time.RFC3339),
	})
	for {
		if err := m.sleep(ctx, interval); err != nil {
			return nil, err
		}
		if m.now().After(deadline) {
			return nil, ErrDeviceExpired
		}
		if tok, err := attempt(); tok != nil || err != nil {
			return tok, err
		}
	}
}
