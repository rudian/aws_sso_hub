package api

// Terminal data path (A2-02). The page pulls bytes by offset; the event is only a small "new data up to
// offset N" ping (plan 5, "Terminal data path"). The names and fields below are the contract shared with the
// terminal component (A2-07); do not rename them.

// EventTerminalPing is emitted from the session's own goroutine, coalesced to at most one pending per
// session. Payload: TerminalPing. Registered in internal/app/events.go by the wiring task.
const EventTerminalPing = "terminal:ping"

// TerminalRead is the reply of TerminalService.Read.
type TerminalRead struct {
	Data string `json:"data"` // base64 of the bytes in [From, Next)
	From int64  `json:"from"` // absolute offset of the first byte of Data
	Next int64  `json:"next"` // offset to ask for next
	Tail int64  `json:"tail"` // current end of the log
	// Gap is the number of bytes skipped because the requested offset had already been trimmed (0 = none).
	Gap int64 `json:"gap"`
	// Done: the session ended and every byte has been read (Next >= Tail).
	Done bool `json:"done"`
	// Prologue (base64) is set only when Gap > 0: the terminal modes as of the new start of the log, as set
	// and reset sequences, to write after resetting the page's terminal and before Data.
	Prologue string `json:"prologue,omitempty"`
}

// TerminalPing is the payload of EventTerminalPing.
type TerminalPing struct {
	Session string `json:"session"`
	Tail    int64  `json:"tail"`
	Done    bool   `json:"done"`
}
