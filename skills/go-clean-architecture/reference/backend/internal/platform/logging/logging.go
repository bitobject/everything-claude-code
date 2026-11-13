// Package logging builds the process logger: JSON lines on stdout, with request-scoped
// attributes taken from the context. Every log call that has a context uses the ...Context
// method (sloglint enforces it), so the request ID and the trace and span IDs reach every line
// without being passed by hand; a log line and its trace find each other by trace_id.
package logging

import (
	"context"
	"io"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
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

// New returns a JSON logger writing to w at level, which adds the context's request ID and
// trace context to every record.
func New(w io.Writer, level slog.Leveler) *slog.Logger {
	return slog.New(ContextHandler{slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})})
}

// ContextHandler adds request-scoped attributes from the context to each record.
type ContextHandler struct{ slog.Handler }

// Handle adds request_id, trace_id and span_id when ctx has them.
func (h ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
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
