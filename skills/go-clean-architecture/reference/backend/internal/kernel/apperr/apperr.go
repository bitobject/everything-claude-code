// Package apperr is the shared kernel for application errors: a kind that adapters map to a
// transport status, and a stable code that clients can rely on. It imports only the standard
// library, so every layer may use it.
package apperr

import (
	"errors"
	"time"
)

// Kind classifies an error for adapters. Inner layers choose the kind; adapters translate it
// (HTTP status, job retry policy) without knowing the concrete error.
type Kind uint8

// Error kinds, from the most common default to the transport-specific ones. Adding a kind means
// adding it to Kinds and to every switch over Kind (the exhaustive linter lists them).
const (
	KindInternal Kind = iota
	KindInvalid
	KindNotFound
	KindConflict
	KindForbidden
	KindUnauthorized
	KindRateLimited
	KindUnavailable
)

// Kinds returns every kind, so adapters can test that each one has a mapping.
func Kinds() []Kind {
	return []Kind{
		KindInternal, KindInvalid, KindNotFound, KindConflict,
		KindForbidden, KindUnauthorized, KindRateLimited, KindUnavailable,
	}
}

func (k Kind) String() string {
	switch k {
	case KindInternal:
		return "internal"
	case KindInvalid:
		return "invalid"
	case KindNotFound:
		return "not_found"
	case KindConflict:
		return "conflict"
	case KindForbidden:
		return "forbidden"
	case KindUnauthorized:
		return "unauthorized"
	case KindRateLimited:
		return "rate_limited"
	case KindUnavailable:
		return "unavailable"
	}
	return "unknown"
}

// Error is an application error. Declare sentinel values with New (package-level vars) and
// compare them with errors.Is; add context with fmt.Errorf("...: %w", err). The fields are
// unexported, so a sentinel can't be changed by the packages that use it.
type Error struct {
	kind       Kind
	code       string
	msg        string
	retryAfter time.Duration
}

// New returns a sentinel application error. code is stable and dotted ("lead.not_found"); msg
// is the fixed, client-safe summary of the problem: no personal data, no internals.
func New(kind Kind, code, msg string) *Error {
	return &Error{kind: kind, code: code, msg: msg}
}

// WithRetryAfter returns a copy of e that tells clients how long to wait before retrying (the
// HTTP adapter sends it as Retry-After). Use it where the sentinel is declared.
func (e *Error) WithRetryAfter(d time.Duration) *Error {
	c := *e
	c.retryAfter = d
	return &c
}

func (e *Error) Error() string { return e.msg }

// Kind returns how adapters should report the error.
func (e *Error) Kind() Kind { return e.kind }

// Code returns the stable machine-readable code.
func (e *Error) Code() string { return e.code }

// RetryAfter returns the retry hint, or zero.
func (e *Error) RetryAfter() time.Duration { return e.retryAfter }

// KindOf reports the kind of the first *Error in err's chain, or KindInternal.
func KindOf(err error) Kind {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.kind
	}
	return KindInternal
}
