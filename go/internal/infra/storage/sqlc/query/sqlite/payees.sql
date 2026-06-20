-- Write-side queries for the payee module. The read-side query lives in
-- payee_read.sql to keep the CQRS boundary visible (matching tags.sql vs
-- tag_read.sql). The payees table has the same shape as tags (no type/icon
-- columns): a payee is a name + position + archived flag.

-- name: GetPayeeByID :one
SELECT id, user_id, name, position, is_archived, created_at, updated_at
FROM payees
WHERE id = ?
;

-- name: CountPayeesByOwner :one
-- New-payee position = count of the owner's existing payees.
SELECT COUNT(*) FROM payees WHERE user_id = ?
;

-- name: ListPayeesByOwner :many
-- The owner's payees ordered by position; used by order-payee-list (load, apply
-- position changes, re-save) and as the basis for the returned list.
SELECT id, user_id, name, position, is_archived, created_at, updated_at
FROM payees
WHERE user_id = ?
ORDER BY position
;

-- name: UpsertPayee :exec
INSERT INTO payees (id, user_id, name, position, is_archived, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    user_id     = excluded.user_id,
    name        = excluded.name,
    position    = excluded.position,
    is_archived = excluded.is_archived,
    updated_at  = excluded.updated_at
;

-- name: DeletePayee :exec
-- Transactions referencing this payee have payee_id set to NULL via the ON
-- DELETE SET NULL FK, matching the PHP delete behaviour.
DELETE FROM payees WHERE id = ?
;
