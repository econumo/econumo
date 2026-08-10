-- Read-model query for the label module (CQRS read side).

-- name: GetLabelListView :many
-- Available labels: the user's OWN labels plus the labels of every user who has
-- shared an account WITH this user. The user id is repeated positionally, which
-- generates a two-field Params struct.
SELECT l.id, l.user_id, l.name, l.icon, l.sort_key, l.is_archived, l.created_at, l.updated_at
FROM labels l
WHERE l.user_id = ?
   OR l.user_id IN (
       SELECT a.user_id
       FROM accounts_access aa
       JOIN accounts a ON a.id = aa.account_id
       WHERE aa.user_id = ? AND aa.is_accepted = 1
   )
ORDER BY l.sort_key, l.id
;
