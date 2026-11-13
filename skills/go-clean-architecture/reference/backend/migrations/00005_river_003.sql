-- River migration 003 (main line, river job tags non null), from github.com/riverqueue/river/riverdriver/riverpgxv5
-- v0.51.2, migration/main/003_*.sql. River's SQL is unchanged except that its schema placeholder is
-- set to river. Written by cmd/rivermigrations; never edit an applied one.
-- squawk skips river_* files (.squawk.toml): the SQL is River's.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE river.river_job ALTER COLUMN tags SET DEFAULT '{}';
UPDATE river.river_job SET tags = '{}' WHERE tags IS NULL;
ALTER TABLE river.river_job ALTER COLUMN tags SET NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE river.river_job
    ALTER COLUMN tags DROP NOT NULL,
    ALTER COLUMN tags DROP DEFAULT;
-- +goose StatementEnd
