-- River's tables live in their own schema. The next files are River's migrations as goose
-- migrations (one history, one lock, one `api migrate`), written by cmd/rivermigrations from the
-- River version in go.mod; River's own migration 001 (its river_migration table) is left out
-- because only River's migrator uses it. After a River upgrade, run `go run ./cmd/rivermigrations`:
-- migrations/river_test.go fails until every River migration is here.

-- +goose Up
CREATE SCHEMA river;

-- +goose Down
-- squawk-ignore ban-drop-schema
DROP SCHEMA river;
