-- Write-side queries for folders + the accounts_folders membership join
-- (SQLite). A folder belongs to a user, has a position and an is_visible flag,
-- and contains accounts via accounts_folders.

-- name: GetFolderByID :one
SELECT id, user_id, name, position, is_visible, created_at, updated_at
FROM folders
WHERE id = ?;

-- name: ListFoldersByUser :many
-- The user's folders. Ordering is applied by the caller/assembler (by position).
SELECT id, user_id, name, position, is_visible, created_at, updated_at
FROM folders
WHERE user_id = ?;

-- name: CountFoldersByUser :one
SELECT COUNT(*) FROM folders WHERE user_id = ?;

-- name: UpsertFolder :exec
INSERT INTO folders (id, user_id, name, position, is_visible, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    name       = excluded.name,
    position   = excluded.position,
    is_visible = excluded.is_visible,
    updated_at = excluded.updated_at;

-- name: DeleteFolder :exec
DELETE FROM folders WHERE id = ?;

-- name: ListFolderAccountIDs :many
-- Account ids in a folder (the accounts_folders join).
SELECT account_id FROM accounts_folders WHERE folder_id = ?;

-- name: ListFolderMembershipsByUser :many
-- All (folder_id, account_id) memberships for a user's folders. Lets the caller
-- resolve "which folder contains account X" in one query.
SELECT af.folder_id, af.account_id
FROM accounts_folders af
JOIN folders f ON f.id = af.folder_id
WHERE f.user_id = ?;

-- name: AddAccountToFolder :exec
INSERT INTO accounts_folders (folder_id, account_id)
VALUES (?, ?)
ON CONFLICT (folder_id, account_id) DO NOTHING;

-- name: RemoveAccountFromFolder :exec
DELETE FROM accounts_folders WHERE folder_id = ? AND account_id = ?;

-- name: RemoveAccountFromAllFolders :exec
DELETE FROM accounts_folders WHERE account_id = ?;
