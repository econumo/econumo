// Package txorder places transactions within their day. Each account side
// carries its own fractional key (internal/shared/sortkey): transactions.sort_key
// on the source account, recipient_sort_key on the recipient. A higher key is
// listed first, so a new row lands at the top of its day with a key just above
// the highest either column holds for that account on that day.
package txorder

import (
	"context"
	"database/sql"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/txrevision"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/sortkey"
)

type Placer struct {
	driver string
	tm     *backend.TxManager
	rev    *txrevision.Bumper
}

func New(driver string, tm *backend.TxManager) *Placer {
	return &Placer{driver: driver, tm: tm, rev: txrevision.New(driver, tm)}
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
func (p *Placer) Top(ctx context.Context, accountID string, spentAt time.Time) (string, error) {
	start, end := DayBounds(spentAt)
	from, to := start.Format(datetime.Layout), end.Format(datetime.Layout)
	q := `SELECT COALESCE(MAX(k), '') FROM (
		SELECT sort_key AS k FROM transactions WHERE account_id = ? AND spent_at >= ? AND spent_at < ?
		UNION ALL
		SELECT recipient_sort_key AS k FROM transactions WHERE account_recipient_id = ? AND spent_at >= ? AND spent_at < ?
	) keys`
	var top string
	if err := p.tm.Querier(ctx).QueryRowContext(ctx, p.rebind(q), accountID, from, to, accountID, from, to).Scan(&top); err != nil {
		return "", err
	}
	key, err := sortkey.Between(sortkey.Key(top), "")
	return string(key), err
}

// SeedMissing places every row that has no key on a side yet: rows written
// before the keys existed (the migration step) or by an interrupted run. Each
// account is seeded in its own transaction, so a failure leaves whole accounts
// done or untouched and a rerun picks up where it stopped. Within a day the
// unplaced rows go above the placed ones, in the order the lists used before
// keys existed (spent_at DESC, id). Returns how many accounts it touched.
func (p *Placer) SeedMissing(ctx context.Context) (int, error) {
	accounts, err := p.unplacedAccounts(ctx)
	if err != nil {
		return 0, err
	}
	for _, accountID := range accounts {
		if err := p.tm.WithTx(ctx, func(ctx context.Context) error { return p.seedAccount(ctx, accountID) }); err != nil {
			return 0, err
		}
	}
	return len(accounts), nil
}

func (p *Placer) unplacedAccounts(ctx context.Context) ([]string, error) {
	rows, err := p.tm.Querier(ctx).QueryContext(ctx, `SELECT account_id FROM transactions WHERE sort_key = ''
		UNION SELECT account_recipient_id FROM transactions WHERE account_recipient_id IS NOT NULL AND recipient_sort_key IS NULL
		ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

type sideRow struct {
	id        string
	recipient bool
	spentAt   time.Time
	key       string
}

func (p *Placer) seedAccount(ctx context.Context, accountID string) error {
	q := `SELECT id, 0, spent_at, sort_key FROM transactions WHERE account_id = ?
		UNION ALL
		SELECT id, 1, spent_at, recipient_sort_key FROM transactions WHERE account_recipient_id = ?`
	rows, err := p.tm.Querier(ctx).QueryContext(ctx, p.rebind(q), accountID, accountID)
	if err != nil {
		return err
	}
	days := map[string][]sideRow{}
	for rows.Next() {
		var r sideRow
		var side int
		var key sql.NullString
		if err := rows.Scan(&r.id, &side, &r.spentAt, &key); err != nil {
			rows.Close()
			return err
		}
		r.recipient, r.key = side == 1, key.String
		day := r.spentAt.Format(datetime.DateLayout)
		days[day] = append(days[day], r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, list := range days {
		top := ""
		var unplaced []sideRow
		for _, r := range list {
			if r.key == "" {
				unplaced = append(unplaced, r)
			} else if r.key > top {
				top = r.key
			}
		}
		// bottom first, so each next key goes above the previous one
		sort.Slice(unplaced, func(i, j int) bool {
			if !unplaced[i].spentAt.Equal(unplaced[j].spentAt) {
				return unplaced[i].spentAt.Before(unplaced[j].spentAt)
			}
			return unplaced[i].id > unplaced[j].id
		})
		for _, r := range unplaced {
			key, err := sortkey.Between(sortkey.Key(top), "")
			if err != nil {
				return err
			}
			top = string(key)
			col := "sort_key"
			if r.recipient {
				col = "recipient_sort_key"
			}
			if _, err := p.tm.Querier(ctx).ExecContext(ctx, p.rebind("UPDATE transactions SET "+col+" = ? WHERE id = ?"), top, r.id); err != nil {
				return err
			}
		}
	}
	return p.rev.Bump(ctx, accountID)
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
