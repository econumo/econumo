-- Connection module queries (PostgreSQL). accounts_access holds per-account
-- grants to connected users; users_connections is the symmetric user link.
-- Roles are admin=0, user=1, guest=2.

-- name: GetAccountAccess :one
SELECT account_id, user_id, role, created_at, updated_at
FROM accounts_access
WHERE account_id = $1 AND user_id = $2;

-- name: UpsertAccountAccess :exec
INSERT INTO accounts_access (account_id, user_id, role, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (account_id, user_id) DO UPDATE SET
    role       = excluded.role,
    updated_at = excluded.updated_at;

-- name: DeleteAccountAccess :exec
DELETE FROM accounts_access
WHERE account_id = $1 AND user_id = $2;

-- name: ListReceivedAccountAccess :many
-- Grants TO this user (accounts shared with them).
SELECT account_id, user_id, role, created_at, updated_at
FROM accounts_access
WHERE user_id = $1;

-- name: ListAccountAccessByAccount :many
-- All grants ON one account (for the account's sharedAccess[] embed).
SELECT account_id, user_id, role, created_at, updated_at
FROM accounts_access
WHERE account_id = $1;

-- name: ListIssuedAccountAccess :many
-- Grants on accounts OWNED by this user (issued to others).
SELECT aa.account_id, aa.user_id, aa.role, aa.created_at, aa.updated_at
FROM accounts_access aa
JOIN accounts a ON a.id = aa.account_id
WHERE a.user_id = $1;

-- name: ListConnectedUserIDs :many
SELECT connected_user_id
FROM users_connections
WHERE user_id = $1;

-- name: AccountOwnerID :one
SELECT user_id FROM accounts WHERE id = $1;

-- name: DeleteAccountOptionForUser :exec
DELETE FROM accounts_options
WHERE account_id = $1 AND user_id = $2;

-- name: DeleteConnectionLink :exec
DELETE FROM users_connections
WHERE (user_id = $1 AND connected_user_id = $2)
   OR (user_id = $3 AND connected_user_id = $4);
