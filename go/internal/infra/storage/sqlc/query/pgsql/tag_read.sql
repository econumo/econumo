-- Read-model query for the tag module (PostgreSQL variant: $N placeholders).
-- See the sqlite variant for documentation.

-- name: GetTagListView :many
SELECT id, user_id, name, position, is_archived, created_at, updated_at
FROM tags
WHERE user_id = $1
ORDER BY position
;
