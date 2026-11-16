package api

// Event names. Payload types are fixed by the RegisterEvent calls in internal/app/events.go.
const (
	EventAccountsChanged   = "accounts:changed"      // AccountsState
	EventSessionChanged    = "session:changed"       // SessionStatus
	EventSessionRenewed    = "session:renewed"       // SessionStatus
	EventSessionReminder   = "session:reminder"      // SessionStatus; the sign-in heads-up (once per sign-in, lead time left)
	EventSessionEnded      = "session:ended"         // SessionStatus; the sign-in has ended (once per sign-in, F1-22)
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
