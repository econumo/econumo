-- See the sqlite sibling for the flow; expiry is compared in the app layer, not SQL.

-- name: DeleteUserEmailChangeRequestsByUser :exec
DELETE FROM users_email_change_requests WHERE user_id = $1;

-- name: InsertUserEmailChangeRequestIfGeneration :execrows
-- See the sqlite sibling: the pending grant is fenced on the generation the
-- password check read.
INSERT INTO users_email_change_requests (id, user_id, new_email, code, created_at, updated_at, expired_at)
SELECT $1, $2, $3, $4, $5, $6, $7
WHERE EXISTS (SELECT 1 FROM users u WHERE u.id = $8 AND u.credentials_generation = $9);

-- name: ConsumeUserEmailChangeRequest :execrows
-- See the sqlite sibling: the confirm path consumes its own evidence row.
DELETE FROM users_email_change_requests WHERE id = $1 AND user_id = $2;

-- name: GetUserEmailChangeRequestByUser :one
SELECT id, user_id, new_email, code, created_at, updated_at, expired_at
FROM users_email_change_requests
WHERE user_id = $1;
