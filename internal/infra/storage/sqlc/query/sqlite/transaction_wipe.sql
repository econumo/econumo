-- What a deleted user leaves in other people's accounts (SQLite). See
-- internal/transaction/wipe.go.

-- name: ReassignAuthorInForeignAccounts :exec
-- Args: user_id (x2).
UPDATE transactions
SET user_id = (SELECT a.user_id FROM accounts a WHERE a.id = transactions.account_id)
WHERE transactions.user_id = ? AND transactions.account_id IN (SELECT o.id FROM accounts o WHERE o.user_id <> ?);

-- name: ListCrossOwnerTransfers :many
-- Transfers with exactly one side on the user's accounts. Args: user_id (x4).
SELECT
    t.id, t.account_id, t.account_recipient_id,
    sa.user_id AS source_owner_id, ra.user_id AS recipient_owner_id,
    t.amount, t.amount_recipient, t.description, t.spent_at,
    t.sort_key, t.recipient_sort_key
FROM transactions t
JOIN accounts sa ON sa.id = t.account_id
JOIN accounts ra ON ra.id = t.account_recipient_id
WHERE t.type = 2
  AND ((sa.user_id = ? AND ra.user_id <> ?) OR (sa.user_id <> ? AND ra.user_id = ?))
ORDER BY t.id;

-- name: InsertWipeCorrection :exec
-- The replacement keeps the transfer's key on the surviving side, so sort_key
-- is explicit and never placed by txorder.
INSERT INTO transactions (
    id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id,
    description, created_at, updated_at, spent_at, type, amount, amount_recipient, sort_key
) VALUES (
    ?, ?, ?, NULL, NULL, NULL, NULL,
    ?, ?, ?, ?, ?, ?, NULL, ?
);
