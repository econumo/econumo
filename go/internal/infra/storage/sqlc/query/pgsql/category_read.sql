-- Read-model queries for the category module (PostgreSQL engine, $N placeholders).

-- name: GetCategoryListView :many
SELECT id, user_id, name, position, type, icon, is_archived, created_at, updated_at
FROM categories
WHERE user_id = $1
ORDER BY position;
