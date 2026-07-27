-- See the sqlite sibling for the flow; expiry is compared in the app layer, not SQL.

-- name: DeleteUserEmailChangeRequestsByUser :exec
DELETE FROM users_email_change_requests WHERE user_id = $1;

-- name: InsertUserEmailChangeRequest :exec
INSERT INTO users_email_change_requests (id, user_id, new_email, code, created_at, updated_at, expired_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetUserEmailChangeRequestByUser :one
SELECT id, user_id, new_email, code, created_at, updated_at, expired_at
FROM users_email_change_requests
WHERE user_id = $1;
