-- OAuth authorization server (MCP clients): clients, codes, grants.

-- name: InsertOAuthClient :exec
INSERT INTO oauth_clients (id, name, redirect_uris, secret_hash, created_at, last_used_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetOAuthClient :one
SELECT id, name, redirect_uris, secret_hash, created_at, last_used_at FROM oauth_clients WHERE id = $1;

-- name: MarkOAuthClientUsed :exec
UPDATE oauth_clients SET last_used_at = $1 WHERE id = $2;

-- name: PurgeUnusedOAuthClients :execrows
DELETE FROM oauth_clients WHERE last_used_at IS NULL AND created_at < $1;

-- name: InsertOAuthCode :exec
INSERT INTO oauth_authorization_codes (code_hash, client_id, user_id, redirect_uri, code_challenge, resource, credentials_generation, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ConsumeOAuthCode :one
-- Single use: the DELETE is the read, so two concurrent exchanges of one code
-- cannot both get a row.
DELETE FROM oauth_authorization_codes WHERE code_hash = $1
RETURNING code_hash, client_id, user_id, redirect_uri, code_challenge, resource, credentials_generation, created_at, expires_at;

-- name: PurgeExpiredOAuthCodes :exec
DELETE FROM oauth_authorization_codes WHERE expires_at < $1;

-- name: InsertOAuthGrant :exec
INSERT INTO oauth_grants (id, user_id, client_id, refresh_token_hash, prev_refresh_token_hash, rotated_at, created_at, last_used_at, expires_at, revoked_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: GetOAuthGrant :one
SELECT id, user_id, client_id, refresh_token_hash, prev_refresh_token_hash, rotated_at, created_at, last_used_at, expires_at, revoked_at
FROM oauth_grants WHERE id = $1;

-- name: GetOAuthGrantByRefreshHash :one
SELECT id, user_id, client_id, refresh_token_hash, prev_refresh_token_hash, rotated_at, created_at, last_used_at, expires_at, revoked_at
FROM oauth_grants WHERE refresh_token_hash = $1;

-- name: GetOAuthGrantByPrevRefreshHash :one
SELECT id, user_id, client_id, refresh_token_hash, prev_refresh_token_hash, rotated_at, created_at, last_used_at, expires_at, revoked_at
FROM oauth_grants WHERE prev_refresh_token_hash = $1;

-- name: RotateOAuthGrant :execrows
-- Conditional on the hash being rotated away, so of two concurrent refreshes
-- presenting the same token exactly one rotates.
UPDATE oauth_grants
SET prev_refresh_token_hash = refresh_token_hash, refresh_token_hash = $1, rotated_at = $2, last_used_at = $3, expires_at = $4
WHERE id = $5 AND refresh_token_hash = $6 AND revoked_at IS NULL;

-- name: RevokeOAuthGrant :execrows
UPDATE oauth_grants SET revoked_at = $1 WHERE id = $2 AND revoked_at IS NULL;

-- name: RevokeUserOAuthGrants :execrows
UPDATE oauth_grants SET revoked_at = $1 WHERE user_id = $2 AND revoked_at IS NULL;

-- name: ListUnrevokedOAuthGrants :many
SELECT g.id, g.user_id, g.client_id, g.refresh_token_hash, g.prev_refresh_token_hash, g.rotated_at,
       g.created_at, g.last_used_at, g.expires_at, g.revoked_at, c.name, c.redirect_uris
FROM oauth_grants g
JOIN oauth_clients c ON c.id = g.client_id
WHERE g.user_id = $1 AND g.revoked_at IS NULL
ORDER BY g.created_at, g.id;

-- name: DeleteDeadOAuthGrants :execrows
DELETE FROM oauth_grants
WHERE (revoked_at IS NOT NULL AND revoked_at < $1)
   OR expires_at < $2;

