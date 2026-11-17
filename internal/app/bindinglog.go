package app

import (
	"context"
	"log/slog"
)

// bindingCallMessage is the record Wails writes (at debug level) after every bound call. It carries the
// call's arguments and result as JSON text, so for the terminal it would hold every typed key and every
// byte of output (base64) in app.log, and for other services whatever they return.
const bindingCallMessage = "Binding call complete:"

// QuietBindingLogger wraps l so that the arguments and result of Wails' "Binding call complete:" records never
// reach the log (the call's id and method stay). main.go gives the wrapped logger to application.Options.Logger,
// so app.log stays free of terminal bytes even with AWSSSOHUB_DEBUG set (plan 4.7: output never reaches the log).
func QuietBindingLogger(l *slog.Logger) *slog.Logger {
	return slog.New(&quietBindingHandler{inner: l.Handler()})
}

type quietBindingHandler struct{ inner slog.Handler }

func (h *quietBindingHandler) Enabled(ctx context.Context, lv slog.Level) bool {
	return h.inner.Enabled(ctx, lv)
}

func (h *quietBindingHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Message == bindingCallMessage {
		q := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
		r.Attrs(func(a slog.Attr) bool {
			if a.Key != "args" && a.Key != "result" {
				q.AddAttrs(a)
			}
			return true
		})
		r = q
	}
	return h.inner.Handle(ctx, r)
}

func (h *quietBindingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &quietBindingHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *quietBindingHandler) WithGroup(name string) slog.Handler {
	return &quietBindingHandler{inner: h.inner.WithGroup(name)}
}
