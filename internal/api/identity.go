package api

// Author identity (plan 3.4, design 5.12). Three levels, one rule: repository > project > global. What a row
// shows is what git itself reports (git config --show-origin and git var GIT_AUTHOR_IDENT, run with the
// environment a terminal gets), not what the app thinks it wrote.

// EventIdentityChanged is emitted after every identity write and undo.
const EventIdentityChanged = "identity:changed"

// Author is a git author name and email.
type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// IdentitySourceKind says where a value comes from.
type IdentitySourceKind string

const (
	// IdentitySrcNone: nothing sets it.
	IdentitySrcNone IdentitySourceKind = "none"
	// IdentitySrcGlobal: the global git config file (the main one, not an included file).
	IdentitySrcGlobal IdentitySourceKind = "global"
	// IdentitySrcSystem: the system git config.
	IdentitySrcSystem IdentitySourceKind = "system"
	// IdentitySrcProject: the project's own setting (only on a project row).
	IdentitySrcProject IdentitySourceKind = "project"
	// IdentitySrcFromProject: the repository follows its project; the local config holds the project's value.
	IdentitySrcFromProject IdentitySourceKind = "from-project"
	// IdentitySrcRepo: the repository's own local config. External tells whether the app did not write it.
	IdentitySrcRepo IdentitySourceKind = "repo"
	// IdentitySrcConditional: a file pulled in by an includeIf rule (Rule holds its condition).
	IdentitySrcConditional IdentitySourceKind = "conditional"
	// IdentitySrcInclude: a file pulled in by an unconditional include.
	IdentitySrcInclude IdentitySourceKind = "include"
	// IdentitySrcWorktree: the worktree's own config.
	IdentitySrcWorktree IdentitySourceKind = "worktree"
	// IdentitySrcCommand: git's command-line / GIT_CONFIG_COUNT scope.
	IdentitySrcCommand IdentitySourceKind = "command"
	// IdentitySrcEnv: GIT_AUTHOR_NAME, GIT_AUTHOR_EMAIL or EMAIL exported in the terminal environment. Wins
	// over every setting for commits made in terminals and AI tasks.
	IdentitySrcEnv IdentitySourceKind = "env"
)

// IdentitySource is one source tag.
type IdentitySource struct {
	Kind     IdentitySourceKind `json:"kind"`
	Scope    string             `json:"scope,omitempty"` // git's scope: system | global | local | worktree | command
	File     string             `json:"file,omitempty"`  // display path of the file that sets it (a profile file for env)
	Path     string             `json:"path,omitempty"`  // absolute path of that file
	Rule     string             `json:"rule,omitempty"`  // includeIf condition, e.g. "gitdir:~/work/"
	RuleFile string             `json:"ruleFile,omitempty"`
	Var      string             `json:"var,omitempty"` // environment variable (kind env)
	External bool               `json:"external"`      // kind repo: set outside the app
}

// EffectiveIdentity is a resolved identity with its sources. Source is the one tag a row shows (the more
// specific of the two); NameSource and EmailSource are exact.
type EffectiveIdentity struct {
	Set         bool           `json:"set"`
	Name        string         `json:"name"`
	Email       string         `json:"email"`
	NameSource  IdentitySource `json:"nameSource"`
	EmailSource IdentitySource `json:"emailSource"`
	Source      IdentitySource `json:"source"`
}

// EnvVar is one identity variable exported in the terminal environment. Names and emails are not secrets.
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// IdentityFile names a file (a shell profile).
type IdentityFile struct {
	File string `json:"file"` // display path
	Path string `json:"path"`
}

// ProbeErrorKind says why the login shell could not be probed (the UI translates it).
type ProbeErrorKind string

const (
	// ProbeNoShell: no shell to run (Windows, or none found).
	ProbeNoShell ProbeErrorKind = "no-shell"
	// ProbeTimeout: the shell did not finish in time (a slow profile).
	ProbeTimeout ProbeErrorKind = "timeout"
	// ProbeFailed: the shell ended without answering; ProbeExitCode holds its exit status when it had one.
	ProbeFailed ProbeErrorKind = "failed"
)

// TerminalEnvOverride reports identity variables a shell profile exports. Active is true when
// GIT_AUTHOR_NAME or GIT_AUTHOR_EMAIL is set there (or when some repository's effective identity comes from
// the environment): commits made in terminals and AI tasks use them whatever the app writes.
// CommitterVars (GIT_COMMITTER_*) only changes the committer, never the author; FallbackEmail (EMAIL) is
// git's last resort and is used only when no config sets an email. Both are reported separately.
type TerminalEnvOverride struct {
	Active           bool           `json:"active"`
	CommitterVars    bool           `json:"committerVars"`
	FallbackEmail    bool           `json:"fallbackEmail"`
	Vars             []EnvVar       `json:"vars"`
	Files            []IdentityFile `json:"files"` // profile files that appear to export them (best effort, text scan)
	Probed           bool           `json:"probed"`
	ProbeError       ProbeErrorKind `json:"probeError,omitempty"`
	ProbeExitCode    int            `json:"probeExitCode,omitempty"`
	Shell            string         `json:"shell,omitempty"`
	ConfigRedirected bool           `json:"configRedirected"` // GIT_CONFIG_GLOBAL / GIT_CONFIG_SYSTEM / GIT_CONFIG_COUNT differs from the app's
}

// GlobalIdentity is the global card.
type GlobalIdentity struct {
	Set      bool           `json:"set"`
	Name     string         `json:"name"`
	Email    string         `json:"email"`
	File     string         `json:"file"` // display path of the file a save writes
	Path     string         `json:"path"`
	BaseHash string         `json:"baseHash"`
	Source   IdentitySource `json:"source"`
	UsedBy   int            `json:"usedBy"` // repositories whose setting comes from the global level
}

// RepoIdentity is one repository row of the effective identity table.
type RepoIdentity struct {
	RepoID    string `json:"repoId"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	ProjectID string `json:"projectId,omitempty"`
	Missing   bool   `json:"missing"`
	// Effective is what a terminal commit uses (settings and environment). Setting ignores the environment.
	Effective EffectiveIdentity `json:"effective"`
	Setting   EffectiveIdentity `json:"setting"`
	// Override is the repository's own identity as the app recorded it; nil = follows project or global.
	Override *Author `json:"override,omitempty"`
	// FollowsGlobal: the repository was set to follow the global identity on purpose; project changes skip it.
	FollowsGlobal bool `json:"followsGlobal"`
	// ReadError is why git could not be read (code and params, so the UI can translate it).
	ReadError *AppError `json:"readError,omitempty"`
}

// ProjectIdentity is a project row with its repositories.
type ProjectIdentity struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Identity *Author        `json:"identity,omitempty"` // nil = follows global
	Repos    []RepoIdentity `json:"repos"`
}

// IdentityOverview is the whole effective identity table.
type IdentityOverview struct {
	Global    GlobalIdentity      `json:"global"`
	Projects  []ProjectIdentity   `json:"projects"`
	Ungrouped []RepoIdentity      `json:"ungrouped"`
	Env       TerminalEnvOverride `json:"env"`
}

// RepoAction is what a change does to one repository's local config.
type RepoAction string

const (
	// RepoActionSet writes user.name and user.email.
	RepoActionSet RepoAction = "set"
	// RepoActionRemove removes them.
	RepoActionRemove RepoAction = "remove"
	// RepoActionUnchanged: already as wanted.
	RepoActionUnchanged RepoAction = "unchanged"
	// RepoActionKeepOwn: the repository has its own identity in the app; untouched.
	RepoActionKeepOwn RepoAction = "keep-own"
	// RepoActionKeepExternal: the local config holds values the app did not write; untouched.
	RepoActionKeepExternal RepoAction = "keep-external"
	// RepoActionSkipMissing: the folder is gone.
	RepoActionSkipMissing RepoAction = "skip-missing"
	// RepoActionError: the folder exists but its config cannot be read or written (see Error).
	RepoActionError RepoAction = "error"
)

// RepoChangePlan is one repository in a preview. Before is the local config now (nil = none), After what it
// holds afterwards (nil = nothing). Preview is set only when the file changes.
type RepoChangePlan struct {
	RepoID  string       `json:"repoId"`
	Name    string       `json:"name"`
	Path    string       `json:"path"`
	Action  RepoAction   `json:"action"`
	Before  *Author      `json:"before,omitempty"`
	After   *Author      `json:"after,omitempty"`
	Preview *FilePreview `json:"preview,omitempty"`
	// Error is set with RepoActionError: E_GIT_FAILED or E_FILE_IO, with file = the config's display path.
	Error *AppError `json:"error,omitempty"`
}

// GlobalIdentityPreview previews a global save.
type GlobalIdentityPreview struct {
	Preview   FilePreview `json:"preview"`
	Before    *Author     `json:"before,omitempty"`
	After     Author      `json:"after"`
	Unchanged bool        `json:"unchanged"`
}

// ProjectIdentityRequest sets (Identity) or clears (nil: follow global) a project's identity.
type ProjectIdentityRequest struct {
	ProjectID string  `json:"projectId"`
	Identity  *Author `json:"identity,omitempty"`
}

// ProjectIdentityPreview lists the members and what saving does to each. PlanHash must be passed back.
type ProjectIdentityPreview struct {
	ProjectID string           `json:"projectId"`
	Name      string           `json:"name"`
	Identity  *Author          `json:"identity,omitempty"`
	Previous  *Author          `json:"previous,omitempty"`
	Repos     []RepoChangePlan `json:"repos"`
	Writes    int              `json:"writes"`   // repositories whose config changes
	KeepsOwn  int              `json:"keepsOwn"` // repositories that keep their own setting
	PlanHash  string           `json:"planHash"`
}

// RepoIdentityMode picks what a repository follows.
type RepoIdentityMode string

const (
	// RepoModeOwn: the repository's own identity (Identity required).
	RepoModeOwn RepoIdentityMode = "own"
	// RepoModeProject: follow the project (or global when the project has none).
	RepoModeProject RepoIdentityMode = "project"
	// RepoModeGlobal: follow global, whatever the project says: the local user.* values are removed and the
	// repository is marked so that project changes leave it alone.
	RepoModeGlobal RepoIdentityMode = "global"
)

// RepoIdentityRequest changes one repository.
type RepoIdentityRequest struct {
	RepoID   string           `json:"repoId"`
	Mode     RepoIdentityMode `json:"mode"`
	Identity *Author          `json:"identity,omitempty"`
}

// RepoIdentityPreview previews a repository change.
type RepoIdentityPreview struct {
	RepoID   string           `json:"repoId"`
	Mode     RepoIdentityMode `json:"mode"`
	Plan     RepoChangePlan   `json:"plan"`
	PlanHash string           `json:"planHash"`
}

// MoveIdentityPreview previews the identity side of moving a repository between projects.
type MoveIdentityPreview struct {
	RepoID        string         `json:"repoId"`
	FromProjectID string         `json:"fromProjectId,omitempty"`
	ToProjectID   string         `json:"toProjectId,omitempty"`
	Plan          RepoChangePlan `json:"plan"`
	PlanHash      string         `json:"planHash"`
}

// RepoChangeOutcome is the result for one repository.
type RepoChangeOutcome struct {
	RepoID string     `json:"repoId"`
	Name   string     `json:"name"`
	Action RepoAction `json:"action"`
	OK     bool       `json:"ok"`
	Error  *AppError  `json:"error,omitempty"` // E_FILE_IO with file=<config path>: "cannot write this repository's config"
}

// IdentityChangeResult is returned by every apply call.
type IdentityChangeResult struct {
	Outcomes []RepoChangeOutcome `json:"outcomes"`
	// UndoID names what the engine kept for Undo (backups, hashes and the previous record values stay in Go;
	// the page only hands the ID back). "" = nothing to undo. It is lost when the app quits.
	UndoID        string   `json:"undoId"`
	RecordUpdated bool     `json:"recordUpdated"` // projects.json holds the new value
	BackupIDs     []string `json:"backupIds"`
}

// ErrConfigShared: a per-repository identity change would write a config file that another repository (a linked
// worktree) with an identity of its own reads too. Nothing was written. params: file
const ErrConfigShared ErrorCode = "E_CONFIG_SHARED"
