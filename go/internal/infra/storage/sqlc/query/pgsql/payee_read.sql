-- Read-model query for the payee module (PostgreSQL variant: $N placeholders).
-- See the sqlite variant for documentation.

-- name: GetPayeeListView :many
-- Available payees: own + payees of users who shared an account with this user.
-- $1 is reused for both positions so the generated param stays single.
SELECT p.id, p.user_id, p.name, p.position, p.is_archived, p.created_at, p.updated_at
FROM payees p
WHERE p.user_id = $1
   OR p.user_id IN (
       SELECT a.user_id
       FROM accounts_access aa
       JOIN accounts a ON a.id = aa.account_id
       WHERE aa.user_id = $1
   )
ORDER BY p.position
;
