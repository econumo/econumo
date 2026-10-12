// Package txrevision moves accounts.transactions_revision, the counter clients
// compare to decide which accounts' transactions to download again.
package txrevision

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/econumo/econumo/internal/infra/storage/backend"
)

type Bumper struct {
	driver string
	tm     *backend.TxManager
}

func New(driver string, tm *backend.TxManager) *Bumper {
	return &Bumper{driver: driver, tm: tm}
}

type pendingKey struct{}

type pending struct {
	mu  sync.Mutex
	ids map[string]struct{}
}

// Ids are collected for the whole DB transaction and flushed once before
// COMMIT, so a request moves each account by exactly one however many writes
// touched it; the client's "+1 means only my edit" shortcut depends on that.
func (b *Bumper) Bump(ctx context.Context, accountIDs ...string) error {
	v, inTx := b.tm.TxValue(ctx, pendingKey{}, func() any { return &pending{ids: map[string]struct{}{}} })
	if !inTx {
		return b.flush(ctx, accountIDs)
	}
	p := v.(*pending)
	p.mu.Lock()
	for _, id := range accountIDs {
		if id != "" {
			p.ids[id] = struct{}{}
		}
	}
	p.mu.Unlock()
	b.tm.BeforeCommit(ctx, pendingKey{}, func(ctx context.Context) error {
		p.mu.Lock()
		ids := make([]string, 0, len(p.ids))
		for id := range p.ids {
			ids = append(ids, id)
		}
		p.mu.Unlock()
		return b.flush(ctx, ids)
	})
	return nil
}

// cond is always a fixed string from the calling repo; values travel in args.
// It must be evaluated before the rows it selects change, since it reads them.
func (b *Bumper) BumpWhere(ctx context.Context, cond string, args ...any) error {
	q := "SELECT account_id FROM transactions WHERE " + cond +
		" UNION SELECT account_recipient_id FROM transactions WHERE (" + cond + ") AND account_recipient_id IS NOT NULL"
	all := append(append([]any{}, args...), args...)
	rows, err := b.tm.Querier(ctx).QueryContext(ctx, b.rebind(q), all...)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return b.Bump(ctx, ids...)
}

// One UPDATE per account in sorted order: a consistent row-lock order, so two
// PostgreSQL transactions bumping overlapping accounts cannot deadlock.
func (b *Bumper) flush(ctx context.Context, ids []string) error {
	ids = dedupeSorted(ids)
	q := b.rebind("UPDATE accounts SET transactions_revision = transactions_revision + 1 WHERE id = ?")
	for _, id := range ids {
		if _, err := b.tm.Querier(ctx).ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bumper) rebind(q string) string {
	if b.driver != "postgresql" {
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

func dedupeSorted(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
