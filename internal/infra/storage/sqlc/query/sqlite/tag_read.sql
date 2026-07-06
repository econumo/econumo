-- Read-model query for the tag module (CQRS read side). Tailored to the
-- response shape; bypasses the domain aggregate. Separate from the write queries
-- (tags.sql) to keep the read and write concerns visibly distinct.

-- name: GetTagListView :many
-- Available tags: the user's OWN tags plus the tags of every user who has shared
-- an account WITH this user. Mirrors PHP TagRepository::findAvailableForUserId
-- (self + DISTINCT owners of accounts granted via accounts_access), ordered by
-- position. The user id is repeated positionally -> two-field Params struct.
SELECT t.id, t.user_id, t.name, t.position, t.is_archived, t.created_at, t.updated_at
FROM tags t
WHERE t.user_id = ?
   OR t.user_id IN (
       SELECT a.user_id
       FROM accounts_access aa
       JOIN accounts a ON a.id = aa.account_id
       WHERE aa.user_id = ?
   )
ORDER BY t.position, t.id
;
