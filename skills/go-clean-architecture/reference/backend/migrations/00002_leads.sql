-- The leads module's schema. Only internal/leads reads or writes it (internal/archtest checks
-- the queries). Constraints and indexes the code matches on are named.

-- +goose Up
CREATE SCHEMA leads;

CREATE TABLE leads.leads (
    id         uuid        PRIMARY KEY,           -- uuid.NewV7() from the application
    phone      text        NOT NULL,              -- personal data
    topic      text        NOT NULL,
    city       text        NOT NULL,
    status     text        NOT NULL CONSTRAINT leads_status_check CHECK (status IN ('new', 'assigned', 'closed')),
    created_at timestamptz NOT NULL
);

-- The per-phone limit of CreateLead counts recent leads of one number.
CREATE INDEX leads_phone_created_at_idx ON leads.leads (phone, created_at);

-- Idempotency-Key of createLead: which lead a key created, and a fingerprint of the request.
-- A key older than 24 hours is taken over by the next request that uses it, and deleted by the
-- hourly purge job (the fingerprint is derived from the phone number).
CREATE TABLE leads.idempotency_keys (
    key         text        PRIMARY KEY CONSTRAINT idempotency_keys_key_check CHECK (length(key) BETWEEN 16 AND 64),
    fingerprint bytea       NOT NULL,
    lead_id     uuid        NOT NULL,
    created_at  timestamptz NOT NULL
);

-- +goose Down
-- squawk-ignore ban-drop-table
DROP TABLE leads.idempotency_keys;
-- squawk-ignore ban-drop-table
DROP TABLE leads.leads;
-- squawk-ignore ban-drop-schema
DROP SCHEMA leads;
