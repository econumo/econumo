-- Access-token queries (access_tokens). See the sqlite sibling for the flow;
-- liveness is evaluated in the app layer, not SQL.

-- name: InsertAccessToken :exec
INSERT INTO access_tokens (id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: GetAccessTokenByHash :one
SELECT id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at
FROM access_tokens
WHERE token_hash = $1;

-- name: GetAccessTokenByID :one
SELECT id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at
FROM access_tokens
WHERE id = $1;

-- name: UpdateAccessToken :exec
UPDATE access_tokens SET last_used_at = $1, expires_at = $2, revoked_at = $3 WHERE id = $4;

-- name: ListAccessTokensByUser :many
SELECT id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at
FROM access_tokens
WHERE user_id = $1 AND kind = $2
ORDER BY created_at, id;

-- name: DeleteAccessToken :exec
DELETE FROM access_tokens WHERE id = $1;

-- name: DeleteDeadAccessTokens :execrows
DELETE FROM access_tokens
WHERE (revoked_at IS NOT NULL AND revoked_at < $1)
   OR (expires_at IS NOT NULL AND expires_at < $2);
