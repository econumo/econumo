-- name: GetUserByID :one
SELECT id, identifier, email, name, avatar, password, salt, created_at, updated_at, is_active, algorithm
FROM users
WHERE id = ?;

-- name: GetUserByIdentifier :one
SELECT id, identifier, email, name, avatar, password, salt, created_at, updated_at, is_active, algorithm
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
INSERT INTO users (id, identifier, email, name, avatar, password, salt, algorithm, created_at, updated_at, is_active)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    identifier = excluded.identifier,
    email      = excluded.email,
    name       = excluded.name,
    avatar = excluded.avatar,
    password   = excluded.password,
    salt       = excluded.salt,
    algorithm  = excluded.algorithm,
    updated_at = excluded.updated_at,
    is_active  = excluded.is_active;

-- name: UpdateUserLanguage :exec
UPDATE users SET language = ? WHERE id = ?;
