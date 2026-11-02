-- Budget debts. DATE columns are bound as 'Y-m-d' text (CAST ... AS TEXT):
-- the driver would otherwise store a time part, like currencies_rates.published_at.

-- name: GetDebtByID :one
SELECT id, user_id, name, currency_id, category_id, payee_id, tag_id, start_date, as_of, balance, payment, schedule, last_payment_date, created_at, updated_at
FROM debts
WHERE id = ?;

-- name: ListDebtsByCategory :many
SELECT id, user_id, name, currency_id, category_id, payee_id, tag_id, start_date, as_of, balance, payment, schedule, last_payment_date, created_at, updated_at
FROM debts
WHERE category_id = ?
ORDER BY created_at, id;

-- name: UpsertDebt :exec
INSERT INTO debts (id, user_id, name, currency_id, category_id, payee_id, tag_id, start_date, as_of, balance, payment, schedule, last_payment_date, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(currency_id), sqlc.arg(category_id), sqlc.arg(payee_id), sqlc.arg(tag_id),
    CAST(sqlc.narg(start_date) AS TEXT), CAST(sqlc.arg(as_of) AS TEXT), sqlc.arg(balance), sqlc.arg(payment), sqlc.arg(schedule),
    CAST(sqlc.arg(last_payment_date) AS TEXT), sqlc.arg(created_at), sqlc.arg(updated_at))
ON CONFLICT (id) DO UPDATE SET
    name              = excluded.name,
    currency_id       = excluded.currency_id,
    category_id       = excluded.category_id,
    payee_id          = excluded.payee_id,
    tag_id            = excluded.tag_id,
    start_date        = excluded.start_date,
    as_of             = excluded.as_of,
    balance           = excluded.balance,
    payment           = excluded.payment,
    schedule          = excluded.schedule,
    last_payment_date = excluded.last_payment_date,
    updated_at        = excluded.updated_at;

-- name: DeleteDebt :exec
DELETE FROM debts WHERE id = ?;

-- name: InsertDebtSnapshot :exec
INSERT INTO debts_snapshots (id, debt_id, as_of, balance, payment, schedule, last_payment_date, created_at)
VALUES (sqlc.arg(id), sqlc.arg(debt_id), CAST(sqlc.arg(as_of) AS TEXT), sqlc.arg(balance), sqlc.arg(payment), sqlc.arg(schedule),
    CAST(sqlc.arg(last_payment_date) AS TEXT), sqlc.arg(created_at));

-- name: InsertDebtSplit :exec
-- Through the transaction row: a payment deleted since it was read inserts
-- nothing instead of failing the foreign key.
INSERT INTO debts_payments (debt_id, transaction_id, interest)
SELECT CAST(sqlc.arg(debt_id) AS TEXT), t.id, CAST(sqlc.arg(interest) AS TEXT)
FROM transactions t WHERE t.id = sqlc.arg(transaction_id)
ON CONFLICT (transaction_id) DO UPDATE SET debt_id = excluded.debt_id, interest = excluded.interest;

-- name: CandidateDebtTransactions :many
-- Expenses only (type 0); the currency is the account's.
SELECT t.id, t.spent_at, t.amount, a.currency_id, t.payee_id, t.tag_id
FROM transactions t
JOIN accounts a ON a.id = t.account_id
WHERE t.category_id = ? AND t.type = 0
ORDER BY t.spent_at, t.id;

-- name: DebtUsesCategory :one
SELECT EXISTS (SELECT 1 FROM debts WHERE category_id = ?);

-- name: DebtUsesPayee :one
SELECT EXISTS (SELECT 1 FROM debts WHERE payee_id = ?);

-- name: DebtUsesTag :one
SELECT EXISTS (SELECT 1 FROM debts WHERE tag_id = ?);

-- name: ReassignDebtCategory :exec
UPDATE debts SET category_id = ? WHERE category_id = ?;

-- name: ReassignDebtPayee :exec
UPDATE debts SET payee_id = ? WHERE payee_id = ?;

-- name: ReassignDebtTag :exec
UPDATE debts SET tag_id = ? WHERE tag_id = ?;
