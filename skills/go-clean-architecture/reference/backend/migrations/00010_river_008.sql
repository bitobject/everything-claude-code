-- River migration 008 (main line, job id autoincrement), from github.com/riverqueue/river/riverdriver/riverpgxv5
-- v0.51.2, migration/main/008_*.sql. River's SQL is unchanged except that its schema placeholder is
-- set to river. Written by cmd/rivermigrations; never edit an applied one.
-- squawk skips river_* files (.squawk.toml): the SQL is River's.

-- +goose Up
-- +goose StatementBegin
-- No-op. Postgres sequences already prevent automatically generated job IDs
-- from being reused.
SELECT 1;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- No-op. Postgres sequences already prevent automatically generated job IDs
-- from being reused.
SELECT 1;
-- +goose StatementEnd
