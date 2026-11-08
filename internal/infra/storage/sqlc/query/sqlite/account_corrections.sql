-- Balance-correction transaction insert (SQLite). The account module's create
-- (initial non-zero balance) and update (balance change) write a correction
-- transaction so the computed balance matches the requested one. This is a
-- minimal write the account module owns; the full transaction module (later)
-- will own the broader transaction surface.
--
-- A correction is a plain income/expense (type 0 or 1) with no category/payee/
-- tag/recipient. The factory chooses the type from the sign of the correction
-- amount; the stored amount is the absolute value.

-- name: InsertCorrectionTransaction :exec
INSERT INTO transactions (
    id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id,
    description, created_at, updated_at, spent_at, type, amount, amount_recipient, sort_key
) VALUES (
    ?, ?, ?, NULL, NULL, NULL, NULL,
    ?, ?, ?, ?, ?, ?, NULL, ?
);

-- The keep-balance flow recognises an untouched opening correction by shape:
-- an income/expense with no recipient, category, payee, tag, recurring link,
-- label or description.

-- name: GetFirstAccountTransactionID :one
SELECT id FROM transactions
WHERE account_id = ? OR account_recipient_id = ?
ORDER BY id
LIMIT 1;

-- name: GetOpeningCorrection :one
SELECT t.id, t.user_id, t.account_id, t.description, t.type, t.amount, t.spent_at, t.created_at
FROM transactions t
WHERE t.id = ? AND t.account_id = ?
  AND t.account_recipient_id IS NULL AND t.category_id IS NULL AND t.payee_id IS NULL
  AND t.tag_id IS NULL AND t.recurring_id IS NULL AND t.description = '' AND t.type IN (0, 1)
  AND NOT EXISTS (SELECT 1 FROM transactions_labels tl WHERE tl.transaction_id = t.id);

-- name: GetEarliestSpentSince :one
SELECT spent_at FROM transactions
WHERE (account_id = ? OR account_recipient_id = ?) AND created_at >= ? AND id <> ?
ORDER BY spent_at ASC
LIMIT 1;

-- name: UpdateCorrectionTransaction :exec
UPDATE transactions
SET type = ?, amount = ?, spent_at = ?, updated_at = ?, sort_key = ?
WHERE id = ? AND account_id = ?;

-- name: DeleteCorrectionTransaction :exec
DELETE FROM transactions WHERE id = ? AND account_id = ?;
