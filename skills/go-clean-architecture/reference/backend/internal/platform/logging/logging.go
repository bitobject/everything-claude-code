// Package logging builds the process logger: JSON lines on stdout, with request-scoped
// attributes taken from the context. Every log call that has a context uses the ...Context
// method (sloglint enforces it), so the request ID reaches every line without being passed by
// hand. Trace and span IDs join here once tracing is wired (golang-patterns, "Logging").
package logging

import (
	"context"
	"io"
	"log/slog"
)

type requestIDKey struct{}

// WithRequestID returns ctx carrying the request ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the request ID in ctx, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// New returns a JSON logger writing to w at level, which adds the context's request ID to
// every record.
func New(w io.Writer, level slog.Leveler) *slog.Logger {
	return slog.New(ContextHandler{slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})})
}

// ContextHandler adds request-scoped attributes from the context to each record.
type ContextHandler struct{ slog.Handler }

// Handle adds request_id when ctx has one.
func (h ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs keeps the wrapper around the derived handler.
func (h ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return ContextHandler{h.Handler.WithAttrs(attrs)}
}

// WithGroup keeps the wrapper around the derived handler.
func (h ContextHandler) WithGroup(name string) slog.Handler {
	return ContextHandler{h.Handler.WithGroup(name)}
}
