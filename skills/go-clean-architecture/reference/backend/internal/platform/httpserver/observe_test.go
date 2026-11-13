package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	"example.com/app/internal/platform/httpserver"
	"example.com/app/internal/platform/telemetry/telemetrytest"
)

const leadID = "0b8f2a6c-1d4e-4f7a-9c3b-5e6d7f8a9b0c"

// newObserved mounts an API mux under /v1/ the way cmd/api does: outer mux → Observe → Route.
// A path the mux doesn't know gets its 404 with no pattern (in production the validator answers
// it first).
func newObserved() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /v1/leads/{leadId}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	ip := httpserver.NewClientIP([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	outer := http.NewServeMux()
	// RequestID stands for the middleware between Observe and the API: each one passes a copy of
	// the request on, so the pattern the API's mux sets never reaches otelhttp's own request.
	outer.Handle("/v1/", httpserver.Observe(httpserver.Chain(httpserver.Route(api), httpserver.RequestID), "api:8080", ip))
	return outer
}

func TestObserveNamesSpansAndSeriesByRouteTemplate(t *testing.T) {
	tests := []struct {
		name, path, wantSpan, wantRoute string
	}{
		{name: "an operation", path: "/v1/leads/" + leadID, wantSpan: "GET /v1/leads/{leadId}", wantRoute: "/v1/leads/{leadId}"},
		{name: "no operation: the method alone, no route", path: "/v1/nope/" + leadID, wantSpan: "GET"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := telemetrytest.Install(t)
			h := newObserved()

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, nil)
			req.Host = "attacker.example:31337"
			h.ServeHTTP(httptest.NewRecorder(), req)

			spans := rec.Spans()
			if len(spans) != 1 || spans[0].Name() != tt.wantSpan {
				t.Fatalf("spans = %v, want one named %q", telemetrytest.Names(spans), tt.wantSpan)
			}
			route, ok := telemetrytest.Attr(spans[0].Attributes(), semconv.HTTPRouteKey)
			if route != tt.wantRoute || ok != (tt.wantRoute != "") {
				t.Errorf("span http.route = %q (set %v), want %q", route, ok, tt.wantRoute)
			}

			series := rec.Series(t, "http.server.request.duration")
			if len(series) != 1 {
				t.Fatalf("duration series = %d, want 1", len(series))
			}
			got, ok := series[0].Value(semconv.HTTPRouteKey)
			if got.String() != tt.wantRoute || ok != (tt.wantRoute != "") {
				t.Errorf("metric http.route = %q (set %v), want %q", got.String(), ok, tt.wantRoute)
			}
			for _, kv := range series[0].ToSlice() {
				if v := kv.Value.String(); strings.Contains(v, leadID) || strings.Contains(v, "attacker") || strings.Contains(v, "31337") {
					t.Errorf("metric attribute %s = %q: raw IDs and the client's Host header must not create series", kv.Key, v)
				}
			}
			if host, _ := series[0].Value(semconv.ServerAddressKey); host.String() != "api" {
				t.Errorf("server.address = %q, want the fixed server name", host.String())
			}
		})
	}
}

func TestObserveContinuesTheCallersTrace(t *testing.T) {
	rec := telemetrytest.Install(t)
	h := newObserved()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/leads/"+leadID, nil)
	req.Header.Set("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	h.ServeHTTP(httptest.NewRecorder(), req)

	spans := rec.Spans()
	if len(spans) != 1 {
		t.Fatalf("spans = %v, want 1", telemetrytest.Names(spans))
	}
	if got := spans[0].SpanContext().TraceID().String(); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace ID = %s, want the caller's (Next.js renders and API calls are one trace)", got)
	}
	if got := spans[0].Parent().SpanID().String(); got != "00f067aa0ba902b7" {
		t.Errorf("parent span = %s, want the caller's span", got)
	}
}

func TestObserveRecordsTheClientIPItCanVouchFor(t *testing.T) {
	tests := []struct {
		name, remote, forwarded, want string
	}{
		{name: "through the proxy: the rightmost untrusted hop", remote: "10.0.0.5:4000", forwarded: "198.51.100.7, 203.0.113.9", want: "203.0.113.9"},
		{name: "directly: the header is ignored", remote: "203.0.113.9:4000", forwarded: "198.51.100.7", want: "203.0.113.9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := telemetrytest.Install(t)
			h := newObserved()

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/leads/"+leadID, nil)
			req.RemoteAddr = tt.remote
			req.Header.Set("X-Forwarded-For", tt.forwarded)
			h.ServeHTTP(httptest.NewRecorder(), req)

			spans := rec.Spans()
			if len(spans) != 1 {
				t.Fatalf("spans = %v, want 1", telemetrytest.Names(spans))
			}
			if got, _ := telemetrytest.Attr(spans[0].Attributes(), semconv.ClientAddressKey); got != tt.want {
				t.Errorf("client.address = %q, want %q (ClientIP, not the leftmost X-Forwarded-For)", got, tt.want)
			}
		})
	}
}

func TestAPIHeaders(t *testing.T) {
	h := httpserver.APIHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/leads/"+leadID, nil))

	for k, want := range map[string]string{"Cache-Control": "no-store", "X-Content-Type-Options": "nosniff"} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}
