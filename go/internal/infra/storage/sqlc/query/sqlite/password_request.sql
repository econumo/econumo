-- Password-reset request queries (users_password_requests). The reset flow:
-- remind-password deletes the user's old codes and inserts a fresh one;
-- reset-password looks it up by (user, code), checks expiry in Go, then deletes
-- it. Expiry is compared in the app layer (Go time.Time), not in SQL, to avoid
-- engine date-format differences.

-- name: DeleteUserPasswordRequestsByUser :exec
DELETE FROM users_password_requests WHERE user_id = ?;

-- name: InsertUserPasswordRequest :exec
INSERT INTO users_password_requests (id, user_id, code, created_at, updated_at, expired_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetUserPasswordRequestByUserAndCode :one
SELECT id, user_id, code, created_at, updated_at, expired_at
FROM users_password_requests
WHERE user_id = ? AND code = ?;

-- name: DeleteUserPasswordRequest :exec
DELETE FROM users_password_requests WHERE id = ?;
