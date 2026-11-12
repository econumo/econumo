package debt_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestGetDebt_OtherUserNotFound(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.create(t, nil)
	other := vo.MustParseId(e.f.User(fixture.User{Name: "other", Email: "other@example.test"}))

	_, err := e.svc.GetDebt(ctx, other, model.GetDebtRequest{Id: res.Id})
	requireNotFound(t, err)
	_, err = e.svc.UpdateDebt(ctx, other, e.updateReq(res))
	requireNotFound(t, err)
	_, err = e.svc.DeleteDebt(ctx, other, model.DeleteDebtRequest{Id: res.Id})
	requireNotFound(t, err)
	_, err = e.svc.MarkDebtPaidOff(ctx, other, model.MarkDebtPaidOffRequest{Id: res.Id})
	requireNotFound(t, err)

	list, err := e.svc.GetDebtList(ctx, other)
	if err != nil || len(list.Items) != 0 {
		t.Fatalf("other user's list = %+v, %v", list, err)
	}
	if _, err := e.svc.GetDebt(ctx, e.uid(), model.GetDebtRequest{Id: res.Id}); err != nil {
		t.Fatal(err)
	}
}

func TestGetDebt_DetailsListPaymentsNewestFirst(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	e.clock.now = ymd(2026, 7, 1)
	first := e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	second := e.expense(t, e.account, ymd(2026, 5, 1), "1000", "")

	got, err := e.svc.GetDebt(context.Background(), e.uid(), model.GetDebtRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Snapshots) != 1 || got.Snapshots[0].AsOf != "2026-03-10" || got.Snapshots[0].Balance != "10000" {
		t.Fatalf("snapshots = %+v", got.Snapshots)
	}
	if len(got.Payments) != 2 || got.Payments[0].TransactionId != second || got.Payments[1].TransactionId != first {
		t.Fatalf("payments = %+v", got.Payments)
	}
	p := got.Payments[1]
	sum := vo.NewDecimal(p.Interest).Add(vo.NewDecimal(p.Principal))
	if p.Amount != "1000" || !sum.Equals(vo.NewDecimal("1000")) || p.Date != "2026-04-01" {
		t.Fatalf("payment = %+v", p)
	}
	if got.Item.OverpaidSoFar == "0" {
		t.Fatalf("item = %+v", got.Item)
	}
}

func TestViews_RangesAndPlanningBounds(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.clock.now = ymd(2026, 9, 1)
	oldDebt := e.create(t, nil)
	newer := e.create(t, func(r *model.CreateDebtRequest) { r.StartDate, r.LastPaymentDate = ptr("2026-06-15"), "2027-06-01" })
	payee := e.newPayee()
	cat2 := e.newCategory(e.user, true)
	narrow := e.create(t, func(r *model.CreateDebtRequest) { r.CategoryId = cat2; r.PayeeId = &payee })

	e.f.Transaction(fixture.Transaction{UserID: e.user, AccountID: e.account, CategoryID: cat2, PayeeID: payee, Type: 0, Amount: "1000", SpentAt: ymd(2026, 4, 20)})

	views, err := e.svc.Views(ctx, []vo.Id{e.uid()})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]model.DebtView{}
	for _, v := range views {
		byID[v.ID] = v
	}
	if len(views) != 3 {
		t.Fatalf("views = %d, want 3", len(views))
	}
	o, n, nr := byID[oldDebt.Id], byID[newer.Id], byID[narrow.Id]
	if o.From == nil || !o.From.Equal(ymd(2026, 3, 10)) || o.Until == nil || !o.Until.Equal(ymd(2026, 6, 15)) || o.Icon != "i" ||
		o.OwnerID != e.user || o.CurrencyID != fixture.USD {
		t.Errorf("old = %+v", o)
	}
	if n.From == nil || !n.From.Equal(ymd(2026, 6, 15)) || n.Until != nil {
		t.Errorf("newer = %+v", n)
	}
	if !o.StartMonth.Equal(ymd(2026, 3, 1)) || !o.EndMonth.Equal(ymd(2027, 3, 1)) {
		t.Errorf("old bounds = %v .. %v", o.StartMonth, o.EndMonth)
	}
	if !n.StartMonth.Equal(ymd(2026, 6, 1)) {
		t.Errorf("newer start = %v, want the start date's month", n.StartMonth)
	}
	// a first payment after the start does not move the start
	if nr.PayeeID == nil || *nr.PayeeID != payee || !nr.StartMonth.Equal(ymd(2026, 3, 1)) {
		t.Errorf("narrow = %+v (start is the start date's month)", nr)
	}
	if o.PayeeID != nil || o.TagID != nil || o.LabelID != nil {
		t.Errorf("old should be the whole-category debt: %+v", o)
	}
}

func TestViews_PaidOffEndsAtPayoffMonth(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	e.clock.now = ymd(2026, 5, 5)
	if _, err := e.svc.MarkDebtPaidOff(context.Background(), e.uid(), model.MarkDebtPaidOffRequest{Id: res.Id}); err != nil {
		t.Fatal(err)
	}
	views, err := e.svc.Views(context.Background(), []vo.Id{e.uid()})
	if err != nil || len(views) != 1 {
		t.Fatalf("views = %+v, %v", views, err)
	}
	if v := views[0]; !v.PaidOff || !v.EndMonth.Equal(ymd(2026, 5, 1)) {
		t.Fatalf("view = %+v", v)
	}
}

// Views runs inside budget transactions, so it must neither write nor lock:
// a new matching payment counts in the view but gets no stored split until a
// Sync.
func TestViews_WritesNothing(t *testing.T) {
	e := newEnv(t)
	res := e.createWithoutTerms(t, nil)
	e.clock.now = ymd(2026, 4, 20)
	tx := e.expense(t, e.account, ymd(2026, 2, 20), "105", "")

	views, err := e.svc.Views(context.Background(), []vo.Id{e.uid()})
	if err != nil || len(views) != 1 {
		t.Fatalf("views = %+v, %v", views, err)
	}
	if !views[0].StartMonth.Equal(ymd(2026, 2, 1)) {
		t.Errorf("start = %v, want the unsplit payment's month", views[0].StartMonth)
	}
	if got := e.splits(t, res.Id); len(got) != 0 {
		t.Fatalf("Views stored splits: %v", got)
	}

	e.sync(t)
	if _, ok := e.splits(t, res.Id)[tx]; !ok {
		t.Fatal("Sync stored no split for the payment")
	}
}

// A payment before the creation day starts the life of a debt not set up.
func TestViews_StartAtAnEarlierFirstPayment(t *testing.T) {
	e := newEnv(t)
	res := e.createWithoutTerms(t, nil)
	e.expense(t, e.account, ymd(2026, 1, 20), "1000", "")
	views, err := e.svc.Views(context.Background(), []vo.Id{e.uid()})
	if err != nil || len(views) != 1 {
		t.Fatalf("views = %v, %v", views, err)
	}
	if views[0].ID != res.Id || !views[0].StartMonth.Equal(ymd(2026, 1, 1)) {
		t.Fatalf("start = %v, want January", views[0].StartMonth)
	}
}

// Each payment carries the balance left after it, from the snapshots and the
// stored splits: a later payment on the same day comes off the earlier one's.
func TestGetDebt_PaymentsCarryTheBalanceAfterThem(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	e.clock.now = ymd(2026, 7, 1)
	e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	e.expense(t, e.account, ymd(2026, 5, 1), "1000", "")
	e.expense(t, e.account, ymd(2026, 5, 1), "300", "")

	got, err := e.svc.GetDebt(context.Background(), e.uid(), model.GetDebtRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Payments) != 3 {
		t.Fatalf("payments = %+v", got.Payments)
	}
	// oldest first: the balance falls by each payment's debt part
	balance := vo.NewDecimal("10000")
	for i := len(got.Payments) - 1; i >= 0; i-- {
		p := got.Payments[i]
		balance = balance.Sub(vo.NewDecimal(p.Principal))
		if !vo.NewDecimal(p.Balance).Equals(balance) {
			t.Fatalf("payment %d (%s %s): balance %q, want %s", i, p.Date, p.Amount, p.Balance, balance)
		}
	}
	if !vo.NewDecimal(got.Payments[0].Balance).Equals(vo.NewDecimal(got.Item.CurrentBalance)) {
		t.Fatalf("latest balance %q, owed now %q", got.Payments[0].Balance, got.Item.CurrentBalance)
	}
}

func TestGetDebt_NotSetUpPaymentsHaveNoBalance(t *testing.T) {
	e := newEnv(t)
	res := e.createWithoutTerms(t, nil)
	e.clock.now = ymd(2026, 7, 1)
	e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")

	got, err := e.svc.GetDebt(context.Background(), e.uid(), model.GetDebtRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Payments) != 1 || got.Payments[0].Balance != "" {
		t.Fatalf("payments = %+v", got.Payments)
	}
}

// A snapshot holds the balance after its day's payments: a payment on the day
// of a balance now leaves that balance, and one before it comes off the
// amount borrowed.
func TestGetDebt_BalanceAroundTheSnapshotDay(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, func(r *model.CreateDebtRequest) {
		r.AmountBorrowed, r.StartDate, r.Balance = vo.NewFlexString("11000"), ptr("2026-02-15"), vo.NewFlexString("10000")
	}) // balance now as of March 10: 10000
	e.clock.now = ymd(2026, 3, 20)
	e.expense(t, e.account, ymd(2026, 3, 1), "500", "")
	e.expense(t, e.account, ymd(2026, 3, 10), "1000", "")

	got, err := e.svc.GetDebt(context.Background(), e.uid(), model.GetDebtRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Payments) != 2 || got.Payments[0].Date != "2026-03-10" {
		t.Fatalf("payments = %+v", got.Payments)
	}
	onDay, before := got.Payments[0], got.Payments[1]
	if !vo.NewDecimal(onDay.Balance).Equals(vo.NewDecimal("10000")) {
		t.Fatalf("on the snapshot's day: %q, want 10000", onDay.Balance)
	}
	if want := vo.NewDecimal("11000").Sub(vo.NewDecimal(before.Principal)); !vo.NewDecimal(before.Balance).Equals(want) {
		t.Fatalf("before the snapshot: %q, want %s", before.Balance, want)
	}
}

// extraPaid is what the recorded payments went above the regular payment,
// window by window, in the debt's digits; "" while the debt is not set up.
func TestGetDebt_ExtraPaid(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	if res.ExtraPaid != "0" {
		t.Fatalf("extra paid with no payments = %q, want 0", res.ExtraPaid)
	}
	e.clock.now = ymd(2026, 7, 1)
	e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")
	e.expense(t, e.account, ymd(2026, 5, 1), "1250.5", "")
	e.expense(t, e.account, ymd(2026, 8, 1), "3000", "")

	got, err := e.svc.GetDebt(context.Background(), e.uid(), model.GetDebtRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	if got.Item.ExtraPaid != "250.5" {
		t.Fatalf("extra paid = %q, want 250.5", got.Item.ExtraPaid)
	}
	list, err := e.svc.GetDebtList(context.Background(), e.uid())
	if err != nil || list.Items[0].ExtraPaid != "250.5" {
		t.Fatalf("list = %+v, %v", list, err)
	}

	bare := e.createWithoutTerms(t, func(r *model.CreateDebtRequest) { r.CategoryId = e.newCategory(e.user, true) })
	if bare.ExtraPaid != "" {
		t.Fatalf("not set up extra paid = %q, want empty", bare.ExtraPaid)
	}
}
