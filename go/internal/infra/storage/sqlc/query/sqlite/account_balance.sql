-- Account balance, computed from the transactions table (SQLite). Ported from
-- the PHP TransactionRepository::getAccountBalance (single) and
-- AccountRepository::getAccountsBalancesBeforeDate (bulk). The balance is the
-- running total of everything spent_at < :before (the caller passes "tomorrow
-- 00:00" so it includes everything through today):
--
--   balance = incomes(type=1) + transfer_incomes(amount_recipient, type=2 as
--             recipient) - expenses(type=0) - transfer_expenses(type=2 as source)
--
-- These live with the account module (its only consumer until transaction/budget
-- land); they read the transactions table directly without needing the
-- transaction write-module.
--
-- SQLite's sqlc parser rejects sqlc.arg() (numbered params) inside subqueries,
-- so we use plain positional '?' and repeat each value at the call site. The
-- repo passes the args in the exact order they appear below.

-- name: GetAccountBalance :one
-- Args, in order: account_id, before (x4 each, interleaved per subquery).
-- Returns "0" when the account has no transactions.
SELECT CAST(COALESCE(incomes, 0) + COALESCE(transfer_incomes, 0) - COALESCE(expenses, 0) - COALESCE(transfer_expenses, 0) AS TEXT) as balance
FROM (
    SELECT
        (SELECT COALESCE(SUM(t0.amount), 0) FROM transactions t0 WHERE t0.account_id = ? AND t0.type = 0 AND t0.spent_at < ?) as expenses,
        (SELECT COALESCE(SUM(t1.amount), 0) FROM transactions t1 WHERE t1.account_id = ? AND t1.type = 1 AND t1.spent_at < ?) as incomes,
        (SELECT COALESCE(SUM(t2.amount), 0) FROM transactions t2 WHERE t2.account_id = ? AND t2.type = 2 AND t2.spent_at < ?) as transfer_expenses,
        (SELECT COALESCE(SUM(t3.amount_recipient), 0) FROM transactions t3 WHERE t3.account_recipient_id = ? AND t3.type = 2 AND t3.spent_at < ?) as transfer_incomes
) bln;

-- name: ListAccountBalancesForUser :many
-- Args, in order: before (x4), user_id. account_id + balance per row; the app
-- layer normalizes the balance string via vo.DecimalNumber.
SELECT
    a.id as account_id,
    CAST(
        (SELECT COALESCE(SUM(ti.amount), 0) FROM transactions ti WHERE ti.account_id = a.id AND ti.type = 1 AND ti.spent_at < ?)
      + (SELECT COALESCE(SUM(tri.amount_recipient), 0) FROM transactions tri WHERE tri.account_recipient_id = a.id AND tri.type = 2 AND tri.spent_at < ?)
      - (SELECT COALESCE(SUM(te.amount), 0) FROM transactions te WHERE te.account_id = a.id AND te.type = 0 AND te.spent_at < ?)
      - (SELECT COALESCE(SUM(tre.amount), 0) FROM transactions tre WHERE tre.account_id = a.id AND tre.type = 2 AND tre.spent_at < ?)
    AS TEXT) as balance
FROM accounts a
WHERE a.user_id = ? AND a.is_deleted = 0;
