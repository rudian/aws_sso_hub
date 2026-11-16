package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/store"
	"github.com/rudian/codepass/internal/update"
	"github.com/rudian/codepass/internal/version"
)

// The automatic update check (design 5.13), in one place. The tick is short on purpose: Go's timers do not advance
// while a Mac sleeps, so a 24 h timer would fire every two or three calendar days on a laptop. Each tick asks
// autoChecker, which wants the person's preference on and (after the launch check) 24 h of wall-clock time since the
// last successful check. Checks never download.
const (
	updateFirstCheck = 30 * time.Second
	updateCheckEvery = 1 * time.Hour
	updateCheckGap   = 24 * time.Hour
)

type autoChecker struct {
	on      func() bool
	state   func() update.State
	now     func() time.Time
	started bool
}

// due is the controller's AutoCheck gate. The first call is the launch tick: it checks whenever the setting is on.
func (a *autoChecker) due() bool {
	if !a.on() {
		return false
	}
	if !a.started {
		a.started = true
		return true
	}
	s := a.state()
	if s.Phase == update.PhaseError || s.CheckedAt.IsZero() {
		return true // a failed or missing check is retried on the next tick
	}
	return a.now().Round(0).Sub(s.CheckedAt.Round(0)) >= updateCheckGap
}

// ReleasesURL is where "download in the browser instead" goes.
const ReleasesURL = "https://github.com/rudian/codepass/releases/latest"

// legacyUpdatePrefsFiles are the files the update preference lived in before it moved into settings.json
// (plan 3.1): "<data>/update/update-prefs.json", and the shorter name some early builds used.
func legacyUpdatePrefsFiles(c *Container) []string {
	dir := filepath.Join(c.Paths.AppDataDir, "update")
	return []string{filepath.Join(dir, "update-prefs.json"), filepath.Join(dir, "prefs.json")}
}

// legacyUpdatePrefsDoc is the on-disk form of that file; Off is inverted so a missing field means on.
type legacyUpdatePrefsDoc struct {
	AutoCheckOff bool `json:"autoCheckOff"`
}

var legacyUpdatePrefsSpec = store.Spec{Current: 1}

// migrateUpdatePrefs moves an old update-prefs file into settings.json once and removes it. A damaged file is
// set aside (it reads as the default, on); a file of a newer version is left alone and ignored. When
// settings.json cannot be written the old file stays, and the move is tried at the next start.
func (c *Container) migrateUpdatePrefs() {
	for _, path := range legacyUpdatePrefsFiles(c) {
		if _, err := os.Lstat(path); err != nil {
			continue
		}
		f, err := store.Open(path, legacyUpdatePrefsSpec)
		if err != nil {
			continue
		}
		var d legacyUpdatePrefsDoc
		found, err := f.Load(&d)
		switch {
		case errors.Is(err, store.ErrCorrupt):
			if _, qerr := f.Quarantine(&legacyUpdatePrefsDoc{}); qerr != nil {
				c.Log.Warn("cannot set aside the damaged update preference file", "error", qerr.Error())
			}
			continue
		case err != nil || !found:
			continue // newer version or unreadable: leave it
		}
		if d.AutoCheckOff && c.Settings.UpdateCheckOn() {
			off := true
			if _, err := c.Settings.Update(api.SettingsPatch{UpdateCheckOff: &off}); err != nil {
				c.Log.Warn("cannot move the update preference into settings.json", "error", err.Error())
				continue
			}
		}
		if err := os.Remove(path); err != nil {
			c.Log.Warn("cannot remove the old update preference file", "error", err.Error())
		}
	}
}

// toAPIUpdateState converts the controller's snapshot. The English Error text is dropped: it is for the log.
func toAPIUpdateState(s update.State) api.UpdateState {
	out := api.UpdateState{
		Phase: api.UpdatePhase(s.Phase), Current: version.Version, Version: s.Version, Notes: s.Notes,
		Written: s.Written, Total: s.Total, Code: s.Code, Step: s.Step, Block: s.Block, Folder: s.Folder,
	}
	if !s.CheckedAt.IsZero() {
		out.CheckedAt = s.CheckedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// UpdateService is the bound Wails service of the update dialog, banner and Settings card. It is a thin layer over
// update.Controller (Container.Update, built by StartUpdates): every trust decision stays in that package.
//
// Registration: application.NewService(app.NewUpdateService(c)) and
// application.RegisterEvent[api.UpdateState](EventUpdateState).
type UpdateService struct {
	c *Container
}

func NewUpdateService(c *Container) *UpdateService { return &UpdateService{c: c} }

func (s *UpdateService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

func (s *UpdateService) ServiceShutdown() error { return nil }

// work is the context of a check or download: it ends with the app, not with the one call that started it, so a
// download goes on when the dialog closes.
func (s *UpdateService) work() context.Context {
	if s.c.ctx != nil {
		return s.c.ctx
	}
	return context.Background()
}

func (s *UpdateService) ctl() (*update.Controller, error) {
	if s.c.Update == nil {
		return nil, api.NewError(api.ErrInternal, "the update check is not running")
	}
	return s.c.Update, nil
}

// State is the current snapshot.
func (s *UpdateService) State(ctx context.Context) (api.UpdateState, error) {
	ctl, err := s.ctl()
	if err != nil {
		return api.UpdateState{Phase: api.UpdateIdle, Current: version.Version}, nil
	}
	return toAPIUpdateState(ctl.State()), nil
}

// Check looks for a newer release now (the "Check now" button). Failures are part of the returned state.
func (s *UpdateService) Check(ctx context.Context) (api.UpdateState, error) {
	ctl, err := s.ctl()
	if err != nil {
		return api.UpdateState{}, err
	}
	switch ctl.State().Phase {
	case update.PhaseDownloading, update.PhaseVerifying, update.PhaseReady, update.PhaseRestarting:
		// a check would throw away the verified copy; keep the flow as it is
		return toAPIUpdateState(ctl.State()), nil
	}
	_, _ = ctl.Check(s.work()) // the outcome, including "up to date" and every refusal, is in State()
	return toAPIUpdateState(ctl.State()), nil
}

// Retry forgets a refused release and checks again.
func (s *UpdateService) Retry(ctx context.Context) (api.UpdateState, error) {
	ctl, err := s.ctl()
	if err != nil {
		return api.UpdateState{}, err
	}
	_, _ = ctl.Retry(s.work())
	return toAPIUpdateState(ctl.State()), nil
}

// Download fetches, checks and stages the release found by Check. It returns when the download has finished or
// failed; progress arrives as update:state events. The app stays usable meanwhile.
func (s *UpdateService) Download(ctx context.Context) (api.UpdateState, error) {
	ctl, err := s.ctl()
	if err != nil {
		return api.UpdateState{}, err
	}
	_ = ctl.Download(s.work())
	return toAPIUpdateState(ctl.State()), nil
}

// Restart replaces the app with the staged copy and reopens it. The controller re-runs the gate first and then asks
// the host to quit through the approved path main.go gave it (no quit dialog: the person chose Reopen).
func (s *UpdateService) Restart(ctx context.Context) (api.UpdateState, error) {
	ctl, err := s.ctl()
	if err != nil {
		return api.UpdateState{}, err
	}
	_ = ctl.Restart(s.work())
	return toAPIUpdateState(ctl.State()), nil
}

// GetPreferences returns the update preferences.
func (s *UpdateService) GetPreferences(ctx context.Context) (api.UpdatePreferences, error) {
	return api.UpdatePreferences{AutoCheck: s.c.Settings.UpdateCheckOn()}, nil
}

// SetAutoCheck turns the daily automatic check on or off. A manual check always works.
func (s *UpdateService) SetAutoCheck(ctx context.Context, on bool) (api.UpdatePreferences, error) {
	off := !on
	next, err := s.c.Settings.Update(api.SettingsPatch{UpdateCheckOff: &off})
	if err != nil {
		return api.UpdatePreferences{}, api.NewError(api.ErrInternal, "cannot save the update setting").WithDetail(err.Error())
	}
	s.c.emit(api.EventSettingsChanged, nn(next))
	return api.UpdatePreferences{AutoCheck: !next.UpdateCheckOff}, nil
}

// OpenDownloadPage opens the releases page in the browser ("download in the browser instead").
func (s *UpdateService) OpenDownloadPage(ctx context.Context) error {
	return s.c.RT.OpenURL(ReleasesURL)
}
