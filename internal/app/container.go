package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/rudian/aws_sso_hub/internal/accounts"
	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/awsapi"
	"github.com/rudian/aws_sso_hub/internal/backup"
	"github.com/rudian/aws_sso_hub/internal/codecommit"
	"github.com/rudian/aws_sso_hub/internal/credstore"
	"github.com/rudian/aws_sso_hub/internal/envcheck"
	"github.com/rudian/aws_sso_hub/internal/execx"
	"github.com/rudian/aws_sso_hub/internal/gitglobal"
	"github.com/rudian/aws_sso_hub/internal/gitx"
	"github.com/rudian/aws_sso_hub/internal/migrate"
	"github.com/rudian/aws_sso_hub/internal/paths"
	"github.com/rudian/aws_sso_hub/internal/repos"
	"github.com/rudian/aws_sso_hub/internal/settings"
	"github.com/rudian/aws_sso_hub/internal/sso"
	"github.com/rudian/aws_sso_hub/internal/transfer"
)

// Endpoints override the AWS service endpoints (sandbox mode and tests). "" = AWS default.
type Endpoints struct{ OIDC, Portal, STS, IAM, CodeCommit string }

// ContainerOptions configures NewContainer.
type ContainerOptions struct {
	Endpoints Endpoints
	Now       func() time.Time
	Logger    *slog.Logger // nil = discard

	// Tests only.
	DisableLoginShell bool          // execx must not spawn a login shell
	TickInterval      time.Duration // accounts ticker, default 30 s
	WatchInterval     time.Duration // file watcher, default 3 s
	ScanDelay         time.Duration // delay before the background scan, default 1 s
}

// EndpointsFromEnv reads the AWSSSOHUB_*_ENDPOINT overrides. They are honoured in sandbox mode only.
func EndpointsFromEnv(p paths.Paths) Endpoints {
	if !p.Sandbox {
		return Endpoints{}
	}
	return Endpoints{
		OIDC:       os.Getenv("AWSSSOHUB_OIDC_ENDPOINT"),
		Portal:     os.Getenv("AWSSSOHUB_PORTAL_ENDPOINT"),
		STS:        os.Getenv("AWSSSOHUB_STS_ENDPOINT"),
		IAM:        os.Getenv("AWSSSOHUB_IAM_ENDPOINT"),
		CodeCommit: os.Getenv("AWSSSOHUB_CODECOMMIT_ENDPOINT"),
	}
}

// Container owns every engine and the glue between them. The five Wails services are thin views on it.
type Container struct {
	Paths    paths.Paths
	RT       Runtime
	Log      *slog.Logger
	Settings *settings.Store
	FirstRun bool
	Backup   *backup.Store
	SSO      *sso.Manager
	Accounts *accounts.Engine
	Transfer *transfer.Service
	Migrate  *migrate.Engine
	Repos    *repos.Engine
	Global   *gitglobal.Engine
	Creds    credstore.Store

	opts ContainerOptions
	now  func() time.Time

	subMu sync.Mutex
	subs  []func(name string, data any)

	gitMu     sync.Mutex
	gitHandle *gitx.Git
	gitFor    string // settings.gitPath the handle was built for

	envMu     sync.Mutex
	envReport *api.EnvReport

	files *fileWatcher

	lifeMu    sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
	started   bool
	closed    bool
	wg        sync.WaitGroup
	onClose   []func()
	settingsH []func(prev, next api.Settings)
	langH     []func(lang string)
}

// NewContainer builds the engines. Nothing runs in the background until StartLoops.
func NewContainer(p paths.Paths, rt Runtime, o ContainerOptions) (*Container, error) {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.NewTextHandler(discardWriter{}, nil))
	}
	execx.Configure(execx.Options{ExtraEnv: p.ChildEnv(), DisableLoginShell: o.DisableLoginShell})

	c := &Container{Paths: p, RT: rt, Log: o.Logger, opts: o, now: o.Now}
	c.ctx, c.cancel = context.WithCancel(context.Background())

	st, first, err := settings.Open(p.SettingsFile, p.DefaultScanRoot, o.Now)
	if err != nil {
		return nil, err
	}
	c.Settings, c.FirstRun = st, first
	if dropped := st.DroppedToolPaths(); len(dropped) > 0 && o.Logger != nil {
		o.Logger.Warn("ignoring invalid tool paths in the settings file", "settings", dropped)
	}

	bk, err := backup.Open(p.BackupDir, st.BackupRetention, o.Now)
	if err != nil {
		return nil, api.AsAppError(err)
	}
	c.Backup = bk

	c.files = newFileWatcher(c)
	bk.SetOnSnapshot(c.files.onSnapshot)
	emit := c.emit

	c.SSO = sso.NewManager(sso.Config{
		Home:             p.Home,
		RegistrationsDir: p.RegistrationsDir,
		Endpoints:        sso.Endpoints{OIDC: o.Endpoints.OIDC, Portal: o.Endpoints.Portal},
		OpenBrowser:      rt.OpenURL,
		Lang:             st.UILanguage,
		Now:              o.Now,
	})
	awsEP := awsapi.Endpoints{STS: o.Endpoints.STS, IAM: o.Endpoints.IAM}

	c.Accounts = accounts.New(accounts.Deps{
		Paths: p, Sessions: c.SSO, Backup: bk, Store: st, Endpoints: awsEP,
		OpenBrowser: rt.OpenURL, Emit: emit, Now: o.Now, OnWrite: c.files.note,
	})
	c.Transfer = transfer.New(transfer.Deps{
		Paths: p, Backup: bk, Store: st, State: c.Accounts.State, Changed: c.Accounts.EmitState,
		WriteLock: c.Accounts.LockWrites, Now: o.Now, OnWrite: c.files.note,
	})
	c.Migrate = migrate.New(migrate.Deps{
		Paths: p, Sessions: c.SSO, Portals: c.Accounts, Backup: bk, Endpoints: awsEP, Store: st,
		Emit: emit, Now: o.Now, OnWrite: c.files.note,
	})
	c.Repos = repos.New(repos.Deps{
		Paths:      p,
		Git:        c.git,
		AWSPath:    c.awsPath,
		AWSReady:   c.awsReady,
		Profiles:   c.profileSource,
		Store:      st,
		Backup:     bk,
		CodeCommit: codecommit.NewFactory(o.Endpoints.CodeCommit),
		Emit:       emit,
		Now:        o.Now,
		GOOS:       runtime.GOOS,
	})
	c.Accounts.SetRepoStats(c.Repos)
	c.Creds = credstore.New(runtime.GOOS, c.git, p.Sandbox)
	c.Global = gitglobal.New(gitglobal.Deps{
		Paths: p, Git: c.git, AWSPath: c.awsPath, Repos: c.Repos, Profiles: c.profileSource,
		Backup: bk, Creds: c.Creds, Lang: st.UILanguage, GOOS: runtime.GOOS, Emit: emit, OnWrite: c.files.note,
	})
	return c, nil
}

type discardWriter struct{}

func (discardWriter) Write(b []byte) (int, error) { return len(b), nil }

// emit sends an event to the frontend and to in-process subscribers (tray).
func (c *Container) emit(name string, data any) {
	c.RT.Emit(name, data)
	c.subMu.Lock()
	subs := append([]func(string, any){}, c.subs...)
	c.subMu.Unlock()
	for _, f := range subs {
		f(name, data)
	}
}

// Subscribe registers an in-process listener for every emitted event.
func (c *Container) Subscribe(f func(name string, data any)) {
	c.subMu.Lock()
	c.subs = append(c.subs, f)
	c.subMu.Unlock()
}

// OnSettingsChanged registers a callback invoked by SystemService.UpdateSettings with the old and new settings.
func (c *Container) OnSettingsChanged(f func(prev, next api.Settings)) {
	c.lifeMu.Lock()
	c.settingsH = append(c.settingsH, f)
	c.lifeMu.Unlock()
}

// OnShutdown registers a function run once by Shutdown.
func (c *Container) OnShutdown(f func()) {
	c.lifeMu.Lock()
	c.onClose = append(c.onClose, f)
	c.lifeMu.Unlock()
}

// ---- tools -----------------------------------------------------------------------------------------------

// git returns the (cached) git handle for the configured path.
func (c *Container) git(ctx context.Context) (*gitx.Git, error) {
	override := c.Settings.GitPath()
	c.gitMu.Lock()
	if c.gitHandle != nil && c.gitFor == override {
		g := c.gitHandle
		c.gitMu.Unlock()
		return g, nil
	}
	c.gitMu.Unlock()
	g, err := gitx.New(ctx, override, c.Paths.ChildEnv())
	if err != nil {
		return nil, err
	}
	c.gitMu.Lock()
	c.gitHandle, c.gitFor = g, override
	c.gitMu.Unlock()
	return g, nil
}

// cachedGit returns the git handle if one was already built (never spawns a process).
func (c *Container) cachedGit() *gitx.Git {
	c.gitMu.Lock()
	defer c.gitMu.Unlock()
	return c.gitHandle
}

func (c *Container) forgetGit() {
	c.gitMu.Lock()
	c.gitHandle = nil
	c.gitMu.Unlock()
}

func (c *Container) awsPath() (string, error) {
	p, err := execx.FindAWS(c.Settings.AWSCLIPath())
	if err != nil {
		return "", api.NewError(api.ErrAWSCLIMissing, "the AWS CLI was not found")
	}
	return p, nil
}

// gitGlobalPath is the file `git config --global` writes, without spawning git when possible.
func (c *Container) gitGlobalPath() string {
	if c.Paths.GitGlobalFile != "" {
		return c.Paths.GitGlobalFile
	}
	if g := c.cachedGit(); g != nil {
		return g.GlobalConfigPath(c.Paths.Home, "")
	}
	if v := os.Getenv("GIT_CONFIG_GLOBAL"); v != "" {
		return v
	}
	return filepath.Join(c.Paths.Home, ".gitconfig")
}

func (c *Container) profileSource(ctx context.Context) ([]repos.ProfileInfo, error) {
	pcs, err := c.Accounts.ProfileCreds(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]repos.ProfileInfo, 0, len(pcs))
	for _, p := range pcs {
		out = append(out, repos.ProfileInfo{
			Name: p.Name, AccountID: p.AccountID, Portal: p.Portal, Kind: p.Kind, ColorSlot: p.ColorSlot,
			Monogram: p.Monogram, Usable: p.Usable, NotSignedInPortal: p.NotSignedInPortal, Provider: p.Provider,
		})
	}
	return out, nil
}

// ---- environment check -----------------------------------------------------------------------------------

// CheckEnvironment runs the tool and file checks and caches the report for AWSReady.
func (c *Container) CheckEnvironment(ctx context.Context) api.EnvReport {
	rep := api.EnvReport{
		AWSCLI:    envcheck.CheckAWSCLI(ctx, c.Settings.AWSCLIPath()),
		Git:       envcheck.CheckGit(ctx, c.Settings.GitPath()),
		GRC:       envcheck.CheckGRC(ctx),
		Files:     c.fileChecks(),
		CheckedAt: c.now().UTC().Format(time.RFC3339),
	}
	c.envMu.Lock()
	c.envReport = &rep
	c.envMu.Unlock()
	return rep
}

func (c *Container) fileChecks() []api.FileCheck {
	var issues []api.ConfigIssue
	if snap, err := c.Accounts.Snapshot(); err == nil {
		issues = snap.Issues(c.Paths.Display)
	}
	mk := func(kind, path string) api.FileCheck {
		fc := api.FileCheck{Kind: kind, File: c.Paths.Display(path)}
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			fc.Exists = true
			if f, err := os.Open(path); err == nil {
				fc.Readable = true
				f.Close()
			}
			if f, err := os.OpenFile(path, os.O_WRONLY, 0); err == nil { // never truncates
				fc.Writable = true
				f.Close()
			}
		}
		for i := range issues {
			if issues[i].File == fc.File {
				is := issues[i]
				fc.Issue = &is
				break
			}
		}
		return fc
	}
	return []api.FileCheck{
		mk("aws-config", c.Paths.AWSConfigFile),
		mk("aws-credentials", c.Paths.AWSCredentialsFile),
		mk("git-global", c.gitGlobalPath()),
	}
}

// awsReady is true when the cached environment check says the AWS CLI is usable for git (ok or outdated).
// The first call runs the check.
func (c *Container) awsReady() bool {
	c.envMu.Lock()
	rep := c.envReport
	c.envMu.Unlock()
	if rep == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		r := c.CheckEnvironment(ctx)
		rep = &r
	}
	return rep.AWSCLI.Status == api.ToolOK || rep.AWSCLI.Status == api.ToolOutdated
}

// invalidateEnv drops the cached report (tool path settings changed).
func (c *Container) invalidateEnv() {
	c.envMu.Lock()
	c.envReport = nil
	c.envMu.Unlock()
	c.forgetGit()
}

// ---- helpers shared by the services ------------------------------------------------------------------------

func fail(err error) error {
	if err == nil {
		return nil
	}
	return api.AsAppError(err)
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// hashFile returns the sha256 of a file, or "" when it does not exist.
func hashFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return hashBytes(b)
}

// notifySettings tells the shell (tray, window behaviour) that settings changed.
func (c *Container) notifySettings(prev, next api.Settings) {
	c.lifeMu.Lock()
	hs := append([]func(prev, next api.Settings){}, c.settingsH...)
	c.lifeMu.Unlock()
	for _, h := range hs {
		h(prev, next)
	}
}

// OnUILanguage registers a callback invoked after SystemService.SetUILanguage.
func (c *Container) OnUILanguage(f func(lang string)) {
	c.lifeMu.Lock()
	c.langH = append(c.langH, f)
	c.lifeMu.Unlock()
}

func (c *Container) notifyUILanguage(lang string) {
	c.lifeMu.Lock()
	hs := append([]func(string){}, c.langH...)
	c.lifeMu.Unlock()
	for _, h := range hs {
		h(lang)
	}
}
