package taskstate

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

var fixedNow = func() time.Time { return time.Unix(1700000000, 5) }

const secret = "SECRET-PROMPT-TEXT-do-not-forward"

func opts() SanitizeOptions { return SanitizeOptions{Task: "t1", Now: fixedNow} }

func TestPromptPayloadIsReducedToForwardedFields(t *testing.T) {
	payload := `{"session_id":"11111111-2222-3333-4444-111122223333","transcript_path":"/x/y.jsonl","cwd":"/Users/alex/acme",
	"hook_event_name":"UserPromptSubmit","prompt_id":"p1","permission_mode":"acceptEdits","prompt":"` + secret + `",
	"tool_input":{"command":"` + secret + `"},"last_assistant_message":"` + secret + `"}`
	r, ok := Sanitize(ToolClaude, []byte(payload), opts())
	if !ok {
		t.Fatal("not accepted")
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "SECRET") || strings.Contains(string(b), "acme") || strings.Contains(string(b), "transcript") {
		t.Fatalf("leaked: %s", b)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	allowed := map[string]bool{"task": true, "tool": true, "event": true, "session": true, "mode": true, "at": true}
	for k := range m {
		if !allowed[k] {
			t.Errorf("unexpected field %q", k)
		}
	}
	if r.Task != "t1" || r.Session != "11111111-2222-3333-4444-111122223333" || r.Mode != "acceptEdits" || r.Event != "UserPromptSubmit" {
		t.Fatalf("%+v", r)
	}
}

func TestPromptForwardedOnlyWhenArmedAndOnlyForThatEvent(t *testing.T) {
	o := opts()
	o.WantPrompt = true
	r, _ := Sanitize(ToolClaude, []byte(`{"hook_event_name":"UserPromptSubmit","prompt":"hello"}`), o)
	if r.Prompt != "hello" {
		t.Fatalf("armed: %+v", r)
	}
	r, _ = Sanitize(ToolClaude, []byte(`{"hook_event_name":"Stop","prompt":"hello","last_assistant_message":"x"}`), o)
	if r.Prompt != "" {
		t.Fatal("prompt on another event")
	}
	long := strings.Repeat("é", 9000)
	r, _ = Sanitize(ToolClaude, []byte(`{"hook_event_name":"UserPromptSubmit","prompt":"`+long+`"}`), o)
	if n := len([]rune(r.Prompt)); n != maxPrompt {
		t.Fatalf("cap: %d", n)
	}
}

func TestSanitizeDropsWhatIsNotSubscribed(t *testing.T) {
	for _, p := range []string{
		`not json`,
		`{"hook_event_name":"PostToolUse","session_id":"s"}`,
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","session_id":"s"}`,
		`{"hook_event_name":"PreToolUse","session_id":"s"}`,
		`{}`,
	} {
		if _, ok := Sanitize(ToolClaude, []byte(p), opts()); ok {
			t.Errorf("accepted %s", p)
		}
	}
	if _, ok := Sanitize(ToolClaude, []byte(`{"hook_event_name":"PreToolUse","tool_name":"AskUserQuestion","session_id":"s"}`), opts()); !ok {
		t.Error("AskUserQuestion dropped")
	}
	if _, ok := Sanitize("nope", []byte(`{}`), opts()); ok {
		t.Error("unknown tool")
	}
}

func TestEnumsAreClampedAndUnsafeIDsDropped(t *testing.T) {
	r, ok := Sanitize(ToolClaude, []byte(`{"hook_event_name":"StopFailure","error":"`+secret+`","error_details":"x","session_id":"bad id with spaces","permission_mode":"`+secret+`"}`), opts())
	if !ok || r.ErrorKind != "other" || r.Session != "" || r.Mode != "other" {
		t.Fatalf("%+v", r)
	}
	r, _ = Sanitize(ToolClaude, []byte(`{"hook_event_name":"StopFailure","error":"rate_limit"}`), opts())
	if r.ErrorKind != "rate_limit" || r.ConvMark != "none" {
		t.Fatalf("%+v", r)
	}
	r, _ = Sanitize(ToolClaude, []byte(`{"hook_event_name":"Notification","notification_type":"idle_prompt","message":"`+secret+`"}`), opts())
	if r.NotificationType != "idle_prompt" {
		t.Fatalf("%+v", r)
	}
	r, _ = Sanitize(ToolClaude, []byte(`{"hook_event_name":"Notification","notification_type":"weird"}`), opts())
	if r.NotificationType != "other" {
		t.Fatalf("%+v", r)
	}
	r, _ = Sanitize(ToolClaude, []byte(`{"hook_event_name":"SessionStart","source":"resume"}`), opts())
	if r.Source != "resume" {
		t.Fatalf("%+v", r)
	}
}

func TestNormalizeRejectsTamperedWireReports(t *testing.T) {
	if _, ok := Normalize(Report{Task: "../x", Tool: ToolClaude, Event: "Stop"}, true); ok {
		t.Error("bad task id")
	}
	if _, ok := Normalize(Report{Task: "t", Tool: ToolClaude, Event: "PostToolUse"}, true); ok {
		t.Error("bad event")
	}
	r, ok := Normalize(Report{Task: "t", Tool: ToolClaude, Event: "Stop", EndMarker: secret, ConvMark: secret, ErrorKind: secret}, true)
	if !ok || r.EndMarker != "" || r.ConvMark != "" || r.ErrorKind != "other" {
		t.Fatalf("%+v", r)
	}
}

func TestQueueEndMarkerAndConversationMarkAreComputedLocally(t *testing.T) {
	o := opts()
	o.Queue = true
	cases := map[string]string{
		"all good\nCODEPASS-DONE\n":          EndDone,
		"need a name?\nCODEPASS-QUESTION":    EndQuestion,
		"CODEPASS-DONE\nbut then more words": EndNone,
		"nothing":                            EndNone,
		"":                                   EndNone,
	}
	for msg, want := range cases {
		b, _ := json.Marshal(map[string]string{"hook_event_name": "Stop", "last_assistant_message": msg})
		r, _ := Sanitize(ToolClaude, b, o)
		if r.EndMarker != want {
			t.Errorf("%q: %q want %q", msg, r.EndMarker, want)
		}
		if strings.Contains(string(mustJSON(r)), "more words") {
			t.Error("text leaked")
		}
	}
	r, _ := Sanitize(ToolClaude, []byte(`{"hook_event_name":"Stop","last_assistant_message":"CODEPASS-DONE"}`), opts())
	if r.EndMarker != "" {
		t.Error("marker computed outside the queue")
	}
	for _, s := range ConversationErrorStrings {
		b, _ := json.Marshal(map[string]string{"hook_event_name": "StopFailure", "error": "invalid_request", "error_details": "400 " + s})
		r, _ := Sanitize(ToolClaude, b, opts())
		if r.ConvMark != ConvConversation {
			t.Errorf("%q not a conversation error", s)
		}
	}
	b, _ := json.Marshal(map[string]string{"hook_event_name": "StopFailure", "error": "rate_limit", "last_assistant_message": "Prompt is too long"})
	if r, _ := Sanitize(ToolClaude, b, opts()); r.ConvMark != ConvConversation {
		t.Error("last_assistant_message not checked")
	}
}

func TestOpenCodeAndCodexNotify(t *testing.T) {
	r, ok := Sanitize(ToolOpenCode, []byte(`{"type":"session.execution.failed","data":{"sessionID":"ses_abc","error":{"type":"provider.auth","message":"`+secret+`"}}}`), opts())
	if !ok || r.Session != "ses_abc" || r.ErrorKind != "authentication_failed" || strings.Contains(string(mustJSON(r)), "SECRET") {
		t.Fatalf("%+v", r)
	}
	r, ok = Sanitize(ToolOpenCode, []byte(`{"type":"session.inbox.enqueued","data":{"sessionID":"ses_abc","item":{"payload":{"text":"`+secret+`"}}}}`), opts())
	if ok {
		t.Fatalf("inbox event accepted: %+v", r)
	}
	r, ok = SanitizeCodexNotify([]byte(`{"type":"agent-turn-complete","thread-id":"th-1","last-assistant-message":"`+secret+`","input-messages":["`+secret+`"]}`), opts())
	if !ok || r.Session != "th-1" || r.Tool != ToolCodex || strings.Contains(string(mustJSON(r)), "SECRET") {
		t.Fatalf("%+v", r)
	}
	if _, ok := SanitizeCodexNotify([]byte(`{"type":"other"}`), opts()); ok {
		t.Error("other type")
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func TestOpenCodeReportsAreOrderedByThePluginStamp(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	o := SanitizeOptions{Task: "o", Now: func() time.Time { return now }}
	ms := now.UnixMilli()
	// The event "started" happened first (seq 1) but its report process is built second.
	failed, _ := Sanitize(ToolOpenCode, []byte(fmt.Sprintf(`{"type":"session.execution.failed","atMs":%d,"seq":2,"data":{"sessionID":"ses_1","error":{"type":"x"}}}`, ms)), o)
	started, _ := Sanitize(ToolOpenCode, []byte(fmt.Sprintf(`{"type":"session.execution.started","atMs":%d,"seq":1,"data":{"sessionID":"ses_1"}}`, ms)), o)
	tr := NewTracker(Options{}, nil)
	tr.Register("o", ToolOpenCode, ModeHooks)
	tr.Apply(failed)
	tr.Apply(started) // arrives late: older stamp, ignored
	if i, _ := tr.Get("o"); i.State != StateWaiting || i.Reason != ReasonError {
		t.Fatalf("%+v", i)
	}
	// A stamp far from now is not trusted.
	bad, _ := Sanitize(ToolOpenCode, []byte(`{"type":"session.idle","atMs":1,"seq":0}`), o)
	if bad.At != now.UnixNano() {
		t.Fatalf("At %d", bad.At)
	}
}
