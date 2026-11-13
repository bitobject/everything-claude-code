// Package httpserver holds transport plumbing shared by every module's delivery adapter:
// RFC 9457 problem responses, error mapping, middleware, health endpoints and the configured
// *http.Server.
package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"example.com/app/internal/kernel/apperr"
	"example.com/app/internal/platform/logging"
)

// ProblemTypeBase prefixes a problem code to make its RFC 9457 type URI. Replace example.com
// with a page that describes the codes; once clients have seen a URI, it must not change.
const ProblemTypeBase = "https://example.com/problems/"

// statusClientClosedRequest is nginx's convention for "the client went away before the answer";
// nobody receives it, it only makes access logs honest.
const statusClientClosedRequest = 499

// Problem is an RFC 9457 problem document. It mirrors components/schemas/Problem in
// api/openapi.yaml; the delivery tests validate every response against the contract.
type Problem struct {
	Type      string       `json:"type"`
	Title     string       `json:"title"`
	Status    int          `json:"status"`
	Detail    string       `json:"detail,omitempty"`
	Code      string       `json:"code"`
	RequestID string       `json:"requestId,omitempty"`
	Errors    []FieldError `json:"errors,omitempty"`
}

// FieldError names the part of a request that broke a rule. It never carries the submitted
// value: values can be personal data, and clients already have them.
type FieldError struct {
	Pointer   string `json:"pointer,omitempty"`   // JSON Pointer into the body, as a URI fragment: "#/phone"
	Parameter string `json:"parameter,omitempty"` // path, query or header parameter name
	Code      string `json:"code"`                // the rule: "required", "pattern", "maxLength", ...
}

// NewProblem returns the problem for code. title is the fixed summary of the problem type.
func NewProblem(status int, code, title string) Problem {
	return Problem{Type: ProblemTypeBase + code, Title: title, Status: status, Code: code}
}

// StatusOf maps an error kind to an HTTP status. This is the only place that does it, and every
// kind is listed (exhaustive, without a default: a new kind is a lint error here).
func StatusOf(k apperr.Kind) int {
	switch k {
	case apperr.KindInternal:
		return http.StatusInternalServerError
	case apperr.KindInvalid:
		return http.StatusUnprocessableEntity
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindConflict:
		return http.StatusConflict
	case apperr.KindForbidden:
		return http.StatusForbidden
	case apperr.KindUnauthorized:
		return http.StatusUnauthorized
	case apperr.KindRateLimited:
		return http.StatusTooManyRequests
	case apperr.KindUnavailable:
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}

// WriteProblem writes p as application/problem+json, with the request ID from r's context.
func WriteProblem(w http.ResponseWriter, r *http.Request, p Problem) {
	p.RequestID = logging.RequestID(r.Context())
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p) // the client is gone if this fails; nothing left to do
}

// SetRetryAfter sets the Retry-After header in whole seconds, rounded up.
func SetRetryAfter(w http.ResponseWriter, d time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(d.Seconds()))))
}

// timeoutRetryAfter is the hint sent with a 503 for a request that ran out of time.
const timeoutRetryAfter = 5 * time.Second

// ErrorWriter returns the function the generated strict handler calls when a handler returns
// an error. It logs by status, once, here at the edge:
//   - the client went away (context.Canceled): DEBUG, status 499, no body;
//   - the request ran out of time (context.DeadlineExceeded): WARN, 503 "timeout";
//   - an *apperr.Error: its problem; WARN when the status is 5xx (503 "unavailable"), not logged
//     when it is 4xx (the client's mistake, not ours);
//   - anything else: ERROR, and a generic 500, so internals never reach the client.
func ErrorWriter(log *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		ctx := r.Context()
		switch {
		case errors.Is(err, context.Canceled) && errors.Is(ctx.Err(), context.Canceled):
			log.DebugContext(ctx, "request canceled by the client", "method", r.Method, "route", r.Pattern)
			w.WriteHeader(statusClientClosedRequest)
			return
		case errors.Is(err, context.DeadlineExceeded):
			log.WarnContext(ctx, "request timed out", "method", r.Method, "route", r.Pattern, "err", err)
			SetRetryAfter(w, timeoutRetryAfter)
			WriteProblem(w, r, NewProblem(http.StatusServiceUnavailable, "timeout", "the request took too long, retry later"))
			return
		}
		if e, ok := errors.AsType[*apperr.Error](err); ok && e.Kind() != apperr.KindInternal {
			status := StatusOf(e.Kind())
			if status >= http.StatusInternalServerError {
				log.WarnContext(ctx, "request failed", "method", r.Method, "route", r.Pattern, "code", e.Code(), "err", err)
			}
			if d := e.RetryAfter(); d > 0 {
				SetRetryAfter(w, d)
			}
			WriteProblem(w, r, NewProblem(status, e.Code(), e.Error()))
			return
		}
		log.ErrorContext(ctx, "request failed", "method", r.Method, "route", r.Pattern, "err", err)
		WriteProblem(w, r, NewProblem(http.StatusInternalServerError, "internal", "internal error"))
	}
}
