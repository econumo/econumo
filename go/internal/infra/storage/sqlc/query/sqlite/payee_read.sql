-- Read-model query for the payee module (CQRS read side). Tailored to the
-- response shape; bypasses the domain aggregate. Separate from the write queries
-- (payees.sql) to keep the read and write concerns visibly distinct.

-- name: GetPayeeListView :many
-- All of the user's payees (archived and not) ordered by position.
SELECT id, user_id, name, position, is_archived, created_at, updated_at
FROM payees
WHERE user_id = ?
ORDER BY position
;
