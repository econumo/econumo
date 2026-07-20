-- Write + read queries for the transaction module (PostgreSQL: $N placeholders).
-- See the sqlite variant for documentation.

-- name: GetTransactionByID :one
SELECT id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id, description, created_at, updated_at, spent_at, type, amount, amount_recipient
FROM transactions
WHERE id = $1;

-- name: UpsertTransaction :exec
INSERT INTO transactions (id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id, description, created_at, updated_at, spent_at, type, amount, amount_recipient)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
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

-- name: DeleteTransaction :exec
DELETE FROM transactions WHERE id = $1;

-- name: ListTransactionsByAccount :many
SELECT id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id, description, created_at, updated_at, spent_at, type, amount, amount_recipient
FROM transactions
WHERE account_id = $1 OR account_recipient_id = $2
ORDER BY spent_at DESC, id;
