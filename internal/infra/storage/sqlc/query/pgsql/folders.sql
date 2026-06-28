-- Write-side queries for folders + accounts_folders (PostgreSQL: $N
-- placeholders). See the sqlite variant for documentation.

-- name: GetFolderByID :one
SELECT id, user_id, name, position, is_visible, created_at, updated_at
FROM folders
WHERE id = $1;

-- name: ListFoldersByUser :many
SELECT id, user_id, name, position, is_visible, created_at, updated_at
FROM folders
WHERE user_id = $1;

-- name: CountFoldersByUser :one
SELECT COUNT(*) FROM folders WHERE user_id = $1;

-- name: UpsertFolder :exec
INSERT INTO folders (id, user_id, name, position, is_visible, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO UPDATE SET
    name       = excluded.name,
    position   = excluded.position,
    is_visible = excluded.is_visible,
    updated_at = excluded.updated_at;

-- name: DeleteFolder :exec
DELETE FROM folders WHERE id = $1;

-- name: ListFolderAccountIDs :many
SELECT account_id FROM accounts_folders WHERE folder_id = $1;

-- name: ListFolderMembershipsByUser :many
SELECT af.folder_id, af.account_id
FROM accounts_folders af
JOIN folders f ON f.id = af.folder_id
WHERE f.user_id = $1;

-- name: AddAccountToFolder :exec
INSERT INTO accounts_folders (folder_id, account_id)
VALUES ($1, $2)
ON CONFLICT (folder_id, account_id) DO NOTHING;

-- name: RemoveAccountFromFolder :exec
DELETE FROM accounts_folders WHERE folder_id = $1 AND account_id = $2;

-- name: RemoveAccountFromAllFolders :exec
DELETE FROM accounts_folders WHERE account_id = $1;
