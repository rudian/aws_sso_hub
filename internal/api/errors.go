package api

import (
	"errors"
	"fmt"

	"github.com/rudian/codepass/internal/redact"
)

// ErrorCode is a stable, UI-translatable identifier. Never rename or reuse a value.
type ErrorCode string

const (
	ErrInternal            ErrorCode = "E_INTERNAL"
	ErrCancelled           ErrorCode = "E_CANCELLED"
	ErrNotImplemented      ErrorCode = "E_NOT_IMPLEMENTED"
	ErrInvalidInput        ErrorCode = "E_INVALID_INPUT"  // params: field
	ErrConfigParse         ErrorCode = "E_CONFIG_PARSE"   // params: file, line
	ErrConfigChanged       ErrorCode = "E_CONFIG_CHANGED" // params: file
	ErrReadOnly            ErrorCode = "E_READ_ONLY"
	ErrFileIO              ErrorCode = "E_FILE_IO"           // params: file
	ErrProfileNotFound     ErrorCode = "E_PROFILE_NOT_FOUND" // params: name
	ErrProfileExists       ErrorCode = "E_PROFILE_EXISTS"    // params: name
	ErrPortalNotFound      ErrorCode = "E_PORTAL_NOT_FOUND"  // params: portal
	ErrPortalExists        ErrorCode = "E_PORTAL_EXISTS"     // params: portal
	ErrPortalInUse         ErrorCode = "E_PORTAL_IN_USE"     // params: portal, n
	ErrInvalidName         ErrorCode = "E_INVALID_NAME"      // params: name
	ErrInvalidStartURL     ErrorCode = "E_INVALID_START_URL"
	ErrInvalidRegion       ErrorCode = "E_INVALID_REGION"
	ErrNotSignedIn         ErrorCode = "E_NOT_SIGNED_IN"     // params: portal
	ErrSessionExpired      ErrorCode = "E_SESSION_EXPIRED"   // params: portal
	ErrLoginInProgress     ErrorCode = "E_LOGIN_IN_PROGRESS" // params: portal
	ErrLoginDenied         ErrorCode = "E_LOGIN_DENIED"
	ErrLoginTimeout        ErrorCode = "E_LOGIN_TIMEOUT"
	ErrLoginPort           ErrorCode = "E_LOGIN_PORT"
	ErrLoginState          ErrorCode = "E_LOGIN_STATE"
	ErrLoginFailed         ErrorCode = "E_LOGIN_FAILED"      // detail: OAuth error value
	ErrNetwork             ErrorCode = "E_NETWORK"           // detail: endpoint host
	ErrAWSAccessDenied     ErrorCode = "E_AWS_ACCESS_DENIED" // detail: action, e.g. iam:UpdateAccessKey
	ErrAWSInvalidKey       ErrorCode = "E_AWS_INVALID_KEY"   // detail: AWS error code
	ErrAWSAPI              ErrorCode = "E_AWS_API"           // detail: AWS error code
	ErrRoleNotAssigned     ErrorCode = "E_ROLE_NOT_ASSIGNED"
	ErrVerifyWrongAccount  ErrorCode = "E_VERIFY_WRONG_ACCOUNT" // params: got, want
	ErrMigrateConfirmOther ErrorCode = "E_MIGRATE_CONFIRM_OTHER"
	ErrMigrateState        ErrorCode = "E_MIGRATE_STATE"
	ErrNoStaticKey         ErrorCode = "E_NO_STATIC_KEY" // params: name
	ErrAWSCLIMissing       ErrorCode = "E_AWS_CLI_MISSING"
	ErrAWSCLITooOld        ErrorCode = "E_AWS_CLI_TOO_OLD" // params: version, min
	ErrGitMissing          ErrorCode = "E_GIT_MISSING"
	ErrGitTooOld           ErrorCode = "E_GIT_TOO_OLD"    // params: version, min
	ErrGitFailed           ErrorCode = "E_GIT_FAILED"     // detail: redacted stderr tail
	ErrRepoNotFound        ErrorCode = "E_REPO_NOT_FOUND" // params: path
	ErrNotCodeCommit       ErrorCode = "E_NOT_CODECOMMIT"
	ErrRemoteSSH           ErrorCode = "E_REMOTE_SSH"
	ErrRemoteGRC           ErrorCode = "E_REMOTE_GRC"
	ErrUnsafeValue         ErrorCode = "E_UNSAFE_VALUE"    // params: value
	ErrDestExists          ErrorCode = "E_DEST_EXISTS"     // params: path
	ErrCloneFailed         ErrorCode = "E_CLONE_FAILED"    // detail: redacted stderr tail
	ErrURLRulesFirst       ErrorCode = "E_URL_RULES_FIRST" // params: n
	ErrBackupNotFound      ErrorCode = "E_BACKUP_NOT_FOUND"
	ErrRestoreNeedsConfirm ErrorCode = "E_RESTORE_NEEDS_CONFIRM" // params: profiles (comma separated)
	ErrSandbox             ErrorCode = "E_SANDBOX"
	ErrUnsupportedOS       ErrorCode = "E_UNSUPPORTED_OS"
	ErrImportFormat        ErrorCode = "E_IMPORT_FORMAT" // params: reason (not-json | format | version | size | limit)
)

// AppError is the only error type returned by bound methods. Wails delivers its JSON to JS as error.cause.
// Message is English for logs. Detail and Params never contain secrets.
type AppError struct {
	Code    ErrorCode         `json:"code"`
	Message string            `json:"message"`
	Detail  string            `json:"detail,omitempty"`
	Params  map[string]string `json:"params,omitempty"`
}

func (e *AppError) Error() string { return string(e.Code) + ": " + e.Message }

// NewError builds an AppError; Message is fmt.Sprintf(format, args...) passed through redact.String.
// format must be a constant string (go vet printf check).
func NewError(code ErrorCode, format string, args ...any) *AppError {
	return &AppError{Code: code, Message: redact.String(fmt.Sprintf(format, args...))}
}

// WithDetail sets Detail (redacted) and returns e.
func (e *AppError) WithDetail(detail string) *AppError { e.Detail = redact.String(detail); return e }

// WithParam sets Params[key] = value (redacted) and returns e.
func (e *AppError) WithParam(key, value string) *AppError {
	if e.Params == nil {
		e.Params = map[string]string{}
	}
	e.Params[key] = redact.String(value)
	return e
}

func NotImplemented() *AppError { return NewError(ErrNotImplemented, "not implemented") }

// AsAppError returns nil for nil, the *AppError inside err (errors.As), or E_INTERNAL with a redacted Detail.
func AsAppError(err error) *AppError {
	if err == nil {
		return nil
	}
	var ae *AppError
	if errors.As(err, &ae) {
		return ae
	}
	return NewError(ErrInternal, "internal error").WithDetail(err.Error())
}
