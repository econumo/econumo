package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	transactionrepo "github.com/econumo/econumo/internal/transaction/repo"
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

func transfer(id, from, to string, spentAt time.Time) *model.Transaction {
	recip := vo.MustParseId(to)
	return model.FromState(model.NewState{
		ID: vo.MustParseId(id), UserID: vo.MustParseId(userA), Type: model.TransactionTypeTransfer,
		AccountID: vo.MustParseId(from), AccountRecipID: &recip, Amount: "1",
		SpentAt: spentAt, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	})
}

func keysOf(t *testing.T, repo *transactionrepo.Repo, id string) (int64, *int64) {
	t.Helper()
	got, err := repo.GetByID(context.Background(), vo.MustParseId(id))
	if err != nil {
		t.Fatal(err)
	}
	return got.SortKey, got.RecipientSortKey
}

func TestSaveNewGoesToTopOfDay(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	day := time.Date(2024, 3, 15, 9, 30, 0, 0, time.UTC)
	first, second, tr := "7c000000-0000-0000-0000-0000000005b1", "7c000000-0000-0000-0000-0000000005b2", "7c000000-0000-0000-0000-0000000005b3"
	for _, tx := range []*model.Transaction{
		expense(first, acct1, "1", day),
		expense(second, acct1, "1", day.Add(-time.Hour)),
		transfer(tr, acct2, acct1, day.Add(time.Hour)),
	} {
		if err := repo.Save(ctx, tx); err != nil {
			t.Fatal(err)
		}
	}
	if k, _ := keysOf(t, repo, first); k != 1 {
		t.Errorf("first: %d, want 1", k)
	}
	if k, _ := keysOf(t, repo, second); k != 2 {
		t.Errorf("second: %d, want 2 (newest entry on top, whatever its time)", k)
	}
	k, rk := keysOf(t, repo, tr)
	if k != 1 || rk == nil || *rk != 3 {
		t.Errorf("transfer: %d %v, want 1 on acct2 and 3 on acct1", k, rk)
	}
}

func TestSaveKeepsKeysOnPlainEdit(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	day := time.Date(2024, 3, 15, 9, 30, 0, 0, time.UTC)
	a, b := "7c000000-0000-0000-0000-0000000005c1", "7c000000-0000-0000-0000-0000000005c2"
	for _, tx := range []*model.Transaction{expense(a, acct1, "1", day), expense(b, acct1, "1", day)} {
		if err := repo.Save(ctx, tx); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.GetByID(ctx, vo.MustParseId(a))
	if err != nil {
		t.Fatal(err)
	}
	st := model.NewState{Type: got.Type, AccountID: got.AccountID, Amount: "2", Description: "edited", SpentAt: got.SpentAt.Add(time.Minute)}
	got.Update(st, fixedTime)
	if err := repo.Save(ctx, got); err != nil {
		t.Fatal(err)
	}
	if k, _ := keysOf(t, repo, a); k != 1 {
		t.Errorf("edited row moved: key %d, want 1", k)
	}
}
