-- OAuth feature queries: identities, in-flight states, one-shot handoffs.

-- name: GetIdentityByProviderSubject :one
SELECT id, user_id, provider, subject, email, created_at, updated_at
FROM users_identities
WHERE provider = $1 AND subject = $2;

-- name: GetIdentityByUserProvider :one
SELECT id, user_id, provider, subject, email, created_at, updated_at
FROM users_identities
WHERE user_id = $1 AND provider = $2;

-- name: ListIdentitiesByUser :many
SELECT id, user_id, provider, subject, email, created_at, updated_at
FROM users_identities
WHERE user_id = $1
ORDER BY created_at, id;

-- name: CountIdentitiesByUser :one
SELECT COUNT(*) FROM users_identities WHERE user_id = $1;

-- name: UpsertIdentity :exec
INSERT INTO users_identities (id, user_id, provider, subject, email, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO UPDATE SET
    email      = excluded.email,
    updated_at = excluded.updated_at;

-- name: DeleteIdentityByUserProvider :execrows
DELETE FROM users_identities WHERE user_id = $1 AND provider = $2;

-- name: InsertOAuthState :exec
INSERT INTO oauth_states (state_hash, provider, nonce, code_verifier, client, intent, link_user_id, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: GetOAuthState :one
SELECT state_hash, provider, nonce, code_verifier, client, intent, link_user_id, created_at, expires_at
FROM oauth_states
WHERE state_hash = $1;

-- name: DeleteOAuthState :exec
DELETE FROM oauth_states WHERE state_hash = $1;

-- name: DeleteExpiredOAuthStates :execrows
DELETE FROM oauth_states WHERE expires_at < $1;

-- name: InsertOAuthHandoff :exec
INSERT INTO oauth_handoffs (code_hash, user_id, provider, id_token, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetOAuthHandoff :one
SELECT code_hash, user_id, provider, id_token, created_at, expires_at
FROM oauth_handoffs
WHERE code_hash = $1;

-- name: DeleteOAuthHandoff :exec
DELETE FROM oauth_handoffs WHERE code_hash = $1;

-- name: DeleteExpiredOAuthHandoffs :execrows
DELETE FROM oauth_handoffs WHERE expires_at < $1;
