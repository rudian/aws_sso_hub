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
	BindingLocal         BindingSource = "local"
	BindingGlobalURL     BindingSource = "global-url"
	BindingGlobalDefault BindingSource = "global-default"
	BindingRemoteURL     BindingSource = "remote-url" // git-remote-codecommit URL names the profile
	BindingGuard         BindingSource = "guard"      // only the global "not bound" guard applies
	BindingNone          BindingSource = "none"       // no CodeCommit helper; git prompts or uses the keychain
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

type Repo struct {
	ID          string      `json:"id"`          // first 16 hex chars of sha1(absolute path)
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
}

type RepoCounts struct {
	All   int `json:"all"`
	ToFix int `json:"toFix"`
	Bound int `json:"bound"`
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
	Profile    string `json:"profile"`
	Region     string `json:"region"`
	RepoName   string `json:"repoName"`
	ParentDir  string `json:"parentDir"`
	FolderName string `json:"folderName"`
}

type ClonePreview struct {
	Dest     string `json:"dest"`
	Exists   bool   `json:"exists"`
	Command  string `json:"command"` // "git clone https://git-codecommit.<region>.amazonaws.com/v1/repos/<name>"
	HTTPSURL string `json:"httpsUrl"`
}

type CloneProgress struct {
	Line string `json:"line"`
}
