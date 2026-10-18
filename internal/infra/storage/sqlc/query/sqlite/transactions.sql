-- Write + read queries for the transaction module (SQLite). A transaction
-- belongs to a user + account, has a type (0 expense, 1 income, 2 transfer),
-- an amount, an optional recipient account + amount (transfers), and optional
-- category/payee/tag (non-transfers). Listing filters by account id sets the
-- app layer computes (excluding deleted/hidden-folder accounts).

-- name: GetTransactionByID :one
SELECT id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id, description, created_at, updated_at, spent_at, type, amount, amount_recipient, recurring_id
FROM transactions
WHERE id = ?;

-- name: UpsertTransaction :exec
-- recurring_id is intentionally absent from the DO UPDATE SET list below: it
-- records where the row came from, so editing a posted instance must not
-- overwrite its provenance.
INSERT INTO transactions (id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id, description, created_at, updated_at, spent_at, type, amount, amount_recipient, recurring_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    account_id           = excluded.account_id,
    account_recipient_id = excluded.account_recipient_id,
    category_id          = excluded.category_id,
    payee_id             = excluded.payee_id,
    tag_id               = excluded.tag_id,
    description          = excluded.description,
    updated_at           = excluded.updated_at,
    spent_at             = excluded.spent_at,
    type                 = excluded.type,
    amount               = excluded.amount,
    amount_recipient     = excluded.amount_recipient;

-- name: LinkTransactionToRecurring :exec
-- The one write that touches recurring_id on an existing row: an explicit
-- "make recurring from this transaction" link. UpsertTransaction deliberately
-- never updates the column, so the link needs its own statement.
UPDATE transactions SET recurring_id = ?, updated_at = ? WHERE id = ?;

-- name: DeleteTransaction :exec
DELETE FROM transactions WHERE id = ?;

-- name: ListTransactionsByAccount :many
-- Transactions on an account (as source or recipient), newest first; id is the
-- stable tie-break so row order is deterministic across engines.
SELECT id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id, description, created_at, updated_at, spent_at, type, amount, amount_recipient, recurring_id
FROM transactions
WHERE account_id = ? OR account_recipient_id = ?
ORDER BY spent_at DESC, id;
