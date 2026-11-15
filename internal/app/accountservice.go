package app

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/awsconfig"
	"github.com/rudian/aws_sso_hub/internal/transfer"
)

// Thin layer over accounts.Engine; the engine emits the accounts:* and session:* events itself.

// AccountService is the bound Wails service for portals, sessions, profiles and account discovery.
type AccountService struct{ c *Container }

// NewAccountService wires the service to the container.
func NewAccountService(c *Container) *AccountService { return &AccountService{c: c} }

// ServiceStartup starts the background loops (ticker, file watcher, initial checks and scan).
func (s *AccountService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	s.c.StartLoops()
	return nil
}

// ServiceShutdown stops the loops. It never signs anyone out.
func (s *AccountService) ServiceShutdown() error {
	s.c.Shutdown()
	return nil
}

func (s *AccountService) GetState(ctx context.Context) (api.AccountsState, error) {
	st, err := s.c.Accounts.State(ctx)
	return nn(st), fail(err)
}

func (s *AccountService) Reload(ctx context.Context) (api.AccountsState, error) {
	st, err := s.c.Accounts.Reload(ctx)
	if err == nil {
		s.c.Accounts.EmitState(ctx)
	}
	return nn(st), fail(err)
}

func (s *AccountService) IdentifyStatic(ctx context.Context, profile string) (api.StaticIdentity, error) {
	id, err := s.c.Accounts.IdentifyStatic(ctx, profile)
	return id, fail(err)
}

func (s *AccountService) CheckPortalInput(in api.PortalInput) (api.PortalInputCheck, error) {
	return nn(s.c.Accounts.CheckPortalInput(in)), nil
}

func (s *AccountService) AddPortalDraft(in api.PortalInput) (api.Portal, error) {
	p, err := s.c.Accounts.AddDraft(in)
	return nn(p), fail(err)
}

func (s *AccountService) DiscardPortalDraft(name string) error {
	s.c.Accounts.DiscardDraft(name)
	return nil
}

func (s *AccountService) Login(ctx context.Context, portal string, mode api.LoginMode) (api.LoginResult, error) {
	if mode != api.LoginModeBrowser && mode != api.LoginModeDevice {
		return api.LoginResult{}, invalidField("mode")
	}
	r, err := s.c.Accounts.Login(ctx, portal, mode)
	return nn(r), fail(err)
}

func (s *AccountService) RefreshSession(ctx context.Context, portal string) (api.SessionStatus, error) {
	st, err := s.c.Accounts.Refresh(ctx, portal)
	return st, fail(err)
}

func (s *AccountService) Logout(ctx context.Context, portal string) (api.LogoutResult, error) {
	r, err := s.c.Accounts.Logout(ctx, portal)
	return r, fail(err)
}

// SetSessionLength stores the per-portal session length in minutes (15..129600); other values fail with
// E_INVALID_INPUT.
func (s *AccountService) SetSessionLength(ctx context.Context, portal string, minutes int) (api.SessionStatus, error) {
	st, err := s.c.Accounts.SetSessionLength(ctx, portal, minutes)
	return st, fail(err)
}

func (s *AccountService) DiscoverAccounts(ctx context.Context, portal string) ([]api.DiscoveredAccount, error) {
	out, err := s.c.Accounts.Discover(ctx, portal)
	return nn(out), fail(err)
}

func (s *AccountService) LoadAccountRoles(ctx context.Context, portal string, accountID string) (api.DiscoveredAccount, error) {
	a, err := s.c.Accounts.LoadRoles(ctx, portal, accountID)
	return nn(a), fail(err)
}

func (s *AccountService) PreviewCreateProfiles(req api.CreateProfilesRequest) (api.FilePreview, error) {
	p, err := s.c.Accounts.PreviewCreate(req)
	return nn(p), fail(err)
}

func (s *AccountService) CreateProfiles(req api.CreateProfilesRequest, baseHash string) (api.CreateProfilesResult, error) {
	r, err := s.c.Accounts.Create(req, baseHash)
	return nn(r), fail(err)
}

func (s *AccountService) PreviewRemoveProfile(name string) (api.FilePreview, error) {
	p, err := s.c.Accounts.PreviewRemove(name)
	return nn(p), fail(err)
}

func (s *AccountService) RemoveProfile(name string, baseHash string) (api.WriteResult, error) {
	r, err := s.c.Accounts.Remove(name, baseHash)
	return r, fail(err)
}

func (s *AccountService) SetProfileIdentity(u api.IdentityUpdate) (api.AccountsState, error) {
	st, err := s.c.Accounts.SetIdentity(u)
	return nn(st), fail(err)
}

func (s *AccountService) OpenPortal(portal string) error {
	u, err := s.c.Accounts.PortalURL(portal)
	if err != nil {
		return fail(err)
	}
	return fail(s.c.RT.OpenURL(u))
}

// signInDeviceHostRE matches the IAM Identity Center device-authorization and OIDC endpoints, which are
// AWS-operated and not on the portal host list. Wildcard amazonaws.com is deliberately not accepted: it
// contains customer-controlled names such as S3 bucket hosts.
var signInDeviceHostRE = regexp.MustCompile(`^(device\.sso|oidc)\.[a-z]{2,4}(-[a-z]+)+-\d+\.amazonaws\.(com|com\.cn|eu)$`)

// checkSignInURL accepts https URLs on an AWS portal host (the awsconfig allow-list, which includes the
// dual-stack *.app.aws and *.app.amazonwebservices.com.cn portals) or an Identity Center device host; sandbox mode also accepts the loopback http
// endpoints of the fake servers.
func checkSignInURL(raw string, sandbox bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return api.NewError(api.ErrInvalidInput, "not a sign-in page URL").WithParam("field", "url")
	}
	if sandbox && u.Scheme == "http" {
		switch u.Hostname() {
		case "127.0.0.1", "localhost", "::1":
			return nil
		}
	}
	host := strings.ToLower(u.Hostname())
	if u.Scheme == "https" && u.User == nil {
		if awsconfig.IsAWSPortalHost(host) || signInDeviceHostRE.MatchString(host) {
			return nil
		}
	}
	return api.NewError(api.ErrInvalidInput, "not a sign-in page URL").WithParam("field", "url")
}

// OpenSignInPage opens a sign-in page again from the login window: the device-code verification page ("Open
// browser") or the browser sign-in authorize URL ("Open again"). It follows the "private sign-in" setting
// and returns whether the page opened in a private browser window. Only https pages on an AWS portal host
// (including dual-stack portals) or an Identity Center device host are accepted.
func (s *AccountService) OpenSignInPage(url string) (bool, error) {
	if err := checkSignInURL(url, s.c.Paths.Sandbox); err != nil {
		return false, fail(err)
	}
	private, err := s.c.openSignIn(url)
	return private, fail(err)
}

// OpenConsole: P1
func (s *AccountService) OpenConsole(ctx context.Context, profile string) error {
	u, err := s.c.Accounts.ConsoleURL(profile, "")
	if err != nil {
		return fail(err)
	}
	return fail(s.c.RT.OpenURL(u))
}

// PreviewRenamePortal: P1
func (s *AccountService) PreviewRenamePortal(oldName string, newName string) (api.FilePreview, error) {
	p, err := s.c.Accounts.PreviewRenamePortal(oldName, newName)
	return nn(p), fail(err)
}

// RenamePortal: P1
func (s *AccountService) RenamePortal(oldName string, newName string, baseHash string) (api.WriteResult, error) {
	r, err := s.c.Accounts.RenamePortal(oldName, newName, baseHash)
	return r, fail(err)
}

// PreviewRemovePortal: P1
func (s *AccountService) PreviewRemovePortal(name string) (api.FilePreview, error) {
	p, err := s.c.Accounts.PreviewRemovePortal(name)
	return nn(p), fail(err)
}

// RemovePortal: P1
func (s *AccountService) RemovePortal(name string, baseHash string) (api.WriteResult, error) {
	r, err := s.c.Accounts.RemovePortal(name, baseHash)
	return r, fail(err)
}

// BuildExport renders the accounts JSON for the selected portals and profiles (preview and "Copy JSON").
// The file never contains keys, tokens or anything outside the allow-list.
func (s *AccountService) BuildExport(ctx context.Context, req api.ExportRequest) (api.ExportResult, error) {
	r, err := s.c.Transfer.BuildExport(ctx, req)
	return nn(r), fail(err)
}

// SaveExport asks for a file with the native save dialog and writes the export there. A cancelled dialog is
// E_CANCELLED.
func (s *AccountService) SaveExport(ctx context.Context, req api.ExportRequest) (api.ExportResult, error) {
	r, err := s.c.Transfer.BuildExport(ctx, req)
	if err != nil {
		return api.ExportResult{}, fail(err)
	}
	if !transfer.HasIncluded(r) {
		return api.ExportResult{}, invalidField("portals")
	}
	path, derr := s.c.RT.ChooseSaveFile(exportDialogTitle(s.c), transfer.DefaultExportName(s.c.now()), jsonFilter)
	if derr != nil {
		return api.ExportResult{}, fail(derr)
	}
	if path == "" {
		return api.ExportResult{}, cancelled()
	}
	r, err = s.c.Transfer.WriteExport(path, r)
	return nn(r), fail(err)
}

// OpenImportFile asks for a JSON file with the native open dialog (at most transfer.MaxFileBytes, 384 KiB). A cancelled dialog is
// E_CANCELLED.
func (s *AccountService) OpenImportFile() (api.ImportSource, error) {
	path, derr := s.c.RT.ChooseOpenFile(importDialogTitle(s.c), jsonFilter)
	if derr != nil {
		return api.ImportSource{}, fail(derr)
	}
	if path == "" {
		return api.ImportSource{}, cancelled()
	}
	src, err := s.c.Transfer.ReadImportFile(path)
	return src, fail(err)
}

// PreviewImport validates the JSON and plans the import. It never writes.
func (s *AccountService) PreviewImport(req api.ImportRequest) (api.ImportPreview, error) {
	p, err := s.c.Transfer.PreviewImport(req)
	return nn(p), fail(err)
}

// ApplyImport writes the planned changes with one backup, guarded by baseHash.
func (s *AccountService) ApplyImport(ctx context.Context, req api.ImportRequest, baseHash string) (api.ImportResult, error) {
	r, err := s.c.Transfer.ApplyImport(ctx, req, baseHash)
	return nn(r), fail(err)
}

var jsonFilter = FileFilter{Name: "JSON", Pattern: "*.json"}

func exportDialogTitle(c *Container) string {
	if c.Settings.UILanguage() == "zh-CN" {
		return "导出账号配置"
	}
	return "Export accounts"
}

func importDialogTitle(c *Container) string {
	if c.Settings.UILanguage() == "zh-CN" {
		return "导入账号配置"
	}
	return "Import accounts"
}

func cancelled() *api.AppError { return api.NewError(api.ErrCancelled, "cancelled") }

func invalidField(field string) *api.AppError {
	return api.NewError(api.ErrInvalidInput, "invalid value for %s", field).WithParam("field", field)
}
