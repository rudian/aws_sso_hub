package notify

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/rudian/codepass/internal/api"
)

// Notice is what a feature sends: a kind, a stable id, the words in the person's language and identifiers.
type Notice struct {
	Kind  Kind
	ID    string // stable per subject (the portal): a second notice with the same id replaces the first on macOS
	Title string // already in the person's language; no secret (plan 4.15 "Text that never appears")
	Body  string
	Data  map[string]string // identifiers only
	// Silent sends without sound.
	Silent bool
	// Tone and Sticky only matter for the toast in the window.
	Tone   api.NotifyTone
	Sticky bool
}

// Click is what a feature gets when the person presses its notification or the button.
type Click struct {
	Kind   Kind
	ID     string
	Action api.NotifyAction
	Data   map[string]string
}

// KindSpec is what a feature registers for its kind.
type KindSpec struct {
	// Button gives the one button's text in a language ("zh-CN" or "en"); nil or "" means no button.
	Button func(lang string) string
	// OnClick receives every click on a notice of this kind. It runs inside the system's response callback (the
	// notification centre's delegate on macOS, the COM activation on Windows), so it must return quickly: a handler
	// that has to wait starts its own goroutine.
	OnClick func(Click)
}

// Delivery says where a notice ended up.
type Delivery string

const (
	DeliveredSystem Delivery = "system" // the operating system shows it
	DeliveredToast  Delivery = "toast"  // only the window shows it
	DeliveredNone   Delivery = "none"   // not shown anywhere (window hidden and the system cannot)
)

// Options configures a Service.
type Options struct {
	Backend Backend // nil: no system notifications at all
	Store   *Store  // nil: a store in memory
	Log     *slog.Logger
	Lang    func() string // the app language, "zh-CN" or "en"
	// Toast draws a notice in the window. nil: nothing is drawn.
	Toast func(api.NotifyToast)
	// Visible reports whether the window is visible; nil means it is.
	Visible func() bool
	// AlsoToast shows every notice in the window too, whatever the system does (sandbox mode).
	AlsoToast bool
	// State is called after a permission question was answered.
	State func(api.NotifyState)
	// Go runs background work (the permission question can take minutes); nil starts a goroutine.
	Go func(func())
}

// Service is the app's one notification service.
type Service struct {
	o     Options
	store *Store

	mu        sync.Mutex
	kinds     map[Kind]KindSpec
	started   bool
	available bool // the backend started
	toasts    map[string]Notice
	toastSeq  int
	order     []string
	asking    bool
}

// New builds a Service. Nothing is started.
func New(o Options) *Service {
	if o.Log == nil {
		o.Log = slog.New(slog.NewTextHandler(discard{}, nil))
	}
	if o.Lang == nil {
		o.Lang = func() string { return "en" }
	}
	if o.Go == nil {
		o.Go = func(f func()) { go f() }
	}
	st := o.Store
	if st == nil {
		st = OpenStore("", nil, o.Log)
	}
	return &Service{o: o, store: st, kinds: map[Kind]KindSpec{}, toasts: map[string]Notice{}}
}

// Store returns the notifications.json store.
func (s *Service) Store() *Store { return s.store }

// RegisterKind registers a kind. Registering it again replaces it. After Start the button text is registered
// with the system at once.
func (s *Service) RegisterKind(kind Kind, spec KindSpec) {
	s.mu.Lock()
	s.kinds[kind] = spec
	avail := s.available
	s.mu.Unlock()
	if avail {
		s.registerCategory(kind, spec)
	}
}

func (s *Service) buttonText(spec KindSpec) string {
	if spec.Button == nil {
		return ""
	}
	return spec.Button(s.o.Lang())
}

func (s *Service) registerCategory(kind Kind, spec KindSpec) {
	btn := s.buttonText(spec)
	if btn == "" || s.o.Backend == nil {
		return
	}
	if err := s.o.Backend.Register(kind, btn); err != nil {
		s.o.Log.Warn("cannot register the button of a notification kind", "kind", string(kind), "err", err)
	}
}

// Start starts the system service. It is called at app start so a click on an old notification reaches the app
// even when that click launched it. A start failure is logged and the service goes on with toasts only; Start
// itself never fails.
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()
	if s.o.Backend == nil {
		return
	}
	s.o.Backend.OnResponse(s.handle)
	if err := safeStart(ctx, s.o.Backend); err != nil {
		s.o.Log.Warn("system notifications are not available; notices appear in the window only", "err", err)
		return
	}
	s.mu.Lock()
	s.available = true
	kinds := make(map[Kind]KindSpec, len(s.kinds))
	for k, v := range s.kinds {
		kinds[k] = v
	}
	s.mu.Unlock()
	for k, v := range kinds {
		s.registerCategory(k, v)
	}
}

func safeStart(ctx context.Context, b Backend) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("the notification backend panicked while starting: %v", r)
		}
	}()
	return b.Start(ctx)
}

// Shutdown stops the system service.
func (s *Service) Shutdown() {
	s.mu.Lock()
	avail := s.available
	s.available = false
	s.mu.Unlock()
	if avail && s.o.Backend != nil {
		s.o.Backend.Shutdown()
	}
}

// Available reports whether the system's service started.
func (s *Service) Available() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.available
}

// LanguageChanged registers the button texts again in the new language (the text belongs to the category, not
// to one notification; research limit 1).
func (s *Service) LanguageChanged() {
	s.mu.Lock()
	avail := s.available
	kinds := make(map[Kind]KindSpec, len(s.kinds))
	for k, v := range s.kinds {
		kinds[k] = v
	}
	s.mu.Unlock()
	if !avail {
		return
	}
	for k, v := range kinds {
		s.registerCategory(k, v)
	}
}

// State is the permission row of Settings.
func (s *Service) State() api.NotifyState {
	s.mu.Lock()
	avail := s.available
	s.mu.Unlock()
	if !avail || s.o.Backend == nil {
		return api.NotifyState{Permission: api.NotifyAllowed, Available: false}
	}
	p, err := s.o.Backend.Permission()
	if err != nil {
		s.o.Log.Warn("cannot read the notification permission", "err", err)
	}
	switch {
	case p.Allowed || !p.CanTellOff:
		return api.NotifyState{Permission: api.NotifyAllowed, Available: true}
	case s.store.Asked():
		return api.NotifyState{Permission: api.NotifyOff, Available: true}
	}
	return api.NotifyState{Permission: api.NotifyNotAsked, Available: true}
}

// Send shows a notice: through the system when it is allowed, else as a toast while the window is visible.
func (s *Service) Send(n Notice) Delivery {
	s.mu.Lock()
	spec, ok := s.kinds[n.Kind]
	s.mu.Unlock()
	if !ok {
		s.o.Log.Warn("a notice of an unregistered kind was dropped", "kind", string(n.Kind))
		return DeliveredNone
	}
	btn := s.buttonText(spec)
	data := make(map[string]string, len(n.Data)+1)
	for k, v := range n.Data {
		data[k] = v
	}
	data["kind"] = string(n.Kind)
	n.Data = data

	delivered := false
	if st := s.State(); st.Available && st.Permission == api.NotifyAllowed && s.o.Backend != nil {
		err := safeSend(s.o.Backend, Message{ID: n.ID, Kind: n.Kind, Title: n.Title, Body: n.Body, Data: data, Silent: n.Silent, HasAction: btn != ""})
		if err != nil {
			s.o.Log.Warn("the system did not show a notification", "kind", string(n.Kind), "err", err)
		} else {
			delivered = true
		}
	}
	toast := s.o.AlsoToast || (!delivered && (s.o.Visible == nil || s.o.Visible()))
	if toast && s.o.Toast != nil {
		s.showToast(n, btn)
	}
	switch {
	case delivered:
		return DeliveredSystem
	case toast && s.o.Toast != nil:
		return DeliveredToast
	}
	return DeliveredNone
}

func safeSend(b Backend, o Message) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("the notification backend panicked: %v", r)
		}
	}()
	return b.Send(o)
}

func (s *Service) showToast(n Notice, button string) {
	s.mu.Lock()
	s.toastSeq++
	id := fmt.Sprintf("%s#%d", n.ID, s.toastSeq)
	s.toasts[id] = n
	s.order = append(s.order, id)
	for len(s.order) > 32 {
		delete(s.toasts, s.order[0])
		s.order = s.order[1:]
	}
	s.mu.Unlock()
	tone := n.Tone
	if tone == "" {
		tone = api.NotifyToneInfo
	}
	s.o.Toast(api.NotifyToast{ID: id, Kind: string(n.Kind), Title: n.Title, Body: n.Body, Button: button, Tone: tone, Sticky: n.Sticky})
}

// RemoveDelivered takes a delivered notification back (macOS; Windows cannot).
func (s *Service) RemoveDelivered(id string) {
	if !s.Available() || s.o.Backend == nil {
		return
	}
	if err := s.o.Backend.RemoveDelivered(id); err != nil {
		s.o.Log.Warn("cannot remove a delivered notification", "err", err)
	}
}

// handle is the one callback of the backend: it routes a click by kind to the feature that registered it.
func (s *Service) handle(r Response) {
	kind := Kind(r.Category)
	if kind == "" {
		kind = Kind(r.Data["kind"])
	}
	act := api.NotifyActionBody
	switch r.Action {
	case ButtonID:
		act = api.NotifyActionButton
	case DefaultActionID:
	default:
		return // an action this app never registered
	}
	s.route(Click{Kind: kind, ID: r.ID, Action: act, Data: r.Data})
}

func (s *Service) route(c Click) {
	s.mu.Lock()
	spec, ok := s.kinds[c.Kind]
	s.mu.Unlock()
	if !ok || spec.OnClick == nil {
		return
	}
	if c.Action == api.NotifyActionButton && s.buttonText(spec) == "" {
		return // a kind without a button cannot have been pressed
	}
	defer func() {
		if r := recover(); r != nil {
			s.o.Log.Error("a notification click handler panicked", "kind", string(c.Kind), "panic", r)
		}
	}()
	spec.OnClick(c)
}

// RespondToast routes a press on an in-window toast the same way a press on the system's notification goes.
// An id the service does not know (already answered, or never shown) is ignored.
func (s *Service) RespondToast(id string, action api.NotifyAction) {
	s.mu.Lock()
	n, ok := s.toasts[id]
	delete(s.toasts, id)
	s.mu.Unlock()
	if !ok || (action != api.NotifyActionBody && action != api.NotifyActionButton) {
		return
	}
	s.route(Click{Kind: n.Kind, ID: n.ID, Action: action, Data: n.Data})
}

// AskOnce asks the system for permission in the background when the app never did and the system has not been
// asked. It is called the first time a portal is signed in with the window in front.
func (s *Service) AskOnce() {
	if st := s.State(); st.Available && st.Permission == api.NotifyNotAsked && !s.store.Asked() {
		s.Request()
	}
}

// Request puts the permission question to the system (the Settings button "Allow notifications" and AskOnce).
// It returns at once; the answer arrives through Options.State. Only one question runs at a time.
func (s *Service) Request() {
	s.mu.Lock()
	if !s.available || s.o.Backend == nil || s.asking {
		s.mu.Unlock()
		return
	}
	s.asking = true
	s.mu.Unlock()
	s.store.SetAsked()
	s.o.Go(func() {
		defer func() {
			s.mu.Lock()
			s.asking = false
			s.mu.Unlock()
			if r := recover(); r != nil {
				s.o.Log.Error("the permission question panicked", "panic", r)
			}
		}()
		if _, err := s.o.Backend.RequestPermission(); err != nil {
			s.o.Log.Warn("the permission question failed", "err", err)
		}
		if s.o.State != nil {
			s.o.State(s.State())
		}
	})
}

// OpenSettings opens the system's notification settings.
func (s *Service) OpenSettings() error {
	if s.o.Backend == nil {
		return nil
	}
	return s.o.Backend.OpenSettings()
}
