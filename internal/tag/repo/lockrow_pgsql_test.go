//go:build enginecompare

package repo_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/shared/vo"
	tagrepo "github.com/econumo/econumo/internal/tag/repo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestTagLockRow_WaitsForAKeyShareHolder(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	id := f.Tag(fixture.Tag{UserID: f.User(fixture.User{Name: "Owner"})})
	dbtest.AssertLockWaitsForKeyShare(t, db, "tags", id, func(ctx context.Context, tx *backend.TxManager) error {
		return tagrepo.NewRepo(db.Engine, tx).LockRow(ctx, vo.MustParseId(id))
	})
}
