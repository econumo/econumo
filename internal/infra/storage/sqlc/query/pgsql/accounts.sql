-- Write-side queries for the account module (PostgreSQL: $N placeholders). See
-- the sqlite variant for documentation.

-- name: GetAccountByID :one
SELECT id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at
FROM accounts
WHERE id = $1;

-- name: ListAvailableAccounts :many
-- Available accounts: own OR shared via accounts_access, not deleted (see the
-- sqlite variant). $1 is reused for both sides so the param stays single.
SELECT DISTINCT a.id, a.currency_id, a.user_id, a.name, a.type, a.icon, a.is_deleted, a.created_at, a.updated_at
FROM accounts a
LEFT JOIN accounts_access aa ON aa.account_id = a.id
WHERE a.is_deleted = false AND (a.user_id = $1 OR aa.user_id = $1);

-- name: CountAvailableAccounts :one
SELECT COUNT(*) FROM (
    SELECT DISTINCT a.id
    FROM accounts a
    LEFT JOIN accounts_access aa ON aa.account_id = a.id
    WHERE a.is_deleted = false AND (a.user_id = $1 OR aa.user_id = $1)
) t;

-- name: UpsertAccount :exec
INSERT INTO accounts (id, currency_id, user_id, name, type, icon, is_deleted, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
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
WHERE account_id = $1 AND user_id = $2;

-- name: ListAccountOptionsByUser :many
SELECT account_id, user_id, position, created_at, updated_at
FROM accounts_options
WHERE user_id = $1;

-- name: UpsertAccountOption :exec
INSERT INTO accounts_options (account_id, user_id, position, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (account_id, user_id) DO UPDATE SET
    position   = excluded.position,
    updated_at = excluded.updated_at;
