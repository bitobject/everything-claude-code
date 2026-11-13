package postgres

import (
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"example.com/app/internal/kernel/apperr"
)

// SQLSTATE codes the adapters match on.
const (
	CodeUniqueViolation      = "23505"
	CodeSerializationFailure = "40001"
	CodeDeadlockDetected     = "40P01"
	CodeLockNotAvailable     = "55P03" // lock_timeout, NOWAIT
	CodeQueryCanceled        = "57014" // statement_timeout
	CodeTransactionTimeout   = "25P04" // transaction_timeout (PostgreSQL 17+)
)

// ErrBusy is the error for contention and server-side timeouts: retrying later can succeed.
var ErrBusy = apperr.New(apperr.KindUnavailable, "unavailable", "the service is busy, retry later").
	WithRetryAfter(time.Second)

// MapError turns contention and timeouts into ErrBusy, keeping the cause for the log. Module
// adapters handle their own facts first (pgx.ErrNoRows → not found, a unique violation of a
// named constraint → conflict) and pass the rest here.
func MapError(err error) error {
	if err == nil {
		return nil
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pgErr.Code {
		case CodeSerializationFailure, CodeDeadlockDetected, CodeLockNotAvailable,
			CodeQueryCanceled, CodeTransactionTimeout:
			return fmt.Errorf("%w: %w", ErrBusy, err)
		}
	}
	return err
}

// IsUniqueViolation reports whether err is a unique violation of constraint.
func IsUniqueViolation(err error, constraint string) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == CodeUniqueViolation && pgErr.ConstraintName == constraint
}
