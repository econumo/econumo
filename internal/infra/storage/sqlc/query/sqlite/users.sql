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
INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active, access_level, access_until, email_verified)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    identifier = excluded.identifier,
    email      = excluded.email,
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
UPDATE users SET email = ?, email_verified = 1, updated_at = ?
WHERE id = ? AND credentials_generation = ? AND algorithm = 'none';

-- name: UpdateUserEmailIfGeneration :execrows
-- The confirm-email-change path writes ONLY the email columns, under the
-- generation it read after taking the user row's lock, so a stale aggregate can
-- never be saved over an account a reset has just reclaimed.
UPDATE users SET email = ?, email_verified = 1, updated_at = ?
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
WHERE lower(email) = lower(?);

-- name: ExistsUserByEmail :one
SELECT EXISTS(SELECT 1 FROM users WHERE lower(email) = lower(?));
