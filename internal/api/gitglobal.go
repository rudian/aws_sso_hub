package api

type HelperEntry struct {
	Scope  string `json:"scope"`  // system | global | local | worktree | command
	Origin string `json:"origin"` // display path of the file
	Key    string `json:"key"`
	Value  string `json:"value"` // redacted
}

type URLRule struct {
	Pattern        string   `json:"pattern"`
	Profile        string   `json:"profile"`
	ProfileKnown   bool     `json:"profileKnown"`
	ColorSlot      int      `json:"colorSlot"`
	Monogram       string   `json:"monogram"`
	MatchedRepoIDs []string `json:"matchedRepoIds"`
	ExtraKeys      bool     `json:"extraKeys"` // section has keys other than helper/usehttppath
}

type GuardInfo struct {
	Installed bool     `json:"installed"`
	Patterns  []string `json:"patterns"`
}

type GitGlobalState struct {
	GlobalFile                string             `json:"globalFile"` // display path
	GlobalPath                string             `json:"globalPath"`
	BaseHash                  string             `json:"baseHash"`
	GlobalDefault             *GlobalDefaultInfo `json:"globalDefault,omitempty"`
	URLRules                  []URLRule          `json:"urlRules"`
	Guard                     GuardInfo          `json:"guard"`
	OtherHelpers              []HelperEntry      `json:"otherHelpers"`    // non-CodeCommit helpers (gh, osxkeychain, manager)
	CredentialLines           []HelperEntry      `json:"credentialLines"` // every credential.* entry from system+global
	SavedCredentialsSupported bool               `json:"savedCredentialsSupported"`
	BoundRepos                int                `json:"boundRepos"`
	UnboundRepos              int                `json:"unboundRepos"`
}

type DefaultAction string

const (
	DefaultKeep    DefaultAction = "keep"
	DefaultRemove  DefaultAction = "remove"
	DefaultReplace DefaultAction = "replace"
)

type DefaultChangeRequest struct {
	Action  DefaultAction `json:"action"`
	Profile string        `json:"profile,omitempty"` // for replace
}

type DefaultChangePreview struct {
	Preview         FilePreview `json:"preview"`
	BoundUnaffected int         `json:"boundUnaffected"`
	UnboundBlocked  int         `json:"unboundBlocked"`
	BlockedRepoIDs  []string    `json:"blockedRepoIds"`
}

type GlobalChangeResult struct {
	State    GitGlobalState `json:"state"`
	BackupID string         `json:"backupId"`
}

type URLRulePlan struct {
	Pattern       string      `json:"pattern"`
	Profile       string      `json:"profile"`
	Repos         []Repo      `json:"repos"`
	DuplicateURLs bool        `json:"duplicateUrls"` // several local clones of one URL: user must assign each
	Preview       FilePreview `json:"preview"`       // ~/.gitconfig change (rule removal)
}

type URLRuleMigration struct {
	Pattern     string     `json:"pattern"`
	Assignments []BindItem `json:"assignments"`
}

type URLRuleMigrationResult struct {
	Outcomes    []BindOutcome `json:"outcomes"`
	RuleRemoved bool          `json:"ruleRemoved"`
	BackupID    string        `json:"backupId"`
}

type SavedCredentialStore string

const (
	StoreKeychain          SavedCredentialStore = "keychain"
	StoreCredentialManager SavedCredentialStore = "credential-manager"
)

type SavedCredential struct {
	ID    string               `json:"id"`
	Host  string               `json:"host"`
	Path  string               `json:"path,omitempty"`
	Store SavedCredentialStore `json:"store"`
}
