package api

type RemoteKind string

const (
	RemoteHTTPS RemoteKind = "https"
	RemoteSSH   RemoteKind = "ssh"
	RemoteGRC   RemoteKind = "grc"
	RemoteOther RemoteKind = "other"
)

type BindingSource string

const (
	// BindingMachine: a GitHub, Bitbucket or other non-CodeCommit repository follows the machine's own git
	// setup (credential helper, SSH agent, keychain). The row says "other binding: machine default".
	BindingMachine       BindingSource = "machine-default"
	BindingLocal         BindingSource = "local"
	BindingGlobalURL     BindingSource = "global-url"
	BindingGlobalDefault BindingSource = "global-default"
	BindingRemoteURL     BindingSource = "remote-url" // git-remote-codecommit URL names the profile
	BindingGuard         BindingSource = "guard"      // only the global "not bound" guard applies
	BindingNone          BindingSource = "none"       // no CodeCommit helper; git prompts or uses the keychain
)

// RepoProvider is the host family of a repository's remote (plan 3.2), worked out from the remote URL.
type RepoProvider string

const (
	ProviderCodeCommit RepoProvider = "codecommit"
	ProviderGitHub     RepoProvider = "github"
	ProviderBitbucket  RepoProvider = "bitbucket"
	ProviderOther      RepoProvider = "other"
)

// Error codes of the repository list (stable, see ErrorCode). F1-14 explains them.
const (
	// ErrMovedHasData: the folder found at the new place already has a project, hidden flag or identity of its
	// own, so "yes, it moved" would delete that data. (Re-attach refuses; nothing changed.)
	ErrMovedHasData ErrorCode = "E_MOVED_HAS_DATA"
	// ErrMovedGone: the moved-folder question is out of date (the old folder came back, the record was dropped
	// after 30 days, or the question was answered already).
	ErrMovedGone ErrorCode = "E_MOVED_GONE"
	// ErrProjectNotFound: the project does not exist (any more). params: id
	ErrProjectNotFound ErrorCode = "E_PROJECT_NOT_FOUND"
)

type OwnerStatus string

const (
	OwnerUnknown   OwnerStatus = "unknown"
	OwnerChecking  OwnerStatus = "checking"
	OwnerSingle    OwnerStatus = "single"
	OwnerAmbiguous OwnerStatus = "ambiguous"
	OwnerNone      OwnerStatus = "none"
	OwnerPartial   OwnerStatus = "partial" // nothing found, some accounts not checked
)

type TestStatus string

const (
	TestOK        TestStatus = "ok"
	TestAuth      TestStatus = "auth"
	TestSignedOut TestStatus = "signed-out"
	TestNotFound  TestStatus = "not-found"
	TestNetwork   TestStatus = "network"
	TestTimeout   TestStatus = "timeout"
	TestError     TestStatus = "error"
)

type OwnerCandidate struct {
	AccountID  string   `json:"accountId"`
	Profile    string   `json:"profile"`  // suggested profile in this account
	Profiles   []string `json:"profiles"` // all profiles of this account
	ColorSlot  int      `json:"colorSlot"`
	Monogram   string   `json:"monogram"`
	HeadMatch  bool     `json:"headMatch"`
	MatchScore int      `json:"matchScore"`
	KMSWarning bool     `json:"kmsWarning"`
}

type OwnerResult struct {
	Status         OwnerStatus      `json:"status"`
	Candidates     []OwnerCandidate `json:"candidates"`
	CheckedWith    int              `json:"checkedWith"` // accounts queried
	Skipped        int              `json:"skipped"`     // accounts not checked (not signed in / denied)
	SkippedPortals []string         `json:"skippedPortals"`
	CheckedAt      string           `json:"checkedAt,omitempty"`
}

type TestResult struct {
	RepoID  string     `json:"repoId"`
	Status  TestStatus `json:"status"`
	At      string     `json:"at"`
	Profile string     `json:"profile,omitempty"`
	Portal  string     `json:"portal,omitempty"` // for signed-out
	Detail  string     `json:"detail,omitempty"` // redacted stderr tail
}

type Binding struct {
	Source        BindingSource `json:"source"`
	Profile       string        `json:"profile,omitempty"`
	ProfileKnown  bool          `json:"profileKnown"`
	AccountID     string        `json:"accountId,omitempty"`
	ColorSlot     int           `json:"colorSlot"` // 0 when the profile is unknown
	Monogram      string        `json:"monogram"`
	URLRule       string        `json:"urlRule,omitempty"`
	LegacyPlain   bool          `json:"legacyPlain"`   // local plain [credential] block (e.g. acme_design)
	NeedsRepair   bool          `json:"needsRepair"`   // managed binding with missing aws path/fallback/reset, or bare "aws"
	NoProfileFlag bool          `json:"noProfileFlag"` // aws helper without --profile
}

// RepoStatus is the git status of one repository, read through the read-only git layer (F1-04). It is filled
// by RefreshStatus (and the file watcher) for repositories that are in the list; hidden and not yet reviewed
// repositories get none.
type RepoStatus struct {
	Branch     string `json:"branch"` // "" when detached
	Detached   bool   `json:"detached"`
	Unborn     bool   `json:"unborn"`   // no commit yet
	Upstream   string `json:"upstream"` // "" when none
	SyncKnown  bool   `json:"syncKnown"`
	Ahead      int    `json:"ahead"`
	Behind     int    `json:"behind"`
	Staged     int    `json:"staged"`
	Unstaged   int    `json:"unstaged"`
	Untracked  int    `json:"untracked"`
	Conflicted int    `json:"conflicted"`
	Changes    int    `json:"changes"`    // files changed or untracked, a file counted once
	Unverified int    `json:"unverified"` // changes that could not be confirmed (filter drivers such as git-lfs)
	// RenamesSkipped: a partial clone could not detect renames, so a renamed file shows as a deletion and a
	// new file (FU-04b).
	RenamesSkipped bool   `json:"renamesSkipped"`
	At             string `json:"at"` // RFC 3339, when it was read
}

// MovedFrom is the question "Is this <name>, moved here?" (F1-18): this folder's first commit and origin URL
// match one record that vanished from disk less than 30 days ago.
type MovedFrom struct {
	OldID string `json:"oldId"`
	Name  string `json:"name"` // the old folder's name
}

type Repo struct {
	ID          string      `json:"id"`          // random, stable (F1-18); never derived from the path
	Path        string      `json:"path"`        // absolute
	DisplayPath string      `json:"displayPath"` // "~/Documents/code/acme_cs_portal"
	Name        string      `json:"name"`        // CodeCommit repository name
	Remote      string      `json:"remote"`      // remote name, usually "origin"
	RemoteURL   string      `json:"remoteUrl"`   // redacted
	RemoteKind  RemoteKind  `json:"remoteKind"`
	Host        string      `json:"host,omitempty"`
	Region      string      `json:"region,omitempty"`
	HTTPSURL    string      `json:"httpsUrl,omitempty"` // canonical https://<host>/v1/repos/<name>
	HasUserinfo bool        `json:"hasUserinfo"`
	Binding     Binding     `json:"binding"`
	Owner       OwnerResult `json:"owner"`
	LastTest    *TestResult `json:"lastTest,omitempty"`
	ToFix       bool        `json:"toFix"`
	// Extra is true for a repository that is listed only because the user added it by hand (it lies outside
	// the scan roots); RemoveExtra applies to exactly these.
	Extra bool `json:"extra"`

	// LocalName is the folder's name; RemoteName is the last segment of the origin URL without ".git" ("" when
	// there is no remote). They may differ (remote acme_api cloned as api-main); the UI shows both then.
	LocalName  string       `json:"localName"`
	RemoteName string       `json:"remoteName"`
	Provider   RepoProvider `json:"provider"`
	ProjectID  string       `json:"projectId,omitempty"`
	Hidden     bool         `json:"hidden"`
	// Pending: found by a scan and waiting for the one-time review (Found list); not in Repos or the counts.
	Pending      bool        `json:"pending"`
	LastCommitAt string      `json:"lastCommitAt,omitempty"` // RFC 3339; filled for Found and Hidden rows
	Status       *RepoStatus `json:"status,omitempty"`
	MovedFrom    *MovedFrom  `json:"movedFrom,omitempty"`
}

type RepoCounts struct {
	All    int `json:"all"`
	ToFix  int `json:"toFix"`
	Bound  int `json:"bound"`
	Found  int `json:"found"`  // waiting for the one-time review
	Hidden int `json:"hidden"` // hidden by the person
}

// ProjectInfo is a project with the number of listed repositories in it.
type ProjectInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Collapsed bool   `json:"collapsed"`
	Order     int    `json:"order"`
	RepoCount int    `json:"repoCount"`
}

type SuggestionKind string

const (
	SuggestPrefix SuggestionKind = "prefix" // three or more ungrouped repositories share a name prefix
	SuggestFolder SuggestionKind = "folder" // three or more ungrouped repositories share a parent folder
)

// ProjectSuggestion is one dismissible "put them in one project?" banner. Key is what DismissSuggestion takes;
// Label is the prefix or the folder's name and is the proposed project name.
type ProjectSuggestion struct {
	Key     string         `json:"key"`
	Kind    SuggestionKind `json:"kind"`
	Label   string         `json:"label"`
	RepoIDs []string       `json:"repoIds"`
}

type WarningCode string

const (
	// WarnProjectsNotWritable: projects.json could not be written, so folders that have no record yet were left
	// out of the list. They are tried again at the next scan. Count is how many; File is "projects.json".
	WarnProjectsNotWritable WarningCode = "projects-not-writable"
)

// RepoWarning is a non-fatal problem the list should mention (FU-18c).
type RepoWarning struct {
	Code  WarningCode `json:"code"`
	Count int         `json:"count"`
	File  string      `json:"file,omitempty"`
}

type GlobalDefaultInfo struct {
	Profile       string      `json:"profile"`
	ProfileKind   ProfileKind `json:"profileKind,omitempty"`
	ColorSlot     int         `json:"colorSlot"`
	Monogram      string      `json:"monogram"`
	Helper        string      `json:"helper"` // the helper line as written (contains no secrets)
	AffectedRepos int         `json:"affectedRepos"`
}

type ReposState struct {
	Roots         []string           `json:"roots"`
	RootsDisplay  []string           `json:"rootsDisplay"`
	ScannedAt     string             `json:"scannedAt,omitempty"`
	Scanning      bool               `json:"scanning"`
	Detecting     bool               `json:"detecting"`
	Repos         []Repo             `json:"repos"` // sorted by Name, then DisplayPath
	Counts        RepoCounts         `json:"counts"`
	GlobalDefault *GlobalDefaultInfo `json:"globalDefault,omitempty"`
	AWSCLIReady   bool               `json:"awsCliReady"`
	NeedsRepair   int                `json:"needsRepair"`

	// Found are the repositories a scan found that the person has not yet chosen to show (the one-time review,
	// plan 4.4). HiddenRepos are the hidden ones. Neither is in Repos or Counts.All.
	Found       []Repo `json:"found"`
	HiddenRepos []Repo `json:"hiddenRepos"`
	// Projects are all projects, in order. Suggestions are the banners not dismissed yet.
	Projects    []ProjectInfo       `json:"projects"`
	Suggestions []ProjectSuggestion `json:"suggestions"`
	Warnings    []RepoWarning       `json:"warnings"`
}

type ScanProgress struct {
	Root  string `json:"root"` // display path
	Found int    `json:"found"`
	Done  bool   `json:"done"`
}

type BindItem struct {
	RepoID  string `json:"repoId"`
	Profile string `json:"profile"`
}

type BindWarning string

const (
	BindWarnReplaces      BindWarning = "replaces-existing"
	BindWarnStatic        BindWarning = "static-profile"
	BindWarnSignedOut     BindWarning = "signed-out"
	BindWarnOwnerMismatch BindWarning = "owner-mismatch"
)

type BindPreview struct {
	RepoID   string        `json:"repoId"`
	Profile  string        `json:"profile"`
	Commands []string      `json:"commands"` // display form: git config --local ... lines
	Warnings []BindWarning `json:"warnings"`
}

type BindOutcome struct {
	RepoID   string    `json:"repoId"`
	OK       bool      `json:"ok"`
	Repo     *Repo     `json:"repo,omitempty"`
	BackupID string    `json:"backupId,omitempty"`
	Error    *AppError `json:"error,omitempty"`
}

// AddRepoResult is what RepoService.AddExisting returns.
type AddRepoResult struct {
	State     ReposState   `json:"state"`
	RepoID    string       `json:"repoId"`
	Added     bool         `json:"added"`             // false: it was already listed (scan root or extraRepos)
	AutoBound bool         `json:"autoBound"`         // the repository was bound to its detected SSO profile
	Outcome   *BindOutcome `json:"outcome,omitempty"` // set when AutoBound
}

type RemoteRepo struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	LastModified string `json:"lastModified,omitempty"`
	RepositoryID string `json:"repositoryId"`
	CloneURL     string `json:"cloneUrl"`
}

type CloneRequest struct {
	Profile   string `json:"profile"`
	Region    string `json:"region"`
	RepoName  string `json:"repoName"`
	ParentDir string `json:"parentDir"`
	// FolderName is the local folder's name; "" means the remote repository's name. It is checked by
	// ValidateFolderName (no separator, not "." or "..", no reserved Windows name, no trailing dot or space).
	FolderName string `json:"folderName"`
}

type ClonePreview struct {
	Dest   string `json:"dest"`
	Exists bool   `json:"exists"`
	// FolderName is the name the clone will use (the request's, or the remote name when it was empty).
	FolderName string `json:"folderName"`
	Command    string `json:"command"` // "git clone https://git-codecommit.<region>.amazonaws.com/v1/repos/<name>"
	HTTPSURL   string `json:"httpsUrl"`
}

type CloneProgress struct {
	Line string `json:"line"`
}
