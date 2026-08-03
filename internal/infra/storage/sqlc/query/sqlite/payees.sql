-- Write-side queries for the payee module. The read-side query lives in
-- payee_read.sql to keep the CQRS boundary visible (matching tags.sql vs
-- tag_read.sql). The payees table has the same shape as tags (no type/icon
-- columns): a payee is a name + sort key + archived flag.

-- name: GetPayeeByID :one
SELECT id, user_id, name, is_archived, created_at, updated_at, sort_key
FROM payees
WHERE id = ?
;

-- name: CountPayeesByOwner :one
SELECT COUNT(*) FROM payees WHERE user_id = ?
;

-- name: ListPayeesByOwner :many
-- The owner's payees ordered by sort key; used by move-payee (load,
-- place the moved row, save it) and as the basis for the returned list.
SELECT id, user_id, name, is_archived, created_at, updated_at, sort_key
FROM payees
WHERE user_id = ?
ORDER BY sort_key, id
;

-- name: UpsertPayee :exec
INSERT INTO payees (id, user_id, name, is_archived, created_at, updated_at, sort_key)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    user_id     = excluded.user_id,
    name        = excluded.name,
    sort_key    = excluded.sort_key,
    is_archived = excluded.is_archived,
    updated_at  = excluded.updated_at
;

-- name: DeletePayee :exec
-- Transactions referencing this payee have payee_id set to NULL via the ON
-- DELETE SET NULL FK, matching the PHP delete behaviour.
DELETE FROM payees WHERE id = ?
;
