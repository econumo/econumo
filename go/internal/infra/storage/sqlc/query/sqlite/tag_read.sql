-- Read-model query for the tag module (CQRS read side). Tailored to the
-- response shape; bypasses the domain aggregate. Separate from the write queries
-- (tags.sql) to keep the read and write concerns visibly distinct.

-- name: GetTagListView :many
-- All of the user's tags (archived and not) ordered by position.
SELECT id, user_id, name, position, is_archived, created_at, updated_at
FROM tags
WHERE user_id = ?
ORDER BY position
;
