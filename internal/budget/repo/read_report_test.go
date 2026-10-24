package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

const acctOther = "aaaa1111-0000-0000-0000-0000000000a2"

func flowKey(r model.MonthlyFlowRow) string {
	g := ""
	if r.GroupID != nil {
		g = *r.GroupID
	}
	return r.Month + "|" + string(rune('0'+r.Type)) + "|" + g + "|" + r.CurrencyID
}

func assertFlow(t *testing.T, rows []model.MonthlyFlowRow, want map[string]string) {
	t.Helper()
	got := map[string]string{}
	for _, r := range rows {
		got[flowKey(r)] = vo.NewDecimal(r.Amount).String()
	}
	if len(got) != len(rows) {
		t.Fatalf("duplicate group keys in %v", rows)
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
}

func TestFlowByMonth_GroupsByCategory(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	cat := "c1000000-0000-0000-0000-0000000000e1"
	incCat := "c1000000-0000-0000-0000-0000000000e2"
	f.Category(fixture.Category{ID: cat, UserID: userA, Name: "Food", Type: 0})
	f.Category(fixture.Category{ID: incCat, UserID: userA, Name: "Salary", Type: 1})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000001", UserID: userA, AccountID: acctA, CategoryID: cat, Type: 0, Amount: "10.00", SpentAt: "2024-03-02 10:00:00"})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000002", UserID: userA, AccountID: acctA, CategoryID: cat, Type: 0, Amount: "5.50", SpentAt: "2024-03-20 10:00:00"})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000003", UserID: userA, AccountID: acctA, CategoryID: incCat, Type: 1, Amount: "100.00", SpentAt: "2024-04-05 10:00:00"})
	// Outside [from, to).
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000004", UserID: userA, AccountID: acctA, CategoryID: cat, Type: 0, Amount: "99.00", SpentAt: "2024-05-01 00:00:00"})

	rows, err := read.FlowByMonth(ctx, []vo.Id{vo.MustParseId(acctA)},
		time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC), "category")
	if err != nil {
		t.Fatal(err)
	}
	assertFlow(t, rows, map[string]string{
		"2024-03-01|0|" + cat + "|" + usdID:    vo.NewDecimal("15.5").String(),
		"2024-04-01|1|" + incCat + "|" + usdID: vo.NewDecimal("100").String(),
	})
}

func TestFlowByMonth_GroupsByPayeeAndTag(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	payee := "9a000000-0000-0000-0000-0000000000e1"
	tag := "7a100000-0000-0000-0000-0000000000e1"
	f.Payee(fixture.Payee{ID: payee, UserID: userA, Name: "Shop"})
	f.Tag(fixture.Tag{ID: tag, UserID: userA, Name: "Trip"})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000011", UserID: userA, AccountID: acctA, PayeeID: payee, TagID: tag, Type: 0, Amount: "7.00", SpentAt: "2024-03-02 10:00:00"})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000012", UserID: userA, AccountID: acctA, Type: 0, Amount: "3.00", SpentAt: "2024-03-03 10:00:00"})
	from := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	ids := []vo.Id{vo.MustParseId(acctA)}

	rows, err := read.FlowByMonth(ctx, ids, from, to, "payee")
	if err != nil {
		t.Fatal(err)
	}
	assertFlow(t, rows, map[string]string{
		"2024-03-01|0|" + payee + "|" + usdID: "7",
		"2024-03-01|0||" + usdID:              "3",
	})

	rows, err = read.FlowByMonth(ctx, ids, from, to, "tag")
	if err != nil {
		t.Fatal(err)
	}
	assertFlow(t, rows, map[string]string{
		"2024-03-01|0|" + tag + "|" + usdID: "7",
		"2024-03-01|0||" + usdID:            "3",
	})

	if _, err := read.FlowByMonth(ctx, ids, from, to, "bogus"); err == nil {
		t.Error("unknown groupBy must error")
	}
	if rows, err := read.FlowByMonth(ctx, nil, from, to, "category"); rows != nil || err != nil {
		t.Errorf("empty accounts = %v, %v", rows, err)
	}
}

func TestFlowByMonth_IgnoresTransfersAndOtherAccounts(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	f.Account(fixture.Account{ID: acctOther, UserID: userA, Name: "Other"})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000021", UserID: userA, AccountID: acctA, Type: 0, Amount: "4.00", SpentAt: "2024-03-02 10:00:00"})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000022", UserID: userA, AccountID: acctA, AccountRecipientID: acctOther, Type: 2, Amount: "50.00", AmountRecipient: "50.00", SpentAt: "2024-03-03 10:00:00"})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000023", UserID: userA, AccountID: acctOther, Type: 0, Amount: "80.00", SpentAt: "2024-03-04 10:00:00"})

	rows, err := read.FlowByMonth(ctx, []vo.Id{vo.MustParseId(acctA)},
		time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC), "category")
	if err != nil {
		t.Fatal(err)
	}
	assertFlow(t, rows, map[string]string{"2024-03-01|0||" + usdID: "4"})
}

func TestFirstTransactionDate(t *testing.T) {
	read, db := newReadRepo(t)
	ctx := context.Background()
	f := fixture.New(t, db)
	f.Account(fixture.Account{ID: acctOther, UserID: userA, Name: "Other"})
	ids := []vo.Id{vo.MustParseId(acctA)}

	got, err := read.FirstTransactionDate(ctx, ids)
	if err != nil || got != nil {
		t.Fatalf("no rows = %v, %v", got, err)
	}

	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000031", UserID: userA, AccountID: acctA, Type: 0, Amount: "1.00", SpentAt: "2024-03-10 08:30:00"})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000032", UserID: userA, AccountID: acctOther, AccountRecipientID: acctA, Type: 2, Amount: "5.00", AmountRecipient: "5.00", SpentAt: "2024-02-15 09:00:00"})
	f.Transaction(fixture.Transaction{ID: "7a000000-0000-0000-0000-000000000033", UserID: userA, AccountID: acctOther, Type: 0, Amount: "1.00", SpentAt: "2023-01-01 00:00:00"})

	got, err = read.FirstTransactionDate(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2024, 2, 15, 9, 0, 0, 0, time.UTC)
	if got == nil || !got.Equal(want) {
		t.Fatalf("first = %v, want %v", got, want)
	}
}
