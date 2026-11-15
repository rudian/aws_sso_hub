package update

import (
	"errors"
	"fmt"
)

// Refusal is a security refusal: the release was found but must not be installed (bad signature, wrong version,
// wrong bundle...). The UI shows it differently from a network error, and a refusal is never retried silently.
type Refusal struct {
	Reason string
	Code   string // stable, see the Code* constants; the UI maps it to text in both languages
}

func (r *Refusal) Error() string { return r.Reason }

// Stable refusal codes. They cross into the frontend, so they never change; the English Reason is for the log.
const (
	CodeManifestMissing   = "manifest-missing"
	CodeManifestSignature = "manifest-signature"
	CodeManifestInvalid   = "manifest-invalid"
	CodeVersionTag        = "version-tag"
	CodeVersionNotNewer   = "version-not-newer"
	CodeBelowHighest      = "below-highest"
	CodeHighestUnreadable = "highest-unreadable"
	CodePlatform          = "platform"
	CodeAsset             = "asset"
	CodeSize              = "size"
	CodeDigest            = "digest"
	CodeBundleInvalid     = "bundle-invalid"
	CodeBundleID          = "bundle-id"
	CodeBundleVersion     = "bundle-version"
	CodeAppleDouble       = "appledouble"
	CodeCodesign          = "codesign"
	CodeGatekeeper        = "gatekeeper"
	CodeNoKey             = "no-key"
	CodeChannel           = "channel"
	CodeConfig            = "config"
	CodeRefused           = "refused" // a refusal without a more specific code
)

// codedError carries a Code through plain error returns until refuseErr turns it into a Refusal.
type codedError struct {
	code string
	err  error
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

func codedf(code, format string, a ...any) error {
	return &codedError{code: code, err: fmt.Errorf(format, a...)}
}

func codeOf(err error) string {
	var c *codedError
	if errors.As(err, &c) {
		return c.code
	}
	var r *Refusal
	if errors.As(err, &r) && r.Code != "" {
		return r.Code
	}
	return ""
}

func refuse(format string, a ...any) error {
	return &Refusal{Reason: fmt.Sprintf(format, a...), Code: CodeRefused}
}

func refuseAs(code, format string, a ...any) error {
	return &Refusal{Reason: fmt.Sprintf(format, a...), Code: code}
}

// refuseErr makes a Refusal of err, keeping the code it carries.
func refuseErr(err error) error {
	code := codeOf(err)
	if code == "" {
		code = CodeRefused
	}
	return &Refusal{Reason: err.Error(), Code: code}
}

// RefusalCode returns the code of a refusal inside err, or "".
func RefusalCode(err error) string {
	var r *Refusal
	if errors.As(err, &r) {
		return r.Code
	}
	return ""
}

// IsRefusal reports whether err is (or wraps) a Refusal.
func IsRefusal(err error) bool { var r *Refusal; return errors.As(err, &r) }

// ErrUpToDate is returned when no newer release exists.
var ErrUpToDate = errors.New("up to date")

// Block kinds of a PreflightError.
const (
	BlockTranslocated = "translocated"   // macOS runs the app from a read-only random location
	BlockNotWritable  = "not-writable"   // the folder holding the app cannot be written by this user
	BlockUnsupported  = "unsupported-os" // no payload check exists for this system yet
)

// PreflightError says, before anything is downloaded, why this copy cannot replace itself.
type PreflightError struct {
	Kind   string
	Folder string // the folder to name in the message
	Detail string
}

func (e *PreflightError) Error() string {
	if e.Folder != "" {
		return fmt.Sprintf("cannot update in place (%s): %s", e.Kind, e.Folder)
	}
	return fmt.Sprintf("cannot update in place (%s): %s", e.Kind, e.Detail)
}
