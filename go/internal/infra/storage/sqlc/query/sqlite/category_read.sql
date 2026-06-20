-- Read-model queries for the category module (CQRS read side). Tailored to the
-- response shape; bypasses the domain aggregate. Separate from the write queries
-- (categories.sql) to keep the read and write concerns visibly distinct.

-- name: GetCategoryListView :many
-- All of the user's categories (archived and not) ordered by position.
SELECT id, user_id, name, position, type, icon, is_archived, created_at, updated_at
FROM categories
WHERE user_id = ?
ORDER BY position
;
