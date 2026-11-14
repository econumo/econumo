-- Write-side queries for the payee module (PostgreSQL variant: $N placeholders).
-- See the sqlite variant for documentation; the SQL is identical apart from the
-- placeholder syntax. The payees table has no type/icon columns.

-- name: GetPayeeByID :one
SELECT id, user_id, name, is_archived, created_at, updated_at, sort_key
FROM payees
WHERE id = $1
;

-- name: CountPayeesByOwner :one
SELECT COUNT(*) FROM payees WHERE user_id = $1
;

-- name: ListPayeesByOwner :many
SELECT id, user_id, name, is_archived, created_at, updated_at, sort_key
FROM payees
WHERE user_id = $1
ORDER BY sort_key, id
;

-- name: UpsertPayee :exec
INSERT INTO payees (id, user_id, name, is_archived, created_at, updated_at, sort_key)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO UPDATE SET
    user_id     = excluded.user_id,
    name        = excluded.name,
    sort_key    = excluded.sort_key,
    is_archived = excluded.is_archived,
    updated_at  = excluded.updated_at
;

-- name: DeletePayee :exec
DELETE FROM payees WHERE id = $1
;

-- name: ReassignPayeeTransactions :exec
UPDATE transactions SET payee_id = $1 WHERE payee_id = $2;

-- name: ReassignPayeeRecurring :exec
UPDATE recurring_transactions SET payee_id = $1 WHERE payee_id = $2;
