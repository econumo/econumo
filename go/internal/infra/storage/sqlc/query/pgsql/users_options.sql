-- name: GetUserOptions :many
SELECT id, user_id, name, value, created_at, updated_at
FROM users_options
WHERE user_id = $1
ORDER BY created_at;

-- name: UpsertUserOption :exec
INSERT INTO users_options (id, user_id, name, value, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id, name) DO UPDATE SET
    value      = excluded.value,
    updated_at = excluded.updated_at;
