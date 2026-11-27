-- See the sqlite sibling for the flow.

-- name: GetUserDeletionState :one
SELECT deletion_marked_at, deletion_reason, last_active_at, created_at FROM users WHERE id = $1;

-- name: MarkUserForDeletion :execrows
UPDATE users SET deletion_marked_at = $1, deletion_reason = $2 WHERE id = $3 AND deletion_marked_at IS NULL;

-- name: UnmarkUserDeletion :execrows
UPDATE users SET deletion_marked_at = NULL, deletion_reason = '' WHERE id = $1 AND deletion_marked_at IS NOT NULL;

-- name: TouchUserLastActive :exec
UPDATE users SET last_active_at = $1 WHERE id = $2 AND (last_active_at IS NULL OR last_active_at < $3);

-- name: ListMarkedUsers :many
SELECT id, email, deletion_reason, deletion_marked_at FROM users
WHERE deletion_marked_at IS NOT NULL
ORDER BY deletion_marked_at, id;

-- name: ListInactiveUserCandidates :many
-- cutoff is the shortest period any account can have: Go applies each
-- candidate's own period (inactivity_months, else the server setting).
SELECT u.id, u.email, u.name, u.language, u.last_active_at, u.created_at, o.value AS inactivity_months
FROM users u
LEFT JOIN users_options o ON o.user_id = u.id AND o.name = 'inactivity_months'
WHERE u.deletion_marked_at IS NULL AND u.is_active = true
  AND COALESCE(u.last_active_at, u.created_at) < sqlc.arg(cutoff)
  AND NOT EXISTS (
    SELECT 1 FROM users_connections c JOIN users p ON p.id = c.connected_user_id
    WHERE c.user_id = u.id AND p.deletion_marked_at IS NULL
      AND COALESCE(p.last_active_at, p.created_at) >= sqlc.arg(cutoff))
ORDER BY u.id;

-- name: UserHasActiveConnection :one
SELECT EXISTS (
  SELECT 1 FROM users_connections c JOIN users p ON p.id = c.connected_user_id
  WHERE c.user_id = sqlc.arg(user_id) AND p.deletion_marked_at IS NULL
    AND COALESCE(p.last_active_at, p.created_at) >= sqlc.arg(cutoff));

-- name: ListWipeConnectedUserIDs :many
SELECT connected_user_id FROM users_connections WHERE user_id = $1 ORDER BY connected_user_id;

-- name: DeleteUserByID :execrows
DELETE FROM users WHERE id = $1;

-- name: LockUserRowForWipe :one
-- FOR UPDATE, unlike LockUserRow's FOR NO KEY UPDATE, also conflicts with the
-- FOR KEY SHARE every FK check against this row takes, so no row referencing
-- the user can be inserted while the wipe holds it. The adapter maps no-rows to
-- success, as LockUserRow does.
SELECT id FROM users WHERE id = $1 FOR UPDATE;

-- name: LockDeletionWipes :exec
SELECT pg_advisory_xact_lock(7203251127);

-- name: InsertUserAudit :execrows
INSERT INTO users_audit (id, user_id, event, email_hash, ref_id, details, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT DO NOTHING;

-- name: ListUserAuditByEmailHash :many
SELECT id, user_id, event, email_hash, ref_id, details, created_at FROM users_audit
WHERE email_hash = $1
ORDER BY created_at, id;

-- name: ListUserAuditByEvent :many
SELECT id, user_id, event, email_hash, ref_id, details, created_at FROM users_audit
WHERE event = sqlc.arg(event) AND (created_at > sqlc.arg(since) OR (created_at = sqlc.arg(since) AND id > sqlc.arg(after_id)))
ORDER BY created_at, id
LIMIT sqlc.arg(row_limit);

-- name: DeleteUserDeletionRequestsByUser :exec
DELETE FROM users_deletion_requests WHERE user_id = $1;

-- name: InsertUserDeletionRequestIfGeneration :execrows
INSERT INTO users_deletion_requests (id, user_id, email, code, created_at, expired_at)
SELECT $1, $2, $3, $4, $5, $6
WHERE EXISTS (SELECT 1 FROM users u WHERE u.id = $7 AND u.credentials_generation = $8);

-- name: ConsumeUserDeletionRequest :execrows
DELETE FROM users_deletion_requests WHERE id = $1 AND user_id = $2;

-- name: GetUserDeletionRequestByUser :one
SELECT id, user_id, email, code, created_at, expired_at
FROM users_deletion_requests
WHERE user_id = $1;
