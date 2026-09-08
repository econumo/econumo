-- OAuth feature queries: identities, in-flight states, one-shot handoffs.

-- name: GetIdentityByProviderSubject :one
SELECT id, user_id, provider, subject, email, created_at, updated_at
FROM users_identities
WHERE provider = ? AND subject = ?;

-- name: GetIdentityByUserProvider :one
SELECT id, user_id, provider, subject, email, created_at, updated_at
FROM users_identities
WHERE user_id = ? AND provider = ?;

-- name: ListIdentitiesByUser :many
SELECT id, user_id, provider, subject, email, created_at, updated_at
FROM users_identities
WHERE user_id = ?
ORDER BY created_at, id;

-- name: CountIdentitiesByUser :one
SELECT COUNT(*) FROM users_identities WHERE user_id = ?;

-- name: UpsertIdentity :exec
INSERT INTO users_identities (id, user_id, provider, subject, email, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    email      = excluded.email,
    updated_at = excluded.updated_at;

-- name: DeleteIdentityByUserProvider :execrows
DELETE FROM users_identities WHERE user_id = ? AND provider = ?;

-- name: InsertOAuthState :exec
INSERT INTO oauth_states (state_hash, provider, nonce, code_verifier, client, intent, link_user_id, created_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetOAuthState :one
SELECT state_hash, provider, nonce, code_verifier, client, intent, link_user_id, created_at, expires_at
FROM oauth_states
WHERE state_hash = ?;

-- name: DeleteOAuthState :exec
DELETE FROM oauth_states WHERE state_hash = ?;

-- name: DeleteExpiredOAuthStates :execrows
DELETE FROM oauth_states WHERE expires_at < ?;

-- name: InsertOAuthHandoff :exec
INSERT INTO oauth_handoffs (code_hash, user_id, provider, id_token, created_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetOAuthHandoff :one
SELECT code_hash, user_id, provider, id_token, created_at, expires_at
FROM oauth_handoffs
WHERE code_hash = ?;

-- name: DeleteOAuthHandoff :exec
DELETE FROM oauth_handoffs WHERE code_hash = ?;

-- name: DeleteExpiredOAuthHandoffs :execrows
DELETE FROM oauth_handoffs WHERE expires_at < ?;
