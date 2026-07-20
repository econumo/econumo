-- Read-model queries for the user module (CQRS read side). These are tailored
-- to the response shape and bypass the domain aggregate. They live separately
-- from the write queries (users.sql / users_options.sql) to keep the read and
-- write concerns visibly distinct.

-- name: GetUserView :one
-- The user's display fields for get-user-data / the login response user
-- object, plus the raw access_level/access_until columns (the service
-- collapses them against the clock before putting them on the wire).
SELECT id, email, name, avatar, access_level, access_until
FROM users
WHERE id = ?;

-- name: GetUserOptionsView :many
-- The persisted options (name/value) in a fully deterministic order, for both
-- get-user-data (which appends a synthetic currency_id) and get-option-list
-- (raw). The tiebreak by id is required: at registration all option rows are
-- created with the SAME created_at, so created_at alone leaves the order
-- engine-specific (SQLite=insertion, PostgreSQL=unspecified). Ordering by id as
-- the secondary key makes SQLite and PostgreSQL agree byte-for-byte.
SELECT name, value
FROM users_options
WHERE user_id = ?
ORDER BY created_at, id;
