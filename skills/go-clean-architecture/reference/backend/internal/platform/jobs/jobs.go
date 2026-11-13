// Package jobs holds what every River worker shares.
package jobs

import (
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/riverqueue/rivercontrib/otelriver"

	"example.com/app/internal/kernel/apperr"
)

// Middleware is what every River client installs, the API's insert-only one and the worker's:
// OpenTelemetry through the global providers. On insert it copies the caller's trace context
// (traceparent) into the job's metadata; on work it starts a span river.work/<kind>, linked to
// the span that enqueued the job, so a request and the jobs it caused find each other. It adds
// the river.* metrics (work duration, insert counts) by kind and queue. Job args are not
// recorded: they hold IDs only anyway.
func Middleware() []rivertype.Middleware {
	return []rivertype.Middleware{otelriver.NewMiddleware(&otelriver.MiddlewareConfig{
		EnableTracePropagation:      true,
		EnableWorkSpanJobKindSuffix: true,
	})}
}

// Outcome turns a worker's error into River's decision. Errors a retry can't fix (the lead is
// gone, the input is invalid, the state doesn't allow it) cancel the job; River would otherwise
// retry them for weeks (25 attempts). Everything else is returned as is and retried with
// backoff. Workers end with `return jobs.Outcome(err)`.
func Outcome(err error) error {
	if err == nil {
		return nil
	}
	switch apperr.KindOf(err) {
	case apperr.KindNotFound, apperr.KindInvalid, apperr.KindConflict:
		return river.JobCancel(err)
	case apperr.KindInternal, apperr.KindForbidden, apperr.KindUnauthorized,
		apperr.KindRateLimited, apperr.KindUnavailable:
		return err
	}
	return err
}
