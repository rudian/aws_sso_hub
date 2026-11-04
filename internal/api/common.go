package api

type DiffKind string

const (
	DiffContext DiffKind = "ctx"
	DiffAdded   DiffKind = "add"
	DiffRemoved DiffKind = "del"
	DiffFolded  DiffKind = "fold"
)

type DiffLine struct {
	Kind  DiffKind `json:"kind"`
	Text  string   `json:"text"`            // already redacted/masked
	Count int      `json:"count,omitempty"` // number of folded unchanged lines (Kind == fold)
}

// FilePreview is shown before every write. BaseHash must be passed back to the write call.
type FilePreview struct {
	File      string     `json:"file"`     // display path, "~/.aws/config"
	Path      string     `json:"path"`     // absolute path
	BaseHash  string     `json:"baseHash"` // sha256 hex of current content, "" if the file does not exist
	Lines     []DiffLine `json:"lines"`
	Added     int        `json:"added"`
	Removed   int        `json:"removed"`
	BackupDir string     `json:"backupDir"` // display path of the backup folder
}

type WriteResult struct {
	File     string `json:"file"`
	BackupID string `json:"backupId"`
}

type ProgressStatus string

const (
	ProgressRunning ProgressStatus = "running"
	ProgressDone    ProgressStatus = "done"
	ProgressFailed  ProgressStatus = "failed"
)
