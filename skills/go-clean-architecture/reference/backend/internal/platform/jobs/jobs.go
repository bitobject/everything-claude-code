// Package jobs holds what every River worker shares.
package jobs

import (
	"github.com/riverqueue/river"

	"example.com/app/internal/kernel/apperr"
)

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
