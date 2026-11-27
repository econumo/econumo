-- What a deleted user leaves in other people's accounts (PostgreSQL: $N
-- placeholders). See the sqlite variant.

-- name: ReassignAuthorInForeignAccounts :exec
UPDATE transactions
SET user_id = (SELECT a.user_id FROM accounts a WHERE a.id = transactions.account_id)
WHERE transactions.user_id = $1 AND transactions.account_id IN (SELECT o.id FROM accounts o WHERE o.user_id <> $1);

-- name: ListCrossOwnerTransfers :many
SELECT
    t.id, t.account_id, t.account_recipient_id,
    sa.user_id AS source_owner_id, ra.user_id AS recipient_owner_id,
    t.amount, t.amount_recipient, t.description, t.spent_at,
    t.sort_key, t.recipient_sort_key
FROM transactions t
JOIN accounts sa ON sa.id = t.account_id
JOIN accounts ra ON ra.id = t.account_recipient_id
WHERE t.type = 2
  AND ((sa.user_id = $1 AND ra.user_id <> $1) OR (sa.user_id <> $1 AND ra.user_id = $1))
ORDER BY t.id;

-- name: InsertWipeCorrection :exec
INSERT INTO transactions (
    id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id,
    description, created_at, updated_at, spent_at, type, amount, amount_recipient, sort_key
) VALUES (
    $1, $2, $3, NULL, NULL, NULL, NULL,
    $4, $5, $6, $7, $8, $9, NULL, $10
);
