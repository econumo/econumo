-- Read-model query for the payee module (CQRS read side). Tailored to the
-- response shape; bypasses the domain aggregate. Separate from the write queries
-- (payees.sql) to keep the read and write concerns visibly distinct.

-- name: GetPayeeListView :many
-- Available payees: the user's OWN payees plus the payees of every user who has
-- shared an account WITH this user. Mirrors PHP
-- PayeeRepository::findAvailableForUserId (self + DISTINCT owners of accounts
-- granted via accounts_access), ordered by position. The user id is repeated
-- positionally -> two-field Params struct.
SELECT p.id, p.user_id, p.name, p.position, p.is_archived, p.created_at, p.updated_at
FROM payees p
WHERE p.user_id = ?
   OR p.user_id IN (
       SELECT a.user_id
       FROM accounts_access aa
       JOIN accounts a ON a.id = aa.account_id
       WHERE aa.user_id = ?
   )
ORDER BY p.position, p.id
;
