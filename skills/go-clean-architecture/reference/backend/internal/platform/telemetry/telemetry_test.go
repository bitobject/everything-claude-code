package telemetry

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// Setup changes process-wide globals and the tests set environment variables: none of them run
// in parallel.

func TestSetupWithoutEndpointStillCreatesTraceIDs(t *testing.T) {
	unsetOTLP(t)
	shutdown := setup(t)

	ctx, span := otel.Tracer("test").Start(t.Context(), "op")
	span.End()

	if !span.SpanContext().IsValid() {
		t.Fatal("span context is invalid: log lines and jobs would carry no trace ID")
	}
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	if carrier.Get("traceparent") == "" {
		t.Error("no traceparent injected: calls and jobs would start new traces")
	}
	if err := shutdown(t.Context()); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

func TestSetupExportsTracesAndMetricsOnShutdown(t *testing.T) {
	unsetOTLP(t)
	var (
		mu    sync.Mutex
		paths = map[string]int{}
	)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths[r.Method+" "+r.URL.Path]++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(collector.Close)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
	shutdown := setup(t)

	_, span := otel.Tracer("test").Start(t.Context(), "op")
	span.End()
	counter, err := otel.Meter("test").Int64Counter("test.count")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(t.Context(), 1)
	if err := shutdown(t.Context()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, want := range []string{"POST /v1/traces", "POST /v1/metrics"} {
		if paths[want] == 0 {
			t.Errorf("collector got %v, want %s (shutdown must flush)", paths, want)
		}
	}
}

func TestServiceNameFromEnvironmentWins(t *testing.T) {
	tests := []struct {
		name, env, want string
	}{
		{name: "the name given in code", want: "api"},
		{name: "OTEL_SERVICE_NAME overrides it", env: "api-canary", want: "api-canary"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsetOTLP(t)
			t.Setenv("OTEL_SERVICE_NAME", tt.env)
			res, err := newResource(t.Context(), "api")
			if err != nil {
				t.Fatal(err)
			}
			got, _ := res.Set().Value(semconv.ServiceNameKey)
			if got.AsString() != tt.want {
				t.Errorf("service.name = %q, want %q", got.AsString(), tt.want)
			}
		})
	}
}

// setup runs Setup and restores the previous globals when t ends.
func setup(t *testing.T) func(context.Context) error {
	t.Helper()
	tp, mp, prop := otel.GetTracerProvider(), otel.GetMeterProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(tp)
		otel.SetMeterProvider(mp)
		otel.SetTextMapPropagator(prop)
	})
	shutdown, err := Setup(t.Context(), "test", slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	return shutdown
}

func unsetOTLP(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
		"OTEL_SERVICE_NAME", "OTEL_RESOURCE_ATTRIBUTES", "OTEL_TRACES_SAMPLER",
	} {
		t.Setenv(k, "")
	}
}
