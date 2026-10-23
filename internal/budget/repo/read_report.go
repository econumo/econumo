package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

// FlowByMonth implements ReadModel.
func (r *ReadRepo) FlowByMonth(ctx context.Context, accountIDs []vo.Id, from, to time.Time, groupBy string) ([]model.MonthlyFlowRow, error) {
	var col string
	switch groupBy {
	case "category":
		col = "t.category_id"
	case "tag":
		col = "t.tag_id"
	case "payee":
		col = "t.payee_id"
	default:
		return nil, fmt.Errorf("flow report: unknown groupBy %q", groupBy)
	}
	if len(accountIDs) == 0 {
		return nil, nil
	}
	accArgs := idArgs(accountIDs)
	month := r.planMonthExpr("t.spent_at")
	accIn := r.ph(1, len(accArgs))
	dStart, dEnd := "?", "?"
	args := append([]any{}, accArgs...)
	if r.driver == "postgresql" {
		dStart = "$" + itoa(1+len(accArgs))
		dEnd = "$" + itoa(2+len(accArgs))
		args = append(args, from, to)
	} else {
		args = append(args, sqliteDatetime(from), sqliteDatetime(to))
	}
	query := "SELECT " + month + " as month, t.type, " + col + " as group_id, a.currency_id, SUM(t.amount) as amount" +
		" FROM transactions t JOIN accounts a ON a.id = t.account_id" +
		" WHERE t.account_id IN (" + accIn + ") AND t.type IN (0, 1) AND t.spent_at >= " + dStart + " AND t.spent_at < " + dEnd +
		" GROUP BY month, t.type, " + col + ", a.currency_id"
	rows, err := r.db(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.MonthlyFlowRow
	for rows.Next() {
		var row model.MonthlyFlowRow
		a := "0"
		if r.driver == "postgresql" {
			var amount *string
			if err := rows.Scan(&row.Month, &row.Type, &row.GroupID, &row.CurrencyID, &amount); err != nil {
				return nil, err
			}
			if amount != nil {
				a = *amount
			}
		} else {
			var amount *float64
			if err := rows.Scan(&row.Month, &row.Type, &row.GroupID, &row.CurrencyID, &amount); err != nil {
				return nil, err
			}
			if amount != nil {
				a = strconv.FormatFloat(*amount, 'f', 8, 64)
			}
		}
		row.Amount = a
		out = append(out, row)
	}
	return out, rows.Err()
}

// FirstTransactionDate implements ReadModel.
func (r *ReadRepo) FirstTransactionDate(ctx context.Context, accountIDs []vo.Id) (*time.Time, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	accArgs := idArgs(accountIDs)
	n := len(accArgs)
	query := "SELECT MIN(t.spent_at) FROM transactions t WHERE t.account_id IN (" + r.ph(1, n) + ") OR t.account_recipient_id IN (" + r.ph(1+n, n) + ")"
	args := append(append([]any{}, accArgs...), accArgs...)
	var raw any
	if err := r.db(ctx).QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case time.Time:
		u := v.UTC()
		return &u, nil
	case string:
		return parseSpentAt(v)
	case []byte:
		return parseSpentAt(string(v))
	default:
		return nil, fmt.Errorf("first transaction date: unexpected %T", raw)
	}
}

func parseSpentAt(s string) (*time.Time, error) {
	t, err := time.ParseInLocation(datetime.Layout, s, time.UTC)
	if err != nil {
		return nil, fmt.Errorf("first transaction date %q: %w", s, err)
	}
	return &t, nil
}
