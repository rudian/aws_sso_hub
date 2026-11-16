package notify

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// System is the Backend that talks to the operating system through Wails' notifications service. The Wails
// service is started here, never registered in application.Options.Services: a service that fails to start stops
// the whole app (docs/research/notifications.md limit 6), and a bare binary without a bundle identifier fails.
//
// Nothing in this file may run in a test, in sandbox mode or in a development build without the opt-in; the
// choice is made by ChooseReal (choose.go).
type System struct {
	mu      sync.Mutex
	svc     *notifications.NotificationService
	started bool
}

// NewSystem returns the system backend. It touches nothing until Start.
func NewSystem() *System { return &System{} }

// Start starts the Wails service. A panic or an error is returned; the caller then runs on toasts only.
func (s *System) Start(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("the notification service panicked while starting: %v", r)
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.svc == nil {
		s.svc = notifications.New()
	}
	if err := s.svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		return err
	}
	s.started = true
	return nil
}

func (s *System) service() (*notifications.NotificationService, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.svc == nil || !s.started {
		return nil, errors.New("the system notification service is not running")
	}
	return s.svc, nil
}

// OnResponse sets the one response callback. It is called before Start; the Wails service object exists
// from notifications.New() on, and its callback is read when a response arrives.
func (s *System) OnResponse(cb func(Response)) {
	s.mu.Lock()
	if s.svc == nil {
		s.svc = notifications.New()
	}
	svc := s.svc
	s.mu.Unlock()
	svc.OnNotificationResponse(func(r notifications.NotificationResult) {
		if r.Error != nil {
			return
		}
		act := r.Response.ActionIdentifier
		if act == notifications.DefaultActionIdentifier {
			act = DefaultActionID
		}
		cb(Response{ID: r.Response.ID, Category: r.Response.CategoryID, Action: act, Data: stringMap(r.Response.UserInfo)})
	})
}

func stringMap(in map[string]interface{}) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		if str, ok := v.(string); ok {
			out[k] = str
		}
	}
	return out
}

func (s *System) Register(kind Kind, button string) error {
	svc, err := s.service()
	if err != nil {
		return err
	}
	return svc.RegisterNotificationCategory(notifications.NotificationCategory{
		ID:      string(kind),
		Actions: []notifications.NotificationAction{{ID: ButtonID, Title: button}},
	})
}

func (s *System) Send(o Message) error {
	svc, err := s.service()
	if err != nil {
		return err
	}
	data := make(map[string]interface{}, len(o.Data))
	for k, v := range o.Data {
		data[k] = v
	}
	n := notifications.NotificationOptions{ID: o.ID, Title: o.Title, Body: o.Body, Data: data}
	if o.Silent {
		n.Sound = &notifications.NotificationSound{Silent: true}
	}
	if o.HasAction {
		n.CategoryID = string(o.Kind)
		return svc.SendNotificationWithActions(n)
	}
	return svc.SendNotification(n)
}

func (s *System) RemoveDelivered(id string) error {
	svc, err := s.service()
	if err != nil {
		return err
	}
	return svc.RemoveDeliveredNotification(id)
}

func (s *System) Permission() (Permission, error) {
	svc, err := s.service()
	if err != nil {
		return Permission{}, err
	}
	ok, err := svc.CheckNotificationAuthorization()
	// Windows always answers "allowed" and cannot tell that notifications are off.
	return Permission{Allowed: ok, CanTellOff: runtime.GOOS == "darwin"}, err
}

func (s *System) RequestPermission() (bool, error) {
	svc, err := s.service()
	if err != nil {
		return false, err
	}
	return svc.RequestNotificationAuthorization()
}

// OpenSettings opens the notification settings of the system. The macOS URL and the Windows one are from
// docs/research/notifications.md (manual checks M8 and W4).
func (s *System) OpenSettings() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	switch runtime.GOOS {
	case "darwin":
		return exec.CommandContext(ctx, "/usr/bin/open", "x-apple.systempreferences:com.apple.Notifications-Settings.extension").Run()
	case "windows":
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		return exec.CommandContext(ctx, filepath.Join(root, "System32", "rundll32.exe"), "url.dll,FileProtocolHandler", "ms-settings:notifications").Run()
	}
	return errors.New("no notification settings on this system")
}

func (s *System) Shutdown() {
	s.mu.Lock()
	svc, started := s.svc, s.started
	s.mu.Unlock()
	if svc != nil && started {
		_ = svc.ServiceShutdown()
	}
}
