package fixture

import (
	"context"
)

// Revisions reads accounts.transactions_revision for each account id.
func (b *Builder) Revisions(accountIDs ...string) map[string]int64 {
	b.t.Helper()
	out := make(map[string]int64, len(accountIDs))
	for _, id := range accountIDs {
		var rev int64
		row := b.db.Raw.QueryRowContext(context.Background(), rebind(b.db.Engine, `SELECT transactions_revision FROM accounts WHERE id = ?`), id)
		if err := row.Scan(&rev); err != nil {
			b.t.Fatalf("read transactions_revision of %s: %v", id, err)
		}
		out[id] = rev
	}
	return out
}

// RequireRevisionsMoved asserts that, against the before snapshot, each account
// in moved gained exactly one and every other account is unchanged.
func (b *Builder) RequireRevisionsMoved(before map[string]int64, moved ...string) {
	b.t.Helper()
	isMoved := make(map[string]bool, len(moved))
	for _, id := range moved {
		isMoved[id] = true
	}
	ids := make([]string, 0, len(before))
	for id := range before {
		ids = append(ids, id)
	}
	after := b.Revisions(ids...)
	for id, was := range before {
		want := was
		if isMoved[id] {
			want++
		}
		if after[id] != want {
			b.t.Errorf("account %s: transactions_revision = %d, want %d (before %d, moved=%v)", id, after[id], want, was, isMoved[id])
		}
	}
}
