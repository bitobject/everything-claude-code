// Package clock provides the production implementation of the use cases' Clock port.
package clock

import "time"

// System reads the wall clock.
type System struct{}

// Now returns the current time in UTC, truncated to microseconds: PostgreSQL's precision, so a
// time reads back from the database exactly as it was written and returned (an API response and
// a later read of the same row agree).
func (System) Now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }
