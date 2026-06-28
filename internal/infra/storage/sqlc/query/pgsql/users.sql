-- name: GetUserByID :one
SELECT id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active
FROM users
WHERE id = $1;

-- name: GetUserByIdentifier :one
SELECT id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active
FROM users
WHERE identifier = $1;

-- name: ExistsUserByIdentifier :one
SELECT EXISTS(SELECT 1 FROM users WHERE identifier = $1);

-- name: ListUserIDs :many
SELECT id FROM users;

-- name: InsertUser :exec
INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: UpsertUser :exec
INSERT INTO users (id, identifier, email, name, avatar_url, password, salt, created_at, updated_at, is_active)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (id) DO UPDATE SET
    identifier = excluded.identifier,
    email      = excluded.email,
    name       = excluded.name,
    avatar_url = excluded.avatar_url,
    password   = excluded.password,
    salt       = excluded.salt,
    updated_at = excluded.updated_at,
    is_active  = excluded.is_active;
