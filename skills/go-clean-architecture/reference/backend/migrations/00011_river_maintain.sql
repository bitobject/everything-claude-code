-- River's reindexer rebuilds the indexes of river.river_job every night (REINDEX
-- CONCURRENTLY). That needs the table's owner or the MAINTAIN privilege (PostgreSQL 17+), so
-- app_runtime, which runs the worker, gets MAINTAIN on this one table. MAINTAIN also allows
-- VACUUM, ANALYZE, CLUSTER and LOCK there, never ALTER, TRUNCATE or DROP.

-- +goose Up
GRANT MAINTAIN ON river.river_job TO app_runtime;

-- +goose Down
REVOKE MAINTAIN ON river.river_job FROM app_runtime;
