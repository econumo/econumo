-- Write-side queries for the tag module. The read-side query lives in
-- tag_read.sql to keep the CQRS boundary visible (matching categories.sql vs
-- category_read.sql). Unlike categories, a tag has no type column, but it does
-- have a persisted icon (see internal/model/tag.go).

-- name: GetTagByID :one
SELECT id, user_id, name, icon, position, is_archived, created_at, updated_at
FROM tags
WHERE id = ?
;

-- name: CountTagsByOwner :one
-- New-tag position = count of the owner's existing tags.
SELECT COUNT(*) FROM tags WHERE user_id = ?
;

-- name: ListTagsByOwner :many
-- The owner's tags ordered by position; used by order-tag-list (load, apply
-- position changes, re-save) and as the basis for the returned list.
SELECT id, user_id, name, icon, position, is_archived, created_at, updated_at
FROM tags
WHERE user_id = ?
ORDER BY position, id
;

-- name: UpsertTag :exec
INSERT INTO tags (id, user_id, name, icon, position, is_archived, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    user_id     = excluded.user_id,
    name        = excluded.name,
    icon        = excluded.icon,
    position    = excluded.position,
    is_archived = excluded.is_archived,
    updated_at  = excluded.updated_at
;

-- name: DeleteTag :exec
-- Transactions referencing this tag have tag_id set to NULL via the ON DELETE
-- SET NULL FK, matching the PHP delete behaviour.
DELETE FROM tags WHERE id = ?
;
