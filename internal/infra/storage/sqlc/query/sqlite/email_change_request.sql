-- Pending change-email requests (users_email_change_requests). The request is
-- replaced with a fresh one, read back by user, and deleted once confirmed.
-- Expiry is compared in the app layer (Go time), not in SQL.

-- name: DeleteUserEmailChangeRequestsByUser :exec
DELETE FROM users_email_change_requests WHERE user_id = ?;

-- name: InsertUserEmailChangeRequestIfGeneration :execrows
-- A pending change is a grant to rewrite the login key, so it must not be
-- created by a session an account reclaim has already invalidated: the reclaim
-- bumps the generation, and the insert only lands under the one the password
-- check read.
INSERT INTO users_email_change_requests (id, user_id, new_email, code, created_at, updated_at, expired_at)
SELECT ?, ?, ?, ?, ?, ?, ?
WHERE EXISTS (SELECT 1 FROM users u WHERE u.id = ? AND u.credentials_generation = ?);

-- name: ConsumeUserEmailChangeRequest :execrows
-- The confirm path's evidence and its consumption are the same row: taking it
-- row-counted is how a confirmation learns that the reclaim (or a concurrent
-- confirm) already took the grant.
DELETE FROM users_email_change_requests WHERE id = ? AND user_id = ?;

-- name: GetUserEmailChangeRequestByUser :one
SELECT id, user_id, new_email, code, created_at, updated_at, expired_at
FROM users_email_change_requests
WHERE user_id = ?;
