-- Account balance computed from the transactions table (PostgreSQL). See the
-- sqlite variant for the formula and rationale. Uses sqlc.arg() named params so
-- the generated signatures stay readable; pgsql renders them positionally.

-- name: GetAccountBalance :one
SELECT CAST(COALESCE(incomes, 0) + COALESCE(transfer_incomes, 0) - COALESCE(expenses, 0) - COALESCE(transfer_expenses, 0) AS TEXT) as balance
FROM (
    SELECT
        (SELECT COALESCE(SUM(t0.amount), 0) FROM transactions t0 WHERE t0.account_id = sqlc.arg(account_id) AND t0.type = 0 AND t0.spent_at < sqlc.arg(before)) as expenses,
        (SELECT COALESCE(SUM(t1.amount), 0) FROM transactions t1 WHERE t1.account_id = sqlc.arg(account_id) AND t1.type = 1 AND t1.spent_at < sqlc.arg(before)) as incomes,
        (SELECT COALESCE(SUM(t2.amount), 0) FROM transactions t2 WHERE t2.account_id = sqlc.arg(account_id) AND t2.type = 2 AND t2.spent_at < sqlc.arg(before)) as transfer_expenses,
        (SELECT COALESCE(SUM(t3.amount_recipient), 0) FROM transactions t3 WHERE t3.account_recipient_id = sqlc.arg(account_id) AND t3.type = 2 AND t3.spent_at < sqlc.arg(before)) as transfer_incomes
) bln;

-- name: ListAccountBalancesForUser :many
-- Balances for every AVAILABLE account (own + shared via accounts_access), to
-- match PHP getAccountsBalancesBeforeDate over the available account-id set.
-- PostgreSQL's SUM(NUMERIC) is EXACT (not float like SQLite), so CAST AS TEXT
-- here yields the exact decimal — no precision-14 reformatting needed.
SELECT
    a.id as account_id,
    CAST(
        (SELECT COALESCE(SUM(ti.amount), 0) FROM transactions ti WHERE ti.account_id = a.id AND ti.type = 1 AND ti.spent_at < sqlc.arg(before))
      + (SELECT COALESCE(SUM(tri.amount_recipient), 0) FROM transactions tri WHERE tri.account_recipient_id = a.id AND tri.type = 2 AND tri.spent_at < sqlc.arg(before))
      - (SELECT COALESCE(SUM(te.amount), 0) FROM transactions te WHERE te.account_id = a.id AND te.type = 0 AND te.spent_at < sqlc.arg(before))
      - (SELECT COALESCE(SUM(tre.amount), 0) FROM transactions tre WHERE tre.account_id = a.id AND tre.type = 2 AND tre.spent_at < sqlc.arg(before))
    AS TEXT) as balance
FROM accounts a
LEFT JOIN accounts_access aa ON aa.account_id = a.id
WHERE a.is_deleted = false AND (a.user_id = sqlc.arg(user_id) OR (aa.user_id = sqlc.arg(user_id) AND aa.is_accepted = true))
GROUP BY a.id;
