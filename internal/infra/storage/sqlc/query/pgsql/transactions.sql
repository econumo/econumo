-- Write + read queries for the transaction module (PostgreSQL: $N placeholders).
-- See the sqlite variant for documentation.

-- name: GetTransactionByID :one
SELECT id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id, description, created_at, updated_at, spent_at, type, amount, amount_recipient, recurring_id, sort_key, recipient_sort_key
FROM transactions
WHERE id = $1;

-- name: UpsertTransaction :exec
INSERT INTO transactions (id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id, description, created_at, updated_at, spent_at, type, amount, amount_recipient, recurring_id, sort_key, recipient_sort_key)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
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
    amount_recipient     = excluded.amount_recipient,
    sort_key             = excluded.sort_key,
    recipient_sort_key   = excluded.recipient_sort_key;

-- name: LinkTransactionToRecurring :exec
-- The one write that touches recurring_id on an existing row: an explicit
-- "make recurring from this transaction" link. UpsertTransaction deliberately
-- never updates the column, so the link needs its own statement.
UPDATE transactions SET recurring_id = $1, updated_at = $2 WHERE id = $3;

-- name: DeleteTransaction :exec
DELETE FROM transactions WHERE id = $1;

-- name: ListTransactionsByAccount :many
-- Transactions on an account (as source or recipient) in list order: newest
-- day first, then the row's place in that day on THIS account's side
-- (sort_key as the source, recipient_sort_key as a transfer's recipient), then
-- id. The time of day takes no part.
SELECT id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id, description, created_at, updated_at, spent_at, type, amount, amount_recipient, recurring_id, sort_key, recipient_sort_key
FROM transactions
WHERE account_id = sqlc.arg(account_id) OR account_recipient_id = sqlc.arg(account_id)
ORDER BY CAST(spent_at AS date) DESC,
    CASE WHEN account_id = sqlc.arg(account_id) THEN sort_key ELSE recipient_sort_key END DESC,
    id;
