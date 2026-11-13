-- Two roles (create them once per cluster, see the README): app_owner owns the schemas and runs
-- migrations (`api migrate`, MIGRATE_DATABASE_URL); app_runtime is the running API and worker
-- (DATABASE_URL) and only reads and writes rows: no DDL, no TRUNCATE. Every table, sequence and
-- schema app_owner creates from here on is usable by app_runtime. An append-only table (a
-- ledger, an audit log) revokes UPDATE and DELETE from app_runtime in the migration that
-- creates it. Rename the roles for your project.

-- +goose Up
ALTER DEFAULT PRIVILEGES FOR ROLE app_owner
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO app_runtime;
ALTER DEFAULT PRIVILEGES FOR ROLE app_owner
    GRANT USAGE, SELECT ON SEQUENCES TO app_runtime;
ALTER DEFAULT PRIVILEGES FOR ROLE app_owner
    GRANT USAGE ON SCHEMAS TO app_runtime;

-- +goose Down
ALTER DEFAULT PRIVILEGES FOR ROLE app_owner
    REVOKE USAGE ON SCHEMAS FROM app_runtime;
ALTER DEFAULT PRIVILEGES FOR ROLE app_owner
    REVOKE USAGE, SELECT ON SEQUENCES FROM app_runtime;
ALTER DEFAULT PRIVILEGES FOR ROLE app_owner
    REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLES FROM app_runtime;
