// Package taskstate carries the "what is the AI tool doing" signals of plan 4.8 from the tool's hooks to the app.
//
// The pieces:
//
//   - report.go / sanitize.go: the one wire message (Report) and the code that reduces a hook payload to it.
//     A hook payload holds the prompt, tool input and the last assistant message; Sanitize keeps only fixed
//     fields and values from fixed lists, so nothing else can reach the socket, the tracker or a log.
//   - server.go and transport_*.go: a local socket (Unix) or named pipe (Windows) that only the same user can
//     use.
//   - tracker.go: the per-task state machine (events to state, per tool).
//   - fallback.go: "alive or exited" from the terminal's foreground process when no hook exists.
//   - inject_*.go: what the launcher injects per launch, never into the user's own settings.
//
// Payloads are never logged. The only free text that ever travels is a prompt: report mode is stateless, so
// with EnvNamePrompt set every UserPromptSubmit of that task carries its prompt over the socket for the whole
// session; the tracker hands the first one to the armed naming call and drops it and all later ones unstored.
package taskstate

import (
	"regexp"
	"strings"
)

// Tool names a supported AI tool.
type Tool string

const (
	ToolClaude   Tool = "claude"
	ToolCodex    Tool = "codex"
	ToolOpenCode Tool = "opencode"
)

// Environment variables the launcher sets in the child's environment (plan 4.8).
const (
	EnvTask       = "CODEPASS_TASK"        // task ID; report mode reads it
	EnvSocket     = "CODEPASS_SOCK"        // endpoint of the app's signal socket
	EnvNamePrompt = "CODEPASS_NAME_PROMPT" // "1": forward the first prompt once (empty description)
	EnvQueue      = "CODEPASS_QUEUE"       // "1": a queued ticket; report mode computes the end marker
)

// Report is the whole of what report mode forwards. Every field is an identifier or a value from a fixed
// list; there is no free text except Prompt, which exists only for the one-shot naming call.
type Report struct {
	Task             string `json:"task"`
	Tool             Tool   `json:"tool"`
	Event            string `json:"event"`
	Session          string `json:"session,omitempty"`
	Mode             string `json:"mode,omitempty"`             // permission mode, from PermissionModes
	ErrorKind        string `json:"errorKind,omitempty"`        // StopFailure: from ErrorKinds
	NotificationType string `json:"notificationType,omitempty"` // Notification: from NotificationTypes
	EndMarker        string `json:"endMarker,omitempty"`        // queued ticket, Stop: done | question | none
	ConvMark         string `json:"convMark,omitempty"`         // StopFailure: conversation | none
	Source           string `json:"source,omitempty"`           // SessionStart: from Sources
	At               int64  `json:"at"`                         // Unix nanoseconds, set by report mode
	Prompt           string `json:"prompt,omitempty"`           // only when armed; the tracker hands it on once and drops it
}

// Values from fixed lists (plan 4.8). Anything else becomes OtherValue.
const OtherValue = "other"

var (
	ErrorKinds = []string{"rate_limit", "overloaded", "authentication_failed", "billing_error", "server_error", OtherValue}
	// NotificationTypes: the quota_* ones are the signals of Claude Code's own wait at a usage limit (R4).
	NotificationTypes = []string{"permission_prompt", "idle_prompt", "elicitation_dialog",
		"quota_auto_resume_fired", "quota_auto_resume_stale", "quota_auto_resume_disabled", OtherValue}
	Sources         = []string{"startup", "resume", "clear", "compact", OtherValue}
	PermissionModes = []string{"manual", "default", "acceptEdits", "auto", "plan", "dontAsk", "bypassPermissions",
		"on-request", "never", "read-only", "workspace-write", "danger-full-access", OtherValue}
)

// End markers of a queued ticket (T8-04): the last line of the last assistant message, compared locally.
const (
	MarkerDone     = "CODEPASS-DONE"
	MarkerQuestion = "CODEPASS-QUESTION"
)

// End marker values.
const (
	EndDone     = "done"
	EndQuestion = "question"
	EndNone     = "none"
)

// Conversation mark values (4.16 decisions 22 and 34).
const (
	ConvConversation = "conversation"
	ConvNone         = "none"
)

// ConversationErrorStrings are the four fixed strings that tell an error about the conversation from an error
// about the account. Read from Claude Code 2.1.x; R8's checks confirm them. Kept in one place on purpose.
var ConversationErrorStrings = []string{
	"Invalid `signature` in `thinking` block",
	"Invalid `data` in `redacted_thinking` block",
	"tool_use ids were found without tool_result",
	"Prompt is too long",
}

// Event names per tool; anything else is dropped.
var (
	ClaudeEvents = []string{"SessionStart", "UserPromptSubmit", "Notification", "PermissionRequest", "Stop",
		"StopFailure", "SessionEnd", "PreToolUse"}
	// Codex hooks use the same names; notify reports one event.
	CodexEvents    = []string{"SessionStart", "UserPromptSubmit", "PermissionRequest", "Stop", "SessionEnd", EventCodexTurnComplete}
	OpenCodeEvents = []string{EventPluginLoaded, "session.execution.started", "session.execution.succeeded", "session.execution.failed",
		"session.execution.interrupted", "permission.asked", "permission.replied", "session.idle"}
)

// EventPluginLoaded is sent once by the OpenCode plugin's setup: OpenCode has no session-start event, and a
// loaded plugin proves the tool's signals work (so the task is not shown as "running (no signal)").
const EventPluginLoaded = "plugin.loaded"

// EventCodexTurnComplete is the one event of Codex's notify.
const EventCodexTurnComplete = "agent-turn-complete"

// QuestionTool is the only tool whose PreToolUse hook is installed (matcher exactly this name).
const QuestionTool = "AskUserQuestion"

const (
	maxPrompt  = 4000
	maxIDLen   = 128
	maxWireLen = 64 << 10
)

var idRe = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)

func validID(s string) bool { return len(s) > 0 && len(s) <= maxIDLen && idRe.MatchString(s) }

func oneOf(list []string, v string) string {
	for _, x := range list {
		if x == v {
			return v
		}
	}
	return OtherValue
}

func eventAllowed(t Tool, ev string) bool {
	var list []string
	switch t {
	case ToolClaude:
		list = ClaudeEvents
	case ToolCodex:
		list = CodexEvents
	case ToolOpenCode:
		list = OpenCodeEvents
	}
	for _, x := range list {
		if x == ev {
			return true
		}
	}
	return false
}

// Normalize applies the same reduction to a Report that arrived over the wire (the server never trusts the
// sender): IDs must match a safe pattern, enums are clamped to their lists, the prompt is kept only when
// wantPrompt, and only for the event that carries it. ok is false when the report cannot be used.
func Normalize(r Report, wantPrompt bool) (Report, bool) {
	if !validID(r.Task) || !eventAllowed(r.Tool, r.Event) {
		return Report{}, false
	}
	out := Report{Task: r.Task, Tool: r.Tool, Event: r.Event, At: r.At}
	if validID(r.Session) {
		out.Session = r.Session
	}
	if r.Mode != "" {
		out.Mode = oneOf(PermissionModes, r.Mode)
	}
	if r.ErrorKind != "" {
		out.ErrorKind = oneOf(ErrorKinds, r.ErrorKind)
	}
	if r.NotificationType != "" {
		out.NotificationType = oneOf(NotificationTypes, r.NotificationType)
	}
	switch r.EndMarker {
	case EndDone, EndQuestion, EndNone:
		out.EndMarker = r.EndMarker
	}
	switch r.ConvMark {
	case ConvConversation, ConvNone:
		out.ConvMark = r.ConvMark
	}
	if r.Source != "" {
		out.Source = oneOf(Sources, r.Source)
	}
	if wantPrompt && r.Event == "UserPromptSubmit" && r.Prompt != "" {
		p := strings.ToValidUTF8(r.Prompt, "")
		if rs := []rune(p); len(rs) > maxPrompt {
			p = string(rs[:maxPrompt])
		}
		out.Prompt = p
	}
	return out, true
}
