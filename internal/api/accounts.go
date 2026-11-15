package api

type ProfileKind string

const (
	ProfileKindSSO       ProfileKind = "sso"
	ProfileKindLegacySSO ProfileKind = "legacy-sso"
	ProfileKindStatic    ProfileKind = "static"
	ProfileKindOther     ProfileKind = "other"
)

type StaticState string

const (
	StaticUnverified StaticState = "unverified"
	StaticOK         StaticState = "ok"
	StaticInvalid    StaticState = "invalid"
	StaticKeysLeft   StaticState = "migrated-keys-left"
)

type SessionState string

const (
	SessionActive      SessionState = "active"
	SessionExpiring    SessionState = "expiring"
	SessionRefreshable SessionState = "refreshable"
	SessionExpired     SessionState = "expired"
	SessionSignedOut   SessionState = "signed-out"
	SessionSigningIn   SessionState = "signing-in"
	SessionError       SessionState = "error"
)

type SessionStatus struct {
	Portal         string       `json:"portal"`
	State          SessionState `json:"state"`
	ExpiresAt      string       `json:"expiresAt,omitempty"`     // ACCESS TOKEN expiry (~1 h, renews itself). Details only, never the main countdown
	IssuedAt       string       `json:"issuedAt,omitempty"`      // when this app last wrote/refreshed the access token (record TokenIssuedAt)
	SessionKnown   bool         `json:"sessionKnown"`            // this app did the sign-in that produced the cached token (clientId lineage matches)
	LoginAt        string       `json:"loginAt,omitempty"`       // interactive sign-in time; only when SessionKnown
	SessionEndsAt  string       `json:"sessionEndsAt,omitempty"` // LoginAt + SessionMinutes (estimate); only when SessionKnown
	SessionMinutes int          `json:"sessionMinutes"`          // per-portal setting, default 480 (8 h), range 15..129600
	SessionOverrun bool         `json:"sessionOverrun"`          // SessionKnown, now > SessionEndsAt, token still valid or renewable
	ProgressPct    int          `json:"progressPct"`             // 0..100 = session left / SessionMinutes; 0 when !SessionKnown or SessionOverrun (UI hides the bar)
	CanRefresh     bool         `json:"canRefresh"`
	Refreshing     bool         `json:"refreshing"`
	Error          *AppError    `json:"error,omitempty"` // set when State == error, or the last refresh hit a network error
}

// SessionRecord is the per-portal record kept in settings.json under "sessions". Never contains secrets.
type SessionRecord struct {
	LoginAt        string `json:"loginAt,omitempty"`        // RFC 3339 UTC; set after a successful interactive sign-in by this app
	ClientIDHash   string `json:"clientIdHash,omitempty"`   // first 16 hex chars of sha256(clientId) of the token this app wrote at sign-in
	TokenIssuedAt  string `json:"tokenIssuedAt,omitempty"`  // last time this app wrote/refreshed the token (replaces the old sessionIssuedAt)
	SessionMinutes int    `json:"sessionMinutes,omitempty"` // 0 = default 480
}

type Portal struct {
	Name         string        `json:"name"` // sso-session name; legacy pseudo-portal: "legacy:" + StartURLHost
	StartURL     string        `json:"startUrl"`
	StartURLHost string        `json:"startUrlHost"` // URL without scheme, e.g. "d-1234567890.awsapps.com/start"
	Region       string        `json:"region"`
	LegacyFormat bool          `json:"legacyFormat"`
	Draft        bool          `json:"draft"` // not yet written to ~/.aws/config
	Profiles     []string      `json:"profiles"`
	Session      SessionStatus `json:"session"`
}

type Profile struct {
	Name                string      `json:"name"`
	Kind                ProfileKind `json:"kind"`
	Portal              string      `json:"portal,omitempty"`
	AccountID           string      `json:"accountId,omitempty"`
	AccountName         string      `json:"accountName,omitempty"`
	Role                string      `json:"role,omitempty"`
	Region              string      `json:"region,omitempty"`
	ColorSlot           int         `json:"colorSlot"` // 1..8
	Monogram            string      `json:"monogram"`  // 2 chars (or 1 CJK char)
	StaticState         StaticState `json:"staticState,omitempty"`
	StaticError         string      `json:"staticError,omitempty"` // AWS error code, e.g. InvalidClientTokenId
	MaskedKeyID         string      `json:"maskedKeyId,omitempty"`
	IAMUser             string      `json:"iamUser,omitempty"`
	IsRootKey           bool        `json:"isRootKey"`
	HasStaticKeys       bool        `json:"hasStaticKeys"`
	BoundRepos          int         `json:"boundRepos"`
	IsGlobalDefault     bool        `json:"isGlobalDefault"`
	GlobalDefaultRepos  int         `json:"globalDefaultRepos"`
	MigrationInProgress bool        `json:"migrationInProgress"`
}

type ConfigIssue struct {
	File    string `json:"file"` // display path
	Line    int    `json:"line"`
	Message string `json:"message"`
}

type AccountsState struct {
	ConfigFile      string        `json:"configFile"`      // display path
	CredentialsFile string        `json:"credentialsFile"` // display path
	ReadOnly        bool          `json:"readOnly"`        // true when a parse issue blocks writes
	Issues          []ConfigIssue `json:"issues"`
	Portals         []Portal      `json:"portals"`  // real portals by name, then drafts, then legacy pseudo-portals
	Profiles        []Profile     `json:"profiles"` // by name
	StaticCount     int           `json:"staticCount"`
}

type PortalInput struct {
	Name     string `json:"name"`
	StartURL string `json:"startUrl"`
	Region   string `json:"region"`
}

type PortalInputCheck struct {
	Name     string               `json:"name"`     // given or suggested
	StartURL string               `json:"startUrl"` // normalized
	Region   string               `json:"region"`   // given or guessed ("" if unknown)
	Errors   map[string]ErrorCode `json:"errors"`   // keys: "name" | "startUrl" | "region"; empty when valid
}

type LoginMode string

const (
	LoginModeBrowser LoginMode = "browser"
	LoginModeDevice  LoginMode = "device"
)

type LoginPhase string

const (
	LoginPhaseStarting   LoginPhase = "starting"
	LoginPhaseWaiting    LoginPhase = "waiting"
	LoginPhaseDevice     LoginPhase = "device"
	LoginPhaseExchanging LoginPhase = "exchanging"
)

type LoginProgress struct {
	Portal                  string     `json:"portal"`
	Phase                   LoginPhase `json:"phase"`
	AuthorizeURL            string     `json:"authorizeUrl,omitempty"` // not secret (client id, PKCE challenge, state)
	BrowserOpened           bool       `json:"browserOpened"`
	PrivateWindow           bool       `json:"privateWindow"` // the sign-in page was opened in a private browser window
	UserCode                string     `json:"userCode,omitempty"`
	VerificationURI         string     `json:"verificationUri,omitempty"`
	VerificationURIComplete string     `json:"verificationUriComplete,omitempty"`
	ExpiresAt               string     `json:"expiresAt,omitempty"` // end of the authorization window
}

type LoginResult struct {
	Portal    string   `json:"portal"`
	ExpiresAt string   `json:"expiresAt"`
	Profiles  []string `json:"profiles"`
}

type LogoutResult struct {
	Portal            string `json:"portal"`
	DeletedRoleCaches int    `json:"deletedRoleCaches"`
	ServerLogout      bool   `json:"serverLogout"`
}

type DiscoveredRole struct {
	Name             string `json:"name"`
	ExistingProfile  string `json:"existingProfile,omitempty"` // a profile already points at (account, role)
	SuggestedProfile string `json:"suggestedProfile"`
}

type DiscoveredAccount struct {
	AccountID   string           `json:"accountId"`
	AccountName string           `json:"accountName"`
	Email       string           `json:"email"`
	Roles       []DiscoveredRole `json:"roles"`
	RolesError  *AppError        `json:"rolesError,omitempty"`
	ColorSlot   int              `json:"colorSlot"`
}

type NewProfileSpec struct {
	Name        string `json:"name"`
	AccountID   string `json:"accountId"`
	AccountName string `json:"accountName"`
	Role        string `json:"role"`
	Region      string `json:"region"`
}

type CreateProfilesRequest struct {
	Portal   string           `json:"portal"` // existing or draft portal
	Profiles []NewProfileSpec `json:"profiles"`
}

type CreateProfilesResult struct {
	BackupID string   `json:"backupId"`
	Profiles []string `json:"profiles"`
}

type IdentityUpdate struct {
	Profile   string `json:"profile"`
	ColorSlot int    `json:"colorSlot"` // 1..8; applies to the profile's account
	Monogram  string `json:"monogram"`  // "" = automatic
}

type StaticIdentity struct {
	Profile     string `json:"profile"`
	AccountID   string `json:"accountId,omitempty"`
	Arn         string `json:"arn,omitempty"`
	IAMUser     string `json:"iamUser,omitempty"`
	MaskedKeyID string `json:"maskedKeyId,omitempty"`
	IsRoot      bool   `json:"isRoot"`
	ErrorCode   string `json:"errorCode,omitempty"` // AWS error code when the key was rejected
}
