-- Read-model query for the tag module (PostgreSQL variant: $N placeholders).
-- See the sqlite variant for documentation.

-- name: GetTagListView :many
-- Available tags: own + tags of users who shared an account with this user.
-- $1 is reused for both positions so the generated param stays single.
SELECT t.id, t.user_id, t.name, t.position, t.is_archived, t.created_at, t.updated_at
FROM tags t
WHERE t.user_id = $1
   OR t.user_id IN (
       SELECT a.user_id
       FROM accounts_access aa
       JOIN accounts a ON a.id = aa.account_id
       WHERE aa.user_id = $1
   )
ORDER BY t.position, t.id
;
