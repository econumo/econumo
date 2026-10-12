package repo_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
)

func revisionOf(t *testing.T, db *dbtest.DB, accountID string) int64 {
	t.Helper()
	var rev int64
	if err := db.Raw.QueryRow(db.Rebind(`SELECT transactions_revision FROM accounts WHERE id = ?`), accountID).Scan(&rev); err != nil {
		t.Fatalf("read transactions_revision: %v", err)
	}
	return rev
}

// Several writes to the same rows inside one DB transaction move each account
// by exactly one, including the account a re-saved row left.
func TestTransactionRepo_WritesBumpEachAccountOncePerDBTransaction(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	const id = "7c000000-0000-0000-0000-0000000000b1"
	if err := repo.Save(ctx, expense(id, acct1, "10", fixedTime)); err != nil {
		t.Fatal(err)
	}
	if got := revisionOf(t, db, acct1); got != 1 {
		t.Fatalf("a write outside a transaction must apply at once: acct1 = %d, want 1", got)
	}

	if err := db.TX.WithTx(ctx, func(ctx context.Context) error {
		if err := repo.Save(ctx, expense(id, acct2, "11", fixedTime)); err != nil {
			return err
		}
		return repo.ReplaceLabels(ctx, vo.MustParseId(id), nil)
	}); err != nil {
		t.Fatal(err)
	}
	if a1, a2 := revisionOf(t, db, acct1), revisionOf(t, db, acct2); a1 != 2 || a2 != 1 {
		t.Fatalf("after moving the row: acct1 = %d, acct2 = %d, want 2 and 1", a1, a2)
	}

	if err := repo.Delete(ctx, vo.MustParseId(id)); err != nil {
		t.Fatal(err)
	}
	if a1, a2 := revisionOf(t, db, acct1), revisionOf(t, db, acct2); a1 != 2 || a2 != 2 {
		t.Fatalf("after delete: acct1 = %d, acct2 = %d, want 2 and 2", a1, a2)
	}
}
