package debt_test

import (
	"context"
	"math"
	"strconv"
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
	// the loan began on 10 Mar: the first period accrues 22 of March's 31 days
	rate, _ := strconv.ParseFloat(res.PeriodRate, 64)
	want := vo.NewDecimal(strconv.FormatFloat(math.Round(10000*rate*22/31*100)/100, 'f', 2, 64))
	if !vo.NewDecimal(got[a]).Equals(want) {
		t.Errorf("first payment interest = %s, want %s (balance x rate, prorated)", got[a], want)
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
	e.fakeBudget(e.budget).accounts = append(e.fakeBudget(e.budget).accounts, eurAccount)
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
	oldLoan := e.create(t, func(r *model.CreateDebtRequest) { r.StartDate = ptr("2025-12-01") })
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
	res := e.create(t, func(r *model.CreateDebtRequest) { r.Rules[0].TagId = &tag })
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

func TestSync_StoredInterestCappedAtCorrectedAmount(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.create(t, nil)
	e.clock.now = ymd(2026, 7, 1)
	a := e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	e.sync(t)
	if stored := vo.NewDecimal(e.splits(t, res.Id)[a]); !stored.IsGreaterThan(vo.NewDecimal("100")) {
		t.Fatalf("stored interest = %s, want above 100", stored)
	}

	if _, err := e.db.Raw.ExecContext(ctx, e.db.Rebind("UPDATE transactions SET amount = ? WHERE id = ?"), "100", a); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.GetDebt(ctx, e.uid(), model.GetDebtRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	p := got.Payments[0]
	if p.Amount != "100" || p.Interest != "100" || p.Principal != "0" {
		t.Fatalf("payment = %+v, want the interest capped at the amount", p)
	}
	if got.Item.OverpaidSoFar != "100" || got.Item.CurrentBalance != "10000" {
		t.Fatalf("item = %+v, want overpaid 100 and the balance not risen", got.Item)
	}
}

func TestSync_FutureDatedSplitIsProvisional(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.create(t, nil)
	tx := e.expense(t, e.account, ymd(2026, 6, 1), "1000", "")
	e.sync(t)
	first := e.splits(t, res.Id)
	if first[tx] == "" {
		t.Fatalf("a future-dated payment keeps a provisional split row, got %v", first)
	}

	e.clock.now = ymd(2026, 3, 20)
	req := e.updateReq(res)
	req.Balance = vo.NewFlexString("9000")
	updated, err := e.svc.UpdateDebt(ctx, e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	e.sync(t)
	want := vo.NewDecimal("9000").Mul(vo.NewDecimal(updated.PeriodRate)).Round(2)
	got := e.splits(t, res.Id)[tx]
	if diff := vo.NewDecimal(got).Sub(want).Abs(); diff.IsGreaterThan(vo.NewDecimal("0.01")) || got == first[tx] {
		t.Fatalf("interest = %s (first %s), want about %s under the new snapshot", got, first[tx], want)
	}

	// Once the day arrives the split is final.
	e.clock.now = ymd(2026, 6, 2)
	e.sync(t)
	final := e.splits(t, res.Id)[tx]
	req = e.updateReq(updated)
	req.Balance = vo.NewFlexString("5000")
	if _, err := e.svc.UpdateDebt(ctx, e.uid(), req); err != nil {
		t.Fatal(err)
	}
	e.sync(t)
	if after := e.splits(t, res.Id)[tx]; after != final || final != got {
		t.Fatalf("split moved after its date: %s -> %s -> %s", got, final, after)
	}
}

// A shared mortgage paid from his category and from hers, on their own
// accounts: one debt counts both.
func TestSync_PaymentsFromEveryRule(t *testing.T) {
	e := newEnv(t)
	sam := e.partner(t, "Sam", roleUser)
	hers := e.newCategory(sam, true)
	herAccount := e.f.Account(fixture.Account{UserID: sam})
	e.fakeBudget(e.budget).accounts = append(e.fakeBudget(e.budget).accounts, herAccount)
	res := e.create(t, func(r *model.CreateDebtRequest) { r.Rules = []model.DebtRuleRequest{rule(e.category), rule(hers)} })
	e.clock.now = ymd(2026, 7, 1)
	his := e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	her := e.f.Transaction(fixture.Transaction{UserID: sam, AccountID: herAccount, CategoryID: hers, Type: 0, Amount: "1000", SpentAt: ymd(2026, 5, 1)})
	e.sync(t)
	got := e.splits(t, res.Id)
	if _, ok := got[his]; !ok || len(got) != 2 {
		t.Fatalf("splits = %v, want his and her payments", got)
	}
	if _, ok := got[her]; !ok {
		t.Fatalf("splits = %v, want her payment", got)
	}
}

// Only the budget's member accounts count: a payment in the rule's category
// on another account does not exist for the debt.
func TestSync_OnlyTheBudgetsAccounts(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	outside := e.f.Account(fixture.Account{UserID: e.user})
	e.clock.now = ymd(2026, 7, 1)
	in := e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	e.expense(t, outside, ymd(2026, 5, 1), "1000", "")
	e.sync(t)
	if got := e.splits(t, res.Id); len(got) != 1 || got[in] == "" {
		t.Fatalf("splits = %v, want the member account's payment alone", got)
	}
}

// A soft-deleted member account still counts, like the budget counts it.
func TestSync_SoftDeletedMemberStillCounts(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	e.clock.now = ymd(2026, 7, 1)
	tx := e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	if _, err := e.db.Raw.ExecContext(context.Background(), e.db.Rebind("UPDATE accounts SET is_deleted = TRUE WHERE id = ?"), e.account); err != nil {
		t.Fatal(err)
	}
	e.sync(t)
	if got := e.splits(t, res.Id); got[tx] == "" {
		t.Fatalf("splits = %v, want the deleted member's payment", got)
	}
}

// An account that leaves the budget (a partner revoked, remove-account)
// takes its payments with it: the next sync drops their splits.
func TestSync_AccountLeavingTheBudgetDropsItsSplits(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	second := e.f.Account(fixture.Account{UserID: e.user})
	b := e.fakeBudget(e.budget)
	b.accounts = append(b.accounts, second)
	e.clock.now = ymd(2026, 7, 1)
	stays := e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	leaves := e.expense(t, second, ymd(2026, 5, 1), "1000", "")
	e.sync(t)
	if got := e.splits(t, res.Id); len(got) != 2 {
		t.Fatalf("splits = %v", got)
	}
	b.accounts = []string{e.account}
	e.sync(t)
	got := e.splits(t, res.Id)
	if _, ok := got[leaves]; ok || got[stays] == "" || len(got) != 1 {
		t.Fatalf("splits = %v, want the leaving account's split gone", got)
	}
}

// An account in two budgets with a debt each matching the payment: each debt
// keeps its own split, and syncing one budget never touches the other's.
func TestSync_SameTransactionCountsInTwoBudgets(t *testing.T) {
	e := newEnv(t)
	home := e.create(t, nil)
	second := e.newBudget(e.user, "Second", e.account)
	away := e.create(t, func(r *model.CreateDebtRequest) { r.BudgetId = second })
	e.clock.now = ymd(2026, 7, 1)
	tx := e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	e.sync(t)
	if got := e.splits(t, home.Id); got[tx] == "" {
		t.Fatalf("home splits = %v", got)
	}
	if got := e.splits(t, away.Id); len(got) != 0 {
		t.Fatalf("syncing one budget split the other's debt: %v", got)
	}
	if err := e.svc.Sync(context.Background(), e.scope(second)); err != nil {
		t.Fatal(err)
	}
	if got := e.splits(t, away.Id); got[tx] == "" {
		t.Fatalf("away splits = %v", got)
	}
	if got := e.splits(t, home.Id); got[tx] == "" {
		t.Fatalf("syncing the second budget dropped the first's split: %v", got)
	}
}

// A later debt with an identical rule ends the earlier debt on every one of
// its rules (refinancing moves both categories to the new loan).
func TestSync_LaterDebtEndsEveryRule(t *testing.T) {
	e := newEnv(t)
	b := e.newCategory(e.user, true)
	oldLoan := e.create(t, func(r *model.CreateDebtRequest) {
		r.StartDate = ptr("2025-12-01")
		r.Rules = []model.DebtRuleRequest{rule(e.category), rule(b)}
	})
	newLoan := e.create(t, func(r *model.CreateDebtRequest) { r.StartDate = ptr("2026-02-01") })
	e.clock.now = ymd(2026, 7, 1)
	before := e.f.Transaction(fixtureExpense(e, b, ymd(2026, 1, 10), "100"))
	after := e.f.Transaction(fixtureExpense(e, b, ymd(2026, 3, 10), "100"))
	inNew := e.expense(t, e.account, ymd(2026, 3, 1), "1000", "")
	e.sync(t)
	o, n := e.splits(t, oldLoan.Id), e.splits(t, newLoan.Id)
	if _, ok := o[before]; !ok || len(o) != 1 {
		t.Fatalf("old loan splits = %v, want only b's payment before the new loan", o)
	}
	if _, ok := o[after]; ok {
		t.Fatal("b's payment after the new loan's start still counts for the old one")
	}
	if _, ok := n[inNew]; !ok || len(n) != 1 {
		t.Fatalf("new loan splits = %v", n)
	}
}
