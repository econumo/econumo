-- Write-side queries for the tag module (PostgreSQL variant: $N placeholders).
-- See the sqlite variant for documentation; the SQL is identical apart from the
-- placeholder syntax. Unlike categories, a tag has no type column, but it does
-- have a persisted icon.

-- name: GetTagByID :one
SELECT id, user_id, name, icon, position, is_archived, created_at, updated_at
FROM tags
WHERE id = $1
;

-- name: CountTagsByOwner :one
SELECT COUNT(*) FROM tags WHERE user_id = $1
;

-- name: ListTagsByOwner :many
SELECT id, user_id, name, icon, position, is_archived, created_at, updated_at
FROM tags
WHERE user_id = $1
ORDER BY position, id
;

-- name: UpsertTag :exec
INSERT INTO tags (id, user_id, name, icon, position, is_archived, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE SET
    user_id     = excluded.user_id,
    name        = excluded.name,
    icon        = excluded.icon,
    position    = excluded.position,
    is_archived = excluded.is_archived,
    updated_at  = excluded.updated_at
;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = $1
;
