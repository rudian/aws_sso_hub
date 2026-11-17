package app

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/terminal"
)

// TerminalService is the bound Wails service of the terminal data path (plan 5, "Terminal data path"). The
// page pulls bytes by offset with Read and gets only a small "terminal:ping" event when there is more. The
// methods are the whole page-facing surface, and none of them starts a program: sessions are started by Go
// code (the AI task service, the bottom panel) through the terminal.Manager.
type TerminalService struct{ m *terminal.Manager }

// NewTerminalService wraps a manager.
func NewTerminalService(m *terminal.Manager) *TerminalService { return &TerminalService{m: m} }

// NewTerminalManager builds the app's manager: scrollback under the data folder, events through the container,
// the sandbox's variables added to every child in a development build. The wiring task keeps the manager in
// the container so the task services can start sessions on it.
func NewTerminalManager(c *Container) *terminal.Manager {
	return terminal.NewManager(terminal.Config{
		DataDir:  c.Paths.AppDataDir,
		Emit:     c.emit,
		ExtraEnv: c.Paths.ChildEnv(),
		// ProxyEnv: the proxy the app detected, for children started from Finder, the Dock or the Start menu
		// (A2-W: ProxyEnv: func() []string { return nc.Proxy().ChildEnv() } once nc is reachable here)
	})
}

func (s *TerminalService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	return nil
}

// shutdownWait bounds ServiceShutdown: Wails runs it on the main thread before it closes the windows, so it
// must not wait for the grace time of a child that ignores the polite stop.
const shutdownWait = 500 * time.Millisecond

// ServiceShutdown starts the stop of every running session (in parallel) and waits at most shutdownWait. The
// orderly "stop all" (Manager.Shutdown with its full limit, off the UI thread, before the app quits) belongs
// to the quit path of the app (A2-08, A2-14); this is only the last resort. Closing the terminals then ends
// what is left (SIGHUP to the foreground job on macOS, the job on Windows).
func (s *TerminalService) ServiceShutdown() error {
	s.m.Shutdown(shutdownWait)
	return nil
}

// Read returns up to max bytes of the session's output starting at offset (base64), the offset to ask for
// next and the end of the log. When the offset has been trimmed, Gap says how many bytes were skipped and
// Prologue (base64) holds the terminal modes as of the new start. An unknown session is reported as done.
func (s *TerminalService) Read(session string, offset int64, max int) (api.TerminalRead, error) {
	r := s.m.Read(session, offset, max)
	out := api.TerminalRead{
		Data: base64.StdEncoding.EncodeToString(r.Data),
		From: r.From, Next: r.Next, Tail: r.Tail, Gap: r.Gap, Done: r.Done,
	}
	if r.Gap > 0 {
		out.Prologue = base64.StdEncoding.EncodeToString(r.Prologue)
	}
	return out, nil
}

// Write sends keystrokes (base64) to the session. It returns at once.
func (s *TerminalService) Write(session string, data string) error {
	b, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return api.NewError(api.ErrInvalidInput, "invalid data").WithParam("field", "data")
	}
	return fail(s.m.Write(session, b))
}

// Resize tells the child the terminal's new size. A session that has ended is not an error.
func (s *TerminalService) Resize(session string, cols, rows int) error {
	return fail(s.m.Resize(session, cols, rows))
}
