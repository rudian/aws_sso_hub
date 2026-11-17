package taskstate

import (
	"encoding/json"
	"strings"
	"time"
)

// SanitizeOptions says what the launcher armed for this task.
type SanitizeOptions struct {
	Task       string
	Queue      bool // EnvQueue: compute the end marker
	WantPrompt bool // EnvNamePrompt: forward the prompt of UserPromptSubmit
	Now        func() time.Time
}

// flexText decodes a JSON string and ignores any other value, so a payload field of an unexpected type is not
// an error.
type flexText string

func (f *flexText) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*f = flexText(s)
	}
	return nil
}

// hookPayload names the only fields of a Claude Code / Codex hook payload this package looks at. The others
// (tool_input, tool_response, transcript_path, cwd, ...) are never decoded.
type hookPayload struct {
	Event            string   `json:"hook_event_name"`
	Session          string   `json:"session_id"`
	PermissionMode   string   `json:"permission_mode"`
	Source           string   `json:"source"`
	Error            flexText `json:"error"`
	NotificationType flexText `json:"notification_type"`
	ToolName         flexText `json:"tool_name"`
	Prompt           flexText `json:"prompt"`
	LastMessage      flexText `json:"last_assistant_message"`
	ErrorDetails     flexText `json:"error_details"`
}

// Sanitize reduces one hook payload (JSON on stdin) of the given tool to a Report. Nothing of the payload but
// the Report survives this call. ok is false for an event that is not subscribed, a payload that is not
// JSON, or a PreToolUse for any tool but AskUserQuestion.
func Sanitize(tool Tool, payload []byte, o SanitizeOptions) (Report, bool) {
	switch tool {
	case ToolClaude, ToolCodex:
		return sanitizeHook(tool, payload, o)
	case ToolOpenCode:
		return sanitizeOpenCode(payload, o)
	}
	return Report{}, false
}

// SanitizeCodexNotify reduces the argument of Codex's notify (one JSON object in argv).
func SanitizeCodexNotify(arg []byte, o SanitizeOptions) (Report, bool) {
	var p struct {
		Type     string `json:"type"`
		ThreadID string `json:"thread-id"`
	}
	if json.Unmarshal(arg, &p) != nil || p.Type != EventCodexTurnComplete {
		return Report{}, false
	}
	return finish(Report{Tool: ToolCodex, Event: EventCodexTurnComplete, Session: p.ThreadID}, o)
}

func sanitizeHook(tool Tool, payload []byte, o SanitizeOptions) (Report, bool) {
	var p hookPayload
	if json.Unmarshal(payload, &p) != nil {
		return Report{}, false
	}
	r := Report{Tool: tool, Event: p.Event, Session: p.Session, Mode: p.PermissionMode}
	switch p.Event {
	case "SessionStart":
		r.Source = p.Source
	case "UserPromptSubmit":
		if o.WantPrompt {
			r.Prompt = string(p.Prompt)
		}
	case "Notification":
		r.NotificationType = string(p.NotificationType)
		if r.NotificationType == "" {
			r.NotificationType = OtherValue
		}
	case "PreToolUse":
		if string(p.ToolName) != QuestionTool {
			return Report{}, false
		}
	case "Stop":
		if o.Queue {
			r.EndMarker = endMarker(string(p.LastMessage))
		}
	case "StopFailure":
		r.ErrorKind = string(p.Error)
		if r.ErrorKind == "" {
			r.ErrorKind = OtherValue
		}
		r.ConvMark = convMark(string(p.ErrorDetails), string(p.LastMessage))
	}
	return finish(r, o)
}

func finish(r Report, o SanitizeOptions) (Report, bool) {
	r.Task = o.Task
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	r.At = now().UnixNano()
	return Normalize(r, o.WantPrompt)
}

// endMarker compares the last non-empty line with the two fixed words. The text itself goes nowhere.
func endMarker(last string) string {
	lines := strings.Split(strings.TrimRight(last, " \t\r\n"), "\n")
	line := strings.TrimSpace(lines[len(lines)-1])
	switch line {
	case MarkerDone:
		return EndDone
	case MarkerQuestion:
		return EndQuestion
	}
	return EndNone
}

func convMark(texts ...string) string {
	for _, t := range texts {
		for _, s := range ConversationErrorStrings {
			if strings.Contains(t, s) {
				return ConvConversation
			}
		}
	}
	return ConvNone
}

// sanitizeOpenCode reads {type, data:{sessionID}} (or durable.aggregateID); the plugin already sends less than
// that, and everything else (prompt text in session.inbox.enqueued, error messages) is not decoded.
func sanitizeOpenCode(payload []byte, o SanitizeOptions) (Report, bool) {
	var p struct {
		Type string `json:"type"`
		Data struct {
			SessionID string `json:"sessionID"`
			Error     struct {
				Type string `json:"type"`
			} `json:"error"`
		} `json:"data"`
		SessionID string `json:"sessionID"`
		AtMs      int64  `json:"atMs"` // the plugin's own clock when the event arrived
		Seq       int64  `json:"seq"`  // the plugin's counter: orders events within the same millisecond
		Durable   struct {
			AggregateID string `json:"aggregateID"`
		} `json:"durable"`
	}
	if json.Unmarshal(payload, &p) != nil {
		return Report{}, false
	}
	r := Report{Tool: ToolOpenCode, Event: p.Type, Session: firstNonEmpty(p.Data.SessionID, p.SessionID, p.Durable.AggregateID)}
	if p.Type == "session.execution.failed" {
		// The message is never read; only the kind of the error, reduced to the fixed list.
		if strings.Contains(strings.ToLower(p.Data.Error.Type), "auth") {
			r.ErrorKind = "authentication_failed"
		} else {
			r.ErrorKind = OtherValue
		}
	}
	r, ok := finish(r, o)
	if ok {
		// Report processes start in any order, so their own clocks do not order the events: the plugin's stamp does.
		now := time.Now
		if o.Now != nil {
			now = o.Now
		}
		ms := now().UnixMilli()
		if p.AtMs > ms-86400000 && p.AtMs < ms+86400000 && p.Seq >= 0 && p.Seq < 1_000_000 {
			r.At = p.AtMs*1_000_000 + p.Seq
		}
	}
	return r, ok
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}
