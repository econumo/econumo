-- name: GetUserByID :one
SELECT id, email, name, avatar, password, salt, created_at, updated_at, is_active, algorithm, access_level, access_until, timezone, email_verified, credentials_generation
FROM users
WHERE id = ?;

-- name: ListUserIDs :many
SELECT id FROM users;

-- name: InsertUser :exec
INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpsertUser :exec
INSERT INTO users (id, identifier, email, email_key, name, avatar, password, salt, algorithm, created_at, updated_at, is_active, access_level, access_until, email_verified)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    identifier = excluded.identifier,
    email      = excluded.email,
    -- Only an address that changes moves the key, so a save leaves alone the NULL
    -- of an account that collided when the key was seeded.
    email_key  = CASE WHEN users.email = excluded.email THEN users.email_key ELSE excluded.email_key END,
    name       = excluded.name,
    avatar = excluded.avatar,
    password   = excluded.password,
    salt       = excluded.salt,
    algorithm  = excluded.algorithm,
    updated_at = excluded.updated_at,
    is_active  = excluded.is_active,
    access_level = excluded.access_level,
    access_until = excluded.access_until,
    email_verified = excluded.email_verified;

-- name: LockUserRow :exec
-- The row lock behind every existing-row write and credential mint (see
-- user.Repository.LockRow). SQLite has no row-level lock modes, so this stays
-- a no-op UPDATE to take the row's write lock; the single-writer pool already
-- serializes concurrent transactions regardless, so the extra tuple version
-- this writes per call is not worth chasing here (unlike PostgreSQL, which
-- uses SELECT ... FOR NO KEY UPDATE instead, see the pgsql query).
UPDATE users SET updated_at = updated_at WHERE id = ?;

-- name: BumpUserCredentialsGeneration :execrows
UPDATE users SET credentials_generation = credentials_generation + 1 WHERE id = ?;

-- name: UpdateUserPasswordIfGeneration :execrows
-- The opportunistic legacy-hash upgrade writes ONLY the credential columns and
-- only under the generation the login verified the hash under, so a reset
-- committing mid-login is never overwritten by a stale aggregate save.
UPDATE users SET password = ?, salt = ?, algorithm = ?, updated_at = ?
WHERE id = ? AND credentials_generation = ?;

-- name: UpdateUserEmailIfPasswordlessAndGeneration :execrows
-- The oauth email-drift mirror writes the provider's new address onto the
-- primary email only while the account is still passwordless and still at the
-- generation the callback resolved it under: a password reset committing after
-- those checks must keep the recovered account's own address.
UPDATE users SET email = ?, email_key = ?, email_verified = 1, updated_at = ?
WHERE id = ? AND credentials_generation = ? AND algorithm = 'none';

-- name: UpdateUserEmailIfGeneration :execrows
-- The confirm-email-change path writes ONLY the email columns, under the
-- generation it read after taking the user row's lock, so a stale aggregate can
-- never be saved over an account a reset has just reclaimed.
UPDATE users SET email = ?, email_key = ?, email_verified = 1, updated_at = ?
WHERE id = ? AND credentials_generation = ?;

-- name: UpdateUserLanguage :exec
UPDATE users SET language = ? WHERE id = ?;

-- name: GetUserTimezone :one
SELECT timezone FROM users WHERE id = ?;

-- name: UpdateUserTimezone :exec
UPDATE users SET timezone = ? WHERE id = ?;

-- name: GetUserLanguage :one
SELECT language FROM users WHERE id = ?;

-- name: GetUserByEmail :one
SELECT id, email, name, avatar, password, salt, created_at, updated_at, is_active, algorithm, access_level, access_until, timezone, email_verified, credentials_generation
FROM users
WHERE email_key = sqlc.arg(email_key) OR (email_key IS NULL AND email = sqlc.arg(email))
-- A NULL key row matched on its exact spelling, which the row holding the key
-- does not share, so the account that spelling names wins.
ORDER BY email_key IS NULL DESC
LIMIT 1;

-- name: ExistsUserByEmail :one
SELECT EXISTS(SELECT 1 FROM users WHERE email_key = sqlc.arg(email_key) OR (email_key IS NULL AND email = sqlc.arg(email)));

-- name: ListUsersWithoutEmailKey :many
SELECT id, email FROM users WHERE email_key IS NULL ORDER BY created_at, id;

-- name: SetUserDecryptedEmail :exec
UPDATE users SET email = sqlc.arg(email), email_key = NULL, updated_at = sqlc.arg(updated_at) WHERE id = sqlc.arg(id);

-- name: EmailKeyTaken :one
SELECT EXISTS(SELECT 1 FROM users WHERE email_key = sqlc.arg(email_key));

-- name: SetUserEmailKey :exec
UPDATE users SET email_key = sqlc.arg(email_key) WHERE id = sqlc.arg(id);
