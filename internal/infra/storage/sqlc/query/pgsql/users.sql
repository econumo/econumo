-- name: GetUserByID :one
SELECT id, identifier, email, name, avatar, password, salt, created_at, updated_at, is_active, algorithm, access_level, access_until, timezone
FROM users
WHERE id = $1;

-- name: GetUserByIdentifier :one
SELECT id, identifier, email, name, avatar, password, salt, created_at, updated_at, is_active, algorithm, access_level, access_until, timezone
FROM users
WHERE identifier = $1;

-- name: ExistsUserByIdentifier :one
SELECT EXISTS(SELECT 1 FROM users WHERE identifier = $1);

-- name: ListUserIDs :many
SELECT id FROM users;

-- name: InsertUser :exec
INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: UpsertUser :exec
INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active, access_level, access_until)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
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
    access_until = excluded.access_until;

-- name: UpdateUserLanguage :exec
UPDATE users SET language = $1 WHERE id = $2;

-- name: GetUserTimezone :one
SELECT timezone FROM users WHERE id = $1;

-- name: UpdateUserTimezone :exec
UPDATE users SET timezone = $1 WHERE id = $2;

-- name: GetUserLanguage :one
SELECT language FROM users WHERE id = $1;
