// Package transfer exports the SSO portals and profiles of ~/.aws/config as a versioned JSON document and
// imports such a document again. The document is an allow-list: only the keys below are ever written, and
// nothing that looks like a credential or a command is accepted on import.
package transfer

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/awsconfig"
	"github.com/rudian/aws_sso_hub/internal/identity"
)

const (
	// FormatName is the fixed marker in the "format" key.
	FormatName = "aws-sso-hub.accounts"
	// FormatVersion is the only version this build reads and writes.
	FormatVersion = 1
	// MaxFileBytes bounds both files and pasted text. The text travels to the backend again inside
	// ImportRequest.JSON for every preview and for the final apply, JSON-escaped (about +20 % for a typical
	// export) next to the per-item choices. The web view's runtime splits any call body longer than 512 KiB into
	// chunks, which the runtime guard cannot inspect and refuses. 384 KiB keeps every legitimate import call
	// below that limit; runtimeguard_test.go checks the margin.
	MaxFileBytes = 384 << 10

	maxPortals  = 200
	maxProfiles = 5000

	minSessionMinutes     = 15
	maxSessionMinutes     = 129600
	defaultSessionMinutes = 480
)

var (
	accountIDRE = regexp.MustCompile(`^[0-9]{12}$`)
	roleNameRE  = awsconfig.RoleNameRE
	scopeRE     = awsconfig.ScopeRE
	// sensitiveKeyRE rejects every key that names a secret or something executable.
	sensitiveKeyRE = regexp.MustCompile(`(?i)secret|token|password|process|command`)
	// sensitiveValueRE rejects values shaped like AWS access key ids anywhere in an entry.
	sensitiveValueRE = regexp.MustCompile(`A(KI|SI)A[0-9A-Z]{16}`)
	outputFormats    = map[string]bool{"json": true, "text": true, "table": true, "yaml": true, "yaml-stream": true}
)

// credentialKeys are the config keys that carry credentials or change how credentials are obtained.
// Keys are compared lower-case without "_" and "-" so aws_access_key_id and awsAccessKeyId are the same key.
var credentialKeys = func() map[string]bool {
	m := map[string]bool{}
	for _, k := range []string{"aws_access_key_id", "aws_secret_access_key", "aws_session_token", "aws_security_token",
		"credential_process", "credential_source", "role_arn", "source_profile", "web_identity_token_file"} {
		m[normKey(k)] = true
	}
	return m
}()

func normKey(k string) string {
	return strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(k))
}

// ---- document types (export) ---------------------------------------------------------------------------

type fileDoc struct {
	Format     string       `json:"format"`
	Version    int          `json:"version"`
	ExportedAt string       `json:"exportedAt"`
	ExportedBy string       `json:"exportedBy"`
	Portals    []portalDoc  `json:"portals"`
	Profiles   []profileDoc `json:"profiles"`
}

type portalDoc struct {
	Name           string   `json:"name"`
	StartURL       string   `json:"startUrl"`
	Region         string   `json:"region"`
	Scopes         []string `json:"scopes,omitempty"`
	SessionMinutes int      `json:"sessionMinutes,omitempty"`
}

type profileDoc struct {
	Name        string `json:"name"`
	Portal      string `json:"portal"`
	AccountID   string `json:"accountId"`
	RoleName    string `json:"roleName"`
	Region      string `json:"region,omitempty"`
	Output      string `json:"output,omitempty"`
	AccountName string `json:"accountName,omitempty"`
	Color       int    `json:"color,omitempty"`
	Monogram    string `json:"monogram,omitempty"`
}

func marshalDoc(d fileDoc) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// DefaultExportName is the suggested file name in the save dialog.
func DefaultExportName(t time.Time) string {
	return "aws-sso-hub-accounts-" + t.Format("20060102") + ".json"
}

// ---- parsed document (import) ---------------------------------------------------------------------------

// rejection says why an entry cannot be imported.
type rejection struct {
	reasonKey string
	code      api.ErrorCode
	detail    string
}

const (
	reasonSecret   = "import.rejected.secret"
	reasonInvalid  = "import.rejected.invalid"
	reasonNoPortal = "import.rejected.noPortal"
)

type portalEntry struct {
	Name           string // sanitised for display when rejected
	StartURL       string
	Region         string
	Scopes         []string
	SessionMinutes int
	Warnings       []string
	Rej            *rejection
}

type profileEntry struct {
	Name        string
	Portal      string
	AccountID   string
	Role        string
	Region      string
	Output      string
	AccountName string
	Color       int
	Monogram    string
	Warnings    []string
	Rej         *rejection
}

// Parsed is a validated document. Entries that fail validation are kept (with Rej set) so the preview can
// show them.
type Parsed struct {
	ExportedBy string
	Portals    []portalEntry
	Profiles   []profileEntry
}

func formatErr(reason string) *api.AppError {
	return api.NewError(api.ErrImportFormat, "not an AWS SSO Hub accounts export").WithParam("reason", reason)
}

// Parse reads and validates the document. A malformed or foreign document is an E_IMPORT_FORMAT error; single
// bad entries are marked rejected instead.
func Parse(text string) (*Parsed, error) {
	if len(text) > MaxFileBytes {
		return nil, formatErr("size")
	}
	if !utf8.ValidString(text) {
		return nil, formatErr("not-json")
	}
	text = strings.TrimPrefix(text, "\ufeff")
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var top map[string]json.RawMessage
	if err := dec.Decode(&top); err != nil {
		return nil, formatErr("not-json")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, formatErr("not-json")
	}
	var format string
	if raw, ok := top["format"]; !ok || json.Unmarshal(raw, &format) != nil || format != FormatName {
		return nil, formatErr("format")
	}
	var ver json.Number
	if raw, ok := top["version"]; !ok || bytes.HasPrefix(bytes.TrimSpace(raw), []byte(`"`)) || json.Unmarshal(raw, &ver) != nil || ver.String() != "1" {
		return nil, formatErr("version")
	}
	out := &Parsed{}
	if raw, ok := top["exportedBy"]; ok {
		var by string
		if json.Unmarshal(raw, &by) == nil {
			out.ExportedBy = sanitize(by, 80)
		}
	}
	portals, err := entryList(top["portals"], maxPortals)
	if err != nil {
		return nil, err
	}
	profiles, err := entryList(top["profiles"], maxProfiles)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, raw := range portals {
		e := parsePortal(raw)
		if e.Rej == nil {
			if seen[e.Name] {
				e.Rej = &rejection{reasonKey: reasonInvalid, code: api.ErrInvalidName, detail: "name"}
			}
			seen[e.Name] = true
		}
		out.Portals = append(out.Portals, e)
	}
	seen = map[string]bool{}
	for _, raw := range profiles {
		e := parseProfile(raw)
		if e.Rej == nil {
			if seen[e.Name] {
				e.Rej = &rejection{reasonKey: reasonInvalid, code: api.ErrInvalidName, detail: "name"}
			}
			seen[e.Name] = true
		}
		out.Profiles = append(out.Profiles, e)
	}
	return out, nil
}

func entryList(raw json.RawMessage, limit int) ([]json.RawMessage, error) {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil, nil
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, formatErr("format")
	}
	if len(list) > limit {
		return nil, formatErr("limit")
	}
	return list, nil
}

// sanitize makes untrusted text safe to show: control characters become "?", the result is cut to max runes.
func sanitize(s string, max int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= max {
			b.WriteString("…")
			break
		}
		if unsafeRune(r) {
			r = '?'
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

func unsafeRune(r rune) bool {
	return unicode.IsControl(r) || r == '\u2028' || r == '\u2029' || r == '\ufeff' || r == utf8.RuneError
}

func decodeObject(raw json.RawMessage) (map[string]any, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	m, ok := v.(map[string]any)
	return m, ok
}

// screen applies the safety rules that hold for every entry kind. It returns the decoded object, or the
// reason for rejecting it plus a display name.
func screen(raw json.RawMessage) (map[string]any, string, *rejection) {
	m, ok := decodeObject(raw)
	if !ok {
		return nil, "?", &rejection{reasonKey: reasonInvalid, detail: "entry"}
	}
	name := displayName(m)
	if hasSensitive(m) {
		return nil, name, &rejection{reasonKey: reasonSecret}
	}
	if hasUnsafe(m) {
		return nil, name, &rejection{reasonKey: reasonInvalid, code: api.ErrUnsafeValue}
	}
	return m, name, nil
}

func hasSensitive(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if sensitiveKeyRE.MatchString(k) || credentialKeys[normKey(k)] || hasSensitive(val) {
				return true
			}
		}
	case []any:
		for _, val := range x {
			if hasSensitive(val) {
				return true
			}
		}
	case string:
		return sensitiveValueRE.MatchString(x)
	}
	return false
}

func hasUnsafe(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if hasUnsafeString(k) || hasUnsafe(val) {
				return true
			}
		}
	case []any:
		for _, val := range x {
			if hasUnsafe(val) {
				return true
			}
		}
	case string:
		return hasUnsafeString(x)
	}
	return false
}

func hasUnsafeString(s string) bool {
	for _, r := range s {
		if unsafeRune(r) {
			return true
		}
	}
	return false
}

func displayName(m map[string]any) string {
	if s, ok := m["name"].(string); ok && s != "" {
		return sanitize(s, 64)
	}
	return "?"
}

// fields reads typed values from a screened entry and remembers the first problem.
type fields struct {
	m    map[string]any
	used map[string]bool
	rej  *rejection
}

func newFields(m map[string]any) *fields { return &fields{m: m, used: map[string]bool{}} }

func (f *fields) bad(code api.ErrorCode, detail string) {
	if f.rej == nil {
		f.rej = &rejection{reasonKey: reasonInvalid, code: code, detail: detail}
	}
}

// str returns an optional string; a value of another type is invalid.
func (f *fields) str(key string) string {
	f.used[key] = true
	v, ok := f.m[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		f.bad(api.ErrInvalidInput, key)
		return ""
	}
	return strings.TrimSpace(s)
}

// integer returns an optional whole number.
func (f *fields) integer(key string) int {
	f.used[key] = true
	v, ok := f.m[key]
	if !ok || v == nil {
		return 0
	}
	n, ok := v.(json.Number)
	if !ok {
		f.bad(api.ErrInvalidInput, key)
		return 0
	}
	i, err := n.Int64()
	if err != nil || i < -1<<31 || i > 1<<31 {
		f.bad(api.ErrInvalidInput, key)
		return 0
	}
	return int(i)
}

func (f *fields) strList(key string, max int) []string {
	f.used[key] = true
	v, ok := f.m[key]
	if !ok || v == nil {
		return nil
	}
	list, ok := v.([]any)
	if !ok || len(list) > max {
		f.bad(api.ErrInvalidInput, key)
		return nil
	}
	out := make([]string, 0, len(list))
	for _, it := range list {
		s, ok := it.(string)
		if !ok {
			f.bad(api.ErrInvalidInput, key)
			return nil
		}
		out = append(out, strings.TrimSpace(s))
	}
	return out
}

// unknown lists the keys that are not part of the format, as warning codes.
func (f *fields) unknown() []string {
	var keys []string
	for k := range f.m {
		if !f.used[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, "unknown-key:"+sanitize(k, 40))
	}
	return out
}

func parsePortal(raw json.RawMessage) portalEntry {
	m, name, rej := screen(raw)
	if rej != nil {
		return portalEntry{Name: name, Rej: rej}
	}
	f := newFields(m)
	e := portalEntry{Name: f.str("name")}
	if !awsconfig.SessionNameRE.MatchString(e.Name) {
		f.bad(api.ErrInvalidName, "name")
		e.Name = name
	}
	rawURL := f.str("startUrl")
	region := f.str("region")
	e.Scopes = f.strList("scopes", 10)
	e.SessionMinutes = f.integer("sessionMinutes")
	norm, guessed, nerr := awsconfig.NormalizeStartURL(rawURL)
	if nerr != nil {
		f.bad(api.ErrInvalidStartURL, "startUrl")
	}
	e.StartURL = norm
	if region == "" {
		region = guessed
	}
	if !awsconfig.RegionRE.MatchString(region) {
		f.bad(api.ErrInvalidRegion, "region")
	}
	e.Region = region
	for _, s := range e.Scopes {
		if !scopeRE.MatchString(s) {
			f.bad(api.ErrInvalidInput, "scopes")
		}
	}
	if e.SessionMinutes != 0 && (e.SessionMinutes < minSessionMinutes || e.SessionMinutes > maxSessionMinutes) {
		f.bad(api.ErrInvalidInput, "sessionMinutes")
	}
	e.Warnings = f.unknown()
	e.Rej = f.rej
	return e
}

func parseProfile(raw json.RawMessage) profileEntry {
	m, name, rej := screen(raw)
	if rej != nil {
		return profileEntry{Name: name, Rej: rej}
	}
	f := newFields(m)
	e := profileEntry{Name: f.str("name")}
	if !awsconfig.ProfileNameRE.MatchString(e.Name) {
		f.bad(api.ErrInvalidName, "name")
		e.Name = name
	}
	e.Portal = f.str("portal")
	if !awsconfig.SessionNameRE.MatchString(e.Portal) {
		f.bad(api.ErrInvalidName, "portal")
	}
	e.AccountID = f.str("accountId")
	if !accountIDRE.MatchString(e.AccountID) {
		f.bad(api.ErrInvalidInput, "accountId")
	}
	e.Role = f.str("roleName")
	if !roleNameRE.MatchString(e.Role) {
		f.bad(api.ErrInvalidInput, "roleName")
	}
	e.Region = f.str("region")
	if e.Region != "" && !awsconfig.RegionRE.MatchString(e.Region) {
		f.bad(api.ErrInvalidRegion, "region")
	}
	e.Output = f.str("output")
	if e.Output != "" && !outputFormats[e.Output] {
		f.bad(api.ErrInvalidInput, "output")
	}
	e.AccountName = f.str("accountName")
	if utf8.RuneCountInString(e.AccountName) > 128 {
		f.bad(api.ErrInvalidInput, "accountName")
	}
	e.Color = f.integer("color")
	if e.Color != 0 && (e.Color < 1 || e.Color > 8) {
		f.bad(api.ErrInvalidInput, "color")
	}
	e.Monogram = f.str("monogram")
	if e.Monogram != "" && !identity.ValidMonogram(e.Monogram) {
		f.bad(api.ErrInvalidInput, "monogram")
	}
	e.Warnings = f.unknown()
	e.Rej = f.rej
	return e
}
