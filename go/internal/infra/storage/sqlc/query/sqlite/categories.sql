-- Write-side queries for the category module. The read-side queries live in
-- category_read.sql to keep the CQRS boundary visible (matching users.sql vs
-- user_read.sql).

-- name: GetCategoryByID :one
SELECT id, user_id, name, position, type, icon, is_archived, created_at, updated_at
FROM categories
WHERE id = ?;

-- name: CountCategoriesByOwner :one
-- New-category position = count of the owner's existing categories.
SELECT COUNT(*) FROM categories WHERE user_id = ?;

-- name: ListCategoriesByOwner :many
-- The owner's categories ordered by position; used by order-category-list (load,
-- apply position changes, re-save) and as the basis for the returned list.
SELECT id, user_id, name, position, type, icon, is_archived, created_at, updated_at
FROM categories
WHERE user_id = ?
ORDER BY position;

-- name: UpsertCategory :exec
INSERT INTO categories (id, user_id, name, position, type, icon, is_archived, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    user_id     = excluded.user_id,
    name        = excluded.name,
    position    = excluded.position,
    type        = excluded.type,
    icon        = excluded.icon,
    is_archived = excluded.is_archived,
    updated_at  = excluded.updated_at;

-- name: DeleteCategory :exec
-- Transactions referencing this category have category_id set to NULL via the
-- ON DELETE SET NULL FK, matching the PHP delete-mode behaviour.
DELETE FROM categories WHERE id = ?;

-- name: ReassignCategoryTransactions :exec
-- Replace-mode: point every transaction on the old category at the new one
-- before the old category is deleted (mirrors TransactionRepository::replaceCategory).
UPDATE transactions SET category_id = ? WHERE category_id = ?;

-- The operation_requests_ids idempotency queries moved to operations.sql (shared
-- across modules that take a client-supplied operation id).
