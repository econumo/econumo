-- Write-side queries for the tag module (PostgreSQL variant: $N placeholders).
-- See the sqlite variant for documentation; the SQL is identical apart from the
-- placeholder syntax. Unlike categories, a tag has no type column, but it does
-- have a persisted icon.

-- name: GetTagByID :one
SELECT id, user_id, name, is_archived, created_at, updated_at, sort_key, icon
FROM tags
WHERE id = $1
;

-- name: CountTagsByOwner :one
SELECT COUNT(*) FROM tags WHERE user_id = $1
;

-- name: ListTagsByOwner :many
SELECT id, user_id, name, is_archived, created_at, updated_at, sort_key, icon
FROM tags
WHERE user_id = $1
ORDER BY sort_key, id
;

-- name: UpsertTag :exec
INSERT INTO tags (id, user_id, name, is_archived, created_at, updated_at, sort_key, icon)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE SET
    user_id     = excluded.user_id,
    name        = excluded.name,
    sort_key    = excluded.sort_key,
    icon        = excluded.icon,
    is_archived = excluded.is_archived,
    updated_at  = excluded.updated_at
;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = $1
;

-- name: ReassignTagTransactions :exec
UPDATE transactions SET tag_id = $1 WHERE tag_id = $2;

-- name: ReassignTagRecurring :exec
UPDATE recurring_transactions SET tag_id = $1 WHERE tag_id = $2;
