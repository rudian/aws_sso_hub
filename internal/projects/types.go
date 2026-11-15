// Package projects owns projects.json: projects, per-repository records (random stable ID, path, legacy ID,
// hidden flag, binding, identity override, AI defaults) and the last integrated remote commit per branch
// (plan 3.1, 3.2). It sits on internal/store and never touches settings.json.
package projects

import (
	"time"

	"github.com/rudian/codepass/internal/api"
)

// SchemaVersion is the projects.json version this build writes.
const SchemaVersion = 1

// Identity is a git author name and email.
type Identity struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Project groups repositories. Projects are flat.
type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Collapsed bool      `json:"collapsed"`
	Order     int       `json:"order"`
	Identity  *Identity `json:"identity,omitempty"`
	CreatedAt string    `json:"createdAt"` // RFC 3339, UTC
}

// Provider is the remote host family, cached from the remote URL.
type Provider string

const (
	ProviderCodeCommit Provider = "codecommit"
	ProviderGitHub     Provider = "github"
	ProviderBitbucket  Provider = "bitbucket"
	ProviderOther      Provider = "other"
)

// BindingKind says how a repository gets credentials.
type BindingKind string

const (
	BindingAWSProfile BindingKind = "aws-profile"
	BindingSSHKey     BindingKind = "ssh-key"
)

// Binding: nil on a RepoRecord means the repository follows the machine's own git setup.
type Binding struct {
	Kind      BindingKind `json:"kind"`
	Profile   string      `json:"profile,omitempty"`   // aws-profile
	AccountID string      `json:"accountId,omitempty"` // ssh-key
}

// RepoAIConfig holds per-repository AI defaults.
type RepoAIConfig struct {
	DefaultTool         string `json:"defaultTool,omitempty"`      // claude | codex | opencode
	DefaultIsolation    string `json:"defaultIsolation,omitempty"` // worktree | main
	SetupCommand        string `json:"setupCommand,omitempty"`
	PreselectAWSProfile bool   `json:"preselectAwsProfile"`
}

// RepoRecord is one repository the app knows. ID is random and stable: it is never derived from the path, so
// a renamed or moved folder keeps its record. LegacyID is the 0.0.1 hash of the path (so 0.0.1 data keyed by
// it, such as saved test results, can be matched once).
type RepoRecord struct {
	ID               string          `json:"id"`
	LegacyID         string          `json:"legacyId,omitempty"`
	Path             string          `json:"path"`
	ProjectID        string          `json:"projectId,omitempty"`
	Hidden           bool            `json:"hidden"`
	Provider         Provider        `json:"provider,omitempty"`
	LastTest         *api.TestResult `json:"lastTest,omitempty"`
	Binding          *Binding        `json:"binding,omitempty"`
	IdentityOverride *Identity       `json:"identityOverride,omitempty"`
	AI               *RepoAIConfig   `json:"ai,omitempty"`
	// Integrated maps "<remote>/<branch>" to the last remote commit the user has integrated.
	Integrated map[string]string `json:"integrated,omitempty"`
	// MissingSince is when the folder was first found missing; the record is kept for 30 days. Filled by F1-18.
	MissingSince *time.Time `json:"missingSince,omitempty"`
	// FirstCommit is the repository's first commit hash, used with OriginURL to re-attach a moved folder. Filled by F1-18.
	FirstCommit string `json:"firstCommit,omitempty"`
	// OriginURL is the URL of the "origin" remote when the record was last seen. Filled by F1-18.
	OriginURL string `json:"originUrl,omitempty"`
	// FirstSeen is the start time of the scan that created this record. A folder is offered as "moved here" only
	// when it was first seen no earlier than the scan that marked the old record missing; a zero value never
	// matches (so a second checkout that is kept for a while, then deleted, never takes over the survivor).
	FirstSeen *time.Time `json:"firstSeen,omitempty"`
	// DeclinedMoves lists the IDs of missing records the person said this folder is not a move of.
	DeclinedMoves []string `json:"declinedMoves,omitempty"`
}

// Data is the whole projects.json document.
type Data struct {
	SchemaVersion int          `json:"schemaVersion"`
	Projects      []Project    `json:"projects"`
	Repos         []RepoRecord `json:"repos"`
}
