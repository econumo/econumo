// Package txorder places transactions within their day. Each account side
// carries its own key (transactions.sort_key on the source account,
// recipient_sort_key on the recipient), higher listed first, so a new row
// lands at the top of its day by taking one more than the highest key either
// column holds for that account on that day.
package txorder

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/shared/datetime"
)

type Placer struct {
	driver string
	tm     *backend.TxManager
}

func New(driver string, tm *backend.TxManager) *Placer {
	return &Placer{driver: driver, tm: tm}
}

// DayBounds is [00:00:00 of spentAt's day, the next day). spent_at is a wall
// clock without a zone, so the day is its calendar date as stored.
func DayBounds(spentAt time.Time) (time.Time, time.Time) {
	start := time.Date(spentAt.Year(), spentAt.Month(), spentAt.Day(), 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 0, 1)
}

// Top returns the key that puts a new row on accountID at the top of
// spentAt's day. The bounds are bound as text so the comparison never depends
// on how a driver formats a time.Time.
func (p *Placer) Top(ctx context.Context, accountID string, spentAt time.Time) (int64, error) {
	start, end := DayBounds(spentAt)
	from, to := start.Format(datetime.Layout), end.Format(datetime.Layout)
	q := `SELECT COALESCE(MAX(k), 0) + 1 FROM (
		SELECT sort_key AS k FROM transactions WHERE account_id = ? AND spent_at >= ? AND spent_at < ?
		UNION ALL
		SELECT recipient_sort_key AS k FROM transactions WHERE account_recipient_id = ? AND spent_at >= ? AND spent_at < ?
	) keys`
	var top int64
	err := p.tm.Querier(ctx).QueryRowContext(ctx, p.rebind(q), accountID, from, to, accountID, from, to).Scan(&top)
	return top, err
}

func (p *Placer) rebind(q string) string {
	if p.driver != "postgresql" {
		return q
	}
	var sb strings.Builder
	n := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			sb.WriteByte('$')
			sb.WriteString(strconv.Itoa(n))
			continue
		}
		sb.WriteByte(q[i])
	}
	return sb.String()
}
