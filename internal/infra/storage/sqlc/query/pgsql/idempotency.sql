-- name: GetIdempotencyKey :one
SELECT resource_id
FROM idempotency_keys
WHERE user_id = $1 AND idempotency_key = $2
;

-- name: InsertIdempotencyKey :exec
INSERT INTO idempotency_keys (user_id, idempotency_key, resource_id, created_at)
VALUES ($1, $2, $3, $4)
;

-- name: DeleteIdempotencyKey :exec
DELETE FROM idempotency_keys WHERE user_id = $1 AND idempotency_key = $2
;

-- name: PurgeIdempotencyKeys :exec
DELETE FROM idempotency_keys WHERE created_at < $1
;
