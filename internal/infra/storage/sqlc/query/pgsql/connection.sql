-- Connection module queries (PostgreSQL). accounts_access holds per-account
-- grants to connected users; users_connections is the symmetric user link.
-- Roles are admin=0, user=1, guest=2.

-- name: GetAccountAccess :one
SELECT account_id, user_id, role, created_at, updated_at, is_accepted
FROM accounts_access
WHERE account_id = $1 AND user_id = $2;

-- name: UpsertAccountAccess :exec
INSERT INTO accounts_access (account_id, user_id, role, is_accepted, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (account_id, user_id) DO UPDATE SET
    role        = excluded.role,
    is_accepted = excluded.is_accepted,
    updated_at  = excluded.updated_at;

-- name: DeleteAccountAccess :exec
DELETE FROM accounts_access
WHERE account_id = $1 AND user_id = $2;

-- name: ListReceivedAccountAccess :many
-- Grants TO this user (accounts shared with them).
SELECT account_id, user_id, role, created_at, updated_at, is_accepted
FROM accounts_access
WHERE user_id = $1;

-- name: ListAccountAccessByAccount :many
-- All grants ON one account (for the account's sharedAccess[] embed).
SELECT account_id, user_id, role, created_at, updated_at, is_accepted
FROM accounts_access
WHERE account_id = $1;

-- name: ListIssuedAccountAccess :many
-- Grants on accounts OWNED by this user (issued to others).
SELECT aa.account_id, aa.user_id, aa.role, aa.created_at, aa.updated_at, aa.is_accepted
FROM accounts_access aa
JOIN accounts a ON a.id = aa.account_id
WHERE a.user_id = $1;

-- name: ListPendingReceivedAccountAccess :many
-- Pending grants TO this user (invites awaiting acceptance). Ordered so both
-- engines return identical row order.
SELECT account_id, user_id, role, created_at, updated_at, is_accepted
FROM accounts_access
WHERE user_id = $1 AND is_accepted = false
ORDER BY created_at, account_id;

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

-- name: InsertConnectionLink :exec
INSERT INTO users_connections (user_id, connected_user_id)
VALUES ($1, $2)
ON CONFLICT (user_id, connected_user_id) DO NOTHING;

-- name: GetConnectionInviteByUser :one
SELECT user_id, code, expired_at
FROM users_connections_invites
WHERE user_id = $1;

-- name: GetConnectionInviteByCode :one
SELECT user_id, code, expired_at
FROM users_connections_invites
WHERE code = $1 AND expired_at IS NOT NULL AND expired_at >= $2;

-- name: UpsertConnectionInvite :exec
INSERT INTO users_connections_invites (user_id, code, expired_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE SET
    code       = excluded.code,
    expired_at = excluded.expired_at;
