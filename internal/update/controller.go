package update

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// Phase is what the UI shows. The values are stable strings (they cross into the frontend).
type Phase string

const (
	PhaseIdle        Phase = "idle"
	PhaseChecking    Phase = "checking"
	PhaseUpToDate    Phase = "up-to-date"
	PhaseAvailable   Phase = "available"   // a verified newer release exists; nothing downloaded yet
	PhaseDownloading Phase = "downloading" // Written/Total carry the progress
	PhaseVerifying   Phase = "verifying"   // bundle gate on the staged copy
	PhaseReady       Phase = "ready"       // staged and verified; waiting for Restart
	PhaseRestarting  Phase = "restarting"
	PhaseBlocked     Phase = "blocked" // this copy cannot replace itself (Block, Folder say why)
	PhaseRefused     Phase = "refused" // a release was found and refused: Error says why, never retried silently
	PhaseError       Phase = "error"   // network or disk trouble; the next check tries again
)

// State is a snapshot for the UI. Notes come only from the signed manifest and are plain text.
type State struct {
	Phase     Phase     `json:"phase"`
	Version   string    `json:"version,omitempty"`
	Notes     string    `json:"notes,omitempty"`
	Written   int64     `json:"written,omitempty"`
	Total     int64     `json:"total,omitempty"`
	Error     string    `json:"error,omitempty"`  // English, for the log; the UI shows Code
	Code      string    `json:"code,omitempty"`   // stable reason (Code* constants, or CodeRestart*); the UI text is per language
	Step      string    `json:"step,omitempty"`   // StepCheck, StepDownload, StepVerify or StepRestart: where it went wrong
	Block     string    `json:"block,omitempty"`  // BlockTranslocated, BlockNotWritable, BlockUnsupported
	Folder    string    `json:"folder,omitempty"` // the folder to name in a "cannot update" message
	CheckedAt time.Time `json:"checkedAt,omitzero"`
}

// Steps of the flow, for State.Step.
const (
	StepCheck    = "check"
	StepDownload = "download"
	StepVerify   = "verify"
	StepRestart  = "restart"
)

// Codes of failures that are not refusals.
const (
	CodeRestartTimeout = "restart-timeout" // the helper was started but the app did not quit
	CodeRestartFailed  = "restart-failed"
	CodeNetwork        = "network"
)

// restartWait is longer than the 30 s the Wails helper waits for the app to exit before it relaunches the old copy.
const restartWait = 45 * time.Second

// ControllerConfig wires the controller to the app.
type ControllerConfig struct {
	// Quit ends the app at once, without the quit dialog or the "stay in the background" choice: the user has already
	// chosen to restart, and if the app stays up the helper gives up after 30 s and relaunches the old copy.
	Quit    func()
	Running string // running version
	Gate    Gate
	Highest Highest
	// Channel returns the channel to check, read fresh each time (the test-channel token comes from the Keychain).
	Channel func() (Channel, error)
	// Executable is os.Executable's result with symlinks resolved; nil uses the real one.
	Executable func() (string, error)
	OS, Arch   string       // "" = runtime values
	HTTP       *http.Client // nil: NewHTTPClient(Proxy); tests inject theirs
	// Proxy picks the proxy per request: pass the system-proxy resolver of internal/netx (an app opened from Finder
	// has no proxy variables). Keep no overall timeout on the client built from it.
	Proxy func(*http.Request) (*url.URL, error)
	// HelperLog is the file in the app's data folder where Restart records this process's pid so the next start can
	// remove exactly that helper log from $TMPDIR. Required.
	HelperLog string
	// StagedRecord is the file in the app's data folder (for example <data>/update/staged) where Download records the
	// staging folder it made beside the app. Run removes exactly that folder at the next start if the app quit
	// without Restart. Required.
	StagedRecord string
	// RestartWait overrides restartWait (tests only).
	RestartWait time.Duration
	// IdleTimeout overrides the no-progress limit of a download (default 60 s; tests only).
	IdleTimeout time.Duration
	OnState     func(State)
	// AutoCheck, when set, is asked before every tick of Run; false skips that check (the person turned the daily
	// check off). A manual Check ignores it.
	AutoCheck func() bool
	Logf      func(format string, a ...any) // never given tokens, notes or file contents
}

// Controller owns the update flow. It never exposes the raw Wails Restart, so nothing can start the helper without
// passing the gate first, and it builds a fresh Updater and Provider for every check (after a refusal the Wails
// state machine stays in "ready" and its own periodic loop would skip it for ever).
type Controller struct {
	cfg ControllerConfig

	mu      sync.Mutex
	state   State
	session *session
	busy    bool
	refused *refusedRelease // the release the bundle gate refused; not offered again until Retry

	dlCancel context.CancelFunc // cancels a running Download; Close uses it
	dlDone   chan struct{}      // closed when that Download has returned
	// closeWait bounds how long Close waits for a cancelled download to remove its staging folder.
	closeWait time.Duration
}

// refusedRelease remembers a release (manifest version and sha256) that was refused after the check, with the state
// the UI keeps showing for it.
type refusedRelease struct {
	version, digest string
	state           State
	err             error
}

type session struct {
	u         *updater.Updater
	prov      *Provider
	rel       *updater.Release
	target    string // what the helper replaces
	staged    string
	handedOff bool // Restart started the helper: it owns the staging folder now, only watchRestart may drop it
	progress  func(written, total int64)
}

// NewController returns an idle controller.
func NewController(cfg ControllerConfig) *Controller {
	if cfg.OS == "" {
		cfg.OS = runtime.GOOS
	}
	if cfg.Arch == "" {
		cfg.Arch = runtime.GOARCH
	}
	if cfg.Executable == nil {
		cfg.Executable = realExecutable
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.RestartWait <= 0 {
		cfg.RestartWait = restartWait
	}
	return &Controller{cfg: cfg, state: State{Phase: PhaseIdle}, closeWait: 5 * time.Second}
}

// validate refuses to run with a missing wire, so a slip in the wiring is a visible error and never a rule that
// silently does not apply.
func (c *Controller) validate() error {
	switch {
	case c.cfg.Highest.Path == "":
		return refuseAs(CodeConfig, "the stored highest version has no file configured")
	case c.cfg.Channel == nil:
		return refuseAs(CodeConfig, "no update channel is configured")
	case c.cfg.Gate.Requirement == "":
		return refuseAs(CodeConfig, "no code-signature requirement is configured")
	case c.cfg.HelperLog == "":
		return refuseAs(CodeConfig, "no helper-log record is configured")
	case c.cfg.StagedRecord == "":
		return refuseAs(CodeConfig, "no staged-update record is configured")
	}
	return nil
}

func realExecutable() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}

// State returns the current snapshot.
func (c *Controller) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *Controller) set(s State) {
	c.mu.Lock()
	if s.CheckedAt.IsZero() {
		s.CheckedAt = c.state.CheckedAt
	}
	c.state = s
	cb := c.cfg.OnState
	c.mu.Unlock()
	if cb != nil {
		cb(s)
	}
}

// begin marks the controller busy; two flows never run at once.
func (c *Controller) begin() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.busy {
		return false
	}
	c.busy = true
	return true
}

func (c *Controller) end() { c.mu.Lock(); c.busy = false; c.mu.Unlock() }

var errBusy = errors.New("an update step is already running")

// errRestartPending: Restart started the helper, which now owns the staged copy. Nothing may touch it.
var errRestartPending = errors.New("a restart is already in progress")

// restartPending reports whether the current session was handed to the helper.
func (c *Controller) restartPending() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.session != nil && c.session.handedOff
}

// Check looks for a newer, verified release. It makes no download. A staged update from an earlier check is
// discarded. The result is also the new State.
func (c *Controller) Check(ctx context.Context) (State, error) {
	if !c.begin() {
		return c.State(), errBusy
	}
	defer c.end()
	if c.restartPending() {
		return c.State(), errRestartPending
	}
	c.discardSession()
	now := time.Now()
	c.set(State{Phase: PhaseChecking, CheckedAt: now})

	fail := func(err error) (State, error) {
		s := State{Phase: PhaseError, Error: err.Error(), Code: CodeNetwork, Step: StepCheck, CheckedAt: now}
		var pf *PreflightError
		switch {
		case errors.As(err, &pf):
			s = State{Phase: PhaseBlocked, Block: pf.Kind, Folder: pf.Folder, Error: pf.Error(), Code: pf.Kind, Step: StepCheck, CheckedAt: now}
		case IsRefusal(err):
			s.Phase, s.Code = PhaseRefused, RefusalCode(err)
		}
		c.cfg.Logf("update check: %s (%s): %s", s.Phase, s.Code, s.Error)
		c.set(s)
		return s, err
	}

	if err := c.validate(); err != nil {
		return fail(err)
	}
	if !GateSupported() {
		return fail(&PreflightError{Kind: BlockUnsupported, Detail: "updates are not available on this system yet"})
	}
	exe, err := c.cfg.Executable()
	if err != nil {
		return fail(fmt.Errorf("cannot find the running app: %w", err))
	}
	if err := Preflight(exe, c.cfg.OS); err != nil {
		return fail(err)
	}
	ch, err := c.cfg.Channel()
	if err != nil {
		return fail(refuseAs(CodeChannel, "%v", err))
	}
	prov, err := NewProvider(ProviderConfig{Channel: ch, Running: c.cfg.Running, Highest: c.cfg.Highest.Get, HTTP: c.cfg.HTTP, Proxy: c.cfg.Proxy, IdleTimeout: c.cfg.IdleTimeout})
	if err != nil {
		return fail(err)
	}
	s := &session{prov: prov, target: installTarget(exe, c.cfg.OS)}
	tap := &hostTap{quit: c.cfg.Quit, onProgress: func(written, total int64) {
		if s.rel != nil {
			c.set(State{Phase: PhaseDownloading, Version: s.rel.Version, Notes: s.rel.Notes, Written: written, Total: total, CheckedAt: now})
		}
	}}
	u := updater.New(tap)
	if err := u.Init(updater.Config{
		CurrentVersion: c.cfg.Running,
		Providers:      []updater.Provider{prov},
		Platform:       c.cfg.OS,
		Arch:           c.cfg.Arch,
		Window:         updater.WindowNone,
		CheckInterval:  0, // the app's own timer (Run) drives checks
	}); err != nil {
		return fail(err)
	}
	s.u = u
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	rel, err := u.Check(cctx)
	if err != nil {
		if r := prov.Refusal(); r != nil {
			err = r
		}
		return fail(err)
	}
	if rel == nil {
		c.mu.Lock()
		c.refused = nil
		c.mu.Unlock()
		c.set(State{Phase: PhaseUpToDate, CheckedAt: now})
		return c.State(), ErrUpToDate
	}
	if rel.Verification == nil || len(rel.Verification.Digest) != 32 || len(rel.Verification.Signature) != 0 {
		return fail(refuseAs(CodeDigest, "release has no pinned digest from a signed manifest"))
	}
	digest := hex.EncodeToString(rel.Verification.Digest)
	c.mu.Lock()
	if r := c.refused; r != nil {
		if r.version == rel.Version && r.digest == digest {
			// The gate already refused this exact release: keep saying so instead of inviting the same download.
			st := r.state
			st.CheckedAt = now
			err := r.err
			c.mu.Unlock()
			c.cfg.Logf("update check: release %s was refused earlier and stays refused (%s)", rel.Version, st.Code)
			c.set(st)
			return st, err
		}
		c.refused = nil // a different release appeared
	}
	s.rel = rel
	c.session = s
	c.mu.Unlock()
	st := State{Phase: PhaseAvailable, Version: rel.Version, Notes: rel.Notes, Total: rel.Artifact.Size, CheckedAt: now}
	c.set(st)
	return st, nil
}

// Download fetches the release found by Check, checks the digest while streaming, extracts it and runs the bundle
// gate on the staged copy. On any error the staging folder is removed and the installed app was never touched.
func (c *Controller) Download(ctx context.Context) error {
	if !c.begin() {
		return errBusy
	}
	defer c.end()
	c.mu.Lock()
	s := c.session
	pending := s != nil && s.handedOff
	c.mu.Unlock()
	if pending {
		return errRestartPending
	}
	if s == nil || s.rel == nil {
		return errors.New("no update has been found; check first")
	}
	now := c.State().CheckedAt
	fail := func(step string, err error) error {
		if r := s.prov.Refusal(); r != nil {
			err = r
		}
		if isDigestMismatch(err) && !IsRefusal(err) {
			err = refuseAs(CodeDigest, "%v", err)
		}
		st := State{Phase: PhaseError, Error: err.Error(), Code: CodeNetwork, Step: step, CheckedAt: now}
		if IsRefusal(err) {
			st.Phase, st.Code = PhaseRefused, RefusalCode(err)
		}
		c.cfg.Logf("update %s: %s (%s): %s", step, st.Phase, st.Code, err)
		c.discardSession()
		c.rememberRefused(s, st, err)
		c.set(st)
		return err
	}
	c.set(State{Phase: PhaseDownloading, Version: s.rel.Version, Notes: s.rel.Notes, Total: s.rel.Artifact.Size, CheckedAt: now})
	dctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	c.mu.Lock()
	c.dlCancel, c.dlDone = cancel, done
	c.mu.Unlock()
	err := s.u.DownloadAndInstall(dctx)
	cancel()
	c.mu.Lock()
	c.dlCancel, c.dlDone = nil, nil
	c.mu.Unlock()
	close(done)
	if err != nil {
		return fail(StepDownload, err)
	}
	staged := s.u.DownloadedPath()
	s.staged = staged
	if err := writeStagedRecord(c.cfg.StagedRecord, staged); err != nil {
		// Without the record an abandoned staging folder could not be found at the next start; the update is fine.
		c.cfg.Logf("update download: cannot record the staging folder: %v", err)
	}
	c.set(State{Phase: PhaseVerifying, Version: s.rel.Version, Notes: s.rel.Notes, Step: StepVerify, CheckedAt: now})
	if err := VerifyBundle(staged, s.rel.Version, c.cfg.Gate); err != nil {
		c.removeStaged(s)
		return fail(StepVerify, fmt.Errorf("update refused: %w", err))
	}
	c.set(State{Phase: PhaseReady, Version: s.rel.Version, Notes: s.rel.Notes, CheckedAt: now})
	return nil
}

// Prepare is Check followed by Download; it returns ErrUpToDate when there is nothing newer.
func (c *Controller) Prepare(ctx context.Context) error {
	if _, err := c.Check(ctx); err != nil {
		return err
	}
	return c.Download(ctx)
}

// Restart re-runs the gate (the staged copy is on disk and user-writable) and only then starts the Wails helper and
// asks the host to quit. Quitting is the host's decision: the caller decides "now" or "when no task is running".
func (c *Controller) Restart(ctx context.Context) error {
	if !c.begin() {
		return errBusy
	}
	defer c.end()
	c.mu.Lock()
	s := c.session
	pending := s != nil && s.handedOff
	c.mu.Unlock()
	if pending {
		return errRestartPending
	}
	if s == nil || s.staged == "" {
		return errors.New("no verified update is staged")
	}
	now := c.State().CheckedAt
	if err := VerifyBundle(s.staged, s.rel.Version, c.cfg.Gate); err != nil {
		err = fmt.Errorf("update refused at restart: %w", err)
		c.discardSession()
		st := State{Phase: PhaseRefused, Error: err.Error(), Code: RefusalCode(err), Step: StepRestart, CheckedAt: now}
		c.rememberRefused(s, st, err)
		c.set(st)
		return err
	}
	if err := RecordRestart(c.cfg.HelperLog, os.Getpid()); err != nil {
		// Without the record the helper log could not be removed later; the update itself is not affected.
		c.cfg.Logf("update restart: cannot record the helper log: %v", err)
	}
	c.set(State{Phase: PhaseRestarting, Version: s.rel.Version, Step: StepRestart, CheckedAt: now})
	// The helper owns the staged copy from the moment Restart starts it, and Wails' Restart quits the host before it
	// returns: the shutdown hook (Close) can run, and the process can exit, before this goroutine resumes. So the
	// hand-off is recorded first and undone if Restart fails.
	c.mu.Lock()
	s.handedOff = true
	c.mu.Unlock()
	clearStagedRecord(c.cfg.StagedRecord)
	if err := s.u.Restart(ctx); err != nil {
		c.mu.Lock()
		s.handedOff = false
		c.mu.Unlock()
		if werr := writeStagedRecord(c.cfg.StagedRecord, s.staged); werr != nil {
			c.cfg.Logf("update restart: cannot record the staging folder: %v", werr)
		}
		c.set(State{Phase: PhaseError, Error: err.Error(), Code: CodeRestartFailed, Step: StepRestart, Version: s.rel.Version, CheckedAt: now})
		return err
	}
	c.watchRestart(s, now)
	return nil
}

// watchRestart: the helper is waiting for this process to exit and gives up after 30 s, relaunching the old copy.
// If the host did not quit by then, say so instead of showing "restarting" for ever.
func (c *Controller) watchRestart(s *session, now time.Time) {
	time.AfterFunc(c.cfg.RestartWait, func() {
		c.mu.Lock()
		stuck := c.session == s && c.state.Phase == PhaseRestarting
		c.mu.Unlock()
		if !stuck {
			return
		}
		c.cfg.Logf("update restart: the app did not quit within %v", c.cfg.RestartWait)
		c.mu.Lock()
		s.handedOff = false // the helper has given up after its 30 s: the folder is ours again
		c.mu.Unlock()
		c.discardSession()
		c.set(State{Phase: PhaseError, Error: "the app did not quit; the update was not installed", Code: CodeRestartTimeout, Step: StepRestart, Version: s.rel.Version, CheckedAt: now})
	})
}

// discardSession drops a staged update from an earlier check and removes its staging folder. A session that was
// handed to the helper is left alone: only watchRestart, after RestartWait, may drop it.
func (c *Controller) discardSession() {
	c.mu.Lock()
	s := c.session
	if s != nil && s.handedOff {
		c.mu.Unlock()
		return
	}
	c.session = nil
	c.mu.Unlock()
	if s != nil {
		c.removeStaged(s)
	}
}

// removeStaged removes the session's staging folder and the record of it.
func (c *Controller) removeStaged(s *session) {
	if s.staged != "" {
		removeStaging(s.staged, filepath.Dir(s.target))
	}
	clearStagedRecord(c.cfg.StagedRecord)
}

// Close cancels a download that is still running (waiting a few seconds for Wails to remove its own staging folder)
// and removes the staged copy of an update that was downloaded but never restarted into. The app's shutdown hook
// calls it. Once Restart has started the helper (even while the host is still quitting inside Restart) the helper
// owns the folder and Close leaves it.
func (c *Controller) Close() {
	c.mu.Lock()
	cancel, done, wait := c.dlCancel, c.dlDone, c.closeWait
	c.mu.Unlock()
	if cancel != nil {
		cancel()
		select {
		case <-done:
		case <-time.After(wait):
		}
	}
	c.discardSession()
}

// rememberRefused keeps a refused release so a later check does not offer it again.
func (c *Controller) rememberRefused(s *session, st State, err error) {
	if st.Phase != PhaseRefused || s == nil || s.rel == nil || s.rel.Verification == nil {
		return
	}
	c.mu.Lock()
	c.refused = &refusedRelease{version: s.rel.Version, digest: hex.EncodeToString(s.rel.Verification.Digest), state: st, err: err}
	c.mu.Unlock()
}

// Retry forgets a refused release and checks again. It is what the UI's Retry button calls.
func (c *Controller) Retry(ctx context.Context) (State, error) {
	c.mu.Lock()
	c.refused = nil
	c.mu.Unlock()
	return c.Check(ctx)
}

func isDigestMismatch(err error) bool {
	return err != nil && strings.Contains(err.Error(), "digest mismatch")
}

// Run checks once after firstDelay and then every interval, on the app's own timer. A check that is refused or
// fails does not stop the loop: the next tick starts again from a fresh Updater. It returns when ctx ends.
// Nothing is downloaded by the loop; the UI asks for that.
func (c *Controller) Run(ctx context.Context, firstDelay, interval time.Duration) {
	if gone := CleanHelperLogs(c.cfg.HelperLog); len(gone) > 0 {
		c.cfg.Logf("removed old update helper logs: %s", strings.Join(gone, ", "))
	}
	c.removeAbandonedStaging()
	t := time.NewTimer(firstDelay)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if c.cfg.AutoCheck != nil && !c.cfg.AutoCheck() {
			t.Reset(interval)
			continue
		}
		switch c.State().Phase {
		case PhaseDownloading, PhaseVerifying, PhaseReady, PhaseRestarting:
			// a flow is in progress or waiting for Restart: do not replace it
		default:
			_, _ = c.Check(ctx)
		}
		t.Reset(interval)
	}
}

// removeAbandonedStaging removes the staging folder an earlier run recorded and never installed: exactly the
// recorded path, only if it is a "wails-update-*" folder in the folder that holds the running app.
func (c *Controller) removeAbandonedStaging() {
	staged := readStagedRecord(c.cfg.StagedRecord)
	if staged == "" {
		return
	}
	defer clearStagedRecord(c.cfg.StagedRecord)
	exe, err := c.cfg.Executable()
	if err != nil {
		return
	}
	installDir := filepath.Dir(installTarget(exe, c.cfg.OS))
	dir := filepath.Dir(staged)
	if !strings.HasPrefix(filepath.Base(dir), "wails-update-") || !sameDir(filepath.Dir(dir), installDir) {
		c.cfg.Logf("update: the recorded staging folder is not beside this app; left alone")
		return
	}
	if _, err := os.Stat(dir); err != nil {
		return
	}
	c.cfg.Logf("removing the staged update an earlier run left behind: %s", dir)
	removeStaging(staged, installDir)
}

func writeStagedRecord(path, staged string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(staged), 0o600)
}

func readStagedRecord(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func clearStagedRecord(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}
