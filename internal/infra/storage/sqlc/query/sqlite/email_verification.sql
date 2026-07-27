-- Login email-verification codes (users_email_verifications). The login flow
-- replaces the user's old code with a fresh one, reads it back by user, and
-- deletes it once verified. Expiry is compared in the app layer (Go time),
-- not in SQL, to avoid engine date-format differences.

-- name: DeleteUserEmailVerificationsByUser :exec
DELETE FROM users_email_verifications WHERE user_id = ?;

-- name: InsertUserEmailVerification :exec
INSERT INTO users_email_verifications (id, user_id, code, created_at, updated_at, expired_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetUserEmailVerificationByUser :one
SELECT id, user_id, code, created_at, updated_at, expired_at
FROM users_email_verifications
WHERE user_id = ?;
