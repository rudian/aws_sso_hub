package api

// The update flow (design 5.13). The values of UpdatePhase mirror update.Phase; internal/app converts between them.
// TestUpdatePhasesMatchTheAPI checks each pair in its list; a new update.Phase must be added to that list (the
// test cannot find a phase that is not in it).

type UpdatePhase string

const (
	UpdateIdle        UpdatePhase = "idle"
	UpdateChecking    UpdatePhase = "checking"
	UpdateUpToDate    UpdatePhase = "up-to-date"
	UpdateAvailable   UpdatePhase = "available"   // a verified newer release exists; nothing downloaded yet
	UpdateDownloading UpdatePhase = "downloading" // Written and Total carry the progress
	UpdateVerifying   UpdatePhase = "verifying"
	UpdateReady       UpdatePhase = "ready" // staged and verified; waiting for Restart
	UpdateRestarting  UpdatePhase = "restarting"
	UpdateBlocked     UpdatePhase = "blocked" // this copy cannot replace itself (Block and Folder say why)
	UpdateRefused     UpdatePhase = "refused" // a release was found and refused (Code says why)
	UpdateError       UpdatePhase = "error"   // network or disk trouble
)

// UpdateState is one snapshot of the flow; it is also the payload of the update:state event. Notes come from the
// signed update file and are plain text. The UI words every failure from Code, Step and Block, in the user's language.
type UpdateState struct {
	Phase     UpdatePhase `json:"phase"`
	Current   string      `json:"current"`           // the running version
	Version   string      `json:"version,omitempty"` // the release found
	Notes     string      `json:"notes,omitempty"`
	Written   int64       `json:"written,omitempty"`
	Total     int64       `json:"total,omitempty"`
	Code      string      `json:"code,omitempty"`
	Step      string      `json:"step,omitempty"`   // "check" | "download" | "verify" | "restart"
	Block     string      `json:"block,omitempty"`  // "translocated" | "not-writable" | "unsupported-os"
	Folder    string      `json:"folder,omitempty"` // the folder to name in a "cannot update" message
	CheckedAt string      `json:"checkedAt,omitempty"`
}

// UpdatePreferences are the person's choices about the update check.
type UpdatePreferences struct {
	AutoCheck bool `json:"autoCheck"` // check once a day by itself (default true)
}
