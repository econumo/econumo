package debt_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestSync_SplitsNewPayments(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	e.clock.now = ymd(2026, 7, 1)
	a := e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	b := e.expense(t, e.account, ymd(2026, 4, 5), "500", "")
	e.sync(t)

	got := e.splits(t, res.Id)
	if len(got) != 2 {
		t.Fatalf("splits = %v", got)
	}
	want := vo.NewDecimal("10000").Mul(vo.NewDecimal(res.PeriodRate)).Round(2)
	if !vo.NewDecimal(got[a]).Equals(want) {
		t.Errorf("first payment interest = %s, want %s (balance x rate)", got[a], want)
	}
	if got[b] != "0" {
		t.Errorf("a second payment in the same window takes no interest, got %s", got[b])
	}

	// A second sync changes nothing.
	e.sync(t)
	if again := e.splits(t, res.Id); again[a] != got[a] || again[b] != got[b] {
		t.Fatalf("resync changed splits: %v -> %v", got, again)
	}
}

func TestSync_BackdatedPaymentKeepsLaterSplits(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	e.clock.now = ymd(2026, 7, 1)
	b := e.expense(t, e.account, ymd(2026, 5, 1), "1000", "")
	c := e.expense(t, e.account, ymd(2026, 6, 1), "1000", "")
	e.sync(t)
	before := e.splits(t, res.Id)

	early := e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	e.sync(t)
	after := e.splits(t, res.Id)
	if after[b] != before[b] || after[c] != before[c] {
		t.Fatalf("later splits moved: %v -> %v", before, after)
	}
	if v := vo.NewDecimal(after[early]); !v.IsGreaterThan(vo.NewDecimal("0")) {
		t.Fatalf("backdated payment interest = %s, want > 0", after[early])
	}
}

func TestSync_ConvertsForeignCurrencyPayment(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	eur := e.f.Currency(fixture.Currency{Code: "EUR", Symbol: "E"})
	e.rates.multiplier[eur] = 2
	eurAccount := e.f.Account(fixture.Account{UserID: e.user, CurrencyID: eur})
	e.clock.now = ymd(2026, 7, 1)
	tx := e.expense(t, eurAccount, ymd(2026, 4, 1), "500", "")

	got, err := e.svc.GetDebt(context.Background(), e.uid(), model.GetDebtRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	if e.rates.converted == 0 {
		t.Fatal("the EUR payment was not converted")
	}
	if len(got.Payments) != 1 || got.Payments[0].TransactionId != tx || got.Payments[0].Amount != "1000" {
		t.Fatalf("payments = %+v, want the payment as 1000 in the debt's currency", got.Payments)
	}
	p := got.Payments[0]
	if sum := vo.NewDecimal(p.Interest).Add(vo.NewDecimal(p.Principal)); !sum.Equals(vo.NewDecimal("1000")) {
		t.Fatalf("interest %s + principal %s != 1000", p.Interest, p.Principal)
	}
	if p.Interest == "0" {
		t.Fatalf("the converted payment should take the interest due: %+v", p)
	}
}

func TestSync_UnmatchedPaymentLosesSplit(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	e.clock.now = ymd(2026, 7, 1)
	tx := e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	e.sync(t)
	if got := e.splits(t, res.Id); len(got) != 1 {
		t.Fatalf("splits = %v", got)
	}

	other := e.newCategory(e.user, true)
	if _, err := e.db.Raw.ExecContext(context.Background(), e.db.Rebind("UPDATE transactions SET category_id = ? WHERE id = ?"), other, tx); err != nil {
		t.Fatal(err)
	}
	e.sync(t)
	if got := e.splits(t, res.Id); len(got) != 0 {
		t.Fatalf("the re-categorised payment kept its split: %v", got)
	}
}

func TestSync_PartitionByStartDate(t *testing.T) {
	e := newEnv(t)
	oldLoan := e.create(t, nil)
	newLoan := e.create(t, func(r *model.CreateDebtRequest) { r.StartDate = ptr("2026-01-01") })
	e.clock.now = ymd(2026, 7, 1)
	inOld := e.expense(t, e.account, ymd(2025, 12, 20), "1000", "")
	inNew := e.expense(t, e.account, ymd(2026, 2, 1), "1000", "")
	e.sync(t)

	o, n := e.splits(t, oldLoan.Id), e.splits(t, newLoan.Id)
	if len(o) != 1 || o[inOld] == "" || len(n) != 1 || n[inNew] == "" {
		t.Fatalf("old %v new %v", o, n)
	}
}

func TestSync_TagNarrowsMatching(t *testing.T) {
	e := newEnv(t)
	tag := e.newTag()
	res := e.create(t, func(r *model.CreateDebtRequest) { r.TagId = &tag })
	e.clock.now = ymd(2026, 7, 1)
	e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	e.sync(t)
	if got := e.splits(t, res.Id); len(got) != 0 {
		t.Fatalf("an untagged payment matched a tagged debt: %v", got)
	}
}

func TestSync_EditedAmountKeepsStoredInterest(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	e.clock.now = ymd(2026, 7, 1)
	a := e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	b := e.expense(t, e.account, ymd(2026, 5, 1), "1000", "")
	e.sync(t)
	stored := e.splits(t, res.Id)

	if _, err := e.db.Raw.ExecContext(context.Background(), e.db.Rebind("UPDATE transactions SET amount = ? WHERE id = ?"), "3000", a); err != nil {
		t.Fatal(err)
	}
	c := e.expense(t, e.account, ymd(2026, 6, 1), "1000", "")
	e.sync(t)
	got := e.splits(t, res.Id)
	if got[a] != stored[a] || got[b] != stored[b] {
		t.Fatalf("stored splits moved: %v -> %v", stored, got)
	}

	// The new payment's balance follows the stored interests, not a recomputation.
	balance := vo.NewDecimal("10000").
		Sub(vo.NewDecimal("3000").Sub(vo.NewDecimal(stored[a]))).
		Sub(vo.NewDecimal("1000").Sub(vo.NewDecimal(stored[b])))
	want := balance.Mul(vo.NewDecimal(res.PeriodRate)).Round(2)
	if diff := vo.NewDecimal(got[c]).Sub(want).Abs(); diff.IsGreaterThan(vo.NewDecimal("0.01")) {
		t.Fatalf("interest of the new payment = %s, want about %s", got[c], want)
	}
}
