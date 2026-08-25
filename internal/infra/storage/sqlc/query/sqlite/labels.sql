-- Write-side queries for the label module. The read-side query lives in
-- label_read.sql to keep the CQRS boundary visible (matching tags.sql vs
-- tag_read.sql). Unlike tags, a label's icon IS persisted from the start.

-- name: GetLabelByID :one
SELECT id, user_id, name, icon, sort_key, is_archived, created_at, updated_at
FROM labels
WHERE id = ?
;

-- name: CountLabelsByOwner :one
SELECT COUNT(*) FROM labels WHERE user_id = ?
;

-- name: ListLabelsByOwner :many
-- The owner's labels ordered by sort key; used by move-label (load, place the
-- moved row, save it) and as the basis for the returned list.
SELECT id, user_id, name, icon, sort_key, is_archived, created_at, updated_at
FROM labels
WHERE user_id = ?
ORDER BY sort_key, id
;

-- name: UpsertLabel :exec
INSERT INTO labels (id, user_id, name, icon, sort_key, is_archived, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    user_id     = excluded.user_id,
    name        = excluded.name,
    icon        = excluded.icon,
    sort_key    = excluded.sort_key,
    is_archived = excluded.is_archived,
    updated_at  = excluded.updated_at
;

-- name: DeleteLabel :exec
-- transactions_labels rows for this label are removed by ON DELETE CASCADE;
-- unlike tags there is no SET NULL, because the link is a join table.
DELETE FROM labels WHERE id = ?
;

-- name: ReassignTransactionLabels :exec
-- Merge: transactions_labels is many-to-many, so a transaction may ALREADY hold
-- both labels. Re-pointing has to dedupe rather than overwrite, or the pair
-- collides on the (transaction_id, label_id) primary key. The source rows
-- themselves cascade away when the label is deleted.
INSERT OR IGNORE INTO transactions_labels (transaction_id, label_id)
SELECT tl.transaction_id, ? FROM transactions_labels tl WHERE tl.label_id = ?;

-- name: ReassignRecurringLabels :exec
INSERT OR IGNORE INTO recurring_transactions_labels (recurring_transaction_id, label_id)
SELECT rtl.recurring_transaction_id, ? FROM recurring_transactions_labels rtl WHERE rtl.label_id = ?;
