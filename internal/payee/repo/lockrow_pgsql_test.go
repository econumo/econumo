//go:build enginecompare

package repo_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	payeerepo "github.com/econumo/econumo/internal/payee/repo"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestPayeeLockRow_WaitsForAKeyShareHolder(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	id := f.Payee(fixture.Payee{UserID: f.User(fixture.User{Name: "Owner"})})
	dbtest.AssertLockWaitsForKeyShare(t, db, "payees", id, func(ctx context.Context, tx *backend.TxManager) error {
		return payeerepo.NewRepo(db.Engine, tx).LockRow(ctx, vo.MustParseId(id))
	})
}
