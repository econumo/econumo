-- name: GetIdempotencyKey :one
SELECT resource_id
FROM idempotency_keys
WHERE user_id = ? AND idempotency_key = ?
;

-- name: InsertIdempotencyKey :exec
INSERT INTO idempotency_keys (user_id, idempotency_key, resource_id, created_at)
VALUES (?, ?, ?, ?)
;

-- name: DeleteIdempotencyKey :exec
DELETE FROM idempotency_keys WHERE user_id = ? AND idempotency_key = ?
;

-- name: PurgeIdempotencyKeys :exec
DELETE FROM idempotency_keys WHERE created_at < ?
;
