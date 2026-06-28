-- Read-side query for the transaction CSV export (SQLite). Returns the user's
-- accessible accounts (own + shared via accounts_access, not deleted) with their
-- currency code joined. Mirrors AccountRepository::getUserAccounts (own OR an
-- accounts_access grant). Used to build the selectable-account map and to resolve
-- each row's account name + currency code.

-- name: ListExportAccountsForUser :many
SELECT DISTINCT a.id AS id, a.name AS name, c.code AS currency_code
FROM accounts a
LEFT JOIN accounts_access aa ON aa.account_id = a.id
JOIN currencies c ON c.id = a.currency_id
WHERE a.is_deleted = 0 AND (a.user_id = ? OR aa.user_id = ?);
