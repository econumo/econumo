-- Write-side queries for the tag module. The read-side query lives in
-- tag_read.sql to keep the CQRS boundary visible (matching categories.sql vs
-- category_read.sql). Unlike categories, a tag has no type column, but it does
-- have a persisted icon.

-- name: GetTagByID :one
SELECT id, user_id, name, is_archived, created_at, updated_at, sort_key, icon
FROM tags
WHERE id = ?
;

-- name: CountTagsByOwner :one
SELECT COUNT(*) FROM tags WHERE user_id = ?
;

-- name: ListTagsByOwner :many
-- The owner's tags ordered by sort key; used by move-tag (load,
-- place the moved row, save it) and as the basis for the returned list.
SELECT id, user_id, name, is_archived, created_at, updated_at, sort_key, icon
FROM tags
WHERE user_id = ?
ORDER BY sort_key, id
;

-- name: UpsertTag :exec
INSERT INTO tags (id, user_id, name, is_archived, created_at, updated_at, sort_key, icon)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    user_id     = excluded.user_id,
    name        = excluded.name,
    sort_key    = excluded.sort_key,
    icon        = excluded.icon,
    is_archived = excluded.is_archived,
    updated_at  = excluded.updated_at
;

-- name: DeleteTag :exec
-- Transactions referencing this tag have tag_id set to NULL via the ON DELETE
-- SET NULL FK, matching the PHP delete behaviour.
DELETE FROM tags WHERE id = ?
;

-- name: ReassignTagTransactions :exec
-- Merge: see ReassignPayeeTransactions for why this is not scoped by user.
UPDATE transactions SET tag_id = ? WHERE tag_id = ?;

-- name: ReassignTagRecurring :exec
UPDATE recurring_transactions SET tag_id = ? WHERE tag_id = ?;
