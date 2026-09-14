-- Access-token queries (access_tokens): login sessions + personal access
-- tokens. Liveness (revoked/expired) is evaluated in the app layer (Go
-- time.Time), not in SQL, to avoid engine date-format differences; the
-- list/get queries return raw rows.

-- name: InsertAccessToken :exec
INSERT INTO access_tokens (id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at, provider, id_token)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertAccessTokenIfGeneration :execrows
-- Mints a token only while the user's credentials generation is still the one
-- the caller's evidence was read under: an account reclaim bumps it, so a
-- session built on evidence from before the reclaim inserts nothing.
INSERT INTO access_tokens (id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at, provider, id_token)
SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
WHERE EXISTS (SELECT 1 FROM users u WHERE u.id = ? AND u.credentials_generation = ?);

-- name: InsertAccessTokenIfPresenterLive :execrows
-- Mints a personal token only while the credential that authenticated the
-- request (the presenting token) is still unrevoked: the reclaim revokes
-- every token in the same transaction that bumps the generation, so a
-- request that passed the auth middleware before the reclaim inserts
-- nothing after it.
INSERT INTO access_tokens (id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at, provider, id_token)
SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
WHERE EXISTS (SELECT 1 FROM access_tokens p WHERE p.id = ? AND p.user_id = ? AND p.revoked_at IS NULL);

-- name: GetAccessTokenByHash :one
-- Joins users for access_level/access_until so per-request auth can report
-- the caller's effective access level in the same round trip. This does NOT
-- reuse the is_active shortcut (see GetAccessTokenByHash's Go caller): a
-- lapsed user must still authenticate, just read-only.
SELECT t.id, t.user_id, t.kind, t.token_hash, t.name, t.user_agent,
       t.created_at, t.last_used_at, t.expires_at, t.revoked_at, t.provider, t.id_token,
       u.access_level, u.access_until
FROM access_tokens t
JOIN users u ON u.id = t.user_id
WHERE t.token_hash = ?;

-- name: GetAccessTokenByID :one
SELECT id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at, provider, id_token
FROM access_tokens
WHERE id = ?;

-- name: UpdateAccessToken :exec
UPDATE access_tokens SET last_used_at = ?, expires_at = ?, revoked_at = ? WHERE id = ?;

-- name: ListAccessTokensByUser :many
SELECT id, user_id, kind, token_hash, name, user_agent, created_at, last_used_at, expires_at, revoked_at, provider, id_token
FROM access_tokens
WHERE user_id = ? AND kind = ?
ORDER BY created_at, id;

-- name: DeleteAccessToken :exec
DELETE FROM access_tokens WHERE id = ?;

-- name: DeleteDeadAccessTokens :execrows
DELETE FROM access_tokens
WHERE (revoked_at IS NOT NULL AND revoked_at < ?)
   OR (expires_at IS NOT NULL AND expires_at < ?);
