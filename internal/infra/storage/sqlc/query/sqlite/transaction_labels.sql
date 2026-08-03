-- Link rows between a transaction and its reporting labels. Writes are
-- delete-then-insert inside the caller's transaction, so a re-save is
-- idempotent and never duplicates a pair.

-- name: DeleteTransactionLabels :exec
DELETE FROM transactions_labels WHERE transaction_id = ?
;

-- name: InsertTransactionLabel :exec
INSERT INTO transactions_labels (transaction_id, label_id)
VALUES (?, ?)
ON CONFLICT (transaction_id, label_id) DO NOTHING
;
