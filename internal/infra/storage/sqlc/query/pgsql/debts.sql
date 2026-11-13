-- Budget debts. DATE parameters travel as 'Y-m-d' text and are cast in SQL, so
-- the generated parameter types match the sqlite variants.

-- name: GetDebtByID :one
SELECT id, budget_id, name, icon, currency_id, start_date, amount_borrowed, as_of, balance, payment, schedule, last_payment_date, created_day, created_at, updated_at
FROM debts
WHERE id = $1;

-- name: ListDebtsByBudget :many
SELECT id, budget_id, name, icon, currency_id, start_date, amount_borrowed, as_of, balance, payment, schedule, last_payment_date, created_day, created_at, updated_at
FROM debts
WHERE budget_id = $1
ORDER BY created_at, id;

-- name: UpsertDebt :exec
-- The budget is written once, at insert: a debt never moves to another budget.
INSERT INTO debts (id, budget_id, name, icon, currency_id, start_date, amount_borrowed, as_of, balance, payment, schedule, last_payment_date, created_day, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(budget_id), sqlc.arg(name), sqlc.arg(icon), sqlc.arg(currency_id),
    sqlc.narg(start_date)::text::date, sqlc.narg(amount_borrowed), sqlc.narg(as_of)::text::date, sqlc.narg(balance), sqlc.arg(payment), sqlc.arg(schedule),
    sqlc.narg(last_payment_date)::text::date, sqlc.arg(created_day)::text::date, sqlc.arg(created_at), sqlc.arg(updated_at))
ON CONFLICT (id) DO UPDATE SET
    name              = excluded.name,
    icon              = excluded.icon,
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
DELETE FROM debts WHERE id = $1;

-- name: DeleteDebtRules :exec
DELETE FROM debts_rules WHERE debt_id = $1;

-- name: InsertDebtRule :exec
INSERT INTO debts_rules (id, debt_id, category_id, payee_id, tag_id, label_id, created_at)
VALUES (sqlc.arg(id), sqlc.arg(debt_id), sqlc.arg(category_id), sqlc.narg(payee_id), sqlc.narg(tag_id), sqlc.narg(label_id), sqlc.arg(created_at));

-- name: InsertDebtSnapshot :exec
INSERT INTO debts_snapshots (id, debt_id, as_of, balance, payment, schedule, last_payment_date, created_at)
VALUES (sqlc.arg(id), sqlc.arg(debt_id), sqlc.arg(as_of)::text::date, sqlc.arg(balance), sqlc.arg(payment), sqlc.arg(schedule),
    sqlc.arg(last_payment_date)::text::date, sqlc.arg(created_at));

-- name: UpdateDebtSnapshot :exec
-- A correction of the loan's facts rewrites the origin (a debt's first
-- snapshot) and every later snapshot's payment, schedule and last payment
-- date; a statement on a date that already has one replaces its balance.
UPDATE debts_snapshots
SET as_of = sqlc.arg(as_of)::text::date, balance = sqlc.arg(balance), payment = sqlc.arg(payment), schedule = sqlc.arg(schedule),
    last_payment_date = sqlc.arg(last_payment_date)::text::date
WHERE id = sqlc.arg(id);

-- name: DeleteDebtSnapshot :exec
DELETE FROM debts_snapshots WHERE id = $1;

-- name: InsertDebtSplit :exec
-- Through the transaction row: a payment deleted since it was read inserts
-- nothing instead of failing the foreign key.
INSERT INTO debts_payments (debt_id, transaction_id, interest)
SELECT CAST(sqlc.arg(debt_id) AS UUID), t.id, CAST(sqlc.arg(interest) AS NUMERIC(19,8))
FROM transactions t WHERE t.id = sqlc.arg(transaction_id)
ON CONFLICT (debt_id, transaction_id) DO UPDATE SET interest = excluded.interest;

-- name: LockDebtBudget :one
-- NO KEY UPDATE leaves the KEY SHARE of child inserts (budget rows, debts)
-- unblocked.
SELECT id FROM budgets WHERE id = $1 FOR NO KEY UPDATE;

