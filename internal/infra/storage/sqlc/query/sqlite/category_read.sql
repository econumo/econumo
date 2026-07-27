-- Read-model queries for the category module (CQRS read side). Tailored to the
-- response shape; bypasses the domain aggregate. Separate from the write queries
-- (categories.sql) to keep the read and write concerns visibly distinct.

-- name: GetCategoryListView :many
-- Available categories: the user's OWN categories plus the categories of every
-- user who has shared an account WITH this user. Mirrors PHP
-- CategoryRepository::findAvailableForUserId (self + DISTINCT owners of accounts
-- granted to the user via accounts_access), ordered by position. The user id is
-- repeated positionally, so sqlc generates a two-field Params struct.
SELECT c.id, c.user_id, c.name, c.position, c.type, c.icon, c.is_archived, c.created_at, c.updated_at
FROM categories c
WHERE c.user_id = ?
   OR c.user_id IN (
       SELECT a.user_id
       FROM accounts_access aa
       JOIN accounts a ON a.id = aa.account_id
       WHERE aa.user_id = ? AND aa.is_accepted = 1
   )
ORDER BY c.position, c.id
;
