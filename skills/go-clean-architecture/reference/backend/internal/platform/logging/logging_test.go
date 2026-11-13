package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel/trace"

	"example.com/app/internal/platform/logging"
)

func TestContextAttributesReachEveryLine(t *testing.T) {
	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	traced := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled,
	}))

	tests := []struct {
		name string
		ctx  context.Context
		want map[string]any
	}{
		{name: "nothing in the context", ctx: context.Background(), want: map[string]any{}},
		{
			name: "request ID",
			ctx:  logging.WithRequestID(context.Background(), "req-1"),
			want: map[string]any{"request_id": "req-1"},
		},
		{
			name: "request ID and span",
			ctx:  logging.WithRequestID(traced, "req-1"),
			want: map[string]any{
				"request_id": "req-1",
				"trace_id":   "4bf92f3577b34da6a3ce929d0e0e4736",
				"span_id":    "00f067aa0ba902b7",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := logging.New(&buf, slog.LevelInfo).With("component", "test") // attributes keep the wrapper

			log.InfoContext(tt.ctx, "hello")

			var line map[string]any
			if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
				t.Fatalf("not one JSON line: %q", buf.String())
			}
			got := map[string]any{}
			for _, k := range []string{"request_id", "trace_id", "span_id"} {
				if v, ok := line[k]; ok {
					got[k] = v
				}
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("context attributes (-want +got):\n%s", diff)
			}
			if line["component"] != "test" {
				t.Errorf("component = %v, want the With attribute kept", line["component"])
			}
		})
	}
}
