package sso

import (
	"context"
	"errors"
	"net"
	"net/url"

	"github.com/aws/smithy-go"

	"github.com/rudian/aws_sso_hub/internal/api"
)

var (
	ErrNotSignedIn     = errors.New("sso: not signed in")
	ErrSessionExpired  = errors.New("sso: session expired or revoked")
	ErrDenied          = errors.New("sso: authorization denied")
	ErrTimeout         = errors.New("sso: authorization timed out")
	ErrPort            = errors.New("sso: cannot listen on 127.0.0.1")
	ErrState           = errors.New("sso: state mismatch")
	ErrDeviceExpired   = errors.New("sso: device authorization expired")
	ErrInProgress      = errors.New("sso: login already in progress")
	ErrInvalidGrant    = errors.New("sso: refresh rejected")
	ErrRoleNotAssigned = errors.New("sso: role not assigned")
)

// AuthError is any other OAuth "error" value returned on the callback.
type AuthError struct{ Value string }

func (e *AuthError) Error() string { return "sso: authorization failed: " + e.Value }

// NetworkHost returns the host of a network-level error and true when err is one.
func NetworkHost(err error) (string, bool) {
	var ue *url.Error
	var oe *net.OpError
	var de *net.DNSError
	switch {
	case errors.As(err, &de):
		return de.Name, true
	case errors.As(err, &ue):
		if u, perr := url.Parse(ue.URL); perr == nil && u.Host != "" {
			return u.Host, true
		}
		return "", true
	case errors.As(err, &oe):
		if oe.Addr != nil {
			return oe.Addr.String(), true
		}
		return "", true
	}
	return "", false
}

// ToAppError maps sso errors to contract errors.
func ToAppError(err error, portal string) *api.AppError {
	if err == nil {
		return nil
	}
	var ae *api.AppError
	if errors.As(err, &ae) {
		return ae
	}
	var auth *AuthError
	switch {
	case errors.Is(err, ErrInProgress):
		return api.NewError(api.ErrLoginInProgress, "login already in progress").WithParam("portal", portal)
	case errors.Is(err, ErrDenied):
		return api.NewError(api.ErrLoginDenied, "authorization denied")
	case errors.Is(err, ErrTimeout), errors.Is(err, ErrDeviceExpired):
		return api.NewError(api.ErrLoginTimeout, "authorization timed out")
	case errors.Is(err, ErrPort):
		return api.NewError(api.ErrLoginPort, "cannot listen on 127.0.0.1")
	case errors.Is(err, ErrState):
		return api.NewError(api.ErrLoginState, "state mismatch")
	case errors.As(err, &auth):
		return api.NewError(api.ErrLoginFailed, "sign-in failed").WithDetail(auth.Value)
	case errors.Is(err, context.Canceled):
		return api.NewError(api.ErrCancelled, "cancelled")
	case errors.Is(err, ErrNotSignedIn):
		return api.NewError(api.ErrNotSignedIn, "not signed in").WithParam("portal", portal)
	case errors.Is(err, ErrSessionExpired), errors.Is(err, ErrInvalidGrant):
		return api.NewError(api.ErrSessionExpired, "session expired").WithParam("portal", portal)
	case errors.Is(err, ErrRoleNotAssigned):
		return api.NewError(api.ErrRoleNotAssigned, "role not assigned")
	}
	if host, ok := NetworkHost(err); ok {
		return api.NewError(api.ErrNetwork, "network error").WithDetail(host)
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return api.NewError(api.ErrAWSAPI, "AWS returned an error").WithDetail(apiErr.ErrorCode())
	}
	return api.AsAppError(err)
}
