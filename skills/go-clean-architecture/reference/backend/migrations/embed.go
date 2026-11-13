// Package migrations embeds the goose SQL migrations so the binary carries its schema.
package migrations

import "embed"

// FS holds the migration files, applied in version order by goose.
//
//go:embed *.sql
var FS embed.FS
