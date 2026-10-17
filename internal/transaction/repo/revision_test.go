package repo_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	transactionrepo "github.com/econumo/econumo/internal/transaction/repo"
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

// Save already queued the bump of every account the row sits on, so
// ReplaceLabels in the same DB transaction does not read the row again: a row
// moved behind the repo's back in between leaves its new account untouched.
func TestTransactionRepo_ReplaceLabelsAfterSaveReusesItsBump(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	const id = "7c000000-0000-0000-0000-0000000000b2"
	if err := db.TX.WithTx(ctx, func(ctx context.Context) error {
		if err := repo.Save(ctx, expense(id, acct1, "10", fixedTime)); err != nil {
			return err
		}
		if _, err := db.TX.Querier(ctx).ExecContext(ctx, db.Rebind(`UPDATE transactions SET account_id = ? WHERE id = ?`), acct2, id); err != nil {
			return err
		}
		return repo.ReplaceLabels(ctx, vo.MustParseId(id), nil)
	}); err != nil {
		t.Fatal(err)
	}
	if a1, a2 := revisionOf(t, db, acct1), revisionOf(t, db, acct2); a1 != 1 || a2 != 0 {
		t.Fatalf("acct1 = %d, acct2 = %d, want 1 and 0", a1, a2)
	}

	// on its own, in another transaction, it bumps the row's accounts itself
	if err := repo.ReplaceLabels(ctx, vo.MustParseId(id), nil); err != nil {
		t.Fatal(err)
	}
	if got := revisionOf(t, db, acct2); got != 1 {
		t.Fatalf("acct2 = %d, want 1", got)
	}
}

// The CSV import saves rows under ids from NextIdentity, which cannot exist
// yet, so it writes without looking the row up first.
func TestImportLookup_SaveTransactionTrustsItsIdIsFresh(t *testing.T) {
	repo, db := setup(t)
	ctx := context.Background()
	const id = "7c000000-0000-0000-0000-0000000000b3"
	if err := repo.Save(ctx, expense(id, acct1, "10", fixedTime)); err != nil {
		t.Fatal(err)
	}
	lookup := transactionrepo.NewImportLookup(nil, nil, nil, nil, nil, nil, repo)
	if err := lookup.SaveTransaction(ctx, expense(id, acct2, "10", fixedTime)); err != nil {
		t.Fatal(err)
	}
	if a1, a2 := revisionOf(t, db, acct1), revisionOf(t, db, acct2); a1 != 1 || a2 != 1 {
		t.Fatalf("acct1 = %d, acct2 = %d, want 1 and 1", a1, a2)
	}
}
