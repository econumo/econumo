-- Write-side queries for the label module (PostgreSQL variant: $N placeholders).
-- See the sqlite variant for documentation. Unlike tags, a label's icon IS
-- persisted from the start.

-- name: GetLabelByID :one
SELECT id, user_id, name, icon, sort_key, is_archived, created_at, updated_at
FROM labels
WHERE id = $1
;

-- name: CountLabelsByOwner :one
SELECT COUNT(*) FROM labels WHERE user_id = $1
;

-- name: ListLabelsByOwner :many
-- The owner's labels ordered by sort key; used by move-label (load, place the
-- moved row, save it) and as the basis for the returned list.
SELECT id, user_id, name, icon, sort_key, is_archived, created_at, updated_at
FROM labels
WHERE user_id = $1
ORDER BY sort_key, id
;

-- name: UpsertLabel :exec
INSERT INTO labels (id, user_id, name, icon, sort_key, is_archived, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE SET
    user_id     = excluded.user_id,
    name        = excluded.name,
    icon        = excluded.icon,
    sort_key    = excluded.sort_key,
    is_archived = excluded.is_archived,
    updated_at  = excluded.updated_at
;

-- name: DeleteLabel :exec
DELETE FROM labels WHERE id = $1
;
