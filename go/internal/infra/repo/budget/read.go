// Budget read repo: the heavy budget reports (per-currency account balances /
// flow report / holdings, per-element spending, summed limits). All take
// variadic id sets, so they are hand-built per engine (dynamic IN) rather than
// sqlc — exactly like the transaction module's ListByAccountIDs. The SQL mirrors
// AccountRepository's native budget queries + TransactionRepository::countSpending
// + BudgetElementLimitRepository::getSummarizedLimitsForPeriod.
package budgetrepo

import (
	"context"
	"strconv"
	"strings"
	"time"

	appbudget "github.com/econumo/econumo/internal/app/budget"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	"github.com/econumo/econumo/internal/infra/storage/backend"
)

// ReadRepo implements app/budget.ReadModel.
type ReadRepo struct {
	tx     *backend.TxManager
	driver string
}

var _ appbudget.ReadModel = (*ReadRepo)(nil)

// NewReadRepo constructs the read repo.
func NewReadRepo(driver string, tx *backend.TxManager) *ReadRepo {
	if driver != "sqlite" && driver != "postgresql" {
		panic("budgetrepo: unknown database driver " + driver)
	}
	return &ReadRepo{tx: tx, driver: driver}
}

func (r *ReadRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// ph builds a comma-separated placeholder list of n params starting at position
// `start`: "?,?,..." for sqlite, "$start,..." for pgsql.
func (r *ReadRepo) ph(start, n int) string {
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		if r.driver == "postgresql" {
			parts[i] = "$" + itoa(start+i)
		} else {
			parts[i] = "?"
		}
	}
	return strings.Join(parts, ",")
}

func idArgs(ids []vo.Id) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
}

// balanceSQL builds the per-account balance report SQL. cmp is "<" (before) or
// "<=" (on date). The date is bound once but referenced 6 times; sqlite repeats
// the positional arg, pgsql reuses a single numbered param.
func (r *ReadRepo) balanceSQL(cmp string, nAccounts int) (string, func(date time.Time, ids []any) []any) {
	if r.driver == "postgresql" {
		// $1 = date (reused), $2.. = account ids.
		in := r.phFrom(2, nAccounts)
		sql := `SELECT a.id as account_id, a.currency_id,
COALESCE(incomes,0)+COALESCE(transfer_incomes,0)-COALESCE(expenses,0)-COALESCE(transfer_expenses,0) as balance
FROM accounts a LEFT JOIN (
 SELECT tmp.account_id, SUM(tmp.expenses) expenses, SUM(tmp.incomes) incomes, SUM(tmp.transfer_expenses) transfer_expenses, SUM(tmp.transfer_incomes) transfer_incomes FROM (
  SELECT tr1.account_id,
   (SELECT SUM(t1.amount) FROM transactions t1 WHERE t1.account_id=tr1.account_id AND t1.type=0 AND t1.spent_at ` + cmp + ` $1) as expenses,
   (SELECT SUM(t2.amount) FROM transactions t2 WHERE t2.account_id=tr1.account_id AND t2.type=1 AND t2.spent_at ` + cmp + ` $1) as incomes,
   (SELECT SUM(t3.amount) FROM transactions t3 WHERE t3.account_id=tr1.account_id AND t3.type=2 AND t3.spent_at ` + cmp + ` $1) as transfer_expenses,
   NULL as transfer_incomes
  FROM transactions tr1 WHERE tr1.spent_at ` + cmp + ` $1 GROUP BY tr1.account_id
  UNION ALL
  SELECT tr2.account_recipient_id as account_id, NULL, NULL, NULL,
   (SELECT SUM(t4.amount_recipient) FROM transactions t4 WHERE t4.account_recipient_id=tr2.account_recipient_id AND t4.type=2 AND t4.spent_at ` + cmp + ` $1) as transfer_incomes
  FROM transactions tr2 WHERE tr2.account_recipient_id IS NOT NULL AND tr2.spent_at ` + cmp + ` $1 GROUP BY tr2.account_recipient_id
 ) tmp GROUP BY tmp.account_id
) t ON a.id=t.account_id AND a.id IN (` + in + `)`
		return sql, func(date time.Time, ids []any) []any {
			return append([]any{date}, ids...)
		}
	}
	// sqlite: the date is repeated 6 times positionally before the IN list.
	in := r.ph(1, nAccounts)
	sql := `SELECT a.id as account_id, a.currency_id,
COALESCE(incomes,0)+COALESCE(transfer_incomes,0)-COALESCE(expenses,0)-COALESCE(transfer_expenses,0) as balance
FROM accounts a LEFT JOIN (
 SELECT tmp.account_id, SUM(tmp.expenses) expenses, SUM(tmp.incomes) incomes, SUM(tmp.transfer_expenses) transfer_expenses, SUM(tmp.transfer_incomes) transfer_incomes FROM (
  SELECT tr1.account_id,
   (SELECT SUM(t1.amount) FROM transactions t1 WHERE t1.account_id=tr1.account_id AND t1.type=0 AND t1.spent_at ` + cmp + ` ?) as expenses,
   (SELECT SUM(t2.amount) FROM transactions t2 WHERE t2.account_id=tr1.account_id AND t2.type=1 AND t2.spent_at ` + cmp + ` ?) as incomes,
   (SELECT SUM(t3.amount) FROM transactions t3 WHERE t3.account_id=tr1.account_id AND t3.type=2 AND t3.spent_at ` + cmp + ` ?) as transfer_expenses,
   NULL as transfer_incomes
  FROM transactions tr1 WHERE tr1.spent_at ` + cmp + ` ? GROUP BY tr1.account_id
  UNION ALL
  SELECT tr2.account_recipient_id as account_id, NULL, NULL, NULL,
   (SELECT SUM(t4.amount_recipient) FROM transactions t4 WHERE t4.account_recipient_id=tr2.account_recipient_id AND t4.type=2 AND t4.spent_at ` + cmp + ` ?) as transfer_incomes
  FROM transactions tr2 WHERE tr2.account_recipient_id IS NOT NULL AND tr2.spent_at ` + cmp + ` ? GROUP BY tr2.account_recipient_id
 ) tmp GROUP BY tmp.account_id
) t ON a.id=t.account_id AND a.id IN (` + in + `)`
	return sql, func(date time.Time, ids []any) []any {
		args := make([]any, 0, 6+len(ids))
		for i := 0; i < 6; i++ {
			args = append(args, date)
		}
		return append(args, ids...)
	}
}

// phFrom is ph for pgsql numbered params starting at `start` (sqlite ignores start).
func (r *ReadRepo) phFrom(start, n int) string { return r.ph(start, n) }

func (r *ReadRepo) accountBalances(ctx context.Context, cmp string, accountIDs []vo.Id, date time.Time) ([]appbudget.AccountBalanceRow, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	ids := idArgs(accountIDs)
	sql, args := r.balanceSQL(cmp, len(ids))
	rows, err := r.db(ctx).QueryContext(ctx, sql, args(date, ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []appbudget.AccountBalanceRow
	for rows.Next() {
		var row appbudget.AccountBalanceRow
		// SQLite's SUM over NUMERIC is float; scanning it as a string yields the
		// driver's full-precision rendering (e.g. "358.34999999999127"). PHP's
		// DecimalNumber constructor instead does sprintf('%.8F', $float) — round
		// to 8 decimals (bcmath SCALE) — giving "358.35". Match that by scanning
		// the float and formatting with 'f',8. PostgreSQL's SUM is exact NUMERIC,
		// so scan it as a string and pass through unchanged.
		if r.driver == "postgresql" {
			var bal *string
			if err := rows.Scan(&row.AccountID, &row.CurrencyID, &bal); err != nil {
				return nil, err
			}
			if bal != nil {
				row.Balance = *bal
			} else {
				row.Balance = "0"
			}
		} else {
			var bal *float64
			if err := rows.Scan(&row.AccountID, &row.CurrencyID, &bal); err != nil {
				return nil, err
			}
			if bal != nil {
				row.Balance = strconv.FormatFloat(*bal, 'f', 8, 64)
			} else {
				row.Balance = "0"
			}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// AccountsBalancesOnDate implements ReadModel.
func (r *ReadRepo) AccountsBalancesOnDate(ctx context.Context, accountIDs []vo.Id, date time.Time) ([]appbudget.AccountBalanceRow, error) {
	return r.accountBalances(ctx, "<=", accountIDs, date)
}

// AccountsBalancesBeforeDate implements ReadModel.
func (r *ReadRepo) AccountsBalancesBeforeDate(ctx context.Context, accountIDs []vo.Id, date time.Time) ([]appbudget.AccountBalanceRow, error) {
	return r.accountBalances(ctx, "<", accountIDs, date)
}

// AccountsReport implements ReadModel.
func (r *ReadRepo) AccountsReport(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]appbudget.AccountReportRow, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	ids := idArgs(accountIDs)
	var sql string
	var args []any
	if r.driver == "postgresql" {
		in := r.ph(3, len(ids)) // $1=start, $2=end, $3.. ids
		sql = reportSQLPg(in)
		args = append([]any{start, end}, ids...)
	} else {
		in := r.ph(1, len(ids))
		sql = reportSQLSqlite(in)
		// 8 (start,end) pairs = 16 date args, then ids.
		args = make([]any, 0, 16+len(ids))
		for i := 0; i < 8; i++ {
			args = append(args, start, end)
		}
		args = append(args, ids...)
	}
	rows, err := r.db(ctx).QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []appbudget.AccountReportRow
	for rows.Next() {
		var row appbudget.AccountReportRow
		if err := rows.Scan(&row.AccountID, &row.CurrencyID, &row.Incomes, &row.TransferIncomes, &row.ExchangeIncomes, &row.Expenses, &row.TransferExpenses, &row.ExchangeExpenses); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// HoldingsReport implements ReadModel (two queries, merged per currency).
func (r *ReadRepo) HoldingsReport(ctx context.Context, accountIDs []vo.Id, start, end time.Time) ([]appbudget.HoldingsRow, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	ids := idArgs(accountIDs)
	type acc struct{ from, to string }
	merged := map[string]*acc{}

	run := func(sql string, args []any, set func(m *acc, amount string)) error {
		rows, err := r.db(ctx).QueryContext(ctx, sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var currencyID *string
			var amount *string
			if err := rows.Scan(&amount, &currencyID); err != nil {
				return err
			}
			if currencyID == nil {
				continue
			}
			m := merged[*currencyID]
			if m == nil {
				m = &acc{from: "0", to: "0"}
				merged[*currencyID] = m
			}
			a := "0"
			if amount != nil {
				a = *amount
			}
			set(m, a)
		}
		return rows.Err()
	}

	// to_holdings: same-amount transfers OUT of the set (recipient outside).
	toSQL, toArgs := r.holdingsSQL(true, ids, start, end)
	if err := run(toSQL, toArgs, func(m *acc, amount string) { m.to = vo.NewDecimal(m.to).Add(vo.NewDecimal(amount)).String() }); err != nil {
		return nil, err
	}
	// from_holdings: same-amount transfers INTO the set (source outside).
	fromSQL, fromArgs := r.holdingsSQL(false, ids, start, end)
	if err := run(fromSQL, fromArgs, func(m *acc, amount string) { m.from = vo.NewDecimal(m.from).Add(vo.NewDecimal(amount)).String() }); err != nil {
		return nil, err
	}

	out := make([]appbudget.HoldingsRow, 0, len(merged))
	for cid, m := range merged {
		out = append(out, appbudget.HoldingsRow{CurrencyID: cid, FromHoldings: m.from, ToHoldings: m.to})
	}
	return out, nil
}

// holdingsSQL builds one of the two holdings queries. toHoldings=true selects
// transfers whose recipient is OUTSIDE the set and source INSIDE (money leaving);
// false is the reverse (money entering). Both filter amount = amount_recipient
// (same-currency transfer) and type=2.
func (r *ReadRepo) holdingsSQL(toHoldings bool, ids []any, start, end time.Time) (string, []any) {
	amountCol := "t.amount_recipient"
	joinCol := "t.account_recipient_id"
	if !toHoldings {
		amountCol = "t.amount"
		joinCol = "t.account_id"
	}
	if r.driver == "postgresql" {
		n := len(ids)
		inA := r.ph(1, n)
		inB := r.ph(1+n, n)
		dStart := "$" + itoa(1+2*n)
		dEnd := "$" + itoa(2+2*n)
		var where string
		if toHoldings {
			// recipient outside, source inside
			where = "t.account_recipient_id NOT IN (" + inA + ") AND t.account_id IN (" + inB + ")"
		} else {
			// recipient inside, source outside
			where = "t.account_recipient_id IN (" + inA + ") AND t.account_id NOT IN (" + inB + ")"
		}
		sql := "SELECT SUM(" + amountCol + ") as amount, a.currency_id FROM transactions t LEFT JOIN accounts a ON " + joinCol + " = a.id WHERE t.amount = t.amount_recipient AND " + where + " AND t.type = 2 AND t.spent_at >= " + dStart + " AND t.spent_at < " + dEnd + " GROUP BY a.currency_id"
		args := make([]any, 0, 2*n+2)
		args = append(args, ids...)
		args = append(args, ids...)
		args = append(args, start, end)
		return sql, args
	}
	// sqlite: positional. two IN lists then start,end.
	inA := r.ph(1, len(ids))
	inB := r.ph(1, len(ids))
	var where string
	if toHoldings {
		where = "t.account_recipient_id NOT IN (" + inA + ") AND t.account_id IN (" + inB + ")"
	} else {
		where = "t.account_recipient_id IN (" + inA + ") AND t.account_id NOT IN (" + inB + ")"
	}
	sql := "SELECT SUM(" + amountCol + ") as amount, a.currency_id FROM transactions t LEFT JOIN accounts a ON " + joinCol + " = a.id WHERE t.amount = t.amount_recipient AND " + where + " AND t.type = 2 AND t.spent_at >= ? AND t.spent_at < ? GROUP BY a.currency_id"
	args := make([]any, 0, 2*len(ids)+2)
	args = append(args, ids...)
	args = append(args, ids...)
	args = append(args, start, end)
	return sql, args
}

// CountSpending implements ReadModel.
func (r *ReadRepo) CountSpending(ctx context.Context, categoryIDs, accountIDs []vo.Id, start, end time.Time) ([]appbudget.SpendingRow, error) {
	if len(categoryIDs) == 0 {
		return nil, nil
	}
	catArgs := idArgs(categoryIDs)
	accArgs := idArgs(accountIDs)
	var sql string
	var args []any
	if r.driver == "postgresql" {
		accIn := r.ph(1, len(accArgs))
		catIn := r.ph(1+len(accArgs), len(catArgs))
		dStart := "$" + itoa(1+len(accArgs)+len(catArgs))
		dEnd := "$" + itoa(2+len(accArgs)+len(catArgs))
		sql = "SELECT SUM(t.amount) as amount, t.category_id, t.tag_id, a.currency_id FROM transactions t LEFT JOIN accounts a ON t.account_id = a.id AND a.id IN (" + accIn + ") WHERE t.category_id IN (" + catIn + ") AND t.spent_at >= " + dStart + " AND t.spent_at < " + dEnd + " GROUP BY t.category_id, t.tag_id, a.currency_id"
		args = append(args, accArgs...)
		args = append(args, catArgs...)
		args = append(args, start, end)
	} else {
		accIn := r.ph(1, len(accArgs))
		catIn := r.ph(1, len(catArgs))
		sql = "SELECT SUM(t.amount) as amount, t.category_id, t.tag_id, a.currency_id FROM transactions t LEFT JOIN accounts a ON t.account_id = a.id AND a.id IN (" + accIn + ") WHERE t.category_id IN (" + catIn + ") AND t.spent_at >= ? AND t.spent_at < ? GROUP BY t.category_id, t.tag_id, a.currency_id"
		args = append(args, accArgs...)
		args = append(args, catArgs...)
		args = append(args, start, end)
	}
	rows, err := r.db(ctx).QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []appbudget.SpendingRow
	for rows.Next() {
		var categoryID, currencyID, tagID *string
		// SQLite's SUM(amount) is a float; scan it as float and format with %.8f
		// to match PHP's DecimalNumber float path (round to 8 decimals) instead of
		// the driver's full-precision text (e.g. "32.26999999"). PostgreSQL's SUM
		// is exact NUMERIC, scanned as text and passed through.
		a := "0"
		if r.driver == "postgresql" {
			var amount *string
			if err := rows.Scan(&amount, &categoryID, &tagID, &currencyID); err != nil {
				return nil, err
			}
			if currencyID == nil || categoryID == nil {
				continue
			}
			if amount != nil {
				a = *amount
			}
		} else {
			var amount *float64
			if err := rows.Scan(&amount, &categoryID, &tagID, &currencyID); err != nil {
				return nil, err
			}
			if currencyID == nil || categoryID == nil {
				continue
			}
			if amount != nil {
				a = strconv.FormatFloat(*amount, 'f', 8, 64)
			}
		}
		out = append(out, appbudget.SpendingRow{CategoryID: *categoryID, TagID: tagID, CurrencyID: *currencyID, Amount: a})
	}
	return out, rows.Err()
}

// SummarizedLimits implements ReadModel.
func (r *ReadRepo) SummarizedLimits(ctx context.Context, budgetID vo.Id, start, end time.Time) ([]appbudget.SummarizedLimitRow, error) {
	var sql string
	if r.driver == "postgresql" {
		sql = "SELECT e.external_id, e.type, SUM(l.amount) as amount FROM budgets_elements_limits l JOIN budgets_elements e ON e.id = l.element_id WHERE e.budget_id = $1 AND l.period >= $2 AND l.period < $3 GROUP BY e.external_id, e.type"
	} else {
		sql = "SELECT e.external_id, e.type, SUM(l.amount) as amount FROM budgets_elements_limits l JOIN budgets_elements e ON e.id = l.element_id WHERE e.budget_id = ? AND l.period >= ? AND l.period < ? GROUP BY e.external_id, e.type"
	}
	rows, err := r.db(ctx).QueryContext(ctx, sql, budgetID.String(), start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []appbudget.SummarizedLimitRow
	for rows.Next() {
		var row appbudget.SummarizedLimitRow
		var amount *string
		if err := rows.Scan(&row.ExternalID, &row.Type, &amount); err != nil {
			return nil, err
		}
		if amount != nil {
			row.Amount = *amount
		} else {
			row.Amount = "0"
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// budgetTxCols is the column list for the budget transaction list (account
// currency joined in).
const budgetTxCols = "t.id, t.user_id, a.currency_id, t.amount, t.description, t.spent_at, t.category_id, t.payee_id, t.tag_id"

// scanBudgetTxRows scans budget transaction rows.
func scanBudgetTxRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]appbudget.BudgetTransactionRow, error) {
	var out []appbudget.BudgetTransactionRow
	for rows.Next() {
		var r appbudget.BudgetTransactionRow
		var currencyID, desc *string
		if err := rows.Scan(&r.ID, &r.UserID, &currencyID, &r.Amount, &desc, &r.SpentAt, &r.CategoryID, &r.PayeeID, &r.TagID); err != nil {
			return nil, err
		}
		if currencyID != nil {
			r.CurrencyID = *currencyID
		}
		if desc != nil {
			r.Description = *desc
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// BudgetTransactionsByCategories implements ReadModel.
func (r *ReadRepo) BudgetTransactionsByCategories(ctx context.Context, categoryIDs, accountIDs []vo.Id, start, end time.Time) ([]appbudget.BudgetTransactionRow, error) {
	if len(categoryIDs) == 0 || len(accountIDs) == 0 {
		return nil, nil
	}
	accArgs := idArgs(accountIDs)
	catArgs := idArgs(categoryIDs)
	var sql string
	args := make([]any, 0, len(accArgs)+len(catArgs)+2)
	if r.driver == "postgresql" {
		accIn := r.ph(1, len(accArgs))
		catIn := r.ph(1+len(accArgs), len(catArgs))
		dStart := "$" + itoa(1+len(accArgs)+len(catArgs))
		dEnd := "$" + itoa(2+len(accArgs)+len(catArgs))
		sql = "SELECT " + budgetTxCols + " FROM transactions t JOIN accounts a ON a.id = t.account_id WHERE t.account_id IN (" + accIn + ") AND t.category_id IN (" + catIn + ") AND t.type = 0 AND t.tag_id IS NULL AND t.spent_at >= " + dStart + " AND t.spent_at < " + dEnd + " ORDER BY t.spent_at DESC"
	} else {
		accIn := r.ph(1, len(accArgs))
		catIn := r.ph(1, len(catArgs))
		sql = "SELECT " + budgetTxCols + " FROM transactions t JOIN accounts a ON a.id = t.account_id WHERE t.account_id IN (" + accIn + ") AND t.category_id IN (" + catIn + ") AND t.type = 0 AND t.tag_id IS NULL AND t.spent_at >= ? AND t.spent_at < ? ORDER BY t.spent_at DESC"
	}
	args = append(args, accArgs...)
	args = append(args, catArgs...)
	args = append(args, start, end)
	rows, err := r.db(ctx).QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBudgetTxRows(rows)
}

// BudgetTransactionsByTag implements ReadModel.
func (r *ReadRepo) BudgetTransactionsByTag(ctx context.Context, tagID vo.Id, categoryID *vo.Id, accountIDs []vo.Id, start, end time.Time) ([]appbudget.BudgetTransactionRow, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	accArgs := idArgs(accountIDs)
	var sql string
	var args []any
	if r.driver == "postgresql" {
		accIn := r.ph(1, len(accArgs))
		next := 1 + len(accArgs)
		tagP := "$" + itoa(next)
		next++
		where := "t.account_id IN (" + accIn + ") AND t.tag_id = " + tagP + " AND t.type = 0"
		args = append(args, accArgs...)
		args = append(args, tagID.String())
		if categoryID != nil {
			where += " AND t.category_id = $" + itoa(next)
			next++
			args = append(args, categoryID.String())
		}
		where += " AND t.spent_at >= $" + itoa(next) + " AND t.spent_at < $" + itoa(next+1)
		args = append(args, start, end)
		sql = "SELECT " + budgetTxCols + " FROM transactions t JOIN accounts a ON a.id = t.account_id WHERE " + where + " ORDER BY t.spent_at DESC"
	} else {
		accIn := r.ph(1, len(accArgs))
		where := "t.account_id IN (" + accIn + ") AND t.tag_id = ? AND t.type = 0"
		args = append(args, accArgs...)
		args = append(args, tagID.String())
		if categoryID != nil {
			where += " AND t.category_id = ?"
			args = append(args, categoryID.String())
		}
		where += " AND t.spent_at >= ? AND t.spent_at < ?"
		args = append(args, start, end)
		sql = "SELECT " + budgetTxCols + " FROM transactions t JOIN accounts a ON a.id = t.account_id WHERE " + where + " ORDER BY t.spent_at DESC"
	}
	rows, err := r.db(ctx).QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBudgetTxRows(rows)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
