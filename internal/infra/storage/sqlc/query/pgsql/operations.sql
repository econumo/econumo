-- Idempotency queries over operation_requests_ids (PostgreSQL variant: $N
-- placeholders). Shared by every module whose create endpoint takes a
-- client-supplied operation id. See the sqlite variant for documentation.

-- name: InsertOperationId :exec
INSERT INTO operation_requests_ids (id, is_handled, created_at, updated_at)
VALUES ($1, $2, $3, $4)
;

-- name: GetOperationId :one
SELECT id, is_handled, created_at, updated_at
FROM operation_requests_ids
WHERE id = $1
;

-- name: MarkOperationHandled :exec
UPDATE operation_requests_ids SET is_handled = $1, updated_at = $2 WHERE id = $3
;
