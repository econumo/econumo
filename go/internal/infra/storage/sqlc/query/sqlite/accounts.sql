-- Write-side queries for the account module (SQLite). Accounts are soft-deleted
-- (is_deleted); per-user ordering lives in accounts_options; folder membership
-- in accounts_folders. The balance is NOT stored -- it is computed from the
-- transactions table (see account_balance.sql).

-- name: GetAccountByID :one
SELECT id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at
FROM accounts
WHERE id = ?;

-- name: ListAvailableAccounts :many
-- Available accounts: own OR shared via accounts_access, not deleted. Mirrors
-- AccountRepository::getAvailableForUserId (LEFT JOIN accounts_access, own OR
-- granted). DISTINCT collapses duplicate rows when multiple grants exist.
SELECT DISTINCT a.id, a.currency_id, a.user_id, a.name, a.type, a.icon, a.is_deleted, a.created_at, a.updated_at
FROM accounts a
LEFT JOIN accounts_access aa ON aa.account_id = a.id
WHERE a.is_deleted = 0 AND (a.user_id = ? OR aa.user_id = ?);

-- name: CountAvailableAccounts :one
SELECT COUNT(*) FROM (
    SELECT DISTINCT a.id
    FROM accounts a
    LEFT JOIN accounts_access aa ON aa.account_id = a.id
    WHERE a.is_deleted = 0 AND (a.user_id = ? OR aa.user_id = ?)
) t;

-- name: UpsertAccount :exec
INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    currency_id = excluded.currency_id,
    name        = excluded.name,
    type        = excluded.type,
    icon        = excluded.icon,
    is_deleted  = excluded.is_deleted,
    updated_at  = excluded.updated_at;

-- name: GetAccountOption :one
SELECT account_id, user_id, position, created_at, updated_at
FROM accounts_options
WHERE account_id = ? AND user_id = ?;

-- name: ListAccountOptionsByUser :many
SELECT account_id, user_id, position, created_at, updated_at
FROM accounts_options
WHERE user_id = ?;

-- name: UpsertAccountOption :exec
INSERT INTO accounts_options (account_id, user_id, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (account_id, user_id) DO UPDATE SET
    position   = excluded.position,
    updated_at = excluded.updated_at;
