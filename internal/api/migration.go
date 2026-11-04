package api

type MigrationStep string

const (
	MigrationIdentify   MigrationStep = "identify"
	MigrationPortal     MigrationStep = "portal"
	MigrationAccount    MigrationStep = "account"
	MigrationReview     MigrationStep = "review"
	MigrationVerify     MigrationStep = "verify"
	MigrationRemoveKey  MigrationStep = "remove-key"
	MigrationDeactivate MigrationStep = "deactivate"
	MigrationDone       MigrationStep = "done"
)

type MigrationAccountChoice struct {
	AccountID    string `json:"accountId"`
	AccountName  string `json:"accountName"`
	Role         string `json:"role"`
	Region       string `json:"region"` // "" keeps the profile's current region
	ConfirmOther bool   `json:"confirmOther"`
}

type VerifyResult struct {
	OK        bool      `json:"ok"`
	AccountID string    `json:"accountId,omitempty"`
	Arn       string    `json:"arn,omitempty"`
	Error     *AppError `json:"error,omitempty"`
}

type DeactivationStatus string

const (
	DeactivationDone        DeactivationStatus = "deactivated"
	DeactivationAlready     DeactivationStatus = "already-inactive"
	DeactivationDenied      DeactivationStatus = "denied"
	DeactivationRootKey     DeactivationStatus = "root-key"
	DeactivationKeyNotFound DeactivationStatus = "key-not-found"
	DeactivationUnavailable DeactivationStatus = "unavailable"
)

type DeactivationMethod string

const (
	DeactivateViaSSORole DeactivationMethod = "sso-role"
	DeactivateViaOldKey  DeactivationMethod = "old-key"
)

type DeactivationResult struct {
	Status     DeactivationStatus `json:"status"`
	Method     DeactivationMethod `json:"method,omitempty"`
	ConsoleURL string             `json:"consoleUrl"` // portal shortcut link to the IAM user page (no secrets)
	Error      *AppError          `json:"error,omitempty"`
}

type MigrationState struct {
	Profile             string              `json:"profile"`
	Step                MigrationStep       `json:"step"`
	Identity            StaticIdentity      `json:"identity"`
	KeyInMemory         bool                `json:"keyInMemory"`
	Portal              string              `json:"portal,omitempty"`
	PortalIsDraft       bool                `json:"portalIsDraft"`
	AccountID           string              `json:"accountId,omitempty"`
	AccountName         string              `json:"accountName,omitempty"`
	Role                string              `json:"role,omitempty"`
	Region              string              `json:"region,omitempty"`
	ConfirmOther        bool                `json:"confirmOther"`
	ConfigPreview       *FilePreview        `json:"configPreview,omitempty"`
	CredentialsPreview  *FilePreview        `json:"credentialsPreview,omitempty"`
	Verify              *VerifyResult       `json:"verify,omitempty"`
	ConfigBackupID      string              `json:"configBackupId,omitempty"`
	KeyRemoved          bool                `json:"keyRemoved"`
	CredentialsBackupID string              `json:"credentialsBackupId,omitempty"`
	Deactivation        *DeactivationResult `json:"deactivation,omitempty"`
	LastError           *AppError           `json:"lastError,omitempty"`
}

type MigrationItem string

const (
	MigrationItemBackup MigrationItem = "backup"
	MigrationItemWrite  MigrationItem = "write"
	MigrationItemVerify MigrationItem = "verify"
)

type MigrationProgress struct {
	Profile string         `json:"profile"`
	Item    MigrationItem  `json:"item"`
	Status  ProgressStatus `json:"status"`
	Detail  string         `json:"detail,omitempty"` // backup display path, or "account • arn" after verify
}
