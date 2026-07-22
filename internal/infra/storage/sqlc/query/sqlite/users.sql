-- name: GetUserByID :one
SELECT id, identifier, email, name, avatar, password, salt, created_at, updated_at, is_active, algorithm, access_level, access_until, timezone, email_verified
FROM users
WHERE id = ?;

-- name: GetUserByIdentifier :one
SELECT id, identifier, email, name, avatar, password, salt, created_at, updated_at, is_active, algorithm, access_level, access_until, timezone, email_verified
FROM users
WHERE identifier = ?;

-- name: ExistsUserByIdentifier :one
SELECT EXISTS(SELECT 1 FROM users WHERE identifier = ?);

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

-- name: UpdateUserLanguage :exec
UPDATE users SET language = ? WHERE id = ?;

-- name: GetUserTimezone :one
SELECT timezone FROM users WHERE id = ?;

-- name: UpdateUserTimezone :exec
UPDATE users SET timezone = ? WHERE id = ?;

-- name: GetUserLanguage :one
SELECT language FROM users WHERE id = ?;
