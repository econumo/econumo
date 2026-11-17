-- name: GetUserOptions :many
-- Tiebreak by id so the order is deterministic and identical across engines even
-- when option rows share a created_at (the registration case).
SELECT id, user_id, name, value, created_at, updated_at
FROM users_options
WHERE user_id = $1
ORDER BY created_at, id;

-- name: UpsertUserOption :exec
INSERT INTO users_options (id, user_id, name, value, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id, name) DO UPDATE SET
    value      = excluded.value,
    updated_at = excluded.updated_at;

-- name: ListUserIDsMissingOption :many
-- Ordered by id so the backfill is deterministic across engines and reruns.
-- Same LEFT JOIN + IS NULL shape as the sqlite variant (kept identical across
-- engines even though postgresql's parser handles NOT EXISTS params fine).
SELECT u.id
FROM users u
LEFT JOIN users_options o ON o.user_id = u.id AND o.name = $1
WHERE o.user_id IS NULL
ORDER BY u.id;

-- name: ListAffiliateUsers :many
-- created_at has one-second precision, so a page can end inside a second:
-- the cursor is (since, after_user_id). The nil UUID sorts below every
-- id, which makes the cursor a plain created_at >= since.
SELECT user_id, value, created_at
FROM users_options
WHERE name = 'affiliate' AND value IS NOT NULL AND value <> '' AND (created_at > sqlc.arg(since) OR (created_at = sqlc.arg(since) AND user_id > sqlc.arg(after_user_id)))
ORDER BY created_at, user_id
LIMIT sqlc.arg(row_limit);
