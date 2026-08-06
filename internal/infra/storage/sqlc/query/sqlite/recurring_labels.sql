-- Link rows between a recurring template and its reporting labels. Writes are
-- delete-then-insert inside the caller's transaction, so a re-save is
-- idempotent and never duplicates a pair.

-- name: DeleteRecurringLabels :exec
DELETE FROM recurring_transactions_labels WHERE recurring_transaction_id = ?
;

-- name: InsertRecurringLabel :exec
INSERT INTO recurring_transactions_labels (recurring_transaction_id, label_id)
VALUES (?, ?)
ON CONFLICT (recurring_transaction_id, label_id) DO NOTHING
;
