package api

// Programs a session leaves behind (plan 4.8b, design 5.6a). Command lines are for display: the app never
// writes them to the log or to disk.

// LeftoverProcess is one tracked program.
type LeftoverProcess struct {
	PID     int     `json:"pid"`
	PPID    int     `json:"ppid"`              // parent PID (0 on Windows or after a crash restore); the UI counts the programs the tool started with it
	Name    string  `json:"name"`              // short command name, e.g. "vite"
	Command string  `json:"command,omitempty"` // the executable as the system prints it; empty for a program restored after a crash
	CPU     float64 `json:"cpu"`               // percent of one core
	Memory  int64   `json:"memory"`            // bytes
}

// SessionLeftovers is the tracked set of one session. A session with Live set still has its terminal and tool;
// without it the session is "closed, still running" (5.6a).
type SessionLeftovers struct {
	Session  string            `json:"session"`
	Leader   int               `json:"leader"` // PID of the terminal's shell; 0 when the session was restored after a crash. Programs with this PID or this PPID are the shell and the tool, not "programs it started"
	Live     bool              `json:"live"`
	ClosedAt string            `json:"closedAt,omitempty"` // RFC 3339; empty while Live
	Count    int               `json:"count"`
	CPU      float64           `json:"cpu"`
	Memory   int64             `json:"memory"`
	Procs    []LeftoverProcess `json:"procs"`
}

// ForceStopResult is what a Force stop did. Survivors holds what could not be stopped (another user's program,
// or one the system restarted); the UI shows each PID with "Copy PID".
type ForceStopResult struct {
	Session   string            `json:"session"`
	Found     int               `json:"found"`
	Stopped   int               `json:"stopped"`
	Names     []string          `json:"names"`
	Survivors []LeftoverProcess `json:"survivors"`
	Error     string            `json:"error,omitempty"` // set when the stop was not clean (the table could not be read, or a program survived and the system said so); Survivors is still filled
}

// ProcessCommand is the full command line of one tracked program, read on demand for the expanded list.
// Display only: the app never logs it, caches it or saves it.
type ProcessCommand struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
}

// EventProctrackChanged carries []SessionLeftovers whenever a session's set of programs, or whether it is
// live, changes (CPU and memory changes do not send it: the page refreshes those on a timer).
const EventProctrackChanged = "proctrack:changed"
