package httpserver

import (
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

// Observe traces and measures every request to h: one server span (continuing the caller's
// traceparent) and the http.server.request.duration and body size metrics. cmd wraps the /v1
// chain with it, outermost, so the access log and every log line of the request carry the trace
// ID; the probes stay outside.
//
// Span names and the http.route attribute come from the operation's route template ("POST
// /v1/leads/{leadId}", set by Route and LabelRoute), never from the raw path: IDs in paths would
// make a metric series per lead. A request that matches no operation is named by its method
// alone. server is the fixed host:port the metrics report as server.address and server.port, so
// a client's Host header can't create series either. client.address comes from ip, the one
// reader of X-Forwarded-For (otelhttp's own choice, the leftmost hop, is the client's to forge).
func Observe(h http.Handler, server string, ip ClientIP) http.Handler {
	traced := otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a := ip.Of(r); a.IsValid() {
			trace.SpanFromContext(r.Context()).SetAttributes(semconv.ClientAddress(a.String()))
		}
		h.ServeHTTP(w, r)
	}), "http.server", otelhttp.WithServerName(server))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The outer mux matched "/v1/" and set r.Pattern; otelhttp would name every span and
		// label every metric with it. The API's own mux sets the real pattern (see Route).
		r = r.WithContext(r.Context())
		r.Pattern = ""
		traced.ServeHTTP(w, r)
	})
}

// Route wraps the API's ServeMux: once mux has served the request, it labels the span and the
// request metrics with the pattern mux matched. The mux sets r.Pattern on the request it gets.
func Route(mux http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r)
		if i := strings.IndexByte(r.Pattern, '/'); i >= 0 {
			LabelRoute(r, r.Pattern[i:])
		}
	})
}

// LabelRoute names r's span "METHOD route" and adds http.route to the span and to the request
// metrics. route is a template from the contract ("/v1/leads/{leadId}"), never r.URL.Path. The
// request validator calls it for the requests it answers itself (400, 401, 413, 415), which
// never reach the mux.
func LabelRoute(r *http.Request, route string) {
	attr := semconv.HTTPRoute(route)
	span := trace.SpanFromContext(r.Context())
	span.SetName(r.Method + " " + route)
	span.SetAttributes(attr)
	if l, ok := otelhttp.LabelerFromContext(r.Context()); ok {
		l.Add(attr)
	}
}

// APIHeaders sets the headers every API response carries. no-store: answers depend on the
// caller's session, so no proxy or browser cache may keep one and hand it to someone else (one
// careless proxy cache rule would serve lawyer A's leads to lawyer B). nosniff: a browser never
// treats a JSON body as HTML or script.
func APIHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}
