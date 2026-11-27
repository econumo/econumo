-- User deletion: the deletion state on users, the users_audit log and the
-- users_deletion_requests codes. The three users columns are written only here.

-- name: GetUserDeletionState :one
SELECT deletion_marked_at, deletion_reason, last_active_at, created_at FROM users WHERE id = ?;

-- name: MarkUserForDeletion :execrows
UPDATE users SET deletion_marked_at = ?, deletion_reason = ? WHERE id = ? AND deletion_marked_at IS NULL;

-- name: UnmarkUserDeletion :execrows
UPDATE users SET deletion_marked_at = NULL, deletion_reason = '' WHERE id = ? AND deletion_marked_at IS NOT NULL;

-- name: TouchUserLastActive :exec
-- Blind and monotonic: the stamp only moves forward, and at most once a day.
UPDATE users SET last_active_at = ? WHERE id = ? AND (last_active_at IS NULL OR last_active_at < ?);

-- name: ListMarkedUsers :many
SELECT id, email, deletion_reason, deletion_marked_at FROM users
WHERE deletion_marked_at IS NOT NULL
ORDER BY deletion_marked_at, id;

-- name: ListInactiveUserCandidates :many
-- A user connected to someone still active (and not marked) is kept: the
-- shared data is in use. A literal ?1 rather than sqlc.arg(cutoff): sqlc's
-- sqlite engine leaves a sqlc.arg inside a WHERE NOT EXISTS unrewritten.
SELECT u.id, u.email, u.name, u.language FROM users u
WHERE u.deletion_marked_at IS NULL AND u.is_active = 1
  AND COALESCE(u.last_active_at, u.created_at) < ?1
  AND NOT EXISTS (
    SELECT 1 FROM users_connections c JOIN users p ON p.id = c.connected_user_id
    WHERE c.user_id = u.id AND p.deletion_marked_at IS NULL
      AND COALESCE(p.last_active_at, p.created_at) >= ?1)
ORDER BY u.id;

-- name: UserHasActiveConnection :one
SELECT EXISTS (
  SELECT 1 FROM users_connections c JOIN users p ON p.id = c.connected_user_id
  WHERE c.user_id = sqlc.arg(user_id) AND p.deletion_marked_at IS NULL
    AND COALESCE(p.last_active_at, p.created_at) >= sqlc.arg(cutoff));

-- name: ListWipeConnectedUserIDs :many
SELECT connected_user_id FROM users_connections WHERE user_id = ? ORDER BY connected_user_id;

-- name: DeleteUserByID :execrows
DELETE FROM users WHERE id = ?;

-- name: LockUserRowForWipe :exec
-- SQLite has no row lock modes: the same no-op UPDATE as LockUserRow.
UPDATE users SET updated_at = updated_at WHERE id = ?;

-- name: InsertUserAudit :execrows
-- Zero rows means ref_id is already taken (a lost re-grant race).
INSERT INTO users_audit (id, user_id, event, email_hash, ref_id, details, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT DO NOTHING;

-- name: ListUserAuditByEmailHash :many
SELECT id, user_id, event, email_hash, ref_id, details, created_at FROM users_audit
WHERE email_hash = ?
ORDER BY created_at, id;

-- name: ListUserAuditByEvent :many
-- created_at has one-second precision, so a page can end inside a second: the
-- cursor is (since, after_id), and the nil UUID makes it a plain
-- created_at >= since.
SELECT id, user_id, event, email_hash, ref_id, details, created_at FROM users_audit
WHERE event = sqlc.arg(event) AND (created_at > sqlc.arg(since) OR (created_at = sqlc.arg(since) AND id > sqlc.arg(after_id)))
ORDER BY created_at, id
LIMIT sqlc.arg(row_limit);

-- name: DeleteUserDeletionRequestsByUser :exec
DELETE FROM users_deletion_requests WHERE user_id = ?;

-- name: InsertUserDeletionRequestIfGeneration :execrows
-- Fenced like the email change request: a reclaim bumps the generation, and a
-- code minted under the old one must not land.
INSERT INTO users_deletion_requests (id, user_id, email, code, created_at, expired_at)
SELECT ?, ?, ?, ?, ?, ?
WHERE EXISTS (SELECT 1 FROM users u WHERE u.id = ? AND u.credentials_generation = ?);

-- name: ConsumeUserDeletionRequest :execrows
DELETE FROM users_deletion_requests WHERE id = ? AND user_id = ?;

-- name: GetUserDeletionRequestByUser :one
SELECT id, user_id, email, code, created_at, expired_at
FROM users_deletion_requests
WHERE user_id = ?;
