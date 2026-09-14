-- Access-token queries (access_tokens). See the sqlite sibling for the flow;
-- liveness is evaluated in the app layer, not SQL.

-- name: InsertAccessTokenIfGeneration :execrows
-- See the sqlite sibling.
INSERT INTO access_tokens (id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at, provider, id_token)
SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
WHERE EXISTS (SELECT 1 FROM users u WHERE u.id = $13 AND u.credentials_generation = $14);

-- name: InsertAccessTokenIfPresenterLive :execrows
-- See the sqlite sibling.
INSERT INTO access_tokens (id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at, provider, id_token)
SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
WHERE EXISTS (SELECT 1 FROM access_tokens p WHERE p.id = $13 AND p.user_id = $14 AND p.revoked_at IS NULL);

-- name: GetAccessTokenByHash :one
-- Joins users for access_level/access_until; see the sqlite sibling for why
-- provider/id_token are deliberately omitted here.
SELECT t.id, t.user_id, t.kind, t.token_hash, t.name, t.user_agent,
       t.created_at, t.last_used_at, t.expires_at, t.revoked_at,
       u.access_level, u.access_until
FROM access_tokens t
JOIN users u ON u.id = t.user_id
WHERE t.token_hash = $1;

-- name: GetAccessTokenByID :one
SELECT id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at, provider, id_token
FROM access_tokens
WHERE id = $1;

-- name: UpdateAccessToken :exec
UPDATE access_tokens SET last_used_at = $1, expires_at = $2, revoked_at = $3 WHERE id = $4;

-- name: ListAccessTokensByUser :many
SELECT id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at, provider, id_token
FROM access_tokens
WHERE user_id = $1 AND kind = $2
ORDER BY created_at, id;

-- name: DeleteAccessToken :exec
DELETE FROM access_tokens WHERE id = $1;

-- name: DeleteDeadAccessTokens :execrows
DELETE FROM access_tokens
WHERE (revoked_at IS NOT NULL AND revoked_at < $1)
   OR (expires_at IS NOT NULL AND expires_at < $2);
