// Package settings persists the app's own settings file (<AppDataDir>/settings.json). It never stores
// secrets: key IDs appear only masked, and the client id of a sign-in only as a short hash.
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/backup"
	"github.com/rudian/aws_sso_hub/internal/browser"
	"github.com/rudian/aws_sso_hub/internal/execx"
	"github.com/rudian/aws_sso_hub/internal/gitx"
	"github.com/rudian/aws_sso_hub/internal/migrate"
	"github.com/rudian/aws_sso_hub/internal/redact"
)

const (
	schemaVersion         = 1
	minSessionMinutes     = 15
	maxSessionMinutes     = 129600
	maxRepoTests          = 1000
	defaultUILanguage     = "zh-CN"
	defaultLeadMinutes    = 10
	defaultBackupRetained = 20
	maxCollapsedPortals   = 500
	maxConsoleProxies     = 500
	maxLabelLen           = 200
)

// data is the JSON shape of settings.json.
type data struct {
	SchemaVersion        int                           `json:"schemaVersion"`
	Language             api.Language                  `json:"language"`
	Theme                api.Theme                     `json:"theme"`
	Motion               api.MotionPref                `json:"motion"`
	ScanRoots            []string                      `json:"scanRoots"`
	ExtraRepos           []string                      `json:"extraRepos"`
	Reminders            api.ReminderSettings          `json:"reminders"`
	Tray                 api.TraySettings              `json:"tray"`
	TerminalApp          api.TerminalApp               `json:"terminalApp"`
	BackupRetention      int                           `json:"backupRetention"`
	TutorialDone         []string                      `json:"tutorialDone"`
	TutorialLang         string                        `json:"tutorialLang"`
	AWSCLIPath           string                        `json:"awsCliPath"`
	GitPath              string                        `json:"gitPath"`
	AutoBindOnAdd        bool                          `json:"autoBindOnAdd"`
	PrivateSignIn        *bool                         `json:"privateSignIn,omitempty"` // nil (key absent) = true
	CollapsedPortals     []string                      `json:"collapsedPortals"`
	ConsoleProxies       map[string]string             `json:"consoleProxies"` // portal -> HTTP proxy (host:port) of its console
	LastCloneProfile     string                        `json:"lastCloneProfile"`
	LastCloneRegion      string                        `json:"lastCloneRegion"`
	FirstRunDone         bool                          `json:"firstRunDone"`
	UILanguage           string                        `json:"uiLanguage"`
	AccountColors        map[string]int                `json:"accountColors"`
	Monograms            map[string]string             `json:"monograms"`
	AccountNames         map[string]string             `json:"accountNames"`
	StaticIdentities     map[string]api.StaticIdentity `json:"staticIdentities"`
	Sessions             map[string]api.SessionRecord  `json:"sessions"`
	RepoTests            map[string]api.TestResult     `json:"repoTests"`
	PendingDeactivations []migrate.PendingDeactivation `json:"pendingDeactivations"`
}

// privateSignIn is the effective value: a settings file without the key means "on".
func (d *data) privateSignIn() bool { return d.PrivateSignIn == nil || *d.PrivateSignIn }

// Store is the settings file. All methods are safe for concurrent use.
type Store struct {
	mu   sync.Mutex
	path string
	root string // default scan root
	now  func() time.Time
	d    data

	dropped []string // tool-path settings that failed validation when the file was loaded
}

func defaults(defaultScanRoot string) data {
	return data{
		SchemaVersion:    schemaVersion,
		Language:         api.LanguageSystem,
		Theme:            api.ThemeSystem,
		Motion:           api.MotionSystem,
		ScanRoots:        []string{defaultScanRoot},
		ExtraRepos:       []string{},
		Reminders:        api.ReminderSettings{AutoSignIn: true, LeadMinutes: defaultLeadMinutes, AutoRenew: true},
		TerminalApp:      api.TerminalAuto,
		BackupRetention:  defaultBackupRetained,
		TutorialDone:     []string{},
		CollapsedPortals: []string{},
		ConsoleProxies:   map[string]string{},
		UILanguage:       defaultUILanguage,
		AutoBindOnAdd:    true,

		AccountColors:        map[string]int{},
		Monograms:            map[string]string{},
		AccountNames:         map[string]string{},
		StaticIdentities:     map[string]api.StaticIdentity{},
		Sessions:             map[string]api.SessionRecord{},
		RepoTests:            map[string]api.TestResult{},
		PendingDeactivations: []migrate.PendingDeactivation{},
	}
}

// Open loads the settings file. firstRun is true while the first-run flow has not been completed (no file, a
// corrupt file that was moved aside, or firstRunDone == false).
func Open(path string, defaultScanRoot string, now func() time.Time) (s *Store, firstRun bool, err error) {
	if now == nil {
		now = time.Now
	}
	s = &Store{path: path, root: defaultScanRoot, now: now, d: defaults(defaultScanRoot)}
	b, rerr := os.ReadFile(path)
	switch {
	case rerr == nil:
		// Start from the defaults so that fields absent from an older file keep their default value.
		loaded := defaults(defaultScanRoot)
		if jerr := json.Unmarshal(b, &loaded); jerr != nil {
			_ = os.Rename(path, fmt.Sprintf("%s.corrupt-%d", path, now().Unix()))
			s.d = defaults(defaultScanRoot)
			return s, true, nil
		}
		s.d = loaded
		s.migrateReminders(b)
		s.normalize()
	case os.IsNotExist(rerr):
		// first run; nothing is written yet
	default:
		return nil, false, api.NewError(api.ErrFileIO, "cannot read the settings file").WithParam("file", filepath.Base(path)).WithDetail(rerr.Error())
	}
	return s, !s.d.FirstRunDone, nil
}

// migrateReminders carries the reminder switch of a file written before automatic sign-in existed over to
// it. That switch ("expiryNotice") was for a reminder that never appeared, so the lead goes to the new default.
func (s *Store) migrateReminders(b []byte) {
	var old struct {
		Reminders struct {
			ExpiryNotice *bool `json:"expiryNotice"`
			AutoSignIn   *bool `json:"autoSignIn"`
		} `json:"reminders"`
	}
	if json.Unmarshal(b, &old) != nil || old.Reminders.AutoSignIn != nil || old.Reminders.ExpiryNotice == nil {
		return
	}
	s.d.Reminders.AutoSignIn = *old.Reminders.ExpiryNotice
	s.d.Reminders.LeadMinutes = defaultLeadMinutes
}

func validLanguage(v api.Language) bool {
	return v == api.LanguageSystem || v == api.LanguageZhCN || v == api.LanguageEn
}
func validTheme(v api.Theme) bool {
	return v == api.ThemeSystem || v == api.ThemeLight || v == api.ThemeDark
}
func validMotion(v api.MotionPref) bool {
	return v == api.MotionSystem || v == api.MotionReduce || v == api.MotionFull
}
func validTerminal(v api.TerminalApp) bool {
	switch v {
	case api.TerminalAuto, api.TerminalMacTerminal, api.TerminalITerm2, api.TerminalWindowsTerminal, api.TerminalPowerShell, api.TerminalCmd:
		return true
	}
	return false
}
func validLead(n int) bool      { return n == 10 || n == 15 || n == 30 || n == 60 }
func validRetention(n int) bool { return n == 0 || n == 20 || n == 50 }
func validUILang(v string) bool { return v == "zh-CN" || v == "en" }

// normalize replaces invalid loaded values by defaults.
func (s *Store) normalize() {
	d := &s.d
	d.SchemaVersion = schemaVersion
	// A tool path edited into the file by hand is held to the same rules as one set in the app.
	if execx.ValidateToolPath("aws", d.AWSCLIPath, runtime.GOOS) != nil {
		d.AWSCLIPath = ""
		s.dropped = append(s.dropped, "awsCliPath")
	}
	if execx.ValidateToolPath("git", d.GitPath, runtime.GOOS) != nil {
		d.GitPath = ""
		s.dropped = append(s.dropped, "gitPath")
	}
	if !validLanguage(d.Language) {
		d.Language = api.LanguageSystem
	}
	if !validTheme(d.Theme) {
		d.Theme = api.ThemeSystem
	}
	if !validMotion(d.Motion) {
		d.Motion = api.MotionSystem
	}
	if !validTerminal(d.TerminalApp) {
		d.TerminalApp = api.TerminalAuto
	}
	if !validLead(d.Reminders.LeadMinutes) {
		d.Reminders.LeadMinutes = defaultLeadMinutes
	}
	if !validRetention(d.BackupRetention) {
		d.BackupRetention = defaultBackupRetained
	}
	if !validUILang(d.UILanguage) {
		d.UILanguage = defaultUILanguage
	}
	if d.TutorialLang != "" && !validUILang(d.TutorialLang) {
		d.TutorialLang = ""
	}
	roots := make([]string, 0, len(d.ScanRoots))
	for _, r := range d.ScanRoots {
		if r = strings.TrimSpace(r); r != "" && filepath.IsAbs(r) {
			roots = append(roots, filepath.Clean(r))
		}
	}
	if len(roots) == 0 {
		roots = []string{s.root}
	}
	d.ScanRoots = dedupe(roots)
	d.ExtraRepos = dedupe(cleanAbs(d.ExtraRepos))
	d.TutorialDone = dedupe(d.TutorialDone)
	portals := make([]string, 0, len(d.CollapsedPortals))
	for _, v := range d.CollapsedPortals {
		if v = strings.TrimSpace(v); v != "" {
			portals = append(portals, v)
		}
	}
	if portals = dedupe(portals); len(portals) > maxCollapsedPortals {
		portals = portals[:maxCollapsedPortals]
	}
	d.CollapsedPortals = portals
	d.ConsoleProxies = cleanConsoleProxies(d.ConsoleProxies)
	d.LastCloneProfile = strings.TrimSpace(d.LastCloneProfile)
	if !validLabel(d.LastCloneProfile, true) {
		d.LastCloneProfile = ""
	}
	d.LastCloneRegion = strings.TrimSpace(d.LastCloneRegion)
	if d.LastCloneRegion != "" && !gitx.RegionRE.MatchString(d.LastCloneRegion) {
		d.LastCloneRegion = ""
	}
	if d.AccountColors == nil {
		d.AccountColors = map[string]int{}
	}
	if d.Monograms == nil {
		d.Monograms = map[string]string{}
	}
	if d.AccountNames == nil {
		d.AccountNames = map[string]string{}
	}
	if d.StaticIdentities == nil {
		d.StaticIdentities = map[string]api.StaticIdentity{}
	}
	if d.Sessions == nil {
		d.Sessions = map[string]api.SessionRecord{}
	}
	for k, r := range d.Sessions {
		if r.SessionMinutes != 0 && (r.SessionMinutes < minSessionMinutes || r.SessionMinutes > maxSessionMinutes) {
			r.SessionMinutes = 0
		}
		if _, err := time.Parse(time.RFC3339, r.SessionEndsAt); r.SessionEndsAt != "" && err != nil {
			r.SessionEndsAt = ""
		}
		d.Sessions[k] = r
	}
	if d.RepoTests == nil {
		d.RepoTests = map[string]api.TestResult{}
	}
	if d.PendingDeactivations == nil {
		d.PendingDeactivations = []migrate.PendingDeactivation{}
	}
}

// validLabel reports whether v is at most maxLabelLen characters with no control characters; empty is
// accepted only when allowEmpty.
func validLabel(v string, allowEmpty bool) bool {
	if v == "" {
		return allowEmpty
	}
	if utf8.RuneCountInString(v) > maxLabelLen {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// cleanConsoleProxies keeps the console proxies with a valid portal name and a valid host:port (in canonical
// form), at most maxConsoleProxies of them, by portal name.
func cleanConsoleProxies(in map[string]string) map[string]string {
	names := make([]string, 0, len(in))
	for k := range in {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make(map[string]string, len(in))
	for _, k := range names {
		portal := strings.TrimSpace(k)
		proxy, ok := browser.CleanProxy(in[k])
		if !validLabel(portal, false) || !ok || len(out) == maxConsoleProxies {
			continue
		}
		out[portal] = proxy
	}
	return out
}

func cleanAbs(in []string) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		if p = strings.TrimSpace(p); p != "" && filepath.IsAbs(p) {
			out = append(out, filepath.Clean(p))
		}
	}
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// saveLocked writes the file atomically (mode 0600). Callers hold s.mu.
func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(&s.d, "", "  ")
	if err != nil {
		return api.NewError(api.ErrInternal, "cannot encode settings").WithDetail(err.Error())
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return api.NewError(api.ErrFileIO, "cannot create the settings folder").WithParam("file", filepath.Base(s.path)).WithDetail(err.Error())
	}
	if err := backup.WriteFileAtomic(s.path, append(b, '\n'), 0o600); err != nil {
		return api.NewError(api.ErrFileIO, "cannot write the settings file").WithParam("file", filepath.Base(s.path)).WithDetail(err.Error())
	}
	return nil
}

// mutate runs fn on a snapshot of the data and persists it; on failure the previous value is restored.
func (s *Store) mutate(fn func(d *data)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.clone()
	fn(&s.d)
	if err := s.saveLocked(); err != nil {
		s.d = prev
		return err
	}
	return nil
}

func (s *Store) clone() data {
	b, _ := json.Marshal(&s.d)
	var c data
	_ = json.Unmarshal(b, &c)
	st := &Store{d: c, root: s.root}
	st.normalize()
	return st.d
}

// API returns the part of the settings the frontend sees.
func (s *Store) API() api.Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.apiLocked()
}

func (s *Store) apiLocked() api.Settings {
	d := s.d
	return api.Settings{
		Language: d.Language, Theme: d.Theme, Motion: d.Motion,
		ScanRoots:       append([]string{}, d.ScanRoots...),
		Reminders:       d.Reminders,
		Tray:            d.Tray,
		TerminalApp:     d.TerminalApp,
		BackupRetention: d.BackupRetention,
		TutorialDone:    append([]string{}, d.TutorialDone...),
		TutorialLang:    d.TutorialLang,
		AWSCLIPath:      d.AWSCLIPath,
		GitPath:         d.GitPath,
		AutoBindOnAdd:   d.AutoBindOnAdd,
		PrivateSignIn:   d.privateSignIn(),

		CollapsedPortals: append([]string{}, d.CollapsedPortals...),
		ConsoleProxies:   copyStrings(d.ConsoleProxies),
		LastCloneProfile: d.LastCloneProfile,
		LastCloneRegion:  d.LastCloneRegion,
	}
}

func invalid(field string) *api.AppError {
	return api.NewError(api.ErrInvalidInput, "invalid value for %s", field).WithParam("field", field)
}

// cleanToolPath trims v and checks it as the path of the aws or git program ("" = automatic detection).
func cleanToolPath(kind, v string) (string, bool) {
	v = strings.TrimSpace(v)
	if execx.ValidateToolPath(kind, v, runtime.GOOS) != nil {
		return "", false
	}
	return v, true
}

// DroppedToolPaths names the tool-path settings ("awsCliPath", "gitPath") that were found invalid when the
// file was loaded and reset to automatic detection. The values themselves are not reported.
func (s *Store) DroppedToolPaths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.dropped...)
}

// Update applies a patch. Every field is validated before anything changes.
func (s *Store) Update(p api.SettingsPatch) (api.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.d

	if p.Language != nil {
		if !validLanguage(*p.Language) {
			return api.Settings{}, invalid("language")
		}
		next.Language = *p.Language
	}
	if p.Theme != nil {
		if !validTheme(*p.Theme) {
			return api.Settings{}, invalid("theme")
		}
		next.Theme = *p.Theme
	}
	if p.Motion != nil {
		if !validMotion(*p.Motion) {
			return api.Settings{}, invalid("motion")
		}
		next.Motion = *p.Motion
	}
	if p.ScanRoots != nil {
		roots := make([]string, 0, len(p.ScanRoots))
		for _, r := range p.ScanRoots {
			r = strings.TrimSpace(r)
			if r == "" || !filepath.IsAbs(r) || strings.ContainsAny(r, "\x00\r\n") {
				return api.Settings{}, invalid("scanRoots")
			}
			roots = append(roots, filepath.Clean(r))
		}
		if len(roots) == 0 {
			return api.Settings{}, invalid("scanRoots")
		}
		next.ScanRoots = dedupe(roots)
	}
	if p.Reminders != nil {
		if !validLead(p.Reminders.LeadMinutes) {
			return api.Settings{}, invalid("reminders.leadMinutes")
		}
		next.Reminders = *p.Reminders
	}
	if p.Tray != nil {
		next.Tray = *p.Tray
	}
	if p.TerminalApp != nil {
		if !validTerminal(*p.TerminalApp) {
			return api.Settings{}, invalid("terminalApp")
		}
		next.TerminalApp = *p.TerminalApp
	}
	if p.BackupRetention != nil {
		if !validRetention(*p.BackupRetention) {
			return api.Settings{}, invalid("backupRetention")
		}
		next.BackupRetention = *p.BackupRetention
	}
	if p.TutorialDone != nil {
		next.TutorialDone = dedupe(p.TutorialDone)
	}
	if p.TutorialLang != nil {
		if *p.TutorialLang != "" && !validUILang(*p.TutorialLang) {
			return api.Settings{}, invalid("tutorialLang")
		}
		next.TutorialLang = *p.TutorialLang
	}
	if p.AWSCLIPath != nil {
		v, ok := cleanToolPath("aws", *p.AWSCLIPath)
		if !ok {
			return api.Settings{}, invalid("awsCliPath")
		}
		next.AWSCLIPath = v
	}
	if p.GitPath != nil {
		v, ok := cleanToolPath("git", *p.GitPath)
		if !ok {
			return api.Settings{}, invalid("gitPath")
		}
		next.GitPath = v
	}

	if p.AutoBindOnAdd != nil {
		next.AutoBindOnAdd = *p.AutoBindOnAdd
	}
	if p.PrivateSignIn != nil {
		v := *p.PrivateSignIn
		next.PrivateSignIn = &v
	}
	if p.CollapsedPortals != nil {
		if len(p.CollapsedPortals) > maxCollapsedPortals {
			return api.Settings{}, invalid("collapsedPortals")
		}
		portals := make([]string, 0, len(p.CollapsedPortals))
		for _, v := range p.CollapsedPortals {
			v = strings.TrimSpace(v)
			if !validLabel(v, false) {
				return api.Settings{}, invalid("collapsedPortals")
			}
			portals = append(portals, v)
		}
		next.CollapsedPortals = dedupe(portals)
	}
	if p.ConsoleProxies != nil {
		proxies := copyStrings(s.d.ConsoleProxies)
		for k, v := range p.ConsoleProxies {
			portal := strings.TrimSpace(k)
			if !validLabel(portal, false) {
				return api.Settings{}, invalid("consoleProxies")
			}
			if strings.TrimSpace(v) == "" {
				delete(proxies, portal)
				continue
			}
			proxy, ok := browser.CleanProxy(v)
			if !ok {
				return api.Settings{}, invalid("consoleProxies")
			}
			proxies[portal] = proxy
		}
		if len(proxies) > maxConsoleProxies {
			return api.Settings{}, invalid("consoleProxies")
		}
		next.ConsoleProxies = proxies
	}
	if p.LastCloneProfile != nil {
		v := strings.TrimSpace(*p.LastCloneProfile)
		if !validLabel(v, true) {
			return api.Settings{}, invalid("lastCloneProfile")
		}
		next.LastCloneProfile = v
	}
	if p.LastCloneRegion != nil {
		v := strings.TrimSpace(*p.LastCloneRegion)
		if v != "" && !gitx.RegionRE.MatchString(v) {
			return api.Settings{}, invalid("lastCloneRegion")
		}
		next.LastCloneRegion = v
	}

	prev := s.d
	s.d = next
	if err := s.saveLocked(); err != nil {
		s.d = prev
		return api.Settings{}, err
	}
	return s.apiLocked(), nil
}

// CompleteFirstRun marks the first-run flow as done.
func (s *Store) CompleteFirstRun() error {
	return s.mutate(func(d *data) { d.FirstRunDone = true })
}

// FirstRun reports whether the first-run flow is still pending.
func (s *Store) FirstRun() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.d.FirstRunDone
}

// SetUILanguage stores the language the UI reports ("zh-CN" | "en").
func (s *Store) SetUILanguage(lang string) error {
	if !validUILang(lang) {
		return invalid("lang")
	}
	return s.mutate(func(d *data) { d.UILanguage = lang })
}

// UILanguage returns "zh-CN" or "en".
func (s *Store) UILanguage() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.d.UILanguage
}

// Lang implements repos.Store.
func (s *Store) Lang() string { return s.UILanguage() }

// Simple getters used by the container and the services.
func (s *Store) AWSCLIPath() string { s.mu.Lock(); defer s.mu.Unlock(); return s.d.AWSCLIPath }
func (s *Store) GitPath() string    { s.mu.Lock(); defer s.mu.Unlock(); return s.d.GitPath }
func (s *Store) BackupRetention() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.d.BackupRetention
}
func (s *Store) Tray() api.TraySettings { s.mu.Lock(); defer s.mu.Unlock(); return s.d.Tray }
func (s *Store) TerminalApp() api.TerminalApp {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.d.TerminalApp
}

// ---- accounts.Store ----

func copyInts(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copyStrings(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (s *Store) AccountColors() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyInts(s.d.AccountColors)
}

// SetAccountColors replaces the colour map; it is a no-op (no file write) when nothing changed.
func (s *Store) SetAccountColors(colors map[string]int) error {
	s.mu.Lock()
	same := len(colors) == len(s.d.AccountColors)
	if same {
		for k, v := range colors {
			if cur, ok := s.d.AccountColors[k]; !ok || cur != v {
				same = false
				break
			}
		}
	}
	s.mu.Unlock()
	if same {
		return nil
	}
	return s.mutate(func(d *data) { d.AccountColors = copyInts(colors) })
}

func (s *Store) Monograms() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyStrings(s.d.Monograms)
}

// SetMonogram stores a user override; "" deletes it.
func (s *Store) SetMonogram(profile, monogram string) error {
	return s.mutate(func(d *data) {
		if monogram == "" {
			delete(d.Monograms, profile)
		} else {
			d.Monograms[profile] = monogram
		}
	})
}

func (s *Store) AccountNames() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyStrings(s.d.AccountNames)
}

// MergeAccountNames adds or updates names; it writes only when something changed.
func (s *Store) MergeAccountNames(names map[string]string) error {
	s.mu.Lock()
	changed := false
	for k, v := range names {
		if cur, ok := s.d.AccountNames[k]; !ok || cur != v {
			changed = true
			break
		}
	}
	s.mu.Unlock()
	if !changed {
		return nil
	}
	return s.mutate(func(d *data) {
		for k, v := range names {
			d.AccountNames[k] = redact.String(v)
		}
	})
}

func (s *Store) StaticIdentity(profile string) (api.StaticIdentity, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.d.StaticIdentities[profile]
	return id, ok
}

// SetStaticIdentity stores an identity; every string passes through redact.String as a last defence.
func (s *Store) SetStaticIdentity(id api.StaticIdentity) error {
	id.Arn = redact.String(id.Arn)
	id.IAMUser = redact.String(id.IAMUser)
	id.MaskedKeyID = redact.String(id.MaskedKeyID)
	id.ErrorCode = redact.String(id.ErrorCode)
	return s.mutate(func(d *data) { d.StaticIdentities[id.Profile] = id })
}

func (s *Store) SessionRecord(portal string) (api.SessionRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.d.Sessions[portal]
	return r, ok
}

func (s *Store) SetSessionRecord(portal string, r api.SessionRecord) error {
	if r.SessionMinutes != 0 && (r.SessionMinutes < minSessionMinutes || r.SessionMinutes > maxSessionMinutes) {
		return invalid("sessionMinutes")
	}
	return s.mutate(func(d *data) { d.Sessions[portal] = r })
}

func (s *Store) DeleteSessionRecord(portal string) error {
	s.mu.Lock()
	_, ok := s.d.Sessions[portal]
	s.mu.Unlock()
	if !ok {
		return nil
	}
	return s.mutate(func(d *data) { delete(d.Sessions, portal) })
}

func (s *Store) AutoRenew() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.d.Reminders.AutoRenew }

// AutoSignIn reports whether the app opens the sign-in page by itself before a session ends.
func (s *Store) AutoSignIn() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.d.Reminders.AutoSignIn
}

// SignInLead is how long before a session ends the app opens the sign-in page.
func (s *Store) SignInLead() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Duration(s.d.Reminders.LeadMinutes) * time.Minute
}

// ---- migrate.Store ----

func (s *Store) AddPendingDeactivation(p migrate.PendingDeactivation) error {
	p.MaskedKeyID = redact.String(p.MaskedKeyID)
	return s.mutate(func(d *data) {
		out := make([]migrate.PendingDeactivation, 0, len(d.PendingDeactivations)+1)
		for _, e := range d.PendingDeactivations {
			if e.Profile != p.Profile {
				out = append(out, e)
			}
		}
		d.PendingDeactivations = append(out, p)
	})
}

func (s *Store) RemovePendingDeactivation(profile string) error {
	return s.mutate(func(d *data) {
		out := make([]migrate.PendingDeactivation, 0, len(d.PendingDeactivations))
		for _, e := range d.PendingDeactivations {
			if e.Profile != profile {
				out = append(out, e)
			}
		}
		d.PendingDeactivations = out
	})
}

// PendingDeactivations returns the open deactivations (for the security checkup).
func (s *Store) PendingDeactivations() []migrate.PendingDeactivation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]migrate.PendingDeactivation{}, s.d.PendingDeactivations...)
}

// ---- repos.Store ----

func (s *Store) ScanRoots() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.d.ScanRoots...)
}

func (s *Store) ExtraRepos() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.d.ExtraRepos...)
}

func (s *Store) AddExtraRepo(path string) error {
	path = filepath.Clean(path)
	s.mu.Lock()
	for _, p := range s.d.ExtraRepos {
		if p == path {
			s.mu.Unlock()
			return nil
		}
	}
	s.mu.Unlock()
	return s.mutate(func(d *data) { d.ExtraRepos = append(d.ExtraRepos, path) })
}

// RemoveExtraRepo drops a hand-added repository path from the list. Unknown paths are ignored.
func (s *Store) RemoveExtraRepo(path string) error {
	path = filepath.Clean(path)
	s.mu.Lock()
	found := false
	for _, p := range s.d.ExtraRepos {
		if p == path {
			found = true
		}
	}
	s.mu.Unlock()
	if !found {
		return nil
	}
	return s.mutate(func(d *data) {
		kept := make([]string, 0, len(d.ExtraRepos))
		for _, p := range d.ExtraRepos {
			if p != path {
				kept = append(kept, p)
			}
		}
		d.ExtraRepos = kept
	})
}

// PrivateSignIn reports whether the sign-in page opens in a private browser window (default true).
func (s *Store) PrivateSignIn() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.d.privateSignIn()
}

// ConsoleProxy returns the HTTP proxy (host:port) the console of a portal opens through; "" = none.
func (s *Store) ConsoleProxy(portal string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.d.ConsoleProxies[portal]
}

// AutoBindOnAdd reports whether a hand-added repository is bound to its detected SSO profile.
func (s *Store) AutoBindOnAdd() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.d.AutoBindOnAdd
}

func (s *Store) RepoTests() map[string]api.TestResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]api.TestResult, len(s.d.RepoTests))
	for k, v := range s.d.RepoTests {
		out[k] = v
	}
	return out
}

// SetRepoTest stores the outcome of a connection test. The stderr detail is not persisted.
func (s *Store) SetRepoTest(r api.TestResult) error {
	r.Detail = ""
	return s.mutate(func(d *data) {
		d.RepoTests[r.RepoID] = r
		if len(d.RepoTests) > maxRepoTests {
			ids := make([]string, 0, len(d.RepoTests))
			for id := range d.RepoTests {
				ids = append(ids, id)
			}
			sort.Slice(ids, func(i, j int) bool { return d.RepoTests[ids[i]].At < d.RepoTests[ids[j]].At })
			for _, id := range ids[:len(ids)-maxRepoTests] {
				delete(d.RepoTests, id)
			}
		}
	})
}
