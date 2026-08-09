-- Link rows between a transaction and its reporting labels. See the sqlite
-- variant for documentation.

-- name: DeleteTransactionLabels :exec
DELETE FROM transactions_labels WHERE transaction_id = $1
;

-- name: InsertTransactionLabel :exec
INSERT INTO transactions_labels (transaction_id, label_id)
VALUES ($1, $2)
ON CONFLICT (transaction_id, label_id) DO NOTHING
;
