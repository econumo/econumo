-- Connection module queries (SQLite). accounts_access holds per-account grants
-- to connected users; users_connections is the symmetric user link. Roles are
-- admin=0, user=1, guest=2.

-- name: GetAccountAccess :one
SELECT account_id, user_id, role, created_at, updated_at
FROM accounts_access
WHERE account_id = ? AND user_id = ?;

-- name: UpsertAccountAccess :exec
INSERT INTO accounts_access (account_id, user_id, role, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (account_id, user_id) DO UPDATE SET
    role       = excluded.role,
    updated_at = excluded.updated_at;

-- name: DeleteAccountAccess :exec
DELETE FROM accounts_access
WHERE account_id = ? AND user_id = ?;

-- name: ListReceivedAccountAccess :many
-- Grants TO this user (accounts shared with them).
SELECT account_id, user_id, role, created_at, updated_at
FROM accounts_access
WHERE user_id = ?;

-- name: ListAccountAccessByAccount :many
-- All grants ON one account (for the account's sharedAccess[] embed).
SELECT account_id, user_id, role, created_at, updated_at
FROM accounts_access
WHERE account_id = ?;

-- name: ListIssuedAccountAccess :many
-- Grants on accounts OWNED by this user (issued to others).
SELECT aa.account_id, aa.user_id, aa.role, aa.created_at, aa.updated_at
FROM accounts_access aa
JOIN accounts a ON a.id = aa.account_id
WHERE a.user_id = ?;

-- name: ListConnectedUserIDs :many
SELECT connected_user_id
FROM users_connections
WHERE user_id = ?;

-- name: AccountOwnerID :one
SELECT user_id FROM accounts WHERE id = ?;

-- name: DeleteAccountOptionForUser :exec
DELETE FROM accounts_options
WHERE account_id = ? AND user_id = ?;

-- name: DeleteConnectionLink :exec
DELETE FROM users_connections
WHERE (user_id = ? AND connected_user_id = ?)
   OR (user_id = ? AND connected_user_id = ?);

-- name: InsertConnectionLink :exec
-- Idempotently create one direction of the symmetric users_connections link.
INSERT INTO users_connections (user_id, connected_user_id)
VALUES (?, ?)
ON CONFLICT (user_id, connected_user_id) DO NOTHING;

-- name: GetConnectionInviteByUser :one
SELECT user_id, code, expired_at
FROM users_connections_invites
WHERE user_id = ?;

-- name: GetConnectionInviteByCode :one
-- Look up a non-expired invite by code. The caller passes 'now' as a
-- 'Y-m-d H:i:s' string so the comparison is against the stored datetime TEXT
-- (a time.Time bound mis-compares at the boundary; see the budget read notes).
SELECT user_id, code, expired_at
FROM users_connections_invites
WHERE code = ? AND expired_at IS NOT NULL AND datetime(expired_at) >= datetime(?);

-- name: UpsertConnectionInvite :exec
-- One invite row per user (user_id PK). code/expired_at are nullable (a cleared
-- invite). expired_at is bound as a 'Y-m-d H:i:s' string (or NULL).
INSERT INTO users_connections_invites (user_id, code, expired_at)
VALUES (?, ?, ?)
ON CONFLICT (user_id) DO UPDATE SET
    code       = excluded.code,
    expired_at = excluded.expired_at;
