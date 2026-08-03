-- Write-side queries for the label module. The read-side query lives in
-- label_read.sql to keep the CQRS boundary visible (matching tags.sql vs
-- tag_read.sql). Unlike tags, a label's icon IS persisted from the start.

-- name: GetLabelByID :one
SELECT id, user_id, name, icon, position, is_archived, created_at, updated_at
FROM labels
WHERE id = ?
;

-- name: CountLabelsByOwner :one
-- New-label position = count of the owner's existing labels.
SELECT COUNT(*) FROM labels WHERE user_id = ?
;

-- name: ListLabelsByOwner :many
SELECT id, user_id, name, icon, position, is_archived, created_at, updated_at
FROM labels
WHERE user_id = ?
ORDER BY position, id
;

-- name: UpsertLabel :exec
INSERT INTO labels (id, user_id, name, icon, position, is_archived, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    user_id     = excluded.user_id,
    name        = excluded.name,
    icon        = excluded.icon,
    position    = excluded.position,
    is_archived = excluded.is_archived,
    updated_at  = excluded.updated_at
;

-- name: DeleteLabel :exec
-- transactions_labels rows for this label are removed by ON DELETE CASCADE;
-- unlike tags there is no SET NULL, because the link is a join table.
DELETE FROM labels WHERE id = ?
;
