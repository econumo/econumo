package repo

// reportSQLSqlite / reportSQLPg build the per-account flow report (incomes,
// transfer_incomes, exchange_incomes, expenses, transfer_expenses,
// exchange_expenses) over [start, end).
// sqlite repeats the start/end positionally (8 pairs); pgsql reuses $1/$2.
// `in` is the pre-built account-id placeholder list.

func reportSQLSqlite(in string) string {
	// CAST each aggregate to TEXT so SQLite renders the float SUM with its own
	// shortest round-trip string (e.g. "19024.7"). Scanning the REAL column into a
	// Go float64 first (then formatting at scale 8) accumulates per-row rounding
	// error across accounts (e.g. "19024.69999999"). Returning TEXT keeps the
	// per-account value exact for the decimal summation downstream.
	return `SELECT a.id as account_id, a.currency_id,
 CAST(COALESCE(incomes,0) AS TEXT) as incomes, CAST(COALESCE(transfer_incomes,0) AS TEXT) as transfer_incomes, CAST(COALESCE(exchange_incomes,0) AS TEXT) as exchange_incomes,
 CAST(COALESCE(expenses,0) AS TEXT) as expenses, CAST(COALESCE(transfer_expenses,0) AS TEXT) as transfer_expenses, CAST(COALESCE(exchange_expenses,0) AS TEXT) as exchange_expenses
FROM accounts a LEFT JOIN (
 SELECT tmp.account_id, SUM(tmp.expenses) expenses, SUM(tmp.incomes) incomes, SUM(tmp.transfer_expenses) transfer_expenses, SUM(tmp.transfer_incomes) transfer_incomes, SUM(tmp.exchange_expenses) exchange_expenses, SUM(tmp.exchange_incomes) exchange_incomes FROM (
  SELECT tr1.account_id,
   (SELECT SUM(t1.amount) FROM transactions t1 WHERE t1.account_id=tr1.account_id AND t1.type=0 AND t1.spent_at >= ? AND t1.spent_at < ?) as expenses,
   (SELECT SUM(t2.amount) FROM transactions t2 WHERE t2.account_id=tr1.account_id AND t2.type=1 AND t2.spent_at >= ? AND t2.spent_at < ?) as incomes,
   (SELECT SUM(t3.amount) FROM transactions t3 WHERE t3.account_id=tr1.account_id AND t3.type=2 AND t3.spent_at >= ? AND t3.spent_at < ?) as transfer_expenses,
   NULL as transfer_incomes, NULL as exchange_incomes,
   (SELECT SUM(t4.amount) FROM transactions t4 WHERE t4.account_id=tr1.account_id AND t4.type=2 AND t4.amount != t4.amount_recipient AND t4.spent_at >= ? AND t4.spent_at < ?) as exchange_expenses
  FROM transactions tr1 WHERE tr1.spent_at >= ? AND tr1.spent_at < ? GROUP BY tr1.account_id
  UNION ALL
  SELECT tr2.account_recipient_id as account_id, NULL, NULL, NULL,
   (SELECT SUM(t5.amount_recipient) FROM transactions t5 WHERE t5.account_recipient_id=tr2.account_recipient_id AND t5.type=2 AND t5.spent_at >= ? AND t5.spent_at < ?) as transfer_incomes,
   (SELECT SUM(t6.amount_recipient) FROM transactions t6 WHERE t6.account_recipient_id=tr2.account_recipient_id AND t6.type=2 AND t6.amount != t6.amount_recipient AND t6.spent_at >= ? AND t6.spent_at < ?) as exchange_incomes,
   NULL as exchange_expenses
  FROM transactions tr2 WHERE tr2.account_recipient_id IS NOT NULL AND tr2.spent_at >= ? AND tr2.spent_at < ? GROUP BY tr2.account_recipient_id
 ) tmp GROUP BY tmp.account_id
) t ON a.id=t.account_id AND a.id IN (` + in + `)`
}

func reportSQLPg(in string) string {
	return `SELECT a.id as account_id, a.currency_id,
 COALESCE(incomes,0) as incomes, COALESCE(transfer_incomes,0) as transfer_incomes, COALESCE(exchange_incomes,0) as exchange_incomes,
 COALESCE(expenses,0) as expenses, COALESCE(transfer_expenses,0) as transfer_expenses, COALESCE(exchange_expenses,0) as exchange_expenses
FROM accounts a LEFT JOIN (
 SELECT tmp.account_id, SUM(tmp.expenses) expenses, SUM(tmp.incomes) incomes, SUM(tmp.transfer_expenses) transfer_expenses, SUM(tmp.transfer_incomes) transfer_incomes, SUM(tmp.exchange_expenses) exchange_expenses, SUM(tmp.exchange_incomes) exchange_incomes FROM (
  SELECT tr1.account_id,
   (SELECT SUM(t1.amount) FROM transactions t1 WHERE t1.account_id=tr1.account_id AND t1.type=0 AND t1.spent_at >= $1 AND t1.spent_at < $2) as expenses,
   (SELECT SUM(t2.amount) FROM transactions t2 WHERE t2.account_id=tr1.account_id AND t2.type=1 AND t2.spent_at >= $1 AND t2.spent_at < $2) as incomes,
   (SELECT SUM(t3.amount) FROM transactions t3 WHERE t3.account_id=tr1.account_id AND t3.type=2 AND t3.spent_at >= $1 AND t3.spent_at < $2) as transfer_expenses,
   NULL as transfer_incomes, NULL as exchange_incomes,
   (SELECT SUM(t4.amount) FROM transactions t4 WHERE t4.account_id=tr1.account_id AND t4.type=2 AND t4.amount != t4.amount_recipient AND t4.spent_at >= $1 AND t4.spent_at < $2) as exchange_expenses
  FROM transactions tr1 WHERE tr1.spent_at >= $1 AND tr1.spent_at < $2 GROUP BY tr1.account_id
  UNION ALL
  SELECT tr2.account_recipient_id as account_id, NULL, NULL, NULL,
   (SELECT SUM(t5.amount_recipient) FROM transactions t5 WHERE t5.account_recipient_id=tr2.account_recipient_id AND t5.type=2 AND t5.spent_at >= $1 AND t5.spent_at < $2) as transfer_incomes,
   (SELECT SUM(t6.amount_recipient) FROM transactions t6 WHERE t6.account_recipient_id=tr2.account_recipient_id AND t6.type=2 AND t6.amount != t6.amount_recipient AND t6.spent_at >= $1 AND t6.spent_at < $2) as exchange_incomes,
   NULL as exchange_expenses
  FROM transactions tr2 WHERE tr2.account_recipient_id IS NOT NULL AND tr2.spent_at >= $1 AND tr2.spent_at < $2 GROUP BY tr2.account_recipient_id
 ) tmp GROUP BY tmp.account_id
) t ON a.id=t.account_id AND a.id IN (` + in + `)`
}
