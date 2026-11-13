// Package config holds types for configuration values that cmd parses from the environment.
package config

import "log/slog"

// Secret is a configuration value that must not reach logs: a DSN with a password, an API key.
// fmt and slog print it as [redacted]; Reveal returns the value for the one place that needs it.
// caarlos0/env fills it like a string.
type Secret string

const redacted = "[redacted]"

// Reveal returns the secret value.
func (s Secret) Reveal() string { return string(s) }

// String hides the value from fmt.
func (s Secret) String() string { return redacted }

// GoString hides the value from %#v.
func (s Secret) GoString() string { return redacted }

// LogValue hides the value from slog.
func (s Secret) LogValue() slog.Value { return slog.StringValue(redacted) }
