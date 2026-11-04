package api

// Export and import of account settings (portals and SSO profiles) as JSON. The file never contains secrets.

type ExportItemKind string

const (
	ExportKindPortal  ExportItemKind = "portal"
	ExportKindProfile ExportItemKind = "profile"
)

// ExportRequest selects what to export. Names are portal (sso-session) and profile names exactly as listed in
// the accounts state; an empty selection exports nothing.
type ExportRequest struct {
	Portals       []string `json:"portals"`
	Profiles      []string `json:"profiles"`
	IncludeExtras bool     `json:"includeExtras"` // colour slot, monogram override, account name, session length
}

// ExportItem is one row of the export checklist.
type ExportItem struct {
	Kind      ExportItemKind `json:"kind"`
	Name      string         `json:"name"`
	Portal    string         `json:"portal,omitempty"` // profiles: the portal they belong to
	Included  bool           `json:"included"`
	ReasonKey string         `json:"reasonKey,omitempty"` // why it is not included: "export.static" | "export.legacy" | "export.noPortal"
}

type ExportResult struct {
	JSON  string       `json:"json"`
	Items []ExportItem `json:"items"`
	Path  string       `json:"path,omitempty"` // SaveExport: where the file was written
}

// ImportSource is a file chosen with the native open dialog.
type ImportSource struct {
	Name string `json:"name"` // file name only
	JSON string `json:"json"`
}

type ImportItemKind string

const (
	ImportKindPortal  ImportItemKind = "portal"
	ImportKindProfile ImportItemKind = "profile"
)

type ImportAction string

const (
	ImportActionNew      ImportAction = "new"
	ImportActionSame     ImportAction = "same"
	ImportActionConflict ImportAction = "conflict"
	ImportActionMapped   ImportAction = "mapped"
	ImportActionRejected ImportAction = "rejected"
)

type ImportChoice string

const (
	ImportChoiceSkip    ImportChoice = "skip"
	ImportChoiceReplace ImportChoice = "replace"
	ImportChoiceRename  ImportChoice = "rename"
)

// ImportRequest drives PreviewImport and ApplyImport. Map keys are "portal:<name>" and "profile:<name>" with the
// names used in the file. Missing Include entries mean "import"; missing Choices entries mean "skip".
type ImportRequest struct {
	JSON         string            `json:"json"`
	Include      map[string]bool   `json:"include"`
	Choices      map[string]string `json:"choices"` // skip | replace | rename (conflicts only)
	Renames      map[string]string `json:"renames"` // new name for choice rename; empty = the suggestion
	ImportExtras bool              `json:"importExtras"`
}

// ImportItem is one portal or profile of the file with what importing it would do.
//
// Warnings are short codes, optionally "code:detail": "unknown-key:<key>" (an entry key that is not part of the
// format and is ignored), "portal-differs:<portal>" (the profile uses a portal that exists under this name with
// other settings).
type ImportItem struct {
	Key        string         `json:"key"`
	Kind       ImportItemKind `json:"kind"`
	Name       string         `json:"name"` // name in the file (sanitised for rejected entries)
	Action     ImportAction   `json:"action"`
	Include    bool           `json:"include"` // effective: false for rejected entries and entries the caller unticked
	Choice     string         `json:"choice"`  // effective choice for conflicts, "" otherwise
	Target     string         `json:"target"`  // name it ends up with: the existing portal (mapped), the new name (rename), else Name
	Suggested  string         `json:"suggested"`
	CanReplace bool           `json:"canReplace"`
	ReasonKey  string         `json:"reasonKey,omitempty"` // i18n key for rejected entries: import.rejected.secret | invalid | noPortal
	Code       ErrorCode      `json:"code,omitempty"`      // rejected as invalid: the error the same input would give in the manual forms
	Detail     string         `json:"detail,omitempty"`    // field name or portal name for the reason text
	Warnings   []string       `json:"warnings"`
	StartURL   string         `json:"startUrl,omitempty"`
	Region     string         `json:"region,omitempty"`
	Portal     string         `json:"portal,omitempty"`
	AccountID  string         `json:"accountId,omitempty"`
	Role       string         `json:"role,omitempty"`
}

type ImportPreview struct {
	Items       []ImportItem `json:"items"`
	Preview     FilePreview  `json:"preview"`
	BaseHash    string       `json:"baseHash"`
	ExtrasCount int          `json:"extrasCount"`
	Source      string       `json:"source"`  // "app name and version" of the exporting app, "" when absent
	ToWrite     int          `json:"toWrite"` // entries that change the config file or the settings
}

type ImportResult struct {
	Added      int         `json:"added"`
	Replaced   int         `json:"replaced"`
	Renamed    int         `json:"renamed"`
	Skipped    int         `json:"skipped"`
	Rejected   int         `json:"rejected"`
	NeedSignIn []string    `json:"needSignIn"` // portals that were added or got new profiles and are not signed in
	Write      WriteResult `json:"write"`
}
