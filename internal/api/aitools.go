package api

// AI coding tools (plan 4.7, 4.8): which of Claude Code, Codex and OpenCode are installed, the exact command that
// installs a missing one, the task's name from the tool, and the approval or sandbox mode shown in a task's
// header. internal/aitools builds all of it; these are the types the window sees.

// EventAIToolInstall carries an AIInstallState snapshot every time an install run changes. The wiring task
// registers it in internal/app/events.go (RegisterEvent[api.AIInstallState]) and adds 'aitools:install' to
// EventPayloads in frontend/src/api/events.ts.
const EventAIToolInstall = "aitools:install"

// AIToolID names one AI coding tool.
type AIToolID string

const (
	AIToolClaude   AIToolID = "claude"
	AIToolCodex    AIToolID = "codex"
	AIToolOpenCode AIToolID = "opencode"
)

// AIToolVia says how the program was found.
type AIToolVia string

const (
	AIViaNone     AIToolVia = ""         // not found
	AIViaFile     AIToolVia = "file"     // an executable on the login shell's PATH
	AIViaFunction AIToolVia = "function" // a shell function of the person's own: it still applies (an alias cannot run and is reported as not installed)
)

// AIToolInfo is one row of the AI tools list.
type AIToolInfo struct {
	ID        AIToolID  `json:"id"`
	Name      string    `json:"name"` // "Claude Code", "Codex", "OpenCode"
	Installed bool      `json:"installed"`
	Version   string    `json:"version,omitempty"`
	Path      string    `json:"path,omitempty"` // empty for a shell function
	Via       AIToolVia `json:"via"`
	// InstallCommand is the exact command "Install and start" runs on this system, shown before it runs.
	InstallCommand string `json:"installCommand"`
	// Note is a short English reason when detection failed in an unusual way (a timeout, a shell that would not
	// start). It is for the log line under the row, never a sentence the window must translate.
	Note string `json:"note,omitempty"`
}

// AIToolsState is the answer of AIToolService.Detect.
type AIToolsState struct {
	GOOS  string       `json:"goos"`
	Tools []AIToolInfo `json:"tools"`
}

// AIInstallCause is why an install run failed.
type AIInstallCause string

const (
	AICauseInstallerMissing AIInstallCause = "installer-missing" // brew, winget or npm is not there
	AICauseFailed           AIInstallCause = "failed"            // the installer exited with an error
	AICauseStillMissing     AIInstallCause = "still-missing"     // it succeeded but the tool is not found afterwards
	AICauseCancelled        AIInstallCause = "cancelled"
)

// AIInstallFailure is the failed end of a run.
type AIInstallFailure struct {
	Cause  AIInstallCause `json:"cause"`
	Detail string         `json:"detail,omitempty"` // the installer's own error line
	Exit   int            `json:"exit"`
}

// AIInstallState is the snapshot of the one install run (payload of EventAIToolInstall).
type AIInstallState struct {
	Tool       AIToolID          `json:"tool"`
	Command    string            `json:"command"`
	Running    bool              `json:"running"`
	Cancelling bool              `json:"cancelling"`
	Cancelled  bool              `json:"cancelled"`
	Finished   bool              `json:"finished"`
	Log        []string          `json:"log"` // the last lines of the installer's output, newest last (at most 200)
	Failure    *AIInstallFailure `json:"failure,omitempty"`
	// Installed is the tool's row after a successful run.
	Installed *AIToolInfo `json:"installed,omitempty"`
}

// AIApprovalKind is the vocabulary a tool uses (plan 4.8).
type AIApprovalKind string

const (
	AIApprovalPermissionMode AIApprovalKind = "permission-mode"  // Claude Code
	AIApprovalPolicySandbox  AIApprovalKind = "approval-sandbox" // Codex
	AIApprovalRules          AIApprovalKind = "permission-rules" // OpenCode
)

// AIApproval is what a task's header shows, read-only. Unknown fields are empty strings; the app never sets any
// of these values and never passes a flag that skips a prompt.
type AIApproval struct {
	Tool AIToolID       `json:"tool"`
	Kind AIApprovalKind `json:"kind"`
	// Mode: Claude Code's permission mode (manual, acceptEdits, auto, plan, dontAsk, bypassPermissions); for
	// OpenCode a summary of the rules (allow, ask, deny, mixed); empty for Codex (see Approval and Sandbox).
	Mode string `json:"mode"`
	// Approval and Sandbox are Codex's approval policy (on-request, never, ...) and sandbox mode (read-only,
	// workspace-write, danger-full-access).
	Approval string `json:"approval"`
	Sandbox  string `json:"sandbox"`
	Known    bool   `json:"known"`
	// Source is "hook" (the value of the last event), "config" (read from the tool's own config file) or "".
	Source string `json:"source"`
}

// AINaming is the answer of the naming call (plan 4.7 "Naming").
type AINaming struct {
	Title string `json:"title"` // at most 40 characters, in the description's language
	Slug  string `json:"slug"`  // English kebab-case, at most 40 characters; the branch is ai/<slug>
	// FromAI is true when the title came from the tool's answer; false for the quiet fallback.
	FromAI bool `json:"fromAI"`
	// Reason is why the fallback was used ("", "empty", "timeout", "missing", "bad-answer", "failed").
	Reason string `json:"reason"`
}
