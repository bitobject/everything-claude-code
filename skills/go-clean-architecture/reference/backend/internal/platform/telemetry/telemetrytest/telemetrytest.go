// Package telemetrytest records the spans and metrics a test produces. It installs recording
// providers as the OpenTelemetry globals, which otelhttp, otelpgx and otelriver use, so a test
// runs the production wiring and reads what it recorded. Tests that call Install change process
// state: they must not call t.Parallel.
package telemetrytest

import (
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// Recorder holds what the instrumented code recorded since Install.
type Recorder struct {
	spans  *tracetest.SpanRecorder
	reader *sdkmetric.ManualReader
}

// Install makes recording providers and the W3C propagator global until t ends. Call it before
// building the handler or client under test: some instrumentations create their instruments
// when they are constructed.
func Install(t testing.TB) *Recorder {
	t.Helper()
	tp0, mp0, prop0 := otel.GetTracerProvider(), otel.GetMeterProvider(), otel.GetTextMapPropagator()
	r := &Recorder{spans: tracetest.NewSpanRecorder(), reader: sdkmetric.NewManualReader()}
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(r.spans))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(r.reader))
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(tp0)
		otel.SetMeterProvider(mp0)
		otel.SetTextMapPropagator(prop0)
		_ = tp.Shutdown(t.Context())
		_ = mp.Shutdown(t.Context())
	})
	return r
}

// Spans returns the spans that have ended, in the order they ended.
func (r *Recorder) Spans() []sdktrace.ReadOnlySpan { return r.spans.Ended() }

// SpansNamed returns the ended spans called name.
func (r *Recorder) SpansNamed(name string) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan
	for _, s := range r.spans.Ended() {
		if s.Name() == name {
			out = append(out, s)
		}
	}
	return out
}

// Series returns the attribute sets of the metric called name: one per time series.
func (r *Recorder) Series(t testing.TB, name string) []attribute.Set {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := r.reader.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	var out []attribute.Set
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				out = append(out, seriesOf(m.Data)...)
			}
		}
	}
	return out
}

func seriesOf(data metricdata.Aggregation) []attribute.Set {
	var out []attribute.Set
	switch d := data.(type) {
	case metricdata.Histogram[float64]:
		for _, p := range d.DataPoints {
			out = append(out, p.Attributes)
		}
	case metricdata.Histogram[int64]:
		for _, p := range d.DataPoints {
			out = append(out, p.Attributes)
		}
	case metricdata.Sum[int64]:
		for _, p := range d.DataPoints {
			out = append(out, p.Attributes)
		}
	case metricdata.Sum[float64]:
		for _, p := range d.DataPoints {
			out = append(out, p.Attributes)
		}
	case metricdata.Gauge[int64]:
		for _, p := range d.DataPoints {
			out = append(out, p.Attributes)
		}
	case metricdata.Gauge[float64]:
		for _, p := range d.DataPoints {
			out = append(out, p.Attributes)
		}
	}
	return out
}

// Attr returns the value of key in attrs as a string, and whether it is there.
func Attr(attrs []attribute.KeyValue, key attribute.Key) (string, bool) {
	for _, kv := range attrs {
		if kv.Key == key {
			return kv.Value.String(), true
		}
	}
	return "", false
}

// Names returns the names of spans, for failure messages.
func Names(spans []sdktrace.ReadOnlySpan) []string {
	out := make([]string, len(spans))
	for i, s := range spans {
		out[i] = s.Name()
	}
	return out
}
