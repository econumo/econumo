//go:build enginecompare

package repo_test

import (
	"context"
	"testing"

	importsrepo "github.com/econumo/econumo/internal/imports/repo"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestLockTransactionRow_WaitsForAKeyShareHolder(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	f.User(fixture.User{ID: userA, Email: "a@example.test", Name: "A"})
	f.Account(fixture.Account{ID: acct1, UserID: userA, CurrencyID: usdID, Name: "Card"})
	id := f.Transaction(fixture.Transaction{UserID: userA, AccountID: acct1, Amount: "4.75"})
	dbtest.AssertLockWaitsForKeyShare(t, db, "transactions", id, func(ctx context.Context, tx *backend.TxManager) error {
		return importsrepo.NewRepo(db.Engine, tx).LockTransactionRow(ctx, vo.MustParseId(id))
	})
}
