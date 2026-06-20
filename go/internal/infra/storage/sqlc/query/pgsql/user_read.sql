-- Read-model queries for the user module (CQRS read side). See the sqlite
-- variant for rationale. Postgres uses $N placeholders.

-- name: GetUserView :one
SELECT id, email, name, avatar_url
FROM users
WHERE id = $1;

-- name: GetUserOptionsView :many
SELECT name, value
FROM users_options
WHERE user_id = $1
ORDER BY created_at;
