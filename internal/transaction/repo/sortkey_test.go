package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

func TestSortKeysRoundTrip(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	id := "7c000000-0000-0000-0000-0000000005a1"
	tx := expense(id, acct1, "1", time.Date(2024, 3, 15, 9, 30, 0, 0, time.UTC))
	tx.SortKey = 7
	if err := repo.Save(ctx, tx); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByID(ctx, vo.MustParseId(id))
	if err != nil {
		t.Fatal(err)
	}
	if got.SortKey != 7 || got.RecipientSortKey != nil {
		t.Errorf("keys = %d %v, want 7 <nil>", got.SortKey, got.RecipientSortKey)
	}
	list, err := repo.ListByAccount(ctx, vo.MustParseId(acct1))
	if err != nil || len(list) != 1 || list[0].SortKey != 7 {
		t.Fatalf("ListByAccount: %v %+v", err, list)
	}
}
