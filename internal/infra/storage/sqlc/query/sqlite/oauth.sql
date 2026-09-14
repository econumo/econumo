-- OAuth feature queries: identities, in-flight states, one-shot handoffs.

-- name: GetIdentityByProviderSubject :one
SELECT id, user_id, provider, issuer, subject, email, created_at, updated_at
FROM users_identities
WHERE provider = ? AND issuer = ? AND subject = ?;

-- name: GetIdentityByUserProvider :one
SELECT id, user_id, provider, issuer, subject, email, created_at, updated_at
FROM users_identities
WHERE user_id = ? AND provider = ?;

-- name: ListIdentitiesByUser :many
SELECT id, user_id, provider, issuer, subject, email, created_at, updated_at
FROM users_identities
WHERE user_id = ?
ORDER BY created_at, id;

-- name: CountIdentitiesByUser :one
SELECT COUNT(*) FROM users_identities WHERE user_id = ?;

-- name: InsertIdentityIfGeneration :execrows
-- Same fence as InsertAccessTokenIfGeneration: a callback that resolved its
-- user before an account reclaim must not land an identity after it. A plain
-- INSERT, never an upsert: an identity the reclaim deleted must stay deleted.
INSERT INTO users_identities (id, user_id, provider, issuer, subject, email, created_at, updated_at)
SELECT ?, ?, ?, ?, ?, ?, ?, ?
WHERE EXISTS (SELECT 1 FROM users u WHERE u.id = ? AND u.credentials_generation = ?);

-- name: UpdateIdentityIfGeneration :execrows
UPDATE users_identities
SET issuer = ?, subject = ?, email = ?, updated_at = ?
WHERE users_identities.id = ?
  AND EXISTS (SELECT 1 FROM users u WHERE u.id = users_identities.user_id AND u.credentials_generation = ?);

-- name: DeleteIdentityByUserProvider :execrows
DELETE FROM users_identities WHERE user_id = ? AND provider = ?;

-- name: InsertOAuthState :exec
INSERT INTO oauth_states (state_hash, provider, nonce, code_verifier, flow_hash, client, intent, link_user_id, created_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetOAuthState :one
SELECT state_hash, provider, nonce, code_verifier, flow_hash, client, intent, link_user_id, created_at, expires_at
FROM oauth_states
WHERE state_hash = ?;

-- name: DeleteOAuthState :execrows
DELETE FROM oauth_states WHERE state_hash = ?;

-- name: DeleteOAuthStatesByLinkUser :execrows
DELETE FROM oauth_states WHERE link_user_id = ?;

-- name: DeleteExpiredOAuthStates :execrows
DELETE FROM oauth_states WHERE expires_at < ?;

-- name: InsertOAuthHandoff :exec
INSERT INTO oauth_handoffs (code_hash, kind, user_id, provider, issuer, subject, email, flow_hash, id_token, created_at, expires_at, credentials_generation)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetOAuthHandoff :one
SELECT code_hash, kind, user_id, provider, issuer, subject, email, flow_hash, id_token, created_at, expires_at, credentials_generation
FROM oauth_handoffs
WHERE code_hash = ?;

-- name: DeleteOAuthHandoff :execrows
DELETE FROM oauth_handoffs WHERE code_hash = ?;

-- name: DeleteOAuthHandoffsByUser :execrows
DELETE FROM oauth_handoffs WHERE user_id = ?;

-- name: DeleteExpiredOAuthHandoffs :execrows
DELETE FROM oauth_handoffs WHERE expires_at < ?;
