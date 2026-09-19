package repo_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/shared/vo"
)

// LockRow on a missing user must succeed silently: see user.Repository.LockRow's
// doc — ConfirmEmail's anti-enumeration invalid-code and the mints' fence both
// depend on this returning nil rather than an error. Guards the contract across
// the PostgreSQL switch from a no-op UPDATE to SELECT ... FOR UPDATE, on both
// engines (DBTEST_ENGINE selects; run with -tags enginecompare for pgsql).
func TestLockRow_MissingUserSucceedsSilently(t *testing.T) {
	repo, _, db := newRepos(t)
	ctx := context.Background()

	if err := db.TX.WithTx(ctx, func(ctx context.Context) error {
		return repo.LockRow(ctx, vo.NewId())
	}); err != nil {
		t.Fatalf("LockRow on a missing user: %v", err)
	}
}
