package app

import (
	"context"
	"net/url"
	"runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/execx"
)

// Runtime is the seam between the engines and the GUI toolkit. Everything that needs Wails goes through it so
// the services can be tested with a fake.
type Runtime interface {
	Emit(name string, data any)
	OpenURL(url string) error
	RevealPath(path string) error
	OpenFile(path string) error
	ChooseFolder(title string) (string, error)
	// ChooseSaveFile shows the native save dialog. It returns "" when the user cancels.
	ChooseSaveFile(title, defaultName string, filter FileFilter) (string, error)
	// ChooseOpenFile shows the native open dialog for one file. It returns "" when the user cancels.
	ChooseOpenFile(title string, filter FileFilter) (string, error)
	ShowWindow()
}

// FileFilter restricts the files a native dialog offers, e.g. {Name: "JSON files", Pattern: "*.json"}.
type FileFilter struct{ Name, Pattern string }

// wailsRuntime implements Runtime with the running Wails application (looked up lazily with application.Get()).
type wailsRuntime struct {
	mu      sync.Mutex
	win     *application.WebviewWindow
	sandbox bool // sandbox mode talks to fake servers on the loopback interface over plain http
}

// NewWailsRuntime returns the production Runtime. SetWindow attaches the main window once it exists.
func NewWailsRuntime(sandbox bool) *wailsRuntime { return &wailsRuntime{sandbox: sandbox} }

// SetWindow tells the runtime which window ShowWindow brings to the front.
func (r *wailsRuntime) SetWindow(w *application.WebviewWindow) {
	r.mu.Lock()
	r.win = w
	r.mu.Unlock()
}

func (r *wailsRuntime) window() *application.WebviewWindow {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.win
}

func (r *wailsRuntime) Emit(name string, data any) {
	if a := application.Get(); a != nil {
		a.Event.Emit(name, data)
	}
}

func (r *wailsRuntime) OpenURL(raw string) error {
	if err := checkOpenable(raw, r.sandbox); err != nil {
		return err
	}
	a := application.Get()
	if a == nil {
		return api.NewError(api.ErrInternal, "application is not running")
	}
	return a.Browser.OpenURL(raw)
}

// RevealPath shows the file or folder selected in the system file manager. It never launches the item.
func (r *wailsRuntime) RevealPath(path string) error { return revealPath(path) }

// OpenFile opens a text file in the default editor: macOS `open -t`, Windows notepad.
func (r *wailsRuntime) OpenFile(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{"-t", path}
	case "windows":
		name, args = "notepad.exe", []string{path}
	default:
		return api.NewError(api.ErrUnsupportedOS, "unsupported system")
	}
	res, err := execx.Run(execx.Command(ctx, name, args...), nil)
	if err != nil {
		return api.NewError(api.ErrInternal, "cannot start the editor").WithDetail(err.Error())
	}
	if res.ExitCode != 0 {
		return api.NewError(api.ErrInternal, "the editor did not open").WithDetail(string(res.Stderr))
	}
	return nil
}

func (r *wailsRuntime) ChooseFolder(title string) (string, error) {
	a := application.Get()
	if a == nil {
		return "", api.NewError(api.ErrInternal, "application is not running")
	}
	return a.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		SetTitle(title).
		PromptForSingleSelection()
}

func (r *wailsRuntime) ChooseSaveFile(title, defaultName string, filter FileFilter) (string, error) {
	a := application.Get()
	if a == nil {
		return "", api.NewError(api.ErrInternal, "application is not running")
	}
	d := a.Dialog.SaveFile().
		CanCreateDirectories(true).
		SetMessage(title).
		SetFilename(defaultName)
	if filter.Pattern != "" {
		d.AddFilter(filter.Name, filter.Pattern)
	}
	return d.PromptForSingleSelection()
}

func (r *wailsRuntime) ChooseOpenFile(title string, filter FileFilter) (string, error) {
	a := application.Get()
	if a == nil {
		return "", api.NewError(api.ErrInternal, "application is not running")
	}
	d := a.Dialog.OpenFile().
		CanChooseDirectories(false).
		CanChooseFiles(true).
		SetTitle(title)
	if filter.Pattern != "" {
		d.AddFilter(filter.Name, filter.Pattern)
	}
	return d.PromptForSingleSelection()
}

func (r *wailsRuntime) ShowWindow() {
	w := r.window()
	if w == nil {
		return
	}
	w.Show()
	w.Restore()
	w.Focus()
}

// checkHTTPS accepts only absolute https URLs with a host (SystemService.OpenURL).
func checkHTTPS(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return api.NewError(api.ErrInvalidInput, "only https URLs can be opened").WithParam("field", "url")
	}
	return nil
}

// checkOpenable is the check for URLs the engines open themselves (authorize URL, portal and console links):
// https, or http on the loopback interface when the app runs in sandbox mode (fake servers live there).
func checkOpenable(raw string, sandbox bool) error {
	u, err := url.Parse(raw)
	if sandbox && err == nil && u.Scheme == "http" {
		switch u.Hostname() {
		case "127.0.0.1", "localhost", "::1":
			return nil
		}
	}
	return checkHTTPS(raw)
}
