package repo_test

import (
	"context"
	"fmt"
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
	tx.SortKey = "a7"
	if err := repo.Save(ctx, tx); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetByID(ctx, vo.MustParseId(id))
	if err != nil {
		t.Fatal(err)
	}
	if got.SortKey != "a7" || got.RecipientSortKey != nil {
		t.Errorf("keys = %q %v, want a7 <nil>", got.SortKey, got.RecipientSortKey)
	}
	list, err := repo.ListByAccount(ctx, vo.MustParseId(acct1))
	if err != nil || len(list) != 1 || list[0].SortKey != "a7" {
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

func keysOf(t *testing.T, repo *transactionrepo.Repo, id string) (string, *string) {
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
	if k, _ := keysOf(t, repo, first); k != "a0" {
		t.Errorf("first: %q, want a0", k)
	}
	if k, _ := keysOf(t, repo, second); k != "a1" {
		t.Errorf("second: %q, want a1 (newest entry on top, whatever its time)", k)
	}
	k, rk := keysOf(t, repo, tr)
	if k != "a0" || rk == nil || *rk != "a2" {
		t.Errorf("transfer: %q %v, want a0 on acct2 and a2 on acct1", k, rk)
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
	if k, _ := keysOf(t, repo, a); k != "a0" {
		t.Errorf("edited row moved: key %q, want a0", k)
	}
}

func TestListByAccountOrdersByDayThenKey(t *testing.T) {
	repo, _ := setup(t)
	ctx := context.Background()
	day := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	early, late, tr, older := "7c000000-0000-0000-0000-0000000005d1", "7c000000-0000-0000-0000-0000000005d2", "7c000000-0000-0000-0000-0000000005d3", "7c000000-0000-0000-0000-0000000005d4"
	rk := "a1"
	rows := []*model.Transaction{
		expense(early, acct1, "1", day.Add(8*time.Hour)),
		expense(late, acct1, "1", day.Add(20*time.Hour)),
		transfer(tr, acct2, acct1, day.Add(23*time.Hour)),
		expense(older, acct1, "1", day.Add(-time.Hour)),
	}
	rows[0].SortKey, rows[1].SortKey, rows[3].SortKey = "a3", "a2", "a9"
	rows[2].SortKey, rows[2].RecipientSortKey = "a5", &rk
	for _, tx := range rows {
		if err := repo.Save(ctx, tx); err != nil {
			t.Fatal(err)
		}
	}
	ids := func(list []*model.Transaction) []string {
		out := make([]string, len(list))
		for i, tx := range list {
			out[i] = tx.ID.String()
		}
		return out
	}
	want := []string{early, late, tr, older}
	got, err := repo.ListByAccount(ctx, vo.MustParseId(acct1))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(ids(got)) != fmt.Sprint(want) {
		t.Errorf("ListByAccount = %v, want %v (day, then this account's key)", ids(got), want)
	}
	got, err = repo.ListByAccountIDs(ctx, []vo.Id{vo.MustParseId(acct1)}, model.TransactionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(ids(got)) != fmt.Sprint(want) {
		t.Errorf("ListByAccountIDs = %v, want %v", ids(got), want)
	}
}
