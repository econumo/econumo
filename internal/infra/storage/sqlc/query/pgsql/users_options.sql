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

-- name: ClearUserOptionIfValue :execrows
UPDATE users_options SET value = NULL, updated_at = $1 WHERE user_id = $2 AND name = $3 AND value = $4;

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
SELECT o.user_id, o.value, c.value AS code, o.created_at
FROM users_options o
LEFT JOIN users_options c ON c.user_id = o.user_id AND c.name = 'affiliate_code'
WHERE o.name = 'affiliate' AND o.value IS NOT NULL AND o.value <> '' AND (sqlc.arg(program)::text = '' OR o.value = sqlc.arg(program)) AND (o.created_at > sqlc.arg(since) OR (o.created_at = sqlc.arg(since) AND o.user_id > sqlc.arg(after_user_id)))
ORDER BY o.created_at, o.user_id
LIMIT sqlc.arg(row_limit);
