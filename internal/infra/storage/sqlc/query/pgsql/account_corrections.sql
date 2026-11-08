-- Balance-correction transaction insert (PostgreSQL: $N placeholders). See the
-- sqlite variant for documentation.

-- name: InsertCorrectionTransaction :exec
INSERT INTO transactions (
    id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id,
    description, created_at, updated_at, spent_at, type, amount, amount_recipient, sort_key
) VALUES (
    $1, $2, $3, NULL, NULL, NULL, NULL,
    $4, $5, $6, $7, $8, $9, NULL, $10
);

-- name: GetFirstAccountTransactionID :one
SELECT id FROM transactions
WHERE account_id = $1 OR account_recipient_id = $2
ORDER BY id
LIMIT 1;

-- name: GetOpeningCorrection :one
SELECT t.id, t.user_id, t.account_id, t.description, t.type, t.amount, t.spent_at, t.created_at
FROM transactions t
WHERE t.id = $1 AND t.account_id = $2
  AND t.account_recipient_id IS NULL AND t.category_id IS NULL AND t.payee_id IS NULL
  AND t.tag_id IS NULL AND t.recurring_id IS NULL AND t.description = '' AND t.type IN (0, 1)
  AND NOT EXISTS (SELECT 1 FROM transactions_labels tl WHERE tl.transaction_id = t.id);

-- name: GetEarliestSpentSince :one
SELECT spent_at FROM transactions
WHERE (account_id = $1 OR account_recipient_id = $2) AND created_at >= $3 AND id <> $4
ORDER BY spent_at ASC
LIMIT 1;

-- name: UpdateCorrectionTransaction :execrows
UPDATE transactions
SET type = $1, amount = $2, spent_at = $3, updated_at = $4, sort_key = $5
WHERE id = $6 AND account_id = $7;

-- name: DeleteCorrectionTransaction :execrows
DELETE FROM transactions WHERE id = $1 AND account_id = $2;

-- name: LockAccountRow :one
SELECT id FROM accounts WHERE id = $1 FOR UPDATE;
