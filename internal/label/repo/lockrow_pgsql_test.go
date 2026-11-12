//go:build enginecompare

package repo_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	labelrepo "github.com/econumo/econumo/internal/label/repo"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestLabelLockRow_WaitsForAKeyShareHolder(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	id := f.Label(fixture.Label{UserID: f.User(fixture.User{Name: "Owner"})})
	dbtest.AssertLockWaitsForKeyShare(t, db, "labels", id, func(ctx context.Context, tx *backend.TxManager) error {
		return labelrepo.NewRepo(db.Engine, tx).LockRow(ctx, vo.MustParseId(id))
	})
}
