-- name: InsertLead :exec
INSERT INTO leads.leads (id, phone, topic, city, status, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetLead :one
SELECT id, phone, topic, city, status, created_at
FROM leads.leads
WHERE id = $1;

-- name: GetLeadForUpdate :one
-- FOR NO KEY UPDATE blocks other writers of the row but not inserts into tables whose foreign
-- keys reference it (FOR UPDATE would block their key checks too).
SELECT id, phone, topic, city, status, created_at
FROM leads.leads
WHERE id = $1
FOR NO KEY UPDATE;

-- name: UpdateLead :exec
-- Writes every column the aggregate can change, so a field a new entity method changes is saved
-- without touching the adapter. id and created_at never change.
UPDATE leads.leads
SET phone = sqlc.arg(phone),
    topic = sqlc.arg(topic),
    city = sqlc.arg(city),
    status = sqlc.arg(status)
WHERE id = sqlc.arg(id);

-- name: CountLeadsByPhoneSince :one
SELECT count(*)
FROM leads.leads
WHERE phone = sqlc.arg(phone) AND created_at >= sqlc.arg(since);

-- name: GetLeadView :one
-- The read side: only what a view shows, no phone number.
SELECT id, topic, city, status, created_at
FROM leads.leads
WHERE id = $1;
