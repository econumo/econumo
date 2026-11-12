//go:build enginecompare

package repo_test

import (
	"context"
	"testing"

	categoryrepo "github.com/econumo/econumo/internal/category/repo"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestCategoryLockRow_WaitsForAKeyShareHolder(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	id := f.Category(fixture.Category{UserID: f.User(fixture.User{Name: "Owner"})})
	dbtest.AssertLockWaitsForKeyShare(t, db, "categories", id, func(ctx context.Context, tx *backend.TxManager) error {
		return categoryrepo.NewRepo(db.Engine, tx).LockRow(ctx, vo.MustParseId(id))
	})
}
