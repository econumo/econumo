-- Pending change-email requests (users_email_change_requests). The request is
-- replaced with a fresh one, read back by user, and deleted once confirmed.
-- Expiry is compared in the app layer (Go time), not in SQL.

-- name: DeleteUserEmailChangeRequestsByUser :exec
DELETE FROM users_email_change_requests WHERE user_id = ?;

-- name: InsertUserEmailChangeRequest :exec
INSERT INTO users_email_change_requests (id, user_id, new_email, code, created_at, updated_at, expired_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetUserEmailChangeRequestByUser :one
SELECT id, user_id, new_email, code, created_at, updated_at, expired_at
FROM users_email_change_requests
WHERE user_id = ?;
