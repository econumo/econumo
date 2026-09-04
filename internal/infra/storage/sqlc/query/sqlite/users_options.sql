-- name: GetUserOptions :many
-- Tiebreak by id so the order is deterministic and identical across engines even
-- when option rows share a created_at (the registration case).
SELECT id, user_id, name, value, created_at, updated_at
FROM users_options
WHERE user_id = ?
ORDER BY created_at, id;

-- name: UpsertUserOption :exec
INSERT INTO users_options (id, user_id, name, value, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (user_id, name) DO UPDATE SET
    value      = excluded.value,
    updated_at = excluded.updated_at;

-- name: ListUserIDsMissingOption :many
-- Ordered by id so the backfill is deterministic across engines and reruns.
-- A LEFT JOIN + IS NULL, not NOT EXISTS: sqlc's sqlite parser fails to detect
-- the '?' when it sits inside a NOT EXISTS subquery (drops it from the
-- generated function signature while leaving it in the SQL text), so the join
-- form is used to keep the parameter visible to codegen.
SELECT u.id
FROM users u
LEFT JOIN users_options o ON o.user_id = u.id AND o.name = ?
WHERE o.user_id IS NULL
ORDER BY u.id;
