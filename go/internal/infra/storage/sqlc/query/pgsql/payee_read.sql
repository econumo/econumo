-- Read-model query for the payee module (PostgreSQL variant: $N placeholders).
-- See the sqlite variant for documentation.

-- name: GetPayeeListView :many
SELECT id, user_id, name, position, is_archived, created_at, updated_at
FROM payees
WHERE user_id = $1
ORDER BY position
;
