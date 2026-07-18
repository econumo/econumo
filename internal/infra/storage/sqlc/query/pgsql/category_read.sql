-- Read-model queries for the category module (PostgreSQL engine, $N placeholders).

-- name: GetCategoryListView :many
-- Available categories: own + categories of users who shared an account with
-- this user (see the sqlite variant). $1 is reused for both positions so the
-- generated param stays single.
SELECT c.id, c.user_id, c.name, c.position, c.type, c.icon, c.is_archived, c.created_at, c.updated_at
FROM categories c
WHERE c.user_id = $1
   OR c.user_id IN (
       SELECT a.user_id
       FROM accounts_access aa
       JOIN accounts a ON a.id = aa.account_id
       WHERE aa.user_id = $1 AND aa.is_accepted = true
   )
ORDER BY c.position, c.id;
