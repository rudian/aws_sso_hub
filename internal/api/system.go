package api

type AppInfo struct {
	Version    string `json:"version"`
	Platform   string `json:"platform"` // "darwin" | "windows"
	Arch       string `json:"arch"`
	Sandbox    bool   `json:"sandbox"`
	DataDir    string `json:"dataDir"`   // display path
	BackupDir  string `json:"backupDir"` // display path
	FirstRun   bool   `json:"firstRun"`
	UILanguage string `json:"uiLanguage"` // last language reported by SetUILanguage ("zh-CN" | "en")
}

type ToolStatus string

const (
	ToolOK       ToolStatus = "ok"
	ToolOutdated ToolStatus = "outdated"
	ToolTooOld   ToolStatus = "too-old"
	ToolV1       ToolStatus = "v1"
	ToolMissing  ToolStatus = "missing"
	ToolError    ToolStatus = "error"
)

type ToolInfo struct {
	Name        string     `json:"name"` // "aws" | "git" | "git-remote-codecommit"
	Path        string     `json:"path,omitempty"`
	Version     string     `json:"version,omitempty"`
	Status      ToolStatus `json:"status"`
	Min         string     `json:"min,omitempty"`
	Recommended string     `json:"recommended,omitempty"`
}

type FileCheck struct {
	Kind     string       `json:"kind"` // "aws-config" | "aws-credentials" | "git-global"
	File     string       `json:"file"` // display path
	Exists   bool         `json:"exists"`
	Readable bool         `json:"readable"`
	Writable bool         `json:"writable"`
	Issue    *ConfigIssue `json:"issue,omitempty"`
}

type EnvReport struct {
	AWSCLI    ToolInfo    `json:"awsCli"`
	Git       ToolInfo    `json:"git"`
	GRC       ToolInfo    `json:"grc"`
	Files     []FileCheck `json:"files"`
	CheckedAt string      `json:"checkedAt"`
}

type Severity string

const (
	SeverityOK     Severity = "ok"
	SeverityInfo   Severity = "info"
	SeverityWarn   Severity = "warn"
	SeverityDanger Severity = "danger"
)

type HealthKind string

const (
	HealthShellExport  HealthKind = "shell-export"  // file, line
	HealthEnvVar       HealthKind = "env-var"       // params: name
	HealthLeftoverKeys HealthKind = "leftover-keys" // profile
	HealthDefaultKeys  HealthKind = "default-keys"
	HealthOldKey       HealthKind = "old-key"              // profile, params: days
	HealthGlobalStatic HealthKind = "global-static"        // profile
	HealthURLUserinfo  HealthKind = "url-userinfo"         // params: repo
	HealthPendingKey   HealthKind = "pending-deactivation" // profile, params: maskedKeyId
)

type HealthFinding struct {
	ID       string            `json:"id"`
	Kind     HealthKind        `json:"kind"`
	Severity Severity          `json:"severity"`
	File     string            `json:"file,omitempty"` // display path
	Line     int               `json:"line,omitempty"`
	Profile  string            `json:"profile,omitempty"`
	Params   map[string]string `json:"params,omitempty"`
}

type HealthReport struct {
	Findings  []HealthFinding `json:"findings"`
	CheckedAt string          `json:"checkedAt"`
}

type Language string

const (
	LanguageSystem Language = "system"
	LanguageZhCN   Language = "zh-CN"
	LanguageEn     Language = "en"
)

type Theme string

const (
	ThemeSystem Theme = "system"
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
)

type MotionPref string

const (
	MotionSystem MotionPref = "system"
	MotionReduce MotionPref = "reduce"
	MotionFull   MotionPref = "full"
)

type TerminalApp string

const (
	TerminalAuto            TerminalApp = "auto"
	TerminalMacTerminal     TerminalApp = "terminal"
	TerminalITerm2          TerminalApp = "iterm2"
	TerminalWindowsTerminal TerminalApp = "windows-terminal"
	TerminalPowerShell      TerminalApp = "powershell"
	TerminalCmd             TerminalApp = "cmd"
)

type ReminderSettings struct {
	ExpiryNotice bool `json:"expiryNotice"`
	LeadMinutes  int  `json:"leadMinutes"` // 15 | 30 | 60
	AutoRenew    bool `json:"autoRenew"`
}

type TraySettings struct {
	Enabled     bool `json:"enabled"`
	KeepRunning bool `json:"keepRunning"`
}

type Settings struct {
	Language        Language         `json:"language"`
	Theme           Theme            `json:"theme"`
	Motion          MotionPref       `json:"motion"`
	ScanRoots       []string         `json:"scanRoots"`
	Reminders       ReminderSettings `json:"reminders"`
	Tray            TraySettings     `json:"tray"`
	TerminalApp     TerminalApp      `json:"terminalApp"`
	BackupRetention int              `json:"backupRetention"` // 20 | 50 | 0 (= keep all)
	TutorialDone    []string         `json:"tutorialDone"`    // tutorial section ids
	TutorialLang    string           `json:"tutorialLang"`    // "" follows the UI language
	AWSCLIPath      string           `json:"awsCliPath"`      // "" = auto-detect
	GitPath         string           `json:"gitPath"`         // "" = auto-detect
	AutoBindOnAdd   bool             `json:"autoBindOnAdd"`   // bind a hand-added repository to its detected SSO profile
	PrivateSignIn   bool             `json:"privateSignIn"`   // open the sign-in page in a private browser window (default true)

	CollapsedPortals []string `json:"collapsedPortals"` // portal names collapsed on the Accounts page
	LastCloneProfile string   `json:"lastCloneProfile"` // AWS profile of the last successful clone
	LastCloneRegion  string   `json:"lastCloneRegion"`  // region of the last successful clone
}

// SettingsPatch: nil fields are left unchanged.
type SettingsPatch struct {
	Language        *Language         `json:"language,omitempty"`
	Theme           *Theme            `json:"theme,omitempty"`
	Motion          *MotionPref       `json:"motion,omitempty"`
	ScanRoots       []string          `json:"scanRoots,omitempty"` // nil = unchanged; empty slice is rejected (E_INVALID_INPUT)
	Reminders       *ReminderSettings `json:"reminders,omitempty"`
	Tray            *TraySettings     `json:"tray,omitempty"`
	TerminalApp     *TerminalApp      `json:"terminalApp,omitempty"`
	BackupRetention *int              `json:"backupRetention,omitempty"`
	TutorialDone    []string          `json:"tutorialDone,omitempty"` // nil = unchanged; empty slice = clear
	TutorialLang    *string           `json:"tutorialLang,omitempty"`
	AWSCLIPath      *string           `json:"awsCliPath,omitempty"`
	GitPath         *string           `json:"gitPath,omitempty"`
	AutoBindOnAdd   *bool             `json:"autoBindOnAdd,omitempty"`
	PrivateSignIn   *bool             `json:"privateSignIn,omitempty"`

	CollapsedPortals []string `json:"collapsedPortals,omitempty"` // nil = unchanged; empty slice = clear
	LastCloneProfile *string  `json:"lastCloneProfile,omitempty"`
	LastCloneRegion  *string  `json:"lastCloneRegion,omitempty"`
}

type BackupKind string

const (
	BackupAWSConfig      BackupKind = "aws-config"
	BackupAWSCredentials BackupKind = "aws-credentials"
	BackupGitGlobal      BackupKind = "git-global"
	BackupGitLocal       BackupKind = "git-local"
)

type BackupReason string

const (
	ReasonAddProfiles   BackupReason = "add-profiles"   // params: n
	ReasonRemoveProfile BackupReason = "remove-profile" // params: name
	ReasonMigrate       BackupReason = "migrate"        // params: profile
	ReasonRemoveKey     BackupReason = "remove-key"     // params: profile
	ReasonBind          BackupReason = "bind"           // params: repo
	ReasonUnbind        BackupReason = "unbind"         // params: repo
	ReasonHTTPS         BackupReason = "https"          // params: repo
	ReasonRepair        BackupReason = "repair"         // params: repo
	ReasonGlobalDefault BackupReason = "global-default"
	ReasonURLRule       BackupReason = "url-rule" // params: pattern
	ReasonRestore       BackupReason = "restore"  // params: file
	ReasonPortal        BackupReason = "portal"   // params: portal
	ReasonIdentity      BackupReason = "identity" // reserved
	ReasonImport        BackupReason = "import"   // params: n
)

type BackupEntry struct {
	ID           string            `json:"id"`
	CreatedAt    string            `json:"createdAt"`
	Path         string            `json:"path"` // absolute path of the original file
	File         string            `json:"file"` // display path of the original file
	Kind         BackupKind        `json:"kind"`
	Reason       BackupReason      `json:"reason"`
	ReasonParams map[string]string `json:"reasonParams"`
	Size         int64             `json:"size"`
	Redacted     bool              `json:"redacted"`
}

type RestorePreview struct {
	Entry        BackupEntry `json:"entry"`
	Preview      FilePreview `json:"preview"`
	LostProfiles []string    `json:"lostProfiles"` // credentials whose key values the redacted backup cannot restore
}

type FirstRunSummary struct {
	Profiles  int      `json:"profiles"`
	Portals   int      `json:"portals"`
	Static    int      `json:"static"`
	Repos     int      `json:"repos"`
	ScanRoots []string `json:"scanRoots"` // display paths
}

type ExternalChange struct {
	Kind string `json:"kind"` // "aws-config" | "aws-credentials" | "git-global"
	File string `json:"file"` // display path
}

type NavigateRequest struct {
	Page   string `json:"page"`             // PageId of the frontend
	Action string `json:"action,omitempty"` // "login"
	Arg    string `json:"arg,omitempty"`    // portal name for "login"
}

// ---- tools gate and installers (F1-12) -----------------------------------------------------------------------

// EventToolInstall carries an InstallState snapshot every time the setup run changes. The wiring task registers
// it in internal/app/events.go (RegisterEvent[api.InstallState]).
const EventToolInstall = "tools:install-progress"

// SetupToolID names a row of the setup screen.
type SetupToolID string

const (
	SetupHomebrew SetupToolID = "homebrew" // macOS
	SetupWinget   SetupToolID = "winget"   // Windows
	SetupGit      SetupToolID = "git"
	SetupAWSCLI   SetupToolID = "awscli"
)

// SetupFix says what the one-click install would do for a row that is not ready.
type SetupFix string

const (
	FixNone         SetupFix = ""              // ready
	FixInstall      SetupFix = "install"       // not there: install it
	FixUpgrade      SetupFix = "upgrade"       // found but below the hard minimum: install a newer one next to it
	FixRepairPrefix SetupFix = "repair-prefix" // macOS: /opt/homebrew exists without a working brew; the user repairs it
	FixOpenStore    SetupFix = "open-store"    // Windows: winget is missing; open the Store page for App Installer
	// FixUseDetected: the tool path chosen in Settings is missing or too old, but automatic detection finds an
	// acceptable copy. The setup run clears the chosen path (a normal, backed-up settings write); nothing is installed.
	FixUseDetected SetupFix = "use-detected"
)

// SetupTool is one row of the gate. Status reuses ToolStatus: ok, too-old, v1, missing, error.
type SetupTool struct {
	ID       SetupToolID `json:"id"`
	Status   ToolStatus  `json:"status"`
	Version  string      `json:"version,omitempty"` // the version found, so the screen can say "git 2.39.1 found, 2.44 needed"
	Min      string      `json:"min,omitempty"`
	Path     string      `json:"path,omitempty"` // where it was found (display form)
	Blocking bool        `json:"blocking"`       // true when the gate stays closed because of this row
	Fix      SetupFix    `json:"fix"`
	// Elsewhere is true (macOS only) when a too-old tool was found outside Homebrew's own folder (Apple's git): the
	// fix installs a newer copy that is found first, and leaves the old one alone. On Windows `winget install`
	// upgrades or installs next to whatever is there, and the flag stays false.
	Elsewhere bool `json:"elsewhere,omitempty"`
	// Override is true when the row was judged on a program path the user chose in Settings (git, AWS CLI). The
	// setup run clears that path first, because a chosen path that blocks the gate cannot be changed from the
	// setup screen.
	Override bool `json:"override,omitempty"`
}

// Tri is a fact that may be unknown.
type Tri string

const (
	TriYes     Tri = "yes"
	TriNo      Tri = "no"
	TriUnknown Tri = "unknown"
)

// ProxyInfo is the proxy setting the setup run uses. Credentials are never included.
type ProxyInfo struct {
	HostPort    string `json:"hostPort,omitempty"`    // "proxy.example.com:8080"
	Source      string `json:"source,omitempty"`      // "env" | "system" | "none"
	PACOnly     bool   `json:"pacOnly,omitempty"`     // only a PAC file is set: Codepass cannot use it
	Unsupported string `json:"unsupported,omitempty"` // "socks" | "autodiscovery"
}

// Preflight holds the facts the setup screen shows before anything is installed.
type Preflight struct {
	Admin        Tri       `json:"admin"`        // macOS: `id -Gn` lists "admin". Windows: unknown (UAC decides)
	Managed      Tri       `json:"managed"`      // macOS: `profiles status -type enrollment` says "MDM enrollment: Yes"
	FreeBytes    int64     `json:"freeBytes"`    // free space where the tools go; -1 when it could not be read
	NeedBytes    int64     `json:"needBytes"`    // about 1.2 GB
	EnoughSpace  bool      `json:"enoughSpace"`  // true when unknown
	PrefixExists bool      `json:"prefixExists"` // macOS: /opt/homebrew exists
	Proxy        ProxyInfo `json:"proxy"`
}

// SetupState is the answer of ToolService.GetSetupState. It is built from local checks only.
type SetupState struct {
	Platform  string      `json:"platform"` // "darwin" | "windows"
	Tools     []SetupTool `json:"tools"`    // the manager (Homebrew or winget), git, AWS CLI, in that order
	Ready     int         `json:"ready"`    // rows that are ok
	Total     int         `json:"total"`
	Blocked   bool        `json:"blocked"` // true: the setup screen replaces the window
	Preflight Preflight   `json:"preflight"`
	CheckedAt string      `json:"checkedAt"`
}

// InstallStepKind is what a step does.
type InstallStepKind string

const (
	StepDownload InstallStepKind = "download" // Homebrew.pkg
	StepVerify   InstallStepKind = "verify"   // unprivileged pre-check: size, digest, signature, Team ID
	StepInstall  InstallStepKind = "install"  // pkg (one privileged step), brew install, winget install
)

type InstallStepState string

const (
	StepQueued    InstallStepState = "queued"
	StepRunning   InstallStepState = "running"
	StepWaitingOS InstallStepState = "waiting-os" // the operating system's own password / UAC window is up
	StepDone      InstallStepState = "done"
	StepFailed    InstallStepState = "failed"
	StepSkipped   InstallStepState = "skipped"
)

// InstallStep is one line of progress. The pkg flow of one row has three steps (download, verify, install).
type InstallStep struct {
	ID      string           `json:"id"` // "homebrew:download", "git:install", ...
	Tool    SetupToolID      `json:"tool"`
	Kind    InstallStepKind  `json:"kind"`
	State   InstallStepState `json:"state"`
	Command string           `json:"command,omitempty"` // the exact command shown on the row, e.g. "brew install git"
	// Indeterminate is true for the privileged step: it cannot report progress while it runs.
	Indeterminate bool  `json:"indeterminate,omitempty"`
	Done          int64 `json:"done,omitempty"`  // bytes downloaded so far
	Total         int64 `json:"total,omitempty"` // bytes expected; 0 when unknown
}

// FailureCause is the stable code of one failure, one per sentence of design 5.1.
type FailureCause string

const (
	CauseNotAdmin        FailureCause = "not_admin"        // the account is not an administrator
	CauseCancelled       FailureCause = "cancelled"        // the OS password / UAC window was cancelled
	CauseAuthDenied      FailureCause = "auth_denied"      // the name and password were refused
	CauseNoSession       FailureCause = "no_gui_session"   // no window to ask in
	CauseNetwork         FailureCause = "network"          // timeout, DNS, connection, TLS
	CauseProxyAuth       FailureCause = "proxy_auth"       // the proxy wants a sign-in (HTTP 407)
	CausePolicy          FailureCause = "policy"           // device management / group policy
	CauseDiskFull        FailureCause = "disk_full"        // about 1.2 GB needed
	CauseDamagedDownload FailureCause = "damaged_download" // size or digest differs, or a proxy returned a web page
	CauseBadSignature    FailureCause = "bad_signature"    // the package is not signed for distribution, or is broken
	CauseUnknownSigner   FailureCause = "unknown_signer"   // signed by a team this Codepass does not know
	CauseNotNotarized    FailureCause = "not_notarized"    // Apple's check did not pass
	CausePrefixExists    FailureCause = "prefix_exists"    // /opt/homebrew exists
	CauseWingetMissing   FailureCause = "winget_missing"   // Windows: no winget
	CauseNeedsAdmin      FailureCause = "needs_admin"      // winget: the command needs an administrator
	CauseReboot          FailureCause = "reboot_required"  // Windows: restart, then check again
	CauseInstallerFailed FailureCause = "installer_failed" // a known step failed for an unknown reason: show the log
	CauseUnknown         FailureCause = "unknown"
)

// InstallFailure describes why the run stopped.
type InstallFailure struct {
	Tool    SetupToolID  `json:"tool"`
	StepID  string       `json:"stepId"`
	Cause   FailureCause `json:"cause"`
	Command string       `json:"command,omitempty"`
	// ExitCode is the tool's own exit code (the pkg script's code, brew's, winget's); -1 when none.
	ExitCode int `json:"exitCode"`
	// Proxy is filled for network causes so the screen can say "using the system proxy host:port".
	Proxy string `json:"proxy,omitempty"`
	// Line is the last meaningful line of the tool's own output, never a path of the user's data.
	Line string `json:"line,omitempty"`
}

// InstallState is the snapshot the setup screen draws. It is the payload of EventToolInstall and the answer of
// ToolService.GetInstallState (so a reloaded window can recover).
type InstallState struct {
	Running    bool            `json:"running"`
	Cancelling bool            `json:"cancelling"` // cancel was asked; the run stops after the current step
	Cancelled  bool            `json:"cancelled"`  // the run stopped because of Cancel (no failure)
	Steps      []InstallStep   `json:"steps"`
	Log        []string        `json:"log"` // the last lines of real output, newest last (at most 200)
	Failure    *InstallFailure `json:"failure,omitempty"`
	Finished   bool            `json:"finished"` // the run ended (success, failure or cancel)
}
