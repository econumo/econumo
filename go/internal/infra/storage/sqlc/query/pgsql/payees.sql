-- Write-side queries for the payee module (PostgreSQL variant: $N placeholders).
-- See the sqlite variant for documentation; the SQL is identical apart from the
-- placeholder syntax. The payees table has no type/icon columns.

-- name: GetPayeeByID :one
SELECT id, user_id, name, position, is_archived, created_at, updated_at
FROM payees
WHERE id = $1
;

-- name: CountPayeesByOwner :one
SELECT COUNT(*) FROM payees WHERE user_id = $1
;

-- name: ListPayeesByOwner :many
SELECT id, user_id, name, position, is_archived, created_at, updated_at
FROM payees
WHERE user_id = $1
ORDER BY position
;

-- name: UpsertPayee :exec
INSERT INTO payees (id, user_id, name, position, is_archived, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO UPDATE SET
    user_id     = excluded.user_id,
    name        = excluded.name,
    position    = excluded.position,
    is_archived = excluded.is_archived,
    updated_at  = excluded.updated_at
;

-- name: DeletePayee :exec
DELETE FROM payees WHERE id = $1
;
