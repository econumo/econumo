-- Access-token queries (access_tokens): login sessions + personal access
-- tokens. Liveness (revoked/expired) is evaluated in the app layer (Go
-- time.Time), not in SQL, to avoid engine date-format differences; the
-- list/get queries return raw rows.

-- name: InsertAccessToken :exec
INSERT INTO access_tokens (id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetAccessTokenByHash :one
SELECT id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at
FROM access_tokens
WHERE token_hash = ?;

-- name: GetAccessTokenByID :one
SELECT id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at
FROM access_tokens
WHERE id = ?;

-- name: UpdateAccessToken :exec
UPDATE access_tokens SET last_used_at = ?, expires_at = ?, revoked_at = ? WHERE id = ?;

-- name: ListAccessTokensByUser :many
SELECT id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at
FROM access_tokens
WHERE user_id = ? AND kind = ?
ORDER BY created_at, id;

-- name: DeleteAccessToken :exec
DELETE FROM access_tokens WHERE id = ?;
