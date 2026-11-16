// Package sso implements the AWS CLI compatible SSO token cache, native OIDC sign-in (PKCE and device code),
// silent refresh, portal API calls and per-session logout.
package sso

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rudian/aws_sso_hub/internal/backup"
)

// CLITimeLayout is the exact layout botocore writes: strftime('%Y-%m-%dT%H:%M:%SZ'), always UTC.
const CLITimeLayout = "2006-01-02T15:04:05Z"

// TokenCacheDir returns <home>/.aws/sso/cache.
func TokenCacheDir(home string) string { return filepath.Join(home, ".aws", "sso", "cache") }

// CLICacheDir returns <home>/.aws/cli/cache (role credential cache of the AWS CLI).
func CLICacheDir(home string) string { return filepath.Join(home, ".aws", "cli", "cache") }

// TokenCacheKey = sha1(session name) for [sso-session X]; sha1(start URL as configured) for legacy profiles.
func TokenCacheKey(sessionName, startURL string) string {
	in := startURL
	if sessionName != "" {
		in = sessionName
	}
	sum := sha1.Sum([]byte(in))
	return hex.EncodeToString(sum[:])
}

func jsonString(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(buf.String(), "\n")
}

// RegistrationCacheKey reproduces botocore's client registration cache key (kept for compatibility tests).
// scopes == nil encodes JSON null (the key is absent from the sso-session section).
func RegistrationCacheKey(startURL, region string, scopes []string, sessionName string) string {
	sc := "null"
	if scopes != nil {
		parts := make([]string, len(scopes))
		for i, s := range scopes {
			parts[i] = jsonString(s)
		}
		sc = "[" + strings.Join(parts, ", ") + "]"
	}
	sn := "null"
	if sessionName != "" {
		sn = jsonString(sessionName)
	}
	s := fmt.Sprintf(`{"region": %s, "scopes": %s, "session_name": %s, "startUrl": %s, "tool": "botocore"}`,
		jsonString(region), sc, sn, jsonString(startURL))
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// RoleCacheKey is the AWS CLI's ~/.aws/cli/cache key for an SSO role:
// sha1(json.dumps({"accountId","roleName","sessionName"|"startUrl"}, sort_keys=True, separators=(',',':'))).
// Pass sessionName "" and the start URL for legacy profiles.
func RoleCacheKey(accountID, roleName, sessionName, startURL string) string {
	var s string
	if sessionName != "" {
		s = `{"accountId":` + jsonString(accountID) + `,"roleName":` + jsonString(roleName) + `,"sessionName":` + jsonString(sessionName) + `}`
	} else {
		s = `{"accountId":` + jsonString(accountID) + `,"roleName":` + jsonString(roleName) + `,"startUrl":` + jsonString(startURL) + `}`
	}
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ClientIDHash is the first 16 hex chars of sha256(clientID); "" for an empty id (session lineage marker).
func ClientIDHash(clientID string) string {
	if clientID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(clientID))
	return hex.EncodeToString(sum[:])[:16]
}

// CachedToken is the exact shape the AWS CLI v2 writes to ~/.aws/sso/cache/<sha1(session)>.json.
// Every field except ExpiresAt that is a string may be a SECRET; never log a CachedToken.
type CachedToken struct {
	StartURL              string
	Region                string
	AccessToken           string // SECRET
	ExpiresAt             time.Time
	ClientID              string
	ClientSecret          string // SECRET
	RegistrationExpiresAt time.Time
	RefreshToken          string // SECRET, optional
	Extra                 map[string]json.RawMessage
	// Granted is the lifetime AWS gave the access token when this process received it (sign-in or renewal).
	// It is 0 for a token read from the cache file and is never written to it.
	Granted time.Duration
}

// String and GoString never print secrets.
func (t CachedToken) String() string   { return "CachedToken(" + t.Region + ")" }
func (t CachedToken) GoString() string { return t.String() }

func (t CachedToken) MarshalJSON() ([]byte, error) {
	m := map[string]any{}
	for k, v := range t.Extra {
		m[k] = v
	}
	m["startUrl"] = t.StartURL
	m["region"] = t.Region
	m["accessToken"] = t.AccessToken
	m["expiresAt"] = t.ExpiresAt.UTC().Format(CLITimeLayout)
	if t.ClientID != "" {
		m["clientId"] = t.ClientID
		m["clientSecret"] = t.ClientSecret
	}
	if !t.RegistrationExpiresAt.IsZero() {
		m["registrationExpiresAt"] = t.RegistrationExpiresAt.UTC().Format(CLITimeLayout)
	}
	if t.RefreshToken != "" {
		m["refreshToken"] = t.RefreshToken
	} else {
		delete(m, "refreshToken")
	}
	return json.Marshal(m)
}

func (t *CachedToken) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	str := func(k string) string {
		var s string
		if v, ok := raw[k]; ok {
			_ = json.Unmarshal(v, &s)
			delete(raw, k)
		}
		return s
	}
	tm := func(k string) (time.Time, error) {
		s := str(k)
		if s == "" {
			return time.Time{}, nil
		}
		return time.Parse(time.RFC3339, s)
	}
	var err error
	t.StartURL, t.Region = str("startUrl"), str("region")
	t.AccessToken, t.ClientID = str("accessToken"), str("clientId")
	t.ClientSecret, t.RefreshToken = str("clientSecret"), str("refreshToken")
	if t.ExpiresAt, err = tm("expiresAt"); err != nil {
		return fmt.Errorf("expiresAt: %w", err)
	}
	if t.RegistrationExpiresAt, err = tm("registrationExpiresAt"); err != nil {
		return fmt.Errorf("registrationExpiresAt: %w", err)
	}
	t.Extra = raw
	return nil
}

// ReadToken loads a token file; errors.Is(err, fs.ErrNotExist) when missing. Never include file content in errors.
func ReadToken(dir, key string) (*CachedToken, error) {
	b, err := os.ReadFile(filepath.Join(dir, key+".json"))
	if err != nil {
		return nil, err
	}
	var t CachedToken
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("token cache %s.json is not a valid token file", key)
	}
	if t.AccessToken == "" || t.ExpiresAt.IsZero() {
		return nil, fmt.Errorf("token cache %s.json lacks accessToken or expiresAt", key)
	}
	return &t, nil
}

// WriteToken writes the token file atomically (private temp file, fsync, rename) with mode 0600. The cache
// directory is created 0700; an existing one that others can write to is tightened to 0700.
func WriteToken(dir, key string, t *CachedToken) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() && fi.Mode().Perm()&0o022 != 0 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return backup.WriteFileAtomic(filepath.Join(dir, key+".json"), b, 0o600)
}

// TokenState mirrors the AWS CLI's decisions.
type TokenState int

const (
	TokenValid       TokenState = iota // at least 15 min left
	TokenRefreshable                   // less than 15 min left (or expired) but refresh is possible
	TokenNeedsLogin
)

const refreshWindow = 15 * time.Minute

func canRefresh(t *CachedToken, now time.Time) bool {
	return t.RefreshToken != "" && t.ClientID != "" && t.ClientSecret != "" &&
		!t.RegistrationExpiresAt.IsZero() && t.RegistrationExpiresAt.After(now)
}

// ClassifyToken follows botocore's SSOTokenProvider (_REFRESH_WINDOW = 15 min).
func ClassifyToken(t *CachedToken, now time.Time) TokenState {
	if t.ExpiresAt.Sub(now) >= refreshWindow {
		return TokenValid
	}
	if canRefresh(t, now) {
		return TokenRefreshable
	}
	return TokenNeedsLogin
}
