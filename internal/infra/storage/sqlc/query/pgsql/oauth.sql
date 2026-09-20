-- OAuth feature queries: identities, in-flight states, one-shot handoffs.

-- name: GetIdentityByProviderSubject :one
SELECT id, user_id, provider, issuer, subject, email, created_at, updated_at
FROM users_identities
WHERE provider = $1 AND issuer = $2 AND subject = $3;

-- name: GetIdentityByUserProvider :one
SELECT id, user_id, provider, issuer, subject, email, created_at, updated_at
FROM users_identities
WHERE user_id = $1 AND provider = $2;

-- name: ListIdentitiesByUser :many
SELECT id, user_id, provider, issuer, subject, email, created_at, updated_at
FROM users_identities
WHERE user_id = $1
ORDER BY created_at, id;

-- name: CountIdentitiesByUser :one
SELECT COUNT(*) FROM users_identities WHERE user_id = $1;

-- name: InsertIdentityIfGeneration :execrows
-- See the sqlite sibling.
INSERT INTO users_identities (id, user_id, provider, issuer, subject, email, created_at, updated_at)
SELECT $1, $2, $3, $4, $5, $6, $7, $8
WHERE EXISTS (SELECT 1 FROM users u WHERE u.id = $9 AND u.credentials_generation = $10);

-- name: UpdateIdentityIfGeneration :execrows
UPDATE users_identities
SET issuer = $1, subject = $2, email = $3, updated_at = $4
WHERE users_identities.id = $5
  AND EXISTS (SELECT 1 FROM users u WHERE u.id = users_identities.user_id AND u.credentials_generation = $6);

-- name: DeleteIdentityByUserProvider :execrows
DELETE FROM users_identities WHERE user_id = $1 AND provider = $2;

-- name: InsertOAuthState :exec
INSERT INTO oauth_states (state_hash, provider, nonce, code_verifier, flow_hash, client, intent, link_user_id, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: GetOAuthState :one
SELECT state_hash, provider, nonce, code_verifier, flow_hash, client, intent, link_user_id, created_at, expires_at
FROM oauth_states
WHERE state_hash = $1;

-- name: DeleteOAuthState :execrows
DELETE FROM oauth_states WHERE state_hash = $1;

-- name: DeleteOAuthStatesByLinkUser :execrows
DELETE FROM oauth_states WHERE link_user_id = $1;

-- name: DeleteExpiredOAuthStates :execrows
DELETE FROM oauth_states WHERE expires_at < $1;

-- name: InsertOAuthHandoff :exec
INSERT INTO oauth_handoffs (code_hash, kind, user_id, provider, issuer, subject, email, flow_hash, id_token, created_at, expires_at, credentials_generation)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: GetOAuthHandoff :one
SELECT code_hash, kind, user_id, provider, issuer, subject, email, flow_hash, id_token, created_at, expires_at, credentials_generation
FROM oauth_handoffs
WHERE code_hash = $1;

-- name: DeleteOAuthHandoff :execrows
DELETE FROM oauth_handoffs WHERE code_hash = $1;

-- name: DeleteOAuthHandoffsByUser :execrows
DELETE FROM oauth_handoffs WHERE user_id = $1;

-- name: DeleteOAuthHandoffsByUserProvider :execrows
DELETE FROM oauth_handoffs WHERE user_id = $1 AND provider = $2;

-- name: DeleteExpiredOAuthHandoffs :execrows
DELETE FROM oauth_handoffs WHERE expires_at < $1;
