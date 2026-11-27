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
	AutoSignIn  bool `json:"autoSignIn"`  // open the sign-in page by itself before a session ends
	LeadMinutes int  `json:"leadMinutes"` // 10 | 15 | 30 | 60: how long before the end
	AutoRenew   bool `json:"autoRenew"`
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

	CollapsedPortals []string          `json:"collapsedPortals"` // portal names collapsed on the Accounts page
	ConsoleProxies   map[string]string `json:"consoleProxies"`   // portal name -> HTTP proxy (host:port) its console opens through
	LastCloneProfile string            `json:"lastCloneProfile"` // AWS profile of the last successful clone
	LastCloneRegion  string            `json:"lastCloneRegion"`  // region of the last successful clone
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

	CollapsedPortals []string          `json:"collapsedPortals,omitempty"` // nil = unchanged; empty slice = clear
	ConsoleProxies   map[string]string `json:"consoleProxies,omitempty"`   // nil = unchanged; sets the portals named ("" = off), keeps the others
	LastCloneProfile *string           `json:"lastCloneProfile,omitempty"`
	LastCloneRegion  *string           `json:"lastCloneRegion,omitempty"`
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
