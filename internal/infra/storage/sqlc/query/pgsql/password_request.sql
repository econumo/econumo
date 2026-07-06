-- Password-reset request queries (users_password_requests). See the sqlite
-- sibling for the flow; expiry is compared in the app layer, not SQL.

-- name: DeleteUserPasswordRequestsByUser :exec
DELETE FROM users_password_requests WHERE user_id = $1;

-- name: InsertUserPasswordRequest :exec
INSERT INTO users_password_requests (id, user_id, code, created_at, updated_at, expired_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetUserPasswordRequestByUserAndCode :one
SELECT id, user_id, code, created_at, updated_at, expired_at
FROM users_password_requests
WHERE user_id = $1 AND code = $2;

-- name: DeleteUserPasswordRequest :exec
DELETE FROM users_password_requests WHERE id = $1;
