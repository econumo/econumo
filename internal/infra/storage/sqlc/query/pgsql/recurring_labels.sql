-- Link rows between a recurring template and its reporting labels. See the
-- sqlite variant for documentation.

-- name: DeleteRecurringLabels :exec
DELETE FROM recurring_transactions_labels WHERE recurring_transaction_id = $1
;

-- name: InsertRecurringLabel :exec
INSERT INTO recurring_transactions_labels (recurring_transaction_id, label_id)
VALUES ($1, $2)
ON CONFLICT (recurring_transaction_id, label_id) DO NOTHING
;
