package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"slices"
	"time"
	"uuid"

	"example.com/app/internal/platform/logging"
)

// Middleware wraps a handler. cmd lists them outermost first (see Chain).
type Middleware func(http.Handler) http.Handler

// Chain wraps h so that the first middleware runs first.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for _, mw := range slices.Backward(mws) {
		h = mw(h)
	}
	return h
}

const maxBodyBytes = 1 << 20

// LimitBody caps request bodies so a client cannot exhaust memory; the validator turns the
// overflow into 413.
func LimitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

// requestIDPattern accepts the IDs our proxy generates (nginx $request_id is 32 hex digits) and
// UUIDs, nothing that could inject into logs.
var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]{16,64}$`)

// RequestID puts a request ID in the context (logging adds it to every line) and in the
// X-Request-Id response header. It keeps the proxy's ID when it looks like one, so proxy and
// application logs join up; otherwise it makes a new one.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if !requestIDPattern.MatchString(id) {
			id = uuid.NewV7().String()
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(logging.WithRequestID(r.Context(), id)))
	})
}

// AccessLog writes one line per request: method, path without the query string (it can carry
// personal data), status, duration. Run it inside RequestID so the line has the request ID.
func AccessLog(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			log.InfoContext(r.Context(), "request",
				"method", r.Method, "path", r.URL.Path, "status", rec.status,
				"duration_ms", time.Since(start).Milliseconds())
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status, s.wroteHeader = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.wroteHeader = true
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer (flush, deadlines).
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Recover turns a panic in a handler into a logged 500 (with the stack) instead of a dropped
// connection.
func Recover(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
						panic(v) // the server's own signal to abort the response; let it through
					}
					log.ErrorContext(r.Context(), "panic", "method", r.Method, "path", r.URL.Path,
						"panic", v, "stack", string(debug.Stack()))
					WriteProblem(w, r, NewProblem(http.StatusInternalServerError, "internal", "internal error"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Timeout gives every request a deadline. http.Server.WriteTimeout doesn't cancel the handler's
// context, so without this a slow query keeps its goroutine and connection after the answer can
// no longer be sent. d must be shorter than WriteTimeout, so the 503 still reaches the client
// (ErrorWriter maps context.DeadlineExceeded).
func Timeout(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
