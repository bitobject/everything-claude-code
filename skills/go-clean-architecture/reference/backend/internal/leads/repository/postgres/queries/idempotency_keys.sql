-- name: ClaimIdempotencyKey :execrows
-- Stores the key, or takes over one created before expired_before. 0 rows: the key belongs to a
-- live request (read it with GetIdempotencyKey). A concurrent claim of the same key waits until
-- this transaction ends.
INSERT INTO leads.idempotency_keys AS k (key, fingerprint, lead_id, created_at)
VALUES (sqlc.arg(key), sqlc.arg(fingerprint), sqlc.arg(lead_id), sqlc.arg(created_at))
ON CONFLICT (key) DO UPDATE
SET fingerprint = excluded.fingerprint,
    lead_id = excluded.lead_id,
    created_at = excluded.created_at
WHERE k.created_at < sqlc.arg(expired_before);

-- name: DeleteIdempotencyKeysCreatedBefore :execrows
-- The hourly purge: a key created before created_before can no longer match a retry. The table
-- holds about a day of keys, so this scans it without an index on created_at.
DELETE FROM leads.idempotency_keys
WHERE created_at < sqlc.arg(created_before);

-- name: GetIdempotencyKey :one
SELECT key, fingerprint, lead_id, created_at
FROM leads.idempotency_keys
WHERE key = $1;
