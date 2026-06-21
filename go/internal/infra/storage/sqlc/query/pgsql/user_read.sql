-- Read-model queries for the user module (CQRS read side). See the sqlite
-- variant for rationale. Postgres uses $N placeholders.

-- name: GetUserView :one
SELECT id, email, name, avatar_url
FROM users
WHERE id = $1;

-- name: GetUserOptionsView :many
-- Tiebreak by id so SQLite and PostgreSQL return the same order even when all
-- option rows share a created_at (the registration case). See the sqlite variant.
SELECT name, value
FROM users_options
WHERE user_id = $1
ORDER BY created_at, id;
