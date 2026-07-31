-- See the sqlite sibling for the flow; expiry is compared in the app layer, not SQL.

-- name: DeleteUserEmailVerificationsByUser :exec
DELETE FROM users_email_verifications WHERE user_id = $1;

-- name: InsertUserEmailVerification :exec
INSERT INTO users_email_verifications (id, user_id, code, created_at, updated_at, expired_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetUserEmailVerificationByUser :one
SELECT id, user_id, code, created_at, updated_at, expired_at
FROM users_email_verifications
WHERE user_id = $1;
