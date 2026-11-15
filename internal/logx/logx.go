// Package logx builds the app's structured logger: JSON lines in <LogDir>/app.log, every string attribute and
// message passes through redact.String, and the file rotates at 1 MiB keeping 3 old files.
package logx

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/rudian/codepass/internal/redact"
)

const (
	fileName   = "app.log"
	maxBytes   = 1 << 20
	keepOldLog = 3
)

// New opens <dir>/app.log and returns a logger writing redacted JSON lines. debug lowers the level to Debug.
func New(dir string, debug bool) (*slog.Logger, io.Closer, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}
	w := &rotator{path: filepath.Join(dir, fileName), max: maxBytes}
	if err := w.open(); err != nil {
		return nil, nil, err
	}
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	inner := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level, ReplaceAttr: replaceAttr})
	return slog.New(&redactHandler{inner: inner}), w, nil
}

// replaceAttr redacts every string (and error / stringer) value, including the message.
func replaceAttr(_ []string, a slog.Attr) slog.Attr {
	switch a.Value.Kind() {
	case slog.KindString:
		a.Value = slog.StringValue(redact.String(a.Value.String()))
	case slog.KindAny:
		switch v := a.Value.Any().(type) {
		case error:
			a.Value = slog.StringValue(redact.String(v.Error()))
		case fmt.Stringer:
			a.Value = slog.StringValue(redact.String(v.String()))
		}
	}
	return a
}

// redactHandler additionally redacts the record message (ReplaceAttr already sees it as the "msg" attribute, this
// keeps it safe if the handler options change).
type redactHandler struct{ inner slog.Handler }

func (h *redactHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}
func (h *redactHandler) Handle(ctx context.Context, r slog.Record) error {
	r.Message = redact.String(r.Message)
	return h.inner.Handle(ctx, r)
}
func (h *redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &redactHandler{inner: h.inner.WithAttrs(attrs)}
}
func (h *redactHandler) WithGroup(name string) slog.Handler {
	return &redactHandler{inner: h.inner.WithGroup(name)}
}

// rotator is an io.WriteCloser that rotates app.log -> app.log.1 -> .2 -> .3 when it grows past max bytes.
type rotator struct {
	mu   sync.Mutex
	path string
	max  int64
	f    *os.File
	size int64
}

func (r *rotator) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	r.f, r.size = f, st.Size()
	return nil
}

func (r *rotator) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return 0, os.ErrClosed
	}
	if r.size > 0 && r.size+int64(len(p)) > r.max {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *rotator) rotate() error {
	r.f.Close()
	r.f = nil
	_ = os.Remove(fmt.Sprintf("%s.%d", r.path, keepOldLog))
	for i := keepOldLog - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", r.path, i), fmt.Sprintf("%s.%d", r.path, i+1))
	}
	_ = os.Rename(r.path, r.path+".1")
	return r.open()
}

func (r *rotator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}
