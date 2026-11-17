package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/logx"
	"github.com/rudian/codepass/internal/pty"
	"github.com/rudian/codepass/internal/terminal"
)

// slowKill is a terminal whose child ignores the polite stop: KillSession returns only after the grace time.
type slowKill struct {
	*termFake
	grace time.Duration
}

func (s slowKill) KillSession() error { time.Sleep(s.grace); s.end(); return nil }

// Wails runs ServiceShutdown on the main thread before it closes the windows: it must come back quickly even
// when a child ignores SIGTERM and the stop takes the whole grace time.
func TestServiceShutdownDoesNotWaitForTheGraceTime(t *testing.T) {
	m := terminal.NewManager(terminal.Config{
		Start: func(pty.Options) (pty.Session, error) { return slowKill{newTermFake(), 4 * time.Second}, nil },
	})
	if _, err := m.Start(terminal.StartOptions{ID: "t1", Argv: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	s := NewTerminalService(m)
	t0 := time.Now()
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(t0); d > time.Second {
		t.Fatalf("ServiceShutdown took %v", d)
	}
}

// At debug level Wails logs every bound call with its arguments and result as JSON text. For the terminal that
// is every typed key and every output byte: none of it may reach app.log.
func TestBindingCallLogsNeverHoldTerminalBytes(t *testing.T) {
	dir := t.TempDir()
	logger, closer, err := logx.New(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	quiet := QuietBindingLogger(logger)
	typed := base64.StdEncoding.EncodeToString([]byte("correct-horse-battery-staple\r"))
	output := base64.StdEncoding.EncodeToString([]byte("Password: (the prompt) and secrets the build printed"))
	args, _ := json.Marshal([]any{"t1", typed})
	result, _ := json.Marshal(map[string]any{"data": output, "from": 0, "next": 52})
	// the exact record of pkg/application/messageprocessor_call.go
	quiet.Debug("Binding call complete:", "id", "42", "method", "TerminalService.Read", "args", string(args), "result", string(result))
	quiet.Debug("other debug line", "args", "kept")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	log := string(b)
	for _, leak := range []string{typed, output, "args\":\"[", "result\":\"{"} {
		if strings.Contains(log, leak) {
			t.Errorf("app.log holds %q:\n%s", leak, log)
		}
	}
	if !strings.Contains(log, "Binding call complete:") || !strings.Contains(log, "TerminalService.Read") {
		t.Errorf("the call itself should still be logged:\n%s", log)
	}
	if !strings.Contains(log, `"args":"kept"`) {
		t.Errorf("other records keep their attributes:\n%s", log)
	}
	// attributes and groups added through the logger survive the wrapper
	var buf bytes.Buffer
	l := QuietBindingLogger(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))).With("svc", "x")
	l.Debug("Binding call complete:", "args", "SECRET", "id", "1")
	if strings.Contains(buf.String(), "SECRET") || !strings.Contains(buf.String(), "svc=x") || !strings.Contains(buf.String(), "id=1") {
		t.Errorf("%s", buf.String())
	}
}
