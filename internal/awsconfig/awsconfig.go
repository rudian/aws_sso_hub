// Package awsconfig reads and edits ~/.aws/config and ~/.aws/credentials.
package awsconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/ini"
	"github.com/rudian/aws_sso_hub/internal/redact"
	"github.com/rudian/aws_sso_hub/internal/textdiff"
)

// Files are the two absolute paths.
type Files struct{ Config, Credentials string }

// Profile is one named profile merged from both files.
type Profile struct {
	Name                                                      string
	Kind                                                      api.ProfileKind
	SSOSession, SSOStartURL, SSORegion, AccountID, Role       string
	Region, Output, RoleARN, SourceProfile, CredentialProcess string
	InConfig, InCredentials, HasStaticKeys                    bool
	StaticKeysIn                                              string // "credentials" | "config" | ""
	Order                                                     int    // config file order; credentials-only profiles after
}

// Session is an [sso-session] section.
type Session struct {
	Name, StartURL, Region string
	Scopes                 []string
	ScopesSet              bool
}

// StaticKey is a SECRET; its String method never prints it.
type StaticKey struct{ AccessKeyID, SecretAccessKey, SessionToken string }

func (k StaticKey) String() string { return "StaticKey(" + redact.MaskKeyID(k.AccessKeyID) + ")" }

// GoString keeps %#v from leaking the secret as well.
func (k StaticKey) GoString() string { return k.String() }

var (
	// ProfileNameRE limits profile names to characters that are safe in INI sections and git config values.
	ProfileNameRE = regexp.MustCompile(`^[A-Za-z0-9_.@+=,-]{1,64}$`)
	SessionNameRE = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	RegionRE      = regexp.MustCompile(`^[a-z]{2}(-gov|-iso[a-z]*)?-[a-z]+-[0-9]{1,2}$`)
	// AccountIDRE matches a 12-digit AWS account id.
	AccountIDRE = regexp.MustCompile(`^[0-9]{12}$`)
	// RoleNameRE matches a permission set / IAM role name as it may be written to sso_role_name.
	RoleNameRE = regexp.MustCompile(`^[A-Za-z0-9_+=,.@-]{1,64}$`)
	// ScopeRE matches one OAuth scope of sso_registration_scopes.
	ScopeRE = regexp.MustCompile(`^[A-Za-z0-9:_./-]{1,128}$`)
)

// Snapshot is a consistent read of both files.
type Snapshot struct {
	Files Files

	cfgRaw, credRaw       []byte
	cfgExists, credExists bool
	cfg, cred             *ini.Doc
	cfgErr, credErr       *ini.ParseError
}

// Load reads both files. Missing files are empty; parse errors become Issues, not errors.
func Load(f Files) (*Snapshot, error) {
	s := &Snapshot{Files: f}
	var err error
	if s.cfgRaw, s.cfgExists, err = readOptional(f.Config); err != nil {
		return nil, err
	}
	if s.credRaw, s.credExists, err = readOptional(f.Credentials); err != nil {
		return nil, err
	}
	s.cfg, s.cfgErr = parse(s.cfgRaw)
	s.cred, s.credErr = parse(s.credRaw)
	return s, nil
}

func readOptional(path string) ([]byte, bool, error) {
	if path == "" {
		return nil, false, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, api.NewError(api.ErrFileIO, "cannot read file").WithParam("file", path).WithDetail(err.Error())
	}
	return b, true, nil
}

func parse(b []byte) (*ini.Doc, *ini.ParseError) {
	d, err := ini.Parse(b)
	if err != nil {
		var pe *ini.ParseError
		if errors.As(err, &pe) {
			empty, _ := ini.Parse(nil)
			return empty, pe
		}
		empty, _ := ini.Parse(nil)
		return empty, &ini.ParseError{Line: 1, Msg: err.Error()}
	}
	return d, nil
}

func (s *Snapshot) ConfigBytes() []byte      { return s.cfgRaw }
func (s *Snapshot) CredentialsBytes() []byte { return s.credRaw }

// ConfigHash is the sha256 of the config bytes, "" if the file is missing.
func (s *Snapshot) ConfigHash() string {
	if !s.cfgExists {
		return ""
	}
	return Hash(nonNil(s.cfgRaw))
}

func (s *Snapshot) CredentialsHash() string {
	if !s.credExists {
		return ""
	}
	return Hash(nonNil(s.credRaw))
}

func nonNil(b []byte) []byte {
	if b == nil {
		return []byte{}
	}
	return b
}

// Hash is the sha256 hex of b, "" for nil.
func Hash(b []byte) string {
	if b == nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ReadOnly is true when either file has a parse error.
func (s *Snapshot) ReadOnly() bool { return s.cfgErr != nil || s.credErr != nil }

// Issues lists parse errors and dangling sso_session references. display maps absolute paths to display paths.
func (s *Snapshot) Issues(display func(string) string) []api.ConfigIssue {
	if display == nil {
		display = func(p string) string { return p }
	}
	out := make([]api.ConfigIssue, 0)
	if s.cfgErr != nil {
		out = append(out, api.ConfigIssue{File: display(s.Files.Config), Line: s.cfgErr.Line, Message: s.cfgErr.Msg})
	}
	if s.credErr != nil {
		out = append(out, api.ConfigIssue{File: display(s.Files.Credentials), Line: s.credErr.Line, Message: s.credErr.Msg})
	}
	if s.cfgErr == nil {
		for _, sec := range s.cfg.Sections() {
			name, ok := profileFromSection(sec)
			if !ok {
				continue
			}
			ses, _ := s.cfg.Get(sec, "sso_session")
			if ses != "" && !s.cfg.Has(SessionSection(ses)) {
				out = append(out, api.ConfigIssue{
					File: display(s.Files.Config), Line: s.cfg.SectionLine(sec),
					Message: fmt.Sprintf("sso-session %s not found (profile %s)", ses, name),
				})
			}
		}
	}
	return out
}

func profileFromSection(sec string) (string, bool) {
	switch {
	case sec == "default":
		return "default", true
	case strings.HasPrefix(sec, "profile "):
		n := strings.TrimSpace(strings.TrimPrefix(sec, "profile "))
		if n == "" {
			return "", false
		}
		return n, true
	}
	return "", false
}

// ConfigSection is the config-file section name of a profile.
func ConfigSection(profile string) string {
	if profile == "default" {
		return "default"
	}
	return "profile " + profile
}

// SessionSection is the section name of an sso-session.
func SessionSection(name string) string { return "sso-session " + name }

func hasKeys(d *ini.Doc, sec string) bool {
	id, _ := d.Get(sec, "aws_access_key_id")
	sk, _ := d.Get(sec, "aws_secret_access_key")
	return id != "" && sk != ""
}

// Profiles returns all profiles sorted by name.
func (s *Snapshot) Profiles() []Profile {
	byName := map[string]*Profile{}
	order := 0
	get := func(n string) *Profile {
		p, ok := byName[n]
		if !ok {
			p = &Profile{Name: n}
			byName[n] = p
		}
		return p
	}
	for _, sec := range s.cfg.Sections() {
		n, ok := profileFromSection(sec)
		if !ok {
			continue
		}
		p := get(n)
		if !p.InConfig {
			p.InConfig = true
			p.Order = order
			order++
		}
		g := func(k string) string { v, _ := s.cfg.Get(sec, k); return v }
		p.SSOSession, p.SSOStartURL, p.SSORegion = g("sso_session"), g("sso_start_url"), g("sso_region")
		p.AccountID, p.Role = g("sso_account_id"), g("sso_role_name")
		p.Region, p.Output, p.RoleARN = g("region"), g("output"), g("role_arn")
		p.SourceProfile, p.CredentialProcess = g("source_profile"), g("credential_process")
		if p.SSOSession != "" && p.SSOStartURL == "" {
			if ses, ok := s.Session(p.SSOSession); ok {
				p.SSOStartURL, p.SSORegion = ses.StartURL, ses.Region
			}
		}
		if hasKeys(s.cfg, sec) {
			p.HasStaticKeys = true
			p.StaticKeysIn = "config"
		}
	}
	for _, sec := range s.cred.Sections() {
		n := sec
		if strings.HasPrefix(n, "profile ") {
			n = strings.TrimSpace(strings.TrimPrefix(n, "profile "))
		}
		p := get(n)
		if !p.InCredentials {
			p.InCredentials = true
			if !p.InConfig {
				p.Order = order
				order++
			}
		}
		if hasKeys(s.cred, sec) {
			p.HasStaticKeys = true
			p.StaticKeysIn = "credentials"
		}
	}
	out := make([]Profile, 0, len(byName))
	for _, p := range byName {
		switch {
		case p.SSOSession != "":
			p.Kind = api.ProfileKindSSO
		case p.SSOStartURL != "":
			p.Kind = api.ProfileKindLegacySSO
		case p.HasStaticKeys:
			p.Kind = api.ProfileKindStatic
		default:
			p.Kind = api.ProfileKindOther
		}
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Snapshot) Profile(name string) (Profile, bool) {
	for _, p := range s.Profiles() {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}

// Sessions returns the [sso-session] sections sorted by name.
func (s *Snapshot) Sessions() []Session {
	var out []Session
	for _, sec := range s.cfg.Sections() {
		if !strings.HasPrefix(sec, "sso-session ") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(sec, "sso-session "))
		ses := Session{Name: name}
		ses.StartURL, _ = s.cfg.Get(sec, "sso_start_url")
		ses.Region, _ = s.cfg.Get(sec, "sso_region")
		if v, ok := s.cfg.Get(sec, "sso_registration_scopes"); ok {
			ses.ScopesSet = true
			for _, part := range strings.Split(v, ",") {
				if part = strings.TrimSpace(part); part != "" {
					ses.Scopes = append(ses.Scopes, part)
				}
			}
		}
		out = append(out, ses)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Snapshot) Session(name string) (Session, bool) {
	for _, ses := range s.Sessions() {
		if ses.Name == name {
			return ses, true
		}
	}
	return Session{}, false
}

// StaticKey returns the keys of a profile; credentials-file values win over config-file values.
func (s *Snapshot) StaticKey(profile string) (StaticKey, bool) {
	read := func(d *ini.Doc, secs ...string) (StaticKey, bool) {
		for _, sec := range secs {
			if hasKeys(d, sec) {
				var k StaticKey
				k.AccessKeyID, _ = d.Get(sec, "aws_access_key_id")
				k.SecretAccessKey, _ = d.Get(sec, "aws_secret_access_key")
				k.SessionToken, _ = d.Get(sec, "aws_session_token")
				return k, true
			}
		}
		return StaticKey{}, false
	}
	if k, ok := read(s.cred, profile, "profile "+profile); ok {
		return k, true
	}
	return read(s.cfg, ConfigSection(profile))
}

// CredentialsKeySections lists the credentials-file sections ([name] or [profile name]) that hold static
// keys for the profile, in the order StaticKey reads them.
func (s *Snapshot) CredentialsKeySections(profile string) []string {
	var out []string
	for _, sec := range []string{profile, "profile " + profile} {
		if hasKeys(s.cred, sec) {
			out = append(out, sec)
		}
	}
	return out
}

// NewProfile describes a profile for AddProfiles.
type NewProfile struct{ Name, Session, AccountID, Role, Region string }

func parseDoc(b []byte) (*ini.Doc, error) {
	d, err := ini.Parse(b)
	if err != nil {
		var pe *ini.ParseError
		if errors.As(err, &pe) {
			return nil, api.NewError(api.ErrConfigParse, "config parse error at line %d", pe.Line).WithParam("line", fmt.Sprint(pe.Line))
		}
		return nil, err
	}
	return d, nil
}

// UnsafeValueError converts a refused INI edit into E_UNSAFE_VALUE. The error names the field, never the text.
// It returns nil for a nil error and passes other errors through.
func UnsafeValueError(err error) error {
	if err == nil {
		return nil
	}
	var ue *ini.UnsafeError
	if errors.As(err, &ue) {
		return api.NewError(api.ErrUnsafeValue, "value cannot be written to the AWS config safely").WithParam("value", ue.Field)
	}
	return err
}

// checkNames refuses lookup names that could not be written back safely.
func checkNames(fields map[string]string) error {
	for field, v := range fields {
		if ini.UnsafeText(v) {
			return api.NewError(api.ErrUnsafeValue, "value cannot be written to the AWS config safely").WithParam("value", field)
		}
	}
	return nil
}

// finish serialises d unless an edit was refused.
func finish(d *ini.Doc) ([]byte, error) {
	if err := d.Err(); err != nil {
		return nil, UnsafeValueError(err)
	}
	return d.Bytes(), nil
}

func sessionKVs(s *Session) []ini.KV {
	kvs := []ini.KV{{Key: "sso_start_url", Value: s.StartURL}, {Key: "sso_region", Value: s.Region}}
	if s.ScopesSet && len(s.Scopes) > 0 {
		kvs = append(kvs, ini.KV{Key: "sso_registration_scopes", Value: strings.Join(s.Scopes, ",")})
	}
	return kvs
}

// AddProfiles appends an optional new sso-session and the new profiles to the config.
func AddProfiles(cfg []byte, newSession *Session, profiles []NewProfile) ([]byte, error) {
	d, err := parseDoc(cfg)
	if err != nil {
		return nil, err
	}
	if newSession != nil {
		if d.Has(SessionSection(newSession.Name)) {
			return nil, api.NewError(api.ErrPortalExists, "portal exists").WithParam("portal", newSession.Name)
		}
		d.AddSection(SessionSection(newSession.Name), sessionKVs(newSession))
	}
	for _, p := range profiles {
		sec := ConfigSection(p.Name)
		if d.Has(sec) {
			return nil, api.NewError(api.ErrProfileExists, "profile exists").WithParam("name", p.Name)
		}
		d.AddSection(sec, []ini.KV{
			{Key: "sso_session", Value: p.Session},
			{Key: "sso_account_id", Value: p.AccountID},
			{Key: "sso_role_name", Value: p.Role},
			{Key: "region", Value: p.Region},
			{Key: "output", Value: "json"},
		})
	}
	return finish(d)
}

// MigrateProfile points an existing profile at an sso-session (keeping every other byte).
// region "" keeps the profile's current region.
func MigrateProfile(cfg []byte, profile string, newSession *Session, sessionName, accountID, role, region string) ([]byte, error) {
	d, err := parseDoc(cfg)
	if err != nil {
		return nil, err
	}
	sec := ConfigSection(profile)
	kvs := []ini.KV{{Key: "sso_session", Value: sessionName}, {Key: "sso_account_id", Value: accountID}, {Key: "sso_role_name", Value: role}}
	if d.Has(sec) {
		d.Set(sec, kvs, ini.InsertTop)
		if region != "" {
			d.Set(sec, []ini.KV{{Key: "region", Value: region}}, ini.InsertBottom)
		}
	} else {
		if region != "" {
			kvs = append(kvs, ini.KV{Key: "region", Value: region})
		}
		d.AddSection(sec, kvs)
	}
	if newSession != nil {
		if d.Has(SessionSection(newSession.Name)) {
			return nil, api.NewError(api.ErrPortalExists, "portal exists").WithParam("portal", newSession.Name)
		}
		d.AddSection(SessionSection(newSession.Name), sessionKVs(newSession))
	}
	return finish(d)
}

var staticKeyNames = []string{"aws_access_key_id", "aws_secret_access_key", "aws_session_token", "aws_security_token"}

// RemoveStaticKeys deletes the key lines from a section; a section left without keys is removed.
func RemoveStaticKeys(content []byte, section string) (out []byte, changed bool, err error) {
	if cerr := checkNames(map[string]string{"section": section}); cerr != nil {
		return nil, false, cerr
	}
	d, err := parseDoc(content)
	if err != nil {
		return nil, false, err
	}
	if !d.Has(section) {
		return content, false, nil
	}
	if d.DeleteKeys(section, staticKeyNames...) == 0 {
		return content, false, nil
	}
	if len(d.Keys(section)) == 0 {
		d.DeleteSection(section)
	}
	return d.Bytes(), true, nil
}

// RemoveProfile deletes [profile name] (or [default]) from the config.
func RemoveProfile(cfg []byte, name string) ([]byte, error) {
	if cerr := checkNames(map[string]string{"name": name}); cerr != nil {
		return nil, cerr
	}
	d, err := parseDoc(cfg)
	if err != nil {
		return nil, err
	}
	if !d.DeleteSection(ConfigSection(name)) {
		return nil, api.NewError(api.ErrProfileNotFound, "profile not found").WithParam("name", name)
	}
	return finish(d)
}

// RenameSession renames an sso-session header and every sso_session reference.
func RenameSession(cfg []byte, oldName, newName string) ([]byte, error) {
	if cerr := checkNames(map[string]string{"name": newName}); cerr != nil {
		return nil, cerr
	}
	d, err := parseDoc(cfg)
	if err != nil {
		return nil, err
	}
	if !d.Has(SessionSection(oldName)) {
		return nil, api.NewError(api.ErrPortalNotFound, "portal not found").WithParam("portal", oldName)
	}
	if d.Has(SessionSection(newName)) {
		return nil, api.NewError(api.ErrPortalExists, "portal exists").WithParam("portal", newName)
	}
	d.RenameSection(SessionSection(oldName), SessionSection(newName))
	for _, sec := range d.Sections() {
		if _, ok := profileFromSection(sec); !ok {
			continue
		}
		if v, ok := d.Get(sec, "sso_session"); ok && v == oldName {
			d.Set(sec, []ini.KV{{Key: "sso_session", Value: newName}}, ini.InsertTop)
		}
	}
	return finish(d)
}

// RemoveSession deletes an sso-session section.
func RemoveSession(cfg []byte, name string) ([]byte, error) {
	if cerr := checkNames(map[string]string{"name": name}); cerr != nil {
		return nil, cerr
	}
	d, err := parseDoc(cfg)
	if err != nil {
		return nil, err
	}
	if !d.DeleteSection(SessionSection(name)) {
		return nil, api.NewError(api.ErrPortalNotFound, "portal not found").WithParam("portal", name)
	}
	return finish(d)
}

var secretLineRE = regexp.MustCompile(`(?i)^(\s*(aws_access_key_id|aws_secret_access_key|aws_session_token|aws_security_token)\s*=\s*)(.*?)(\s*)$`)

// MaskForPreview masks key IDs (redact.MaskKeyID) and secrets (redact.Mask) line by line.
func MaskForPreview(content []byte) []byte {
	if len(content) == 0 {
		return content
	}
	lines := strings.Split(string(content), "\n")
	for i, l := range lines {
		cr := strings.HasSuffix(l, "\r")
		if cr {
			l = strings.TrimSuffix(l, "\r")
		}
		if m := secretLineRE.FindStringSubmatch(l); m != nil {
			val := redact.Mask
			if strings.EqualFold(m[2], "aws_access_key_id") {
				if v := redact.MaskKeyID(m[3]); v != "" {
					val = v
				}
			}
			l = m[1] + val
		}
		if cr {
			l += "\r"
		}
		lines[i] = l
	}
	return []byte(redact.String(strings.Join(lines, "\n")))
}

// Preview builds the diff shown before every write; both sides are masked.
func Preview(path, display, backupDirDisplay string, before, after []byte) api.FilePreview {
	lines, added, removed := textdiff.Diff(MaskForPreview(before), MaskForPreview(after), textdiff.DefaultContext)
	if lines == nil {
		lines = make([]api.DiffLine, 0)
	}
	return api.FilePreview{
		File: display, Path: path, BaseHash: Hash(before),
		Lines: lines, Added: added, Removed: removed, BackupDir: backupDirDisplay,
	}
}

var allowedHostSuffixes = []string{".awsapps.com", ".app.aws", ".awsapps.cn", ".portal.amazonaws.com", ".portal.amazonaws.com.cn",
	".app.amazonwebservices.com.cn", ".portal.amazonaws.eu", ".api.amazonwebservices.eu"}
var allowedHosts = []string{"identitycenter.amazonaws.com", "identitycenter.amazonaws.com.cn", "identitycenter.amazonaws.eu", "identitycenter.us-gov.amazonaws.com"}

// IsAWSPortalHost reports whether host (lower-case, no port) is a known AWS access-portal / Identity Center
// host: one of the exact hosts or a proper subdomain of an allowed suffix (including dual-stack portals).
func IsAWSPortalHost(host string) bool {
	host = strings.ToLower(host)
	for _, h := range allowedHosts {
		if host == h {
			return true
		}
	}
	for _, suf := range allowedHostSuffixes {
		if strings.HasSuffix(host, suf) && len(host) > len(suf) {
			return true
		}
	}
	return false
}

var portalRegionRE = regexp.MustCompile(`\.portal\.([a-z0-9-]+)\.app\.aws$`)

var (
	startHostRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
	startPathRE = regexp.MustCompile(`^(/[A-Za-z0-9._~-]+)*$`)
)

// NormalizeStartURL validates and normalizes an access portal URL. Only plain https URLs on an AWS portal host
// are accepted: no userinfo, port, query, fragment, backslash, percent escape or control character.
func NormalizeStartURL(raw string) (normalized, guessedRegion string, err *api.AppError) {
	bad := api.NewError(api.ErrInvalidStartURL, "invalid start URL")
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, "#/")
	s = strings.TrimSuffix(s, "#")
	if strings.ContainsAny(s, `\%`) || ini.UnsafeText(s) {
		return "", "", bad
	}
	if !strings.HasPrefix(strings.ToLower(s), "https://") {
		return "", "", bad
	}
	rest := s[len("https://"):]
	host, path := rest, ""
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		host, path = rest[:i], rest[i:]
	}
	host = strings.ToLower(host)
	if !startHostRE.MatchString(host) {
		return "", "", bad
	}
	if !IsAWSPortalHost(host) {
		return "", "", bad
	}
	if strings.HasSuffix(host, ".awsapps.com") {
		if path == "" || path == "/" {
			path = "/start"
		}
	}
	path = strings.TrimRight(path, "/")
	if !startPathRE.MatchString(path) {
		return "", "", bad
	}
	for _, seg := range strings.Split(path, "/") {
		if seg == "." || seg == ".." {
			return "", "", bad
		}
	}
	norm := "https://" + host + path
	u, perr := url.Parse(norm)
	if perr != nil || u.Scheme != "https" || u.Hostname() != host || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", "", bad
	}
	if m := portalRegionRE.FindStringSubmatch(host); m != nil {
		guessedRegion = m[1]
	}
	return norm, guessedRegion, nil
}

// SuggestSessionName proposes a session name from the first host label ("" if unusable).
func SuggestSessionName(startURL string) string {
	s := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(startURL)), "https://")
	if i := strings.IndexAny(s, "/.?#"); i >= 0 {
		s = s[:i]
	}
	if !SessionNameRE.MatchString(s) {
		return ""
	}
	return s
}

func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

var roleSlugs = map[string]string{"AdministratorAccess": "admin", "PowerUserAccess": "poweruser", "ReadOnlyAccess": "readonly", "ViewOnlyAccess": "viewonly"}

// SuggestProfileName builds <account>-<role> and appends -2, -3 on collisions with taken.
func SuggestProfileName(accountName, role string, taken map[string]bool) string {
	rs, ok := roleSlugs[role]
	if !ok {
		rs = slug(role)
	}
	base := slug(accountName)
	if base == "" {
		base = "profile"
	}
	if rs != "" {
		base += "-" + rs
	}
	if len(base) > 60 {
		base = strings.Trim(base[:60], "-")
	}
	name := base
	for n := 2; taken[name]; n++ {
		name = fmt.Sprintf("%s-%d", base, n)
	}
	return name
}
