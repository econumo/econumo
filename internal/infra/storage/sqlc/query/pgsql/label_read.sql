-- Read-model query for the label module (PostgreSQL variant: $N placeholders).
-- See the sqlite variant for documentation.

-- name: GetLabelListView :many
-- Available labels: own + labels of users who shared an account with this user.
-- $1 is reused for both positions so the generated param stays single.
SELECT l.id, l.user_id, l.name, l.icon, l.sort_key, l.is_archived, l.created_at, l.updated_at
FROM labels l
WHERE l.user_id = $1
   OR l.user_id IN (
       SELECT a.user_id
       FROM accounts_access aa
       JOIN accounts a ON a.id = aa.account_id
       WHERE aa.user_id = $1 AND aa.is_accepted = true
   )
ORDER BY l.sort_key, l.id
;
