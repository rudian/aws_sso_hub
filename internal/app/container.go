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
	"github.com/rudian/aws_sso_hub/internal/browser"
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

// Endpoints override the AWS service endpoints (sandbox mode and tests). "" = AWS default. SignIn is the
// base URL of the console sign-in federation endpoint (SignIn + "/federation").
type Endpoints struct{ OIDC, Portal, STS, IAM, CodeCommit, SignIn string }

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
		SignIn:     os.Getenv("AWSSSOHUB_SIGNIN_ENDPOINT"),
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

	// private opens sign-in pages in an isolated browser or a private window. nil in sandbox mode: a
	// sandboxed app never launches a real browser. Tests may set it to a fake.
	private signInBrowser
	// console opens the console through a portal's proxy in Google Chrome. nil in sandbox mode, like private.
	// Tests may set it to a fake.
	console consoleBrowser
	// signInWin holds the isolated browsers opened for sign-in pages, by page URL, until the sign-in ends;
	// signInDone remembers the pages whose sign-in has ended, so a window that opens late is closed too.
	signInMu    sync.Mutex
	signInWin   map[string][]signInWindow
	signInDone  map[string]time.Time
	signInDelay time.Duration // how long a successful sign-in's window stays open (the success page)
	startedAt   time.Time

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

	c := &Container{Paths: p, RT: rt, Log: o.Logger, opts: o, now: o.Now,
		signInWin: map[string][]signInWindow{}, signInDone: map[string]time.Time{}, signInDelay: 3 * time.Second, startedAt: time.Now()}
	c.ctx, c.cancel = context.WithCancel(context.Background())
	if !p.Sandbox {
		op := &browser.Opener{Home: p.Home}
		c.private, c.console = browserOpener{op}, op
	}
	c.OnShutdown(c.closeSignInWindows)

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
		OpenSignIn:       c.openSignIn,
		SignInEnded:      c.signInEnded,
		Lang:             st.UILanguage,
		Now:              o.Now,
	})
	awsEP := awsapi.Endpoints{STS: o.Endpoints.STS, IAM: o.Endpoints.IAM, SignIn: o.Endpoints.SignIn}

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

// signInBrowser is what the container needs from internal/browser (faked in tests).
type signInBrowser interface {
	OpenPrivate(ctx context.Context, rawURL string) (bool, error)
	OpenIsolated(ctx context.Context, rawURL, dir string) (signInWindow, error)
	RemoveStale(ctx context.Context, root string, keep func(dir string) bool)
}

// signInWindow is an isolated browser opened for a sign-in page.
type signInWindow interface {
	Close(ctx context.Context) error
}

// browserOpener adapts *browser.Opener to signInBrowser.
type browserOpener struct{ *browser.Opener }

func (b browserOpener) OpenIsolated(ctx context.Context, rawURL, dir string) (signInWindow, error) {
	w, err := b.Opener.OpenIsolated(ctx, rawURL, dir)
	if w == nil {
		return nil, err
	}
	return w, err
}

// signInRoot holds the throw-away browser profiles of sign-ins.
func (c *Container) signInRoot() string { return filepath.Join(c.Paths.AppDataDir, "signin-browser") }

// openSignIn opens a sign-in page. With the "private sign-in" setting on it tries, in order: a new instance
// of the default browser on a fresh profile of its own (isolated: it cannot see another user's sign-in, and
// is closed when the sign-in ends), a private window of the default browser (all private windows share one
// session, so another user's sign-in can still be reused there), and the normal open. Sandbox mode and a
// browser without a private switch use the normal open.
func (c *Container) openSignIn(raw string) (api.SignInWindow, error) {
	if err := checkOpenable(raw, c.Paths.Sandbox); err != nil {
		return api.SignInWindow{}, err
	}
	if c.private != nil && !c.Paths.Sandbox && c.Settings.PrivateSignIn() {
		ctx, cancel := context.WithTimeout(c.ctx, 15*time.Second)
		defer cancel()
		if c.openIsolated(ctx, raw) {
			return api.SignInWindow{Private: true, Isolated: true}, nil
		}
		private, err := c.private.OpenPrivate(ctx, raw)
		if err != nil {
			c.Log.Warn("cannot open a private browser window; using the normal one", "error", err.Error())
		} else if private {
			return api.SignInWindow{Private: true}, nil
		}
	}
	return api.SignInWindow{}, c.RT.OpenURL(raw)
}

// openIsolated opens the page in an isolated browser and keeps it under the page URL for signInEnded.
// Browsers already open for the same page (the user chose "Open again") are closed.
func (c *Container) openIsolated(ctx context.Context, raw string) bool {
	root := c.signInRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		c.Log.Warn("cannot create the sign-in browser folder", "error", err.Error())
		return false
	}
	dir, err := os.MkdirTemp(root, "s-")
	if err != nil {
		c.Log.Warn("cannot create a sign-in browser profile", "error", err.Error())
		return false
	}
	w, err := c.private.OpenIsolated(ctx, raw, dir)
	if err != nil || w == nil {
		_ = os.RemoveAll(dir)
		if err != nil {
			c.Log.Warn("cannot open an isolated browser; using a private window", "error", err.Error())
		}
		return false
	}
	c.signInMu.Lock()
	old := c.signInWin[raw]
	if _, ended := c.signInDone[raw]; ended {
		// the sign-in ended while this window was opening
		old = append(old, w)
		delete(c.signInWin, raw)
	} else {
		c.signInWin[raw] = []signInWindow{w}
	}
	c.signInMu.Unlock()
	c.closeSignIn(old)
	return true
}

// signInEnded closes the isolated browsers of a sign-in that has ended. After a successful sign-in they stay
// open for a moment, so the success page can be seen.
func (c *Container) signInEnded(pages []string, ok bool) {
	delay := time.Duration(0)
	if ok {
		delay = c.signInDelay
	}
	now := time.Now()
	c.signInMu.Lock()
	for p, at := range c.signInDone {
		if now.Sub(at) > time.Hour {
			delete(c.signInDone, p)
		}
	}
	for _, p := range pages {
		c.signInDone[p] = now
	}
	c.signInMu.Unlock()
	go func() {
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-c.ctx.Done():
				return // quitting: closeSignInWindows ends them before the app exits
			}
		}
		c.signInMu.Lock()
		var ws []signInWindow
		for _, p := range pages {
			ws = append(ws, c.signInWin[p]...)
			delete(c.signInWin, p)
		}
		c.signInMu.Unlock()
		c.closeSignIn(ws)
	}()
}

// closeSignIn closes isolated browsers in the background.
func (c *Container) closeSignIn(ws []signInWindow) {
	if len(ws) == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		for _, w := range ws {
			if err := w.Close(ctx); err != nil {
				c.Log.Warn("cannot remove a sign-in browser profile", "error", err.Error())
			}
		}
	}()
}

// closeSignInWindows closes every isolated sign-in browser that is still open (app shutdown).
func (c *Container) closeSignInWindows() {
	c.signInMu.Lock()
	var ws []signInWindow
	for p, list := range c.signInWin {
		ws = append(ws, list...)
		delete(c.signInWin, p)
	}
	c.signInMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	for _, w := range ws {
		_ = w.Close(ctx)
	}
}

// removeStaleSignIn ends isolated sign-in browsers left behind by an earlier run (a crash) and deletes their
// profiles. Profiles made since this run started are kept.
func (c *Container) removeStaleSignIn(ctx context.Context) {
	if c.private == nil || c.Paths.Sandbox {
		return
	}
	c.private.RemoveStale(ctx, c.signInRoot(), func(dir string) bool {
		st, err := os.Stat(dir)
		return err != nil || !st.ModTime().Before(c.startedAt)
	})
}
