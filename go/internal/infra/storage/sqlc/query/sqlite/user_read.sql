-- Read-model queries for the user module (CQRS read side). These are tailored
-- to the response shape and bypass the domain aggregate. They live separately
-- from the write queries (users.sql / users_options.sql) to keep the read and
-- write concerns visibly distinct.

-- name: GetUserView :one
-- The user's display fields for get-user-data / the login response user object.
SELECT id, email, name, avatar_url
FROM users
WHERE id = ?;

-- name: GetUserOptionsView :many
-- The persisted options (name/value) in stable order, for both get-user-data
-- (which appends a synthetic currency_id) and get-option-list (raw).
SELECT name, value
FROM users_options
WHERE user_id = ?
ORDER BY created_at;
