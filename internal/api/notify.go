package api

// System notifications (plan 4.15, F1-22). internal/notify is the only code that talks to the operating system;
// these are the types the window sees.

const (
	// EventNotifyToast carries a NotifyToast: a notification shown inside the window (sandbox mode, notifications
	// turned off or unavailable while the window is visible).
	EventNotifyToast = "notify:toast"
	// EventNotifyState carries a NotifyState after the permission question was answered.
	EventNotifyState = "notify:state"
)

// NotifyPermission is what the app knows about the system's permission to notify.
type NotifyPermission string

const (
	// NotifyAllowed: the system accepts notifications (always on Windows, which cannot tell).
	NotifyAllowed NotifyPermission = "allowed"
	// NotifyNotAsked: the app has not asked yet (macOS only).
	NotifyNotAsked NotifyPermission = "notAsked"
	// NotifyOff: the app asked and the system says no (macOS only).
	NotifyOff NotifyPermission = "off"
)

// NotifyState is the permission row of Settings.
type NotifyState struct {
	Permission NotifyPermission `json:"permission"`
	// Available is false when the system's notification service could not start (a binary without a bundle
	// identifier): notices then appear as toasts in the window only.
	Available bool `json:"available"`
}

// NotifyAction says what the person pressed on a notification or its in-window toast.
type NotifyAction string

const (
	NotifyActionBody   NotifyAction = "body"   // the notification itself
	NotifyActionButton NotifyAction = "button" // its one button
)

// NotifyTone colours an in-window toast.
type NotifyTone string

const (
	NotifyToneInfo   NotifyTone = "info"
	NotifyToneWarn   NotifyTone = "warn"
	NotifyToneDanger NotifyTone = "danger"
)

// NotifyToast is one notification drawn by the window. Title, Body and Button are already in the person's
// language and contain no secret (plan 4.15 "Text that never appears").
type NotifyToast struct {
	ID     string     `json:"id"` // pass it back to NotifyService.RespondToast
	Kind   string     `json:"kind"`
	Title  string     `json:"title"`
	Body   string     `json:"body"`
	Button string     `json:"button,omitempty"`
	Tone   NotifyTone `json:"tone"`
	Sticky bool       `json:"sticky"` // stays until dismissed
}
