// Package telemetry sets up OpenTelemetry for one process: a tracer provider, a meter provider
// and W3C trace context propagation, installed as the globals that otelhttp, otelpgx and
// otelriver use.
//
// Configuration is the standard OTEL_* environment, read by the SDK and the exporters:
// OTEL_EXPORTER_OTLP_ENDPOINT (or the per-signal ..._TRACES_ENDPOINT / ..._METRICS_ENDPOINT),
// OTEL_EXPORTER_OTLP_HEADERS, OTEL_SERVICE_NAME, OTEL_RESOURCE_ATTRIBUTES (service.version,
// deployment.environment.name), OTEL_TRACES_SAMPLER(_ARG), OTEL_METRIC_EXPORT_INTERVAL.
// A signal is exported over OTLP/HTTP only when its endpoint is set. Spans are created either
// way, so every log line and every job carries a trace ID even without a collector.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// Setup installs the providers for service ("api", "worker"). The returned shutdown flushes
// what is buffered; call it once, after the server and the job client have stopped.
func Setup(ctx context.Context, service string, log *slog.Logger) (shutdown func(context.Context) error, err error) {
	// The SDK reports export failures here; the default handler prints to stderr unstructured.
	logCtx := context.WithoutCancel(ctx)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.WarnContext(logCtx, "telemetry", "err", err)
	}))
	// Incoming traceparent continues a trace (Next.js → Go); otelriver copies it into jobs.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	res, err := newResource(ctx, service)
	if err != nil {
		return nil, err
	}

	traceOpts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if endpointSet("TRACES") {
		exp, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("trace exporter: %w", err)
		}
		traceOpts = append(traceOpts, sdktrace.WithBatcher(exp))
	}
	tp := sdktrace.NewTracerProvider(traceOpts...)
	otel.SetTracerProvider(tp)

	metricOpts := []sdkmetric.Option{sdkmetric.WithResource(res)}
	if endpointSet("METRICS") {
		exp, err := otlpmetrichttp.New(ctx)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("metric exporter: %w", err), tp.Shutdown(ctx))
		}
		metricOpts = append(metricOpts, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp)))
	}
	mp := sdkmetric.NewMeterProvider(metricOpts...)
	otel.SetMeterProvider(mp)

	return func(ctx context.Context) error {
		return errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx))
	}, nil
}

// Flush runs shutdown and logs a failure. It has its own deadline: by the time a process
// flushes, ctx is usually cancelled by the signal that stopped it.
func Flush(ctx context.Context, log *slog.Logger, shutdown func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		log.WarnContext(ctx, "telemetry flush", "err", err)
	}
}

// newResource names the service; the environment (OTEL_SERVICE_NAME, OTEL_RESOURCE_ATTRIBUTES)
// overrides the name given in code, because later detectors win.
func newResource(ctx context.Context, service string) (*resource.Resource, error) {
	res, err := resource.New(ctx,
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(semconv.ServiceName(service)),
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry resource: %w", err)
	}
	return res, nil
}

// endpointSet reports whether OTLP export is configured for signal (TRACES, METRICS). Without
// it the exporter would default to localhost:4318 and log a failure on every export.
func endpointSet(signal string) bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_"+signal+"_ENDPOINT") != ""
}
