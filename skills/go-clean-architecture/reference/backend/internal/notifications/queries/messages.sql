-- name: InsertMessage :exec
-- One message per lead: a job that runs again inserts nothing.
INSERT INTO notifications.messages (id, lead_id, body)
VALUES ($1, $2, $3)
ON CONFLICT (lead_id) DO NOTHING;
