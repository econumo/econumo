-- Write-side queries for the category module (PostgreSQL engine, $N placeholders).

-- name: GetCategoryByID :one
SELECT id, user_id, name, type, icon, is_archived, created_at, updated_at, sort_key
FROM categories
WHERE id = $1;

-- name: CountCategoriesByOwner :one
SELECT COUNT(*) FROM categories WHERE user_id = $1;

-- name: ListCategoriesByOwner :many
SELECT id, user_id, name, type, icon, is_archived, created_at, updated_at, sort_key
FROM categories
WHERE user_id = $1
ORDER BY sort_key, id;

-- name: UpsertCategory :exec
INSERT INTO categories (id, user_id, name, type, icon, is_archived, created_at, updated_at, sort_key)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (id) DO UPDATE SET
    user_id     = excluded.user_id,
    name        = excluded.name,
    sort_key    = excluded.sort_key,
    type        = excluded.type,
    icon        = excluded.icon,
    is_archived = excluded.is_archived,
    updated_at  = excluded.updated_at;

-- name: DeleteCategory :exec
DELETE FROM categories WHERE id = $1;

-- name: ReassignCategoryTransactions :exec
UPDATE transactions SET category_id = $1 WHERE category_id = $2;

-- The operation_requests_ids idempotency queries moved to operations.sql (shared
-- across modules that take a client-supplied operation id).

-- name: ReassignCategoryRecurring :exec
UPDATE recurring_transactions SET category_id = $1 WHERE category_id = $2;
