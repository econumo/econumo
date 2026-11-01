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
	oldDebt := e.create(t, nil)
	newer := e.create(t, func(r *model.CreateDebtRequest) { r.StartDate = ptr("2026-06-15") })
	payee := e.newPayee()
	cat2 := e.newCategory(e.user, true)
	narrow := e.create(t, func(r *model.CreateDebtRequest) { r.CategoryId = cat2; r.PayeeId = &payee })

	e.clock.now = ymd(2026, 9, 1)
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
	if o.From != nil || o.Until == nil || !o.Until.Equal(ymd(2026, 6, 15)) || o.Icon != "i" || o.OwnerID != e.user || o.CurrencyID != fixture.USD {
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
	if nr.WholeCategory || nr.PayeeID == nil || *nr.PayeeID != payee || !nr.StartMonth.Equal(ymd(2026, 4, 1)) {
		t.Errorf("narrow = %+v (start is the first matched payment's month)", nr)
	}
	if !o.WholeCategory && o.From != nil {
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
