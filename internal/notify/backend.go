// Package notify is the app's only notification code (plan 4.15). A feature registers a Kind (its one button and
// what a click does) and sends Notices; the Service checks the permission, hands the notice to a Backend (the
// system's, or a fake) and routes a click back to the feature. When the system cannot show the notice and the
// window is visible, the same words appear as a toast in the window.
//
// Which Backend runs is decided by ChooseReal: sandbox mode and tests always use the Fake; a development build
// uses the Fake unless AWSSSOHUB_REAL_NOTIFICATIONS=1; only a production build uses the system's service.
package notify

import "context"

// Kind names a family of notices ("signin.headsup"). It is also the id of the category that carries the
// button's text, so a click can be routed by kind.
type Kind string

// ButtonID is the id of the one action every category has.
const ButtonID = "button"

// DefaultActionID is what a click on the notification itself reports.
const DefaultActionID = "DEFAULT_ACTION"

// Message is one notification as the Backend receives it. Data holds identifiers only (the kind, the portal
// name, the sign-in time): on Windows the whole notification is stored inside the button's argument.
type Message struct {
	ID        string
	Kind      Kind // also the category id; "" or an unregistered category gives no button
	Title     string
	Body      string
	Data      map[string]string
	Silent    bool
	HasAction bool // the category has its button
}

// Response is a click, as reported by a Backend.
type Response struct {
	ID       string
	Category string            // the kind
	Action   string            // DefaultActionID for the notification itself, else the action id (ButtonID)
	Data     map[string]string // what Message.Data held
}

// Permission as the Backend reports it: the system only says yes or no (macOS), or always yes (Windows).
type Permission struct {
	Allowed bool
	// CanTellOff is false where the system cannot say that notifications are off (Windows).
	CanTellOff bool
}

// Backend is what the service needs from the system.
type Backend interface {
	// Start prepares the system service. An error means "in-window toasts only"; it never stops the app.
	Start(ctx context.Context) error
	// OnResponse sets the one callback for clicks. Called before Start, so a click that launched the app is
	// not lost.
	OnResponse(func(Response))
	// Register sets the category of a kind: one button with that text (button "" = a category without button is
	// not registered). A second call replaces the first (language change).
	Register(kind Kind, button string) error
	Send(Message) error
	// RemoveDelivered takes a delivered notification back (macOS only; elsewhere it does nothing).
	RemoveDelivered(id string) error
	Permission() (Permission, error)
	// RequestPermission asks the system, which may show its own question. It can take minutes: never call it on
	// the window's thread.
	RequestPermission() (bool, error)
	// OpenSettings opens the system's notification settings.
	OpenSettings() error
	Shutdown()
}
