package transfer

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/awsconfig"
	"github.com/rudian/codepass/internal/backup"
	"github.com/rudian/codepass/internal/ini"
	"github.com/rudian/codepass/internal/paths"
	"github.com/rudian/codepass/internal/version"
)

// Store is the part of the settings the transfer reads and writes (implemented by internal/settings).
type Store interface {
	AccountColors() map[string]int
	SetAccountColors(colors map[string]int) error
	Monograms() map[string]string
	SetMonogram(profile, monogram string) error
	AccountNames() map[string]string
	MergeAccountNames(names map[string]string) error
	SessionRecord(portal string) (api.SessionRecord, bool)
	SetSessionRecord(portal string, r api.SessionRecord) error
}

// Deps wires the Service.
type Deps struct {
	Paths  paths.Paths
	Backup *backup.Store
	Store  Store
	// State returns the accounts state (colours, account names, session lengths, sign-in state).
	State func(ctx context.Context) (api.AccountsState, error)
	// Changed is called after a successful import so the UI refreshes.
	Changed func(ctx context.Context)
	// WriteLock runs fn while no other config write is in progress (nil = no extra locking).
	WriteLock func(fn func())
	Now       func() time.Time
	OnWrite   func(path string, content []byte) // informs the file watcher of the app's own writes
}

// Service exports and imports the account configuration.
type Service struct{ d Deps }

func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.OnWrite == nil {
		d.OnWrite = func(string, []byte) {}
	}
	if d.WriteLock == nil {
		var mu chan struct{} = make(chan struct{}, 1)
		d.WriteLock = func(fn func()) { mu <- struct{}{}; defer func() { <-mu }(); fn() }
	}
	if d.Changed == nil {
		d.Changed = func(context.Context) {}
	}
	return &Service{d: d}
}

func (s *Service) files() awsconfig.Files {
	return awsconfig.Files{Config: s.d.Paths.AWSConfigFile, Credentials: s.d.Paths.AWSCredentialsFile}
}

func (s *Service) display(p string) string { return s.d.Paths.Display(p) }

func invalid(field string) *api.AppError {
	return api.NewError(api.ErrInvalidInput, "invalid input").WithParam("field", field)
}

// ---- export ----------------------------------------------------------------------------------------------

// BuildExport renders the JSON document for the requested portals and profiles and lists what was left out.
func (s *Service) BuildExport(ctx context.Context, req api.ExportRequest) (api.ExportResult, error) {
	snap, err := awsconfig.Load(s.files())
	if err != nil {
		return api.ExportResult{}, api.AsAppError(err)
	}
	st, err := s.d.State(ctx)
	if err != nil {
		return api.ExportResult{}, api.AsAppError(err)
	}
	wantPortal := toSet(req.Portals)
	wantProfile := toSet(req.Profiles)

	sessions := map[string]awsconfig.Session{}
	for _, ses := range snap.Sessions() {
		sessions[ses.Name] = ses
	}
	minutes := map[string]int{}
	for _, p := range st.Portals {
		if m := p.Session.SessionMinutes; m > 0 {
			minutes[p.Name] = m
		}
	}
	colors := map[string]int{}
	acctNames := map[string]string{}
	for _, p := range st.Profiles {
		colors[p.Name] = p.ColorSlot
		if p.AccountName != "" {
			acctNames[p.Name] = p.AccountName
		}
	}
	monograms := s.d.Store.Monograms()

	for n := range wantPortal {
		if _, ok := sessions[n]; !ok {
			return api.ExportResult{}, api.NewError(api.ErrPortalNotFound, "portal not found").WithParam("portal", n)
		}
	}
	profiles := map[string]awsconfig.Profile{}
	for _, p := range snap.Profiles() {
		profiles[p.Name] = p
	}
	for n := range wantProfile {
		if _, ok := profiles[n]; !ok {
			return api.ExportResult{}, api.NewError(api.ErrProfileNotFound, "profile not found").WithParam("name", n)
		}
	}

	doc := fileDoc{
		Format: FormatName, Version: FormatVersion,
		ExportedAt: s.d.Now().UTC().Format(time.RFC3339),
		ExportedBy: version.ProductName + " " + version.Version,
		Portals:    make([]portalDoc, 0), Profiles: make([]profileDoc, 0),
	}
	res := api.ExportResult{Items: make([]api.ExportItem, 0)}

	names := make([]string, 0, len(sessions))
	for n := range sessions {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		ses := sessions[n]
		inc := wantPortal[n]
		res.Items = append(res.Items, api.ExportItem{Kind: api.ExportKindPortal, Name: n, Included: inc})
		if !inc {
			continue
		}
		pd := portalDoc{Name: n, StartURL: ses.StartURL, Region: ses.Region}
		if ses.ScopesSet && len(ses.Scopes) > 0 {
			pd.Scopes = append([]string(nil), ses.Scopes...)
		}
		if req.IncludeExtras {
			pd.SessionMinutes = minutes[n]
		}
		doc.Portals = append(doc.Portals, pd)
	}

	pnames := make([]string, 0, len(profiles))
	for n := range profiles {
		pnames = append(pnames, n)
	}
	sort.Strings(pnames)
	for _, n := range pnames {
		p := profiles[n]
		item := api.ExportItem{Kind: api.ExportKindProfile, Name: n, Portal: p.SSOSession}
		switch p.Kind {
		case api.ProfileKindSSO:
			_, hasPortal := sessions[p.SSOSession]
			switch {
			case !hasPortal || !wantPortal[p.SSOSession]:
				if wantProfile[n] || !hasPortal {
					item.ReasonKey = "export.noPortal"
				}
			default:
				item.Included = wantProfile[n]
			}
		case api.ProfileKindLegacySSO:
			item.ReasonKey = "export.legacy"
		case api.ProfileKindStatic:
			item.ReasonKey = "export.static"
		default:
			continue
		}
		res.Items = append(res.Items, item)
		if !item.Included {
			continue
		}
		pd := profileDoc{Name: n, Portal: p.SSOSession, AccountID: p.AccountID, RoleName: p.Role, Region: p.Region, Output: p.Output}
		if req.IncludeExtras {
			pd.AccountName = acctNames[n]
			pd.Color = colors[n]
			pd.Monogram = monograms[n]
		}
		doc.Profiles = append(doc.Profiles, pd)
	}
	out, merr := marshalDoc(doc)
	if merr != nil {
		return api.ExportResult{}, api.NewError(api.ErrInternal, "cannot render the export").WithDetail(merr.Error())
	}
	res.JSON = out
	return res, nil
}

// WriteExport writes an export to the path the user chose in the save dialog.
func (s *Service) WriteExport(path string, res api.ExportResult) (api.ExportResult, error) {
	if path == "" || !filepath.IsAbs(path) {
		return res, invalid("path")
	}
	if err := backup.WriteFileAtomic(path, []byte(res.JSON), 0o600); err != nil {
		return res, api.NewError(api.ErrFileIO, "cannot write the file").WithParam("file", s.display(path)).WithDetail(err.Error())
	}
	res.Path = path
	return res, nil
}

// HasIncluded reports whether an export contains at least one portal or profile.
func HasIncluded(res api.ExportResult) bool {
	for _, it := range res.Items {
		if it.Included {
			return true
		}
	}
	return false
}

func toSet(l []string) map[string]bool {
	m := make(map[string]bool, len(l))
	for _, s := range l {
		m[s] = true
	}
	return m
}

// ---- import: reading a file --------------------------------------------------------------------------------

// ReadImportFile reads a file the user picked in the open dialog (at most MaxFileBytes, regular files only).
func (s *Service) ReadImportFile(path string) (api.ImportSource, error) {
	f, err := os.Open(path)
	if err != nil {
		return api.ImportSource{}, api.NewError(api.ErrFileIO, "cannot open the file").WithParam("file", s.display(path))
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return api.ImportSource{}, api.NewError(api.ErrFileIO, "not a regular file").WithParam("file", s.display(path))
	}
	if fi.Size() > MaxFileBytes {
		return api.ImportSource{}, formatErr("size")
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return api.ImportSource{}, api.NewError(api.ErrFileIO, "cannot read the file").WithParam("file", s.display(path))
	}
	if len(b) > MaxFileBytes {
		return api.ImportSource{}, formatErr("size")
	}
	if !utf8.Valid(b) {
		return api.ImportSource{}, formatErr("not-json")
	}
	return api.ImportSource{Name: filepath.Base(path), JSON: string(b)}, nil
}

// ---- import: planning ----------------------------------------------------------------------------------------

type portalRes struct {
	ok      bool
	target  string
	region  string
	differs bool // the name exists with other settings and the entry was skipped
}

type extraOps struct {
	colors    map[string]int    // account id -> slot
	names     map[string]string // account id -> name
	monograms map[string]string // profile -> monogram
	minutes   map[string]int    // portal -> session minutes
}

func (x *extraOps) count() int {
	return len(x.colors) + len(x.names) + len(x.monograms) + len(x.minutes)
}

type plan struct {
	pending  []pendingExtra
	snap     *awsconfig.Snapshot
	items    []api.ImportItem
	after    []byte
	extras   extraOps
	source   string
	changed  int // entries that change the config file
	needSign map[string]bool
}

type edit func(d *ini.Doc)

func key(kind api.ImportItemKind, name string) string { return string(kind) + ":" + name }

func scopesOf(s awsconfig.Session) []string {
	if !s.ScopesSet || len(s.Scopes) == 0 {
		return []string{"sso:account:access"}
	}
	return s.Scopes
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

func normURL(u string) string {
	if n, _, err := awsconfig.NormalizeStartURL(u); err == nil {
		return n
	}
	return strings.TrimSpace(u)
}

// uniqueName returns base-2, base-3, ... that is not taken.
func uniqueName(base string, taken map[string]bool, max int) string {
	for i := 2; i < 1000; i++ {
		suffix := "-" + strconv.Itoa(i)
		cand := base
		if len(cand)+len(suffix) > max {
			cand = cand[:max-len(suffix)]
		}
		cand += suffix
		if !taken[cand] {
			return cand
		}
	}
	return base + "-x"
}

func validChoice(c string) bool {
	return c == string(api.ImportChoiceSkip) || c == string(api.ImportChoiceReplace) || c == string(api.ImportChoiceRename)
}

func (s *Service) plan(req api.ImportRequest) (*plan, error) {
	parsed, err := Parse(req.JSON)
	if err != nil {
		return nil, err
	}
	for _, c := range req.Choices {
		if !validChoice(c) {
			return nil, invalid("choices")
		}
	}
	snap, lerr := awsconfig.Load(s.files())
	if lerr != nil {
		return nil, api.AsAppError(lerr)
	}
	if snap.ReadOnly() {
		return nil, api.NewError(api.ErrReadOnly, "the config file has a parse error")
	}

	sessions := map[string]awsconfig.Session{}
	takenPortal := map[string]bool{}
	for _, ses := range snap.Sessions() {
		sessions[ses.Name] = ses
		takenPortal[ses.Name] = true
	}
	profiles := map[string]awsconfig.Profile{}
	takenProfile := map[string]bool{}
	for _, p := range snap.Profiles() {
		profiles[p.Name] = p
		takenProfile[p.Name] = true
	}
	for _, e := range parsed.Portals {
		if e.Rej == nil {
			takenPortal[e.Name] = true
		}
	}
	for _, e := range parsed.Profiles {
		if e.Rej == nil {
			takenProfile[e.Name] = true
		}
	}

	pl := &plan{snap: snap, source: parsed.ExportedBy, needSign: map[string]bool{}}
	pl.extras = extraOps{colors: map[string]int{}, names: map[string]string{}, monograms: map[string]string{}, minutes: map[string]int{}}
	var edits []edit
	include := func(k string) bool {
		v, ok := req.Include[k]
		return !ok || v
	}
	usedRenamesP := map[string]bool{}
	usedRenamesQ := map[string]bool{}

	// ---- portals
	res := map[string]portalRes{} // by name in the file
	rejectedPortal := map[string]bool{}
	for _, e := range parsed.Portals {
		k := key(api.ImportKindPortal, e.Name)
		it := api.ImportItem{Key: k, Kind: api.ImportKindPortal, Name: e.Name, Target: e.Name, Warnings: nonNilStrings(e.Warnings), Include: true}
		if e.Rej != nil {
			rejectedPortal[e.Name] = true
			rejectItem(&it, e.Rej)
			pl.items = append(pl.items, it)
			continue
		}
		it.StartURL, it.Region = e.StartURL, e.Region
		it.Include = include(k)
		scopes := e.Scopes
		if len(scopes) == 0 {
			scopes = []string{"sso:account:access"}
		}
		want := awsconfig.Session{Name: e.Name, StartURL: e.StartURL, Region: e.Region, Scopes: scopes, ScopesSet: true}
		ex, has := sessions[e.Name]
		switch {
		case has && normURL(ex.StartURL) == e.StartURL && ex.Region == e.Region && sameStrings(scopesOf(ex), scopes):
			it.Action = api.ImportActionSame
			res[e.Name] = portalRes{ok: true, target: e.Name, region: ex.Region}
			s.portalExtras(pl, it.Include, e, e.Name, false)
		case has:
			it.Action = api.ImportActionConflict
			it.CanReplace = true
			it.Suggested = uniqueName(e.Name, takenPortal, 64)
			choice := effectiveChoice(&it, req, k)
			if !it.Include {
				choice = string(api.ImportChoiceSkip)
				it.Choice = choice
			}
			switch choice {
			case string(api.ImportChoiceReplace):
				edits = append(edits, replaceSession(want))
				res[e.Name] = portalRes{ok: true, target: e.Name, region: e.Region}
				pl.changed++
				pl.needSign[e.Name] = true
				s.portalExtras(pl, it.Include, e, e.Name, true)
			case string(api.ImportChoiceRename):
				nn, nerr := s.renameTarget(req, k, it.Suggested, awsconfig.SessionNameRE.MatchString, takenPortal, usedRenamesP, api.ErrPortalExists, "portal")
				if nerr != nil {
					return nil, nerr
				}
				it.Target = nn
				renamed := want
				renamed.Name = nn
				edits = append(edits, addSession(renamed))
				res[e.Name] = portalRes{ok: true, target: nn, region: e.Region}
				pl.changed++
				pl.needSign[nn] = true
				s.portalExtras(pl, it.Include, e, nn, false)
			default:
				res[e.Name] = portalRes{ok: true, target: e.Name, region: ex.Region, differs: true}
			}
		default:
			if m, found := findByURL(sessions, e.StartURL, e.Region); found {
				it.Action = api.ImportActionMapped
				it.Target = m.Name
				res[e.Name] = portalRes{ok: true, target: m.Name, region: m.Region}
				s.portalExtras(pl, it.Include, e, m.Name, false)
				break
			}
			it.Action = api.ImportActionNew
			if it.Include {
				edits = append(edits, addSession(want))
				res[e.Name] = portalRes{ok: true, target: e.Name, region: e.Region}
				pl.changed++
				pl.needSign[e.Name] = true
				s.portalExtras(pl, it.Include, e, e.Name, false)
			}
		}
		pl.items = append(pl.items, it)
	}

	// ---- profiles
	for _, e := range parsed.Profiles {
		k := key(api.ImportKindProfile, e.Name)
		it := api.ImportItem{Key: k, Kind: api.ImportKindProfile, Name: e.Name, Target: e.Name, Warnings: nonNilStrings(e.Warnings), Include: true}
		if e.Rej != nil {
			rejectItem(&it, e.Rej)
			pl.items = append(pl.items, it)
			continue
		}
		it.Portal, it.AccountID, it.Role, it.Region = e.Portal, e.AccountID, e.Role, e.Region
		it.Include = include(k)
		pr, ok := res[e.Portal]
		if !ok && !rejectedPortal[e.Portal] {
			if ses, has := sessions[e.Portal]; has {
				pr, ok = portalRes{ok: true, target: ses.Name, region: ses.Region}, true
			}
		}
		if !ok || !pr.ok {
			it.Action = api.ImportActionRejected
			it.Include = false
			it.ReasonKey = reasonNoPortal
			it.Detail = e.Portal
			pl.items = append(pl.items, it)
			continue
		}
		if pr.differs {
			it.Warnings = append(it.Warnings, "portal-differs:"+e.Portal)
		}
		it.Portal = pr.target
		ex, has := profiles[e.Name]
		same := has && ex.Kind == api.ProfileKindSSO && ex.InConfig && ex.SSOSession == pr.target && ex.AccountID == e.AccountID &&
			ex.Role == e.Role && (e.Region == "" || ex.Region == e.Region) && (e.Output == "" || ex.Output == e.Output)
		switch {
		case same:
			it.Action = api.ImportActionSame
			s.profileExtras(pl, it.Include, e, e.Name, false)
		case has:
			it.Action = api.ImportActionConflict
			it.CanReplace = ex.Kind == api.ProfileKindSSO && ex.InConfig
			it.Suggested = uniqueName(e.Name, takenProfile, 64)
			choice := effectiveChoice(&it, req, k)
			if !it.Include {
				choice = string(api.ImportChoiceSkip)
				it.Choice = choice
			}
			if choice == string(api.ImportChoiceReplace) && !it.CanReplace {
				return nil, invalid("choices")
			}
			switch choice {
			case string(api.ImportChoiceReplace):
				edits = append(edits, replaceProfile(e, pr.target))
				pl.changed++
				pl.needSign[pr.target] = true
				s.profileExtras(pl, it.Include, e, e.Name, true)
			case string(api.ImportChoiceRename):
				nn, nerr := s.renameTarget(req, k, it.Suggested, awsconfig.ProfileNameRE.MatchString, takenProfile, usedRenamesQ, api.ErrProfileExists, "name")
				if nerr != nil {
					return nil, nerr
				}
				it.Target = nn
				edits = append(edits, addProfile(e, nn, pr.target, pr.region))
				pl.changed++
				pl.needSign[pr.target] = true
				s.profileExtras(pl, it.Include, e, nn, false)
			}
		default:
			it.Action = api.ImportActionNew
			if it.Include {
				edits = append(edits, addProfile(e, e.Name, pr.target, pr.region))
				pl.changed++
				pl.needSign[pr.target] = true
				s.profileExtras(pl, it.Include, e, e.Name, false)
			}
		}
		pl.items = append(pl.items, it)
	}

	// ---- config edit
	doc, perr := parseIni(snap.ConfigBytes())
	if perr != nil {
		return nil, perr
	}
	for _, ed := range edits {
		ed(doc)
	}
	if derr := doc.Err(); derr != nil {
		return nil, awsconfig.UnsafeValueError(derr)
	}
	pl.after = doc.Bytes()
	if len(edits) == 0 {
		pl.after = snap.ConfigBytes()
	}
	if req.ImportExtras {
		pl.extras = s.filterExtras(pl)
	} else {
		pl.extras = extraOps{colors: map[string]int{}, names: map[string]string{}, monograms: map[string]string{}, minutes: map[string]int{}}
	}
	return pl, nil
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return make([]string, 0)
	}
	return append([]string(nil), s...)
}

func rejectItem(it *api.ImportItem, r *rejection) {
	it.Action = api.ImportActionRejected
	it.Include = false
	it.ReasonKey = r.reasonKey
	it.Code = r.code
	it.Detail = r.detail
}

// effectiveChoice records the caller's choice (default skip) on the item.
func effectiveChoice(it *api.ImportItem, req api.ImportRequest, k string) string {
	c, ok := req.Choices[k]
	if !ok || c == "" {
		c = string(api.ImportChoiceSkip)
	}
	it.Choice = c
	return c
}

func (s *Service) renameTarget(req api.ImportRequest, k, suggested string, valid func(string) bool, taken, used map[string]bool, existsCode api.ErrorCode, param string) (string, error) {
	nn := strings.TrimSpace(req.Renames[k])
	if nn == "" {
		nn = suggested
	}
	if !valid(nn) {
		return "", api.NewError(api.ErrInvalidName, "invalid name").WithParam("name", nn)
	}
	if taken[nn] || used[nn] {
		return "", api.NewError(existsCode, "name already in use").WithParam(param, nn)
	}
	used[nn] = true
	return nn, nil
}

func findByURL(sessions map[string]awsconfig.Session, url, region string) (awsconfig.Session, bool) {
	names := make([]string, 0, len(sessions))
	for n := range sessions {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if ses := sessions[n]; normURL(ses.StartURL) == url && ses.Region == region {
			return ses, true
		}
	}
	return awsconfig.Session{}, false
}

// ---- extras (colours, names, monograms, session lengths) --------------------------------------------------------
//
// Candidates are collected while planning (pendingPortal / pendingProfile) and filtered against the current
// settings in filterExtras, so unchanged values are not counted and existing user choices survive.

type pendingExtra struct {
	portal   string // portal target name; minutes > 0
	minutes  int
	account  string
	color    int
	acctName string
	profile  string
	mono     string
	replace  bool
}

func (s *Service) portalExtras(pl *plan, inc bool, e portalEntry, target string, replace bool) {
	if inc && e.SessionMinutes > 0 {
		pl.pending = append(pl.pending, pendingExtra{portal: target, minutes: e.SessionMinutes, replace: replace})
	}
}

func (s *Service) profileExtras(pl *plan, inc bool, e profileEntry, target string, replace bool) {
	if inc && (e.Color > 0 || e.AccountName != "" || e.Monogram != "") {
		pl.pending = append(pl.pending, pendingExtra{account: e.AccountID, color: e.Color, acctName: e.AccountName, profile: target, mono: e.Monogram, replace: replace})
	}
}

func (s *Service) filterExtras(pl *plan) extraOps {
	out := extraOps{colors: map[string]int{}, names: map[string]string{}, monograms: map[string]string{}, minutes: map[string]int{}}
	colors, names, monos := s.d.Store.AccountColors(), s.d.Store.AccountNames(), s.d.Store.Monograms()
	for _, p := range pl.pending {
		if p.minutes > 0 {
			rec, has := s.d.Store.SessionRecord(p.portal)
			cur := rec.SessionMinutes
			eff := cur
			if eff == 0 {
				eff = defaultSessionMinutes
			}
			if _, done := out.minutes[p.portal]; !done && eff != p.minutes && ((!has || cur == 0) || p.replace) {
				out.minutes[p.portal] = p.minutes
			}
			continue
		}
		if p.color > 0 {
			cur, has := colors[p.account]
			if _, done := out.colors[p.account]; !done && cur != p.color && (!has || p.replace) {
				out.colors[p.account] = p.color
			}
		}
		if p.acctName != "" {
			cur, has := names[p.account]
			if _, done := out.names[p.account]; !done && cur != p.acctName && (!has || cur == "" || p.replace) {
				out.names[p.account] = p.acctName
			}
		}
		if p.mono != "" {
			m := strings.ToUpper(p.mono)
			cur := monos[p.profile]
			if _, done := out.monograms[p.profile]; !done && cur != m && (cur == "" || p.replace) {
				out.monograms[p.profile] = m
			}
		}
	}
	return out
}

// ---- preview and apply -------------------------------------------------------------------------------------------

// PreviewImport validates the document and shows what importing it would do. It never writes anything.
func (s *Service) PreviewImport(req api.ImportRequest) (api.ImportPreview, error) {
	pl, err := s.plan(req)
	if err != nil {
		return api.ImportPreview{}, err
	}
	prev := awsconfig.Preview(s.d.Paths.AWSConfigFile, s.display(s.d.Paths.AWSConfigFile), s.display(s.d.Backup.Dir()), pl.snap.ConfigBytes(), pl.after)
	if pl.items == nil {
		pl.items = make([]api.ImportItem, 0)
	}
	return api.ImportPreview{Items: pl.items, Preview: prev, BaseHash: prev.BaseHash, ExtrasCount: pl.extras.count(), Source: pl.source, ToWrite: pl.changed}, nil
}

// ApplyImport writes the planned changes: one backup and one atomic write of ~/.aws/config guarded by baseHash,
// then the settings extras. ~/.aws/credentials is never touched.
func (s *Service) ApplyImport(ctx context.Context, req api.ImportRequest, baseHash string) (api.ImportResult, error) {
	var res api.ImportResult
	var rerr error
	s.d.WriteLock(func() { res, rerr = s.apply(ctx, req, baseHash) })
	if rerr != nil {
		return api.ImportResult{NeedSignIn: make([]string, 0)}, rerr
	}
	s.d.Changed(ctx)
	return res, nil
}

func (s *Service) apply(ctx context.Context, req api.ImportRequest, baseHash string) (api.ImportResult, error) {
	pl, err := s.plan(req)
	if err != nil {
		return api.ImportResult{}, err
	}
	res := api.ImportResult{NeedSignIn: make([]string, 0)}
	for _, it := range pl.items {
		switch {
		case it.Action == api.ImportActionRejected:
			res.Rejected++
		case it.Action == api.ImportActionNew && !it.Include:
			res.Skipped++
		case it.Action == api.ImportActionNew:
			res.Added++
		case it.Action == api.ImportActionConflict:
			switch it.Choice {
			case string(api.ImportChoiceReplace):
				res.Replaced++
			case string(api.ImportChoiceRename):
				res.Renamed++
			default:
				res.Skipped++
			}
		}
	}
	display := s.display(s.d.Paths.AWSConfigFile)
	res.Write = api.WriteResult{File: display}
	if string(pl.after) != string(pl.snap.ConfigBytes()) {
		if pl.snap.ConfigHash() != baseHash {
			return api.ImportResult{}, api.NewError(api.ErrConfigChanged, "config changed on disk").WithParam("file", display)
		}
		ent, berr := s.d.Backup.Snapshot(s.d.Paths.AWSConfigFile, display, api.BackupAWSConfig, api.ReasonImport, map[string]string{"n": strconv.Itoa(pl.changed)})
		if berr != nil {
			return api.ImportResult{}, api.AsAppError(berr)
		}
		if werr := backup.WriteFileAtomic(s.d.Paths.AWSConfigFile, pl.after, 0o600); werr != nil {
			return api.ImportResult{}, api.NewError(api.ErrFileIO, "cannot write the config file").WithParam("file", display).WithDetail(werr.Error())
		}
		s.d.OnWrite(s.d.Paths.AWSConfigFile, pl.after)
		res.Write.BackupID = ent.ID
	}
	if err := s.applyExtras(pl.extras); err != nil {
		return api.ImportResult{}, err
	}
	res.NeedSignIn = s.needSignIn(ctx, pl)
	return res, nil
}

func (s *Service) applyExtras(x extraOps) error {
	if len(x.colors) > 0 {
		cur := s.d.Store.AccountColors()
		if cur == nil {
			cur = map[string]int{}
		}
		for k, v := range x.colors {
			cur[k] = v
		}
		if err := s.d.Store.SetAccountColors(cur); err != nil {
			return api.AsAppError(err)
		}
	}
	if len(x.names) > 0 {
		if err := s.d.Store.MergeAccountNames(x.names); err != nil {
			return api.AsAppError(err)
		}
	}
	for _, p := range sortedKeys(x.monograms) {
		if err := s.d.Store.SetMonogram(p, x.monograms[p]); err != nil {
			return api.AsAppError(err)
		}
	}
	for _, p := range sortedIntKeys(x.minutes) {
		rec, _ := s.d.Store.SessionRecord(p)
		rec.SessionMinutes = x.minutes[p]
		if err := s.d.Store.SetSessionRecord(p, rec); err != nil {
			return api.AsAppError(err)
		}
	}
	return nil
}

// needSignIn lists the portals that were added or got new profiles and have no usable sign-in.
func (s *Service) needSignIn(ctx context.Context, pl *plan) []string {
	out := make([]string, 0)
	signed := map[string]bool{}
	if s.d.State != nil {
		if st, err := s.d.State(ctx); err == nil {
			for _, p := range st.Portals {
				switch p.Session.State {
				case api.SessionActive, api.SessionExpiring, api.SessionRefreshable, api.SessionSigningIn:
					signed[p.Name] = true
				}
			}
		}
	}
	for n := range pl.needSign {
		if !signed[n] {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedIntKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
