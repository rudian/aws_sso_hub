package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/awsconfig"
	"github.com/rudian/aws_sso_hub/internal/backup"
	"github.com/rudian/aws_sso_hub/internal/envcheck"
	"github.com/rudian/aws_sso_hub/internal/terminal"
	"github.com/rudian/aws_sso_hub/internal/version"
)

// App info, settings, environment, backups and OS helpers.

// SystemService is the bound Wails service for environment checks, settings, backups and OS integration.
type SystemService struct {
	c *Container

	// previewed holds, per backup id, the hash of the target file as PreviewRestore showed it. RestoreBackup
	// refuses to write when the file is no longer what the user reviewed.
	previewMu sync.Mutex
	previewed map[string]string
}

// NewSystemService wires the service to the container.
func NewSystemService(c *Container) *SystemService { return &SystemService{c: c} }

func (s *SystemService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

func (s *SystemService) ServiceShutdown() error { return nil }

func (s *SystemService) GetAppInfo() (api.AppInfo, error) {
	c := s.c
	return api.AppInfo{
		Version:    version.Version,
		Platform:   runtime.GOOS,
		Arch:       runtime.GOARCH,
		Sandbox:    c.Paths.Sandbox,
		DataDir:    c.Paths.Display(c.Paths.AppDataDir),
		BackupDir:  c.Paths.Display(c.Paths.BackupDir),
		FirstRun:   c.Settings.FirstRun(),
		UILanguage: c.Settings.UILanguage(),
	}, nil
}

// Records the UI language; used for the fallback helper text, the sign-in callback page and the tray.

// SetUILanguage: "zh-CN" | "en"; used for fallback helper text, callback page, tray
func (s *SystemService) SetUILanguage(lang string) error {
	if err := s.c.Settings.SetUILanguage(lang); err != nil {
		return fail(err)
	}
	s.c.notifyUILanguage(lang)
	return nil
}

func (s *SystemService) GetSettings() (api.Settings, error) {
	return nn(s.c.Settings.API()), nil
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// UpdateSettings validates and stores a patch, emits settings:changed, rescans when the scan roots changed,
// re-checks the environment when a tool path changed and lets the shell (tray) react.

func (s *SystemService) UpdateSettings(p api.SettingsPatch) (api.Settings, error) {
	c := s.c
	prev := c.Settings.API()
	next, err := c.Settings.Update(p)
	if err != nil {
		return api.Settings{}, fail(err)
	}
	next = nn(next)
	c.emit(api.EventSettingsChanged, next)
	if !sameStrings(prev.ScanRoots, next.ScanRoots) {
		c.Background(func(ctx context.Context) { _, _ = c.Repos.Scan(ctx) })
	}
	if prev.AWSCLIPath != next.AWSCLIPath || prev.GitPath != next.GitPath {
		c.invalidateEnv()
		c.Background(func(ctx context.Context) {
			c.CheckEnvironment(ctx)
			c.Repos.Invalidate(ctx)
			if st, err := c.Repos.State(ctx); err == nil {
				c.emit(api.EventReposChanged, st)
			}
		})
	}
	c.notifySettings(prev, next)
	return next, nil
}

func (s *SystemService) CompleteFirstRun() error {
	return fail(s.c.Settings.CompleteFirstRun())
}

func (s *SystemService) FirstRunSummary(ctx context.Context) (api.FirstRunSummary, error) {
	c := s.c
	st, err := c.Accounts.State(ctx)
	if err != nil {
		return api.FirstRunSummary{}, fail(err)
	}
	sum := api.FirstRunSummary{Profiles: len(st.Profiles), Static: st.StaticCount, ScanRoots: []string{}}
	for _, p := range st.Portals {
		if !p.Draft {
			sum.Portals++
		}
	}
	if rs, err := c.Repos.State(ctx); err == nil {
		sum.Repos = rs.Counts.All
	}
	for _, r := range c.Settings.ScanRoots() {
		sum.ScanRoots = append(sum.ScanRoots, c.Paths.Display(r))
	}
	return sum, nil
}

func (s *SystemService) CheckEnvironment(ctx context.Context) (api.EnvReport, error) {
	return nn(s.c.CheckEnvironment(ctx)), nil
}

var severityRank = map[api.Severity]int{api.SeverityDanger: 0, api.SeverityWarn: 1, api.SeverityInfo: 2, api.SeverityOK: 3}

// CheckHealth is the security checkup: shell files and environment variables that hold long-lived
// keys, plus findings derived from the accounts, repositories and pending key deactivations.

func (s *SystemService) CheckHealth(ctx context.Context) (api.HealthReport, error) {
	c := s.c
	findings := make([]api.HealthFinding, 0)
	findings = append(findings, envcheck.ScanShellFiles(c.Paths.Home, runtime.GOOS)...)
	findings = append(findings, envcheck.EnvVarFindings(os.Environ())...)

	st, err := c.Accounts.State(ctx)
	if err != nil {
		return api.HealthReport{}, fail(err)
	}
	kinds := map[string]api.ProfileKind{}
	for _, p := range st.Profiles {
		kinds[p.Name] = p.Kind
		if p.Kind == api.ProfileKindSSO && p.HasStaticKeys {
			findings = append(findings, api.HealthFinding{ID: "leftover-keys:" + p.Name, Kind: api.HealthLeftoverKeys, Severity: api.SeverityWarn, Profile: p.Name})
		}
		if p.Name == "default" && p.HasStaticKeys {
			findings = append(findings, api.HealthFinding{ID: "default-keys:default", Kind: api.HealthDefaultKeys, Severity: api.SeverityWarn, Profile: p.Name})
		}
	}
	if prof, _, ok := c.Repos.GlobalDefault(); ok && kinds[prof] == api.ProfileKindStatic {
		findings = append(findings, api.HealthFinding{ID: "global-static:" + prof, Kind: api.HealthGlobalStatic, Severity: api.SeverityWarn, Profile: prof})
	}
	if rs, err := c.Repos.State(ctx); err == nil {
		for _, r := range rs.Repos {
			if r.HasUserinfo {
				findings = append(findings, api.HealthFinding{ID: "url-userinfo:" + r.ID, Kind: api.HealthURLUserinfo, Severity: api.SeverityWarn,
					Params: map[string]string{"repo": r.Name}})
			}
		}
	}
	for _, pd := range c.Settings.PendingDeactivations() {
		findings = append(findings, api.HealthFinding{ID: "pending-deactivation:" + pd.Profile, Kind: api.HealthPendingKey, Severity: api.SeverityInfo,
			Profile: pd.Profile, Params: map[string]string{"maskedKeyId": pd.MaskedKeyID}})
	}
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := severityRank[findings[i].Severity], severityRank[findings[j].Severity]
		if a != b {
			return a < b
		}
		return findings[i].ID < findings[j].ID
	})
	return nn(api.HealthReport{Findings: findings, CheckedAt: c.now().UTC().Format("2006-01-02T15:04:05Z")}), nil
}

func (s *SystemService) ListBackups() ([]api.BackupEntry, error) {
	list, err := s.c.Backup.List()
	if err != nil {
		return nil, fail(err)
	}
	return nn(list), nil
}

// restoreContent computes what restoring backup id would write and which credentials cannot be restored.
func (s *SystemService) restoreContent(id string) (entry api.BackupEntry, current, merged []byte, lost []string, err error) {
	entry, content, err := s.c.Backup.Get(id)
	if err != nil {
		return entry, nil, nil, nil, err
	}
	if !s.restoreTargetOK(entry) {
		return entry, nil, nil, nil, api.NewError(api.ErrBackupNotFound, "the backup has no valid target")
	}
	if b, rerr := os.ReadFile(entry.Path); rerr == nil {
		current = b
	}
	merged, lost = content, []string{}
	if entry.Redacted {
		merged, lost, err = backup.MergeRedacted(content, current)
		if err != nil {
			return entry, current, nil, nil, err
		}
	}
	return entry, current, merged, lost, nil
}

// samePath compares two paths after cleaning and, when both exist, after resolving symlinks.
func samePath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, ea := filepath.EvalSymlinks(a)
	rb, eb := filepath.EvalSymlinks(b)
	return ea == nil && eb == nil && ra == rb
}

// inside reports whether path lies below dir (not equal to it).
func inside(dir, path string) (string, bool) {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// restoreTargetOK reports whether a backup entry points at a file the app itself edits: the AWS config, the
// AWS credentials file, the global git config, or the config inside the .git directory of a repository
// currently on the repositories list. The index is a plain file, so its paths are not trusted.
func (s *SystemService) restoreTargetOK(e api.BackupEntry) bool {
	c := s.c
	if e.Path == "" || !filepath.IsAbs(e.Path) {
		return false
	}
	switch e.Kind {
	case api.BackupAWSConfig:
		return samePath(e.Path, c.Paths.AWSConfigFile)
	case api.BackupAWSCredentials:
		return samePath(e.Path, c.Paths.AWSCredentialsFile)
	case api.BackupGitGlobal:
		return samePath(e.Path, c.gitGlobalPath())
	case api.BackupGitLocal:
		if filepath.Base(e.Path) != "config" || c.Repos == nil {
			return false
		}
		for _, r := range c.Repos.Snapshot() {
			if rel, ok := inside(r.Path, e.Path); ok && strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
				return true
			}
		}
		return false
	}
	return false
}

func profileNames(sections []string) []string {
	out := make([]string, 0, len(sections))
	for _, sec := range sections {
		sec = strings.TrimSpace(sec)
		sec = strings.TrimSpace(strings.TrimPrefix(sec, "profile "))
		out = append(out, sec)
	}
	return out
}

func (s *SystemService) PreviewRestore(id string) (api.RestorePreview, error) {
	entry, current, merged, lost, err := s.restoreContent(id)
	if err != nil {
		return api.RestorePreview{}, fail(err)
	}
	s.rememberPreview(id, current)
	prev := awsconfig.Preview(entry.Path, entry.File, s.c.Paths.Display(s.c.Backup.Dir()), current, merged)
	return nn(api.RestorePreview{Entry: entry, Preview: prev, LostProfiles: profileNames(lost)}), nil
}

// contentHash is the hash recorded for a target file's bytes; a missing file (nil) is "".
func contentHash(b []byte) string {
	if b == nil {
		return ""
	}
	return hashBytes(b)
}

func (s *SystemService) rememberPreview(id string, current []byte) {
	s.previewMu.Lock()
	if s.previewed == nil {
		s.previewed = map[string]string{}
	}
	s.previewed[id] = contentHash(current)
	s.previewMu.Unlock()
}

// checkRestoreBase fails with E_CONFIG_CHANGED when the target file is not what the user last saw. With a
// preview that is the file as previewed for this backup. Without one (the Undo toast after a write) it is the
// file as the app left it after the write the backup was taken for; the watcher's latest record is not used
// because it follows external edits too. For a watched file with no such record (for example after a restart)
// the undo is refused. Repository configs are not watched and are not compared; a safety snapshot is taken
// before every restore.
func (s *SystemService) checkRestoreBase(id string, entry api.BackupEntry, current []byte) error {
	cur := contentHash(current)
	s.previewMu.Lock()
	want, previewed := s.previewed[id]
	s.previewMu.Unlock()
	if !previewed {
		if s.c.files == nil || !s.c.files.watches(entry.Path) {
			return nil
		}
		var ok bool
		if want, ok = s.c.files.writtenAfter(id); !ok {
			return api.NewError(api.ErrConfigChanged, "the file changed since it was reviewed").WithParam("file", entry.File)
		}
	}
	if cur != want {
		return api.NewError(api.ErrConfigChanged, "the file changed since it was reviewed").WithParam("file", entry.File)
	}
	return nil
}

// Restores a backup; the safety snapshot taken first lets the UI offer to undo the restore.

// RestoreBackup: returns the safety backup taken first
func (s *SystemService) RestoreBackup(id string, confirmLoss bool) (api.BackupEntry, error) {
	c := s.c
	entry, current, merged, lost, err := s.restoreContent(id)
	if err != nil {
		return api.BackupEntry{}, fail(err)
	}
	if err := s.checkRestoreBase(id, entry, current); err != nil {
		return api.BackupEntry{}, fail(err)
	}
	if len(lost) > 0 && !confirmLoss {
		return api.BackupEntry{}, fail(api.NewError(api.ErrRestoreNeedsConfirm, "the backup cannot restore some key values").
			WithParam("profiles", strings.Join(profileNames(lost), ", ")))
	}
	safety, err := c.Backup.Snapshot(entry.Path, entry.File, entry.Kind, api.ReasonRestore, map[string]string{"file": entry.File})
	if err != nil {
		return api.BackupEntry{}, fail(err)
	}
	if err := backup.WriteFileAtomic(entry.Path, merged, 0o600); err != nil {
		return api.BackupEntry{}, fail(api.NewError(api.ErrFileIO, "cannot write the file").WithParam("file", entry.File).WithDetail(err.Error()))
	}
	c.files.note(entry.Path, merged)
	s.previewMu.Lock()
	delete(s.previewed, id)
	s.previewMu.Unlock()
	ctx := context.Background()
	switch entry.Kind {
	case api.BackupAWSConfig, api.BackupAWSCredentials:
		c.Accounts.EmitState(ctx)
	default: // git-global, git-local
		c.Repos.Invalidate(ctx)
		if st, err := c.Repos.State(ctx); err == nil {
			c.emit(api.EventReposChanged, st)
		}
		c.Accounts.EmitState(ctx)
	}
	return safety, nil
}

func (s *SystemService) RevealBackup(id string) error {
	p, err := s.c.Backup.FilePath(id)
	if err != nil {
		return fail(err)
	}
	return fail(s.c.RT.RevealPath(p))
}

func (s *SystemService) OpenBackupFolder() error {
	return fail(s.c.RT.RevealPath(s.c.Backup.Dir()))
}

// OpenURL: https:// only, else E_INVALID_INPUT
func (s *SystemService) OpenURL(url string) error {
	if err := checkHTTPS(url); err != nil {
		return fail(err)
	}
	return fail(s.c.RT.OpenURL(url))
}

// RevealPath: must exist and be under the home dir. A leading "~/" (the display form used by Health
// findings) is expanded first.
func (s *SystemService) RevealPath(path string) error {
	abs := filepath.Clean(s.c.Paths.Expand(path))
	if !filepath.IsAbs(abs) {
		return fail(invalidField("path"))
	}
	rel, err := filepath.Rel(filepath.Clean(s.c.Paths.Home), abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fail(invalidField("path"))
	}
	if _, err := os.Stat(abs); err != nil {
		return fail(api.NewError(api.ErrFileIO, "path not found").WithParam("file", s.c.Paths.Display(abs)))
	}
	return fail(s.c.RT.RevealPath(abs))
}

// OpenAWSConfig: opens ~/.aws/config in the default text editor
func (s *SystemService) OpenAWSConfig() error {
	p := s.c.Paths.AWSConfigFile
	if _, err := os.Stat(p); err != nil {
		return fail(api.NewError(api.ErrFileIO, "the AWS config file does not exist").WithParam("file", s.c.Paths.Display(p)))
	}
	return fail(s.c.RT.OpenFile(p))
}

// ChooseFolder: "" when cancelled
func (s *SystemService) ChooseFolder(title string) (string, error) {
	dir, err := s.c.RT.ChooseFolder(title)
	if err != nil {
		return "", fail(err)
	}
	return dir, nil
}

// OpenTerminal: P1
func (s *SystemService) OpenTerminal(ctx context.Context, profile string, dir string) error {
	if !awsconfig.ProfileNameRE.MatchString(profile) {
		return fail(invalidField("profile"))
	}
	if dir == "" {
		dir = s.c.Paths.Home
	}
	return fail(terminal.Open(ctx, terminal.Request{App: s.c.Settings.TerminalApp(), Dir: dir, Profile: profile, GOOS: runtime.GOOS}))
}
