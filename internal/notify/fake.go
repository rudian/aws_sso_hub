package notify

import (
	"context"
	"errors"
	"sync"
)

// Sent is one notification the Fake received.
type Sent struct {
	Message
	Button string // the button text of the kind's category when it was sent ("" = none registered)
}

// Fake is the Backend of sandbox mode and of every test. It records what it is given, never touches the system
// and has a Respond call that presses the notification or its button, so the routing runs as for a real click.
type Fake struct {
	mu         sync.Mutex
	cb         func(Response)
	categories map[Kind]string // kind -> button text
	registered []Registration
	sent       []Sent
	removed    []string
	perm       Permission
	// state a test sets
	startErr   error
	sendErr    error
	requestAns bool // what RequestPermission answers
	settings   int  // times OpenSettings was called
	requests   int  // times RequestPermission was called
	started    bool
	shut       bool
}

// Registration is one Register call.
type Registration struct {
	Kind   Kind
	Button string
}

// NewFake returns a Fake whose permission is "allowed" and that can tell "off" apart (the macOS shape).
func NewFake() *Fake {
	return &Fake{categories: map[Kind]string{}, perm: Permission{Allowed: true, CanTellOff: true}, requestAns: true}
}

func (f *Fake) Start(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return f.startErr
	}
	f.started = true
	return nil
}

func (f *Fake) OnResponse(cb func(Response)) {
	f.mu.Lock()
	f.cb = cb
	f.mu.Unlock()
}

func (f *Fake) Register(kind Kind, button string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.categories[kind] = button
	f.registered = append(f.registered, Registration{kind, button})
	return nil
}

func (f *Fake) Send(o Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return f.sendErr
	}
	// like the real service: a category that was not registered gives a notice without its button
	btn := f.categories[o.Kind]
	o.HasAction = o.HasAction && btn != ""
	data := map[string]string{}
	for k, v := range o.Data {
		data[k] = v
	}
	o.Data = data
	f.sent = append(f.sent, Sent{Message: o, Button: btn})
	return nil
}

func (f *Fake) RemoveDelivered(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, id)
	return nil
}

func (f *Fake) Permission() (Permission, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.perm, nil
}

func (f *Fake) RequestPermission() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	if f.requestAns {
		f.perm.Allowed = true
	}
	return f.perm.Allowed, nil
}

func (f *Fake) OpenSettings() error {
	f.mu.Lock()
	f.settings++
	f.mu.Unlock()
	return nil
}

func (f *Fake) Shutdown() {
	f.mu.Lock()
	f.shut = true
	f.mu.Unlock()
}

// ---- what a test sets and reads ------------------------------------------------------------------------------

// SetPermission sets what the system answers.
func (f *Fake) SetPermission(p Permission) { f.mu.Lock(); f.perm = p; f.mu.Unlock() }

// SetStartError makes Start fail (a binary without a bundle identifier).
func (f *Fake) SetStartError(err error) { f.mu.Lock(); f.startErr = err; f.mu.Unlock() }

// SetSendError makes Send fail (permission off: the system refuses).
func (f *Fake) SetSendError(err error) { f.mu.Lock(); f.sendErr = err; f.mu.Unlock() }

// SetRequestAnswer sets whether the next permission question is answered "allow".
func (f *Fake) SetRequestAnswer(allow bool) { f.mu.Lock(); f.requestAns = allow; f.mu.Unlock() }

// Sent returns every notification received, oldest first.
func (f *Fake) Sent() []Sent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Sent(nil), f.sent...)
}

// Removed returns the ids of the delivered notifications that were taken back.
func (f *Fake) Removed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.removed...)
}

// Registrations returns every Register call, in order.
func (f *Fake) Registrations() []Registration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Registration(nil), f.registered...)
}

// Counts returns how often the system's question and its settings were asked for.
func (f *Fake) Counts() (requests, settings int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests, f.settings
}

// Started reports whether Start succeeded.
func (f *Fake) Started() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.started }

// ErrRefused is what a Fake returns when a test makes sending fail the way the system does with permission off.
var ErrRefused = errors.New("notifications are not allowed")

// Respond presses a notification: action is DefaultActionID for the body or ButtonID for the button. It runs
// the callback the service registered, the same code a real click reaches. The notification must have been
// sent; a click on a button the notification does not have is refused (the system would not show it).
func (f *Fake) Respond(id, action string) error {
	f.mu.Lock()
	cb := f.cb
	var found *Sent
	for i := len(f.sent) - 1; i >= 0; i-- {
		if f.sent[i].ID == id {
			s := f.sent[i]
			found = &s
			break
		}
	}
	f.mu.Unlock()
	if found == nil {
		return errors.New("no such notification: " + id)
	}
	if action == ButtonID && !found.HasAction {
		return errors.New("the notification has no button")
	}
	if cb == nil {
		return errors.New("nobody listens to responses")
	}
	data := map[string]string{}
	for k, v := range found.Data {
		data[k] = v
	}
	cb(Response{ID: found.ID, Category: string(found.Kind), Action: action, Data: data})
	return nil
}

// RespondRaw delivers a response as is (a stale or forged one, or a click that launched the app).
func (f *Fake) RespondRaw(r Response) error {
	f.mu.Lock()
	cb := f.cb
	f.mu.Unlock()
	if cb == nil {
		return errors.New("nobody listens to responses")
	}
	cb(r)
	return nil
}
