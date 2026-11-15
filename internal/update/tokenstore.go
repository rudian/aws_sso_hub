package update

import (
	"errors"
	"regexp"
	"strings"
)

// TokenStore keeps the read-only token of the private test repository. It is entered once and never compiled in;
// the public production channel needs none. The token is never logged and never put in an error text.
type TokenStore interface {
	Get() (string, error) // "" with a nil error when none is stored
	Set(token string) error
	Delete() error
}

// ErrTokenStoreUnsupported is returned where no secure store exists.
var ErrTokenStoreUnsupported = errors.New("update: no secure store for the test-channel token on this system")

// A GitHub fine-grained or classic token is letters, digits, "_" and "-". Anything else is refused.
var reToken = regexp.MustCompile(`^[A-Za-z0-9_-]{20,255}$`)

const (
	// tokenService is the name the item carries in the Keychain / Credential Manager; check A16 must look for it.
	tokenService = "Codepass update test channel"
	tokenAccount = "read-only token"
)

// secretBackend is the platform primitive: one secret under one name. macOS talks to Security.framework from this
// process (tokenstore_darwin.go) so the item's access list holds Codepass's own signature, not a helper's; Windows
// uses Credential Manager (tokenstore_windows.go). Nothing here runs a command-line tool: an item made by
// /usr/bin/security trusts that tool, and any process of the user could then read the token without a prompt.
type secretBackend interface {
	read() (secret []byte, found bool, err error)
	write(secret []byte) error
	remove() error
}

// secretStore applies the token rules on top of a backend.
type secretStore struct{ b secretBackend }

func (s secretStore) Get() (string, error) {
	b, found, err := s.b.read()
	if err != nil {
		return "", err
	}
	if !found {
		return "", nil
	}
	tok := strings.TrimSpace(string(b))
	if !reToken.MatchString(tok) {
		return "", errors.New("the stored token is not a valid token")
	}
	return tok, nil
}

func (s secretStore) Set(token string) error {
	if !reToken.MatchString(token) {
		return errors.New("that does not look like a GitHub token")
	}
	return s.b.write([]byte(token))
}

func (s secretStore) Delete() error { return s.b.remove() }

type unsupportedStore struct{}

func (unsupportedStore) Get() (string, error) { return "", ErrTokenStoreUnsupported }
func (unsupportedStore) Set(string) error     { return ErrTokenStoreUnsupported }
func (unsupportedStore) Delete() error        { return ErrTokenStoreUnsupported }

// NewTokenStore returns the store of this system: the macOS Keychain, the Windows Credential Manager, or an
// unsupported store. In sandbox mode (Paths.Sandbox) it never touches the real keychain and returns the
// unsupported store, as credstore.New does for its secrets.
func NewTokenStore(sandbox bool) TokenStore {
	if sandbox {
		return unsupportedStore{}
	}
	if b := platformBackend(tokenService, tokenAccount); b != nil {
		return secretStore{b: b}
	}
	return unsupportedStore{}
}
