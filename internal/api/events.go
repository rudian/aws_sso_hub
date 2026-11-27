package api

// Event names. Payload types are fixed by the RegisterEvent calls in internal/app/events.go.
const (
	EventAccountsChanged   = "accounts:changed"      // AccountsState
	EventSessionChanged    = "session:changed"       // SessionStatus
	EventSessionRenewed    = "session:renewed"       // SessionStatus
	EventLoginProgress     = "login:progress"        // LoginProgress
	EventMigrationProgress = "migration:progress"    // MigrationProgress
	EventReposChanged      = "repos:changed"         // ReposState
	EventScanProgress      = "repos:scan-progress"   // ScanProgress
	EventCloneProgress     = "clone:progress"        // CloneProgress
	EventExternalChange    = "files:external-change" // ExternalChange
	EventSettingsChanged   = "settings:changed"      // Settings
	EventNavigate          = "app:navigate"          // NavigateRequest (tray, second instance)
	EventSecondInstance    = "app:second-instance"   // []string
)
