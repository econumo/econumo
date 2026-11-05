package repo

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

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
