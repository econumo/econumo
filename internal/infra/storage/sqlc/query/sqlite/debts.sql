-- Budget debts. DATE columns are bound as 'Y-m-d' text (CAST ... AS TEXT):
-- the driver would otherwise store a time part, like currencies_rates.published_at.

-- name: GetDebtByID :one
SELECT id, budget_id, element_external_id, element_type, status, ended_at, currency_id, start_date, amount_borrowed, as_of, balance, payment, schedule, last_payment_date, created_day, created_at, updated_at
FROM debts
WHERE id = ?;

-- name: ListDebtsByBudget :many
SELECT id, budget_id, element_external_id, element_type, status, ended_at, currency_id, start_date, amount_borrowed, as_of, balance, payment, schedule, last_payment_date, created_day, created_at, updated_at
FROM debts
WHERE budget_id = ?
ORDER BY created_at, id;

-- name: UpsertDebt :exec
-- The budget is written once, at insert: a debt never moves to another budget.
-- The element moves only with a category merge (MoveDebtsToElement).
INSERT INTO debts (id, budget_id, element_external_id, element_type, status, ended_at, currency_id, start_date, amount_borrowed, as_of, balance, payment, schedule, last_payment_date, created_day, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(budget_id), sqlc.arg(element_external_id), sqlc.arg(element_type), sqlc.arg(status),
    CAST(sqlc.narg(ended_at) AS TEXT), sqlc.arg(currency_id),
    CAST(sqlc.narg(start_date) AS TEXT), sqlc.narg(amount_borrowed), CAST(sqlc.narg(as_of) AS TEXT), sqlc.narg(balance), sqlc.arg(payment), sqlc.arg(schedule),
    CAST(sqlc.narg(last_payment_date) AS TEXT), CAST(sqlc.arg(created_day) AS TEXT), sqlc.arg(created_at), sqlc.arg(updated_at))
ON CONFLICT (id) DO UPDATE SET
    status            = excluded.status,
    ended_at          = excluded.ended_at,
    currency_id       = excluded.currency_id,
    start_date        = excluded.start_date,
    amount_borrowed   = excluded.amount_borrowed,
    as_of             = excluded.as_of,
    balance           = excluded.balance,
    payment           = excluded.payment,
    schedule          = excluded.schedule,
    last_payment_date = excluded.last_payment_date,
    updated_at        = excluded.updated_at;

-- name: DeleteDebt :exec
DELETE FROM debts WHERE id = ?;

-- name: MoveDebtsToElement :exec
UPDATE debts SET element_external_id = sqlc.arg(to_id), element_type = sqlc.arg(element_type), updated_at = sqlc.arg(updated_at)
WHERE budget_id = sqlc.arg(budget_id) AND element_external_id = sqlc.arg(from_id);

-- name: InsertDebtSnapshot :exec
INSERT INTO debts_snapshots (id, debt_id, as_of, balance, payment, schedule, last_payment_date, created_at)
VALUES (sqlc.arg(id), sqlc.arg(debt_id), CAST(sqlc.arg(as_of) AS TEXT), sqlc.arg(balance), sqlc.arg(payment), sqlc.arg(schedule),
    CAST(sqlc.arg(last_payment_date) AS TEXT), sqlc.arg(created_at));

-- name: UpdateDebtSnapshot :exec
-- A correction of the loan's facts rewrites the origin (a debt's first
-- snapshot) and every later snapshot's payment, schedule and last payment
-- date; a statement on a date that already has one replaces its balance.
UPDATE debts_snapshots
SET as_of = CAST(sqlc.arg(as_of) AS TEXT), balance = sqlc.arg(balance), payment = sqlc.arg(payment), schedule = sqlc.arg(schedule),
    last_payment_date = CAST(sqlc.arg(last_payment_date) AS TEXT)
WHERE id = sqlc.arg(id);

-- name: DeleteDebtSnapshot :exec
DELETE FROM debts_snapshots WHERE id = ?;

-- name: InsertDebtSplit :exec
-- Through the transaction row: a payment deleted since it was read inserts
-- nothing instead of failing the foreign key.
INSERT INTO debts_payments (debt_id, transaction_id, interest)
SELECT CAST(sqlc.arg(debt_id) AS TEXT), t.id, CAST(sqlc.arg(interest) AS TEXT)
FROM transactions t WHERE t.id = sqlc.arg(transaction_id)
ON CONFLICT (debt_id, transaction_id) DO UPDATE SET interest = excluded.interest;

-- name: UpsertDebtRateChange :exec
INSERT INTO debts_rate_changes (id, debt_id, effective_date, annual_rate, payment, created_at)
VALUES (sqlc.arg(id), sqlc.arg(debt_id), CAST(sqlc.arg(effective_date) AS TEXT), sqlc.arg(annual_rate), sqlc.arg(payment), sqlc.arg(created_at))
ON CONFLICT (debt_id, effective_date) DO UPDATE
SET id = excluded.id, annual_rate = excluded.annual_rate, payment = excluded.payment, created_at = excluded.created_at;

-- name: DeleteDebtRateChange :execrows
DELETE FROM debts_rate_changes WHERE id = sqlc.arg(id) AND debt_id = sqlc.arg(debt_id);
