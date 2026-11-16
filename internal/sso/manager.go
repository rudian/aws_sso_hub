package sso

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssso "github.com/aws/aws-sdk-go-v2/service/sso"
	ssotypes "github.com/aws/aws-sdk-go-v2/service/sso/types"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	oidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	"github.com/aws/smithy-go"

	"github.com/rudian/aws_sso_hub/internal/api"
)

// Session identifies an sso-session (or a legacy profile when Name is empty).
type Session struct {
	Name, StartURL, Region string
	Scopes                 []string
	ScopesSet              bool
}

// Endpoints override the AWS defaults (tests, sandbox mode).
type Endpoints struct{ OIDC, Portal string }

// Config configures a Manager.
type Config struct {
	Home             string
	RegistrationsDir string
	Endpoints        Endpoints
	OpenBrowser      func(url string) error
	// OpenSignIn, when set, opens the sign-in page instead of OpenBrowser and reports whether it did so in a
	// private browser window.
	OpenSignIn      func(url string) (private bool, err error)
	Lang            func() string // "zh-CN" | "en"
	Now             func() time.Time
	Sleep           func(ctx context.Context, d time.Duration) error // nil = real
	CallbackTimeout time.Duration                                    // 0 = 10 min
}

// Status is the token-level state of a session (the accounts engine derives the user-facing state from it).
type Status struct {
	State        api.SessionState
	ExpiresAt    time.Time
	CanRefresh   bool
	Refreshing   bool
	Err          *api.AppError
	ClientIDHash string
}

type Account struct{ ID, Name, Email string }
type AccountRole struct{ AccountID, Role string }

// Manager owns all SSO state of the process. It is safe for concurrent use.
type Manager struct {
	cfg Config

	mu         sync.Mutex
	locks      map[string]*sync.Mutex
	logging    map[string]bool
	failed     map[string]time.Time
	netErr     map[string]*api.AppError
	refreshing map[string]bool
	roleCreds  map[string]aws.Credentials
}

func NewManager(cfg Config) *Manager {
	return &Manager{cfg: cfg, locks: map[string]*sync.Mutex{}, logging: map[string]bool{}, failed: map[string]time.Time{},
		netErr: map[string]*api.AppError{}, refreshing: map[string]bool{}, roleCreds: map[string]aws.Credentials{}}
}

func (m *Manager) now() time.Time {
	if m.cfg.Now != nil {
		return m.cfg.Now()
	}
	return time.Now()
}

func (m *Manager) sleep(ctx context.Context, d time.Duration) error {
	if m.cfg.Sleep != nil {
		return m.cfg.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) lang() string {
	if m.cfg.Lang != nil {
		return m.cfg.Lang()
	}
	return "en"
}

func keyOf(s Session) string { return TokenCacheKey(s.Name, s.StartURL) }

func (m *Manager) tokenDir() string { return TokenCacheDir(m.cfg.Home) }

func (m *Manager) lockFor(key string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.locks[key]
	if !ok {
		l = &sync.Mutex{}
		m.locks[key] = l
	}
	return l
}

func (m *Manager) oidcClient(s Session) *ssooidc.Client {
	ep := m.cfg.Endpoints.OIDC
	return ssooidc.NewFromConfig(aws.Config{Region: s.Region}, func(o *ssooidc.Options) {
		if ep != "" {
			o.BaseEndpoint = aws.String(ep)
		}
	})
}

func (m *Manager) portalClient(s Session) *awssso.Client {
	ep := m.cfg.Endpoints.Portal
	return awssso.NewFromConfig(aws.Config{Region: s.Region}, func(o *awssso.Options) {
		if ep != "" {
			o.BaseEndpoint = aws.String(ep)
		}
	})
}

// LoggingIn reports whether an interactive login for the session is running.
func (m *Manager) LoggingIn(s Session) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.logging[keyOf(s)]
}

// Status computes the token-level state without any network access.
func (m *Manager) Status(s Session) Status {
	key := keyOf(s)
	m.mu.Lock()
	logging, refreshing, netErr, failedAt := m.logging[key], m.refreshing[key], m.netErr[key], m.failed[key]
	m.mu.Unlock()

	t, err := ReadToken(m.tokenDir(), key)
	st := Status{Refreshing: refreshing}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		st.State = api.SessionSignedOut
	case err != nil:
		st.State = api.SessionError
		st.Err = api.NewError(api.ErrFileIO, "cannot read the SSO token cache").WithParam("file", key+".json")
	default:
		now := m.now()
		st.ExpiresAt = t.ExpiresAt
		st.ClientIDHash = ClientIDHash(t.ClientID)
		st.CanRefresh = s.Name != "" && canRefresh(t, now)
		remaining := t.ExpiresAt.Sub(now)
		switch {
		case remaining >= refreshWindow:
			st.State = api.SessionActive
		case remaining > 0:
			st.State = api.SessionExpiring
		case st.CanRefresh && !failedAt.Equal(t.ExpiresAt):
			st.State = api.SessionRefreshable
		default:
			st.State = api.SessionExpired
		}
		if failedAt.Equal(t.ExpiresAt) {
			st.CanRefresh = false
		}
		st.Err = netErr
	}
	if logging {
		st.State = api.SessionSigningIn
	}
	return st
}

// FailedRefresh reports whether a refresh was rejected for the token currently in the file.
func (m *Manager) FailedRefresh(s Session) bool {
	key := keyOf(s)
	t, err := ReadToken(m.tokenDir(), key)
	if err != nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.failed[key].Equal(t.ExpiresAt)
}

// Token returns a usable token, refreshing it first when it has less than 5 minutes left and is refreshable.
func (m *Manager) Token(ctx context.Context, s Session) (*CachedToken, error) {
	key := keyOf(s)
	t, err := ReadToken(m.tokenDir(), key)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotSignedIn
	}
	if err != nil {
		return nil, api.NewError(api.ErrFileIO, "cannot read the SSO token cache").WithParam("file", key+".json")
	}
	now := m.now()
	if t.ExpiresAt.Sub(now) >= 5*time.Minute {
		return t, nil
	}
	if s.Name != "" && canRefresh(t, now) {
		m.mu.Lock()
		failedAt := m.failed[key]
		m.mu.Unlock()
		if failedAt.Equal(t.ExpiresAt) {
			if t.ExpiresAt.After(now) {
				return t, nil
			}
			return nil, ErrSessionExpired
		}
		nt, err := m.refresh(ctx, s, t)
		if err != nil {
			if t.ExpiresAt.After(now) && !errors.Is(err, context.Canceled) {
				return t, nil // still valid for a while; the refresh failure is surfaced by Status
			}
			return nil, err
		}
		return nt, nil
	}
	if t.ExpiresAt.After(now) {
		return t, nil
	}
	return nil, ErrSessionExpired
}

// Refresh renews the access token with the refresh token. It re-reads the file under the session lock first.
func (m *Manager) Refresh(ctx context.Context, s Session) (*CachedToken, error) {
	key := keyOf(s)
	seen, err := ReadToken(m.tokenDir(), key)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotSignedIn
	}
	if err != nil {
		return nil, api.NewError(api.ErrFileIO, "cannot read the SSO token cache").WithParam("file", key+".json")
	}
	return m.refresh(ctx, s, seen)
}

func isGrantRejection(err error) bool {
	var a *oidctypes.InvalidGrantException
	var b *oidctypes.UnauthorizedClientException
	var c *oidctypes.ExpiredTokenException
	var d *oidctypes.AccessDeniedException
	var e *oidctypes.InvalidClientException
	return errors.As(err, &a) || errors.As(err, &b) || errors.As(err, &c) || errors.As(err, &d) || errors.As(err, &e)
}

// refresh renews the token. seen is the token the caller looked at before taking the lock.
func (m *Manager) refresh(ctx context.Context, s Session, seen *CachedToken) (*CachedToken, error) {
	key := keyOf(s)
	lk := m.lockFor(key)
	lk.Lock()
	defer lk.Unlock()

	cur, err := ReadToken(m.tokenDir(), key)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotSignedIn
	}
	if err != nil {
		return nil, api.NewError(api.ErrFileIO, "cannot read the SSO token cache").WithParam("file", key+".json")
	}
	now := m.now()
	if seen != nil && cur.ExpiresAt.After(seen.ExpiresAt) && cur.ExpiresAt.Sub(now) >= refreshWindow {
		return cur, nil // someone else (the CLI) refreshed it
	}
	if s.Name == "" || !canRefresh(cur, now) {
		return nil, ErrSessionExpired
	}
	m.mu.Lock()
	if m.failed[key].Equal(cur.ExpiresAt) {
		m.mu.Unlock()
		return nil, ErrInvalidGrant
	}
	m.refreshing[key] = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.refreshing, key)
		m.mu.Unlock()
	}()

	out, err := m.oidcClient(s).CreateToken(ctx, &ssooidc.CreateTokenInput{
		GrantType:    aws.String(grantRefresh),
		ClientId:     aws.String(cur.ClientID),
		ClientSecret: aws.String(cur.ClientSecret),
		RefreshToken: aws.String(cur.RefreshToken),
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		if isGrantRejection(err) {
			if again, rerr := ReadToken(m.tokenDir(), key); rerr == nil &&
				(again.ExpiresAt != cur.ExpiresAt || again.AccessToken != cur.AccessToken) && again.ExpiresAt.After(m.now()) {
				return again, nil
			}
			m.mu.Lock()
			m.failed[key] = cur.ExpiresAt
			delete(m.netErr, key)
			m.mu.Unlock()
			return nil, ErrInvalidGrant
		}
		if _, ok := NetworkHost(err); ok {
			m.mu.Lock()
			m.netErr[key] = ToAppError(err, s.Name)
			m.mu.Unlock()
		}
		return nil, err
	}
	n := *cur
	n.StartURL, n.Region = firstNonEmpty(s.StartURL, cur.StartURL), firstNonEmpty(s.Region, cur.Region)
	n.AccessToken = aws.ToString(out.AccessToken)
	n.Granted = time.Duration(out.ExpiresIn) * time.Second
	n.ExpiresAt = m.now().UTC().Add(n.Granted).Truncate(time.Second)
	n.RefreshToken = aws.ToString(out.RefreshToken)
	if err := WriteToken(m.tokenDir(), key, &n); err != nil {
		return nil, api.NewError(api.ErrFileIO, "cannot write the SSO token cache").WithParam("file", key+".json")
	}
	m.mu.Lock()
	delete(m.netErr, key)
	delete(m.failed, key)
	m.mu.Unlock()
	return &n, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Logout signs one session out: best-effort sso:Logout, delete its token file and the matching CLI role
// caches (only files with "ProviderType": "sso"). Nothing else is touched.
func (m *Manager) Logout(ctx context.Context, s Session, roles []AccountRole) (deletedRoleCaches int, serverLogout bool, err error) {
	key := keyOf(s)
	lk := m.lockFor(key)
	lk.Lock()
	defer lk.Unlock()

	if t, rerr := ReadToken(m.tokenDir(), key); rerr == nil {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, lerr := m.portalClient(s).Logout(cctx, &awssso.LogoutInput{AccessToken: aws.String(t.AccessToken)})
		cancel()
		serverLogout = lerr == nil
	}
	if rerr := os.Remove(filepath.Join(m.tokenDir(), key+".json")); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
		return 0, serverLogout, api.NewError(api.ErrFileIO, "cannot delete the SSO token file").WithParam("file", key+".json")
	}
	seen := map[string]bool{}
	for _, r := range roles {
		var ck string
		if s.Name != "" {
			ck = RoleCacheKey(r.AccountID, r.Role, s.Name, "")
		} else {
			ck = RoleCacheKey(r.AccountID, r.Role, "", s.StartURL)
		}
		if seen[ck] {
			continue
		}
		seen[ck] = true
		p := filepath.Join(CLICacheDir(m.cfg.Home), ck+".json")
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			continue
		}
		var probe struct {
			ProviderType string `json:"ProviderType"`
		}
		if json.Unmarshal(b, &probe) != nil || probe.ProviderType != "sso" {
			continue
		}
		if os.Remove(p) == nil {
			deletedRoleCaches++
		}
	}
	m.mu.Lock()
	for k := range m.roleCreds {
		if strings.HasPrefix(k, key+"|") {
			delete(m.roleCreds, k)
		}
	}
	delete(m.failed, key)
	delete(m.netErr, key)
	m.mu.Unlock()
	return deletedRoleCaches, serverLogout, nil
}

func wrapPortalErr(err error, forRole bool) error {
	var un *ssotypes.UnauthorizedException
	var nf *ssotypes.ResourceNotFoundException
	if errors.As(err, &un) {
		return ErrSessionExpired
	}
	var apiErr smithy.APIError
	if forRole && errors.As(err, &apiErr) && apiErr.ErrorCode() == "ForbiddenException" {
		return ErrRoleNotAssigned
	}
	if forRole && errors.As(err, &nf) {
		return ErrRoleNotAssigned
	}
	return err
}

// ListAccounts lists the accounts of the signed-in user (paginated).
func (m *Manager) ListAccounts(ctx context.Context, s Session) ([]Account, error) {
	t, err := m.Token(ctx, s)
	if err != nil {
		return nil, err
	}
	var out []Account
	p := awssso.NewListAccountsPaginator(m.portalClient(s), &awssso.ListAccountsInput{AccessToken: aws.String(t.AccessToken)})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, wrapPortalErr(err, false)
		}
		for _, a := range page.AccountList {
			out = append(out, Account{ID: aws.ToString(a.AccountId), Name: aws.ToString(a.AccountName), Email: aws.ToString(a.EmailAddress)})
		}
	}
	return out, nil
}

// ListAccountRoles lists the role names assigned to the user in one account (paginated).
func (m *Manager) ListAccountRoles(ctx context.Context, s Session, accountID string) ([]string, error) {
	t, err := m.Token(ctx, s)
	if err != nil {
		return nil, err
	}
	var out []string
	p := awssso.NewListAccountRolesPaginator(m.portalClient(s), &awssso.ListAccountRolesInput{AccessToken: aws.String(t.AccessToken), AccountId: aws.String(accountID)})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, wrapPortalErr(err, false)
		}
		for _, r := range page.RoleList {
			out = append(out, aws.ToString(r.RoleName))
		}
	}
	return out, nil
}

// RoleCredentials returns temporary role credentials, cached in memory until Expires-5m. Never written to disk.
func (m *Manager) RoleCredentials(ctx context.Context, s Session, accountID, role string) (aws.Credentials, error) {
	ck := keyOf(s) + "|" + accountID + "|" + role
	m.mu.Lock()
	if c, ok := m.roleCreds[ck]; ok && c.Expires.Sub(m.now()) > 5*time.Minute {
		m.mu.Unlock()
		return c, nil
	}
	m.mu.Unlock()
	t, err := m.Token(ctx, s)
	if err != nil {
		return aws.Credentials{}, err
	}
	out, err := m.portalClient(s).GetRoleCredentials(ctx, &awssso.GetRoleCredentialsInput{
		AccessToken: aws.String(t.AccessToken), AccountId: aws.String(accountID), RoleName: aws.String(role)})
	if err != nil {
		return aws.Credentials{}, wrapPortalErr(err, true)
	}
	rc := out.RoleCredentials
	if rc == nil {
		return aws.Credentials{}, ErrRoleNotAssigned
	}
	c := aws.Credentials{
		AccessKeyID: aws.ToString(rc.AccessKeyId), SecretAccessKey: aws.ToString(rc.SecretAccessKey),
		SessionToken: aws.ToString(rc.SessionToken), CanExpire: true,
		Expires: time.UnixMilli(rc.Expiration).UTC(), Source: "AWSSSOHubSSO", AccountID: accountID,
	}
	m.mu.Lock()
	m.roleCreds[ck] = c
	m.mu.Unlock()
	return c, nil
}

func sha1Hex(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}
