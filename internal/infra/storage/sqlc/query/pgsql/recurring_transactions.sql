-- name: GetRecurringTransactionByID :one
SELECT id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id,
       type, amount, description, schedule, next_payment_at, scheduled_day, created_at, updated_at
FROM recurring_transactions
WHERE id = $1;

-- name: UpsertRecurringTransaction :exec
INSERT INTO recurring_transactions (id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id,
                                    type, amount, description, schedule, next_payment_at, scheduled_day, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
ON CONFLICT (id) DO UPDATE SET
    account_id = excluded.account_id
    , account_recipient_id = excluded.account_recipient_id
    , category_id = excluded.category_id
    , payee_id = excluded.payee_id
    , tag_id = excluded.tag_id
    , type = excluded.type
    , amount = excluded.amount
    , description = excluded.description
    , schedule = excluded.schedule
    , next_payment_at = excluded.next_payment_at
    , scheduled_day = excluded.scheduled_day
    , updated_at = excluded.updated_at;

-- name: DeleteRecurringTransaction :exec
DELETE FROM recurring_transactions WHERE id = $1;
