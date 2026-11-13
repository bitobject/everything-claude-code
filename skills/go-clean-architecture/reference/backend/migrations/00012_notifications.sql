-- The notifications module's schema: messages for the operators' feed, one per lead.

-- +goose Up
CREATE SCHEMA notifications;

CREATE TABLE notifications.messages (
    id         uuid        PRIMARY KEY,
    lead_id    uuid        NOT NULL CONSTRAINT messages_lead_id_key UNIQUE, -- one message per lead: the job may run twice
    body       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
-- squawk-ignore ban-drop-table
DROP TABLE notifications.messages;
-- squawk-ignore ban-drop-schema
DROP SCHEMA notifications;
