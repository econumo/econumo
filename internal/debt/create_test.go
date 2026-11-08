package debt_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestCreateDebt_ValidatesAndSnapshots(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)

	if res.Name != "Car loan" || res.Icon != "i" || res.CurrencyId != fixture.USD || res.CategoryId != e.category {
		t.Errorf("identity = %+v", res)
	}
	if res.Balance != "10000" || res.Payment != "1000" || res.Schedule != "monthly" || res.LastPaymentDate != "2027-03-01" || res.StartDate != "" {
		t.Errorf("terms = %+v", res)
	}
	if res.CurrentBalance != "10000" || res.PaymentsLeft != 12 || res.IsPaidOff != 0 || res.PayeeId != nil || res.TagId != nil {
		t.Errorf("figures = %+v", res)
	}
	if res.PeriodRate == "" || res.PeriodRate == "0.0000000000" || res.NextDueDate != "2026-04-01" || res.PayoffDate != "2027-03-01" {
		t.Errorf("rate/dates = %q %q %q", res.PeriodRate, res.NextDueDate, res.PayoffDate)
	}
	snaps := e.snapshots(t, res.Id)
	if len(snaps) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snaps))
	}
	if sn := snaps[0]; !sn.AsOf.Equal(ymd(2026, 3, 10)) || sn.Balance.String() != "10000" || sn.Payment.String() != "1000" ||
		sn.Schedule != model.RecurringScheduleMonthly || !sn.LastPaymentDate.Equal(ymd(2027, 3, 1)) {
		t.Errorf("snapshot = %+v", sn)
	}
}

func TestCreateDebt_RefusesLastPaymentInPast(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	req.LastPaymentDate = "2026-03-10"
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeDebtLastPaymentInPast)
}

// A debt created without a balance and a last payment date is not set up: no
// snapshot, and every figure that needs the terms is empty.
func TestCreateDebt_WithoutTerms(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	withoutTerms(&req)
	if err := req.Validate(); err != nil {
		t.Fatalf("the request without terms is valid: %v", err)
	}
	res, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsSetUp != 0 || res.Payment != "1000" || res.Schedule != "monthly" || res.IsPaidOff != 0 {
		t.Errorf("result = %+v", res)
	}
	for name, v := range map[string]string{
		"balance": res.Balance, "lastPaymentDate": res.LastPaymentDate, "currentBalance": res.CurrentBalance,
		"moneyLeft": res.MoneyLeft, "overpaymentLeft": res.OverpaymentLeft, "overpaidSoFar": res.OverpaidSoFar,
		"payoffDate": res.PayoffDate, "nextDueDate": res.NextDueDate, "periodRate": res.PeriodRate,
	} {
		if v != "" {
			t.Errorf("%s = %q, want empty", name, v)
		}
	}
	if res.PaymentsLeft != 0 || res.PeriodsPaid != 0 {
		t.Errorf("counts = %d %d, want 0", res.PaymentsLeft, res.PeriodsPaid)
	}
	if snaps := e.snapshots(t, res.Id); len(snaps) != 0 {
		t.Fatalf("snapshots = %d, want none", len(snaps))
	}
	// a whole-category debt without terms takes the category's plans from
	// its creation day on, with no end
	want := transferCall{from: e.category, fromType: model.ElementCategory, to: res.Id, toType: model.ElementDebt,
		currency: fixture.USD, sum: true, first: "2026-03-10", last: ""}
	if len(e.budget.transfers) != 1 || e.budget.transfers[0] != want {
		t.Fatalf("transfers = %+v, want %+v", e.budget.transfers, want)
	}
}

func TestCreateDebt_TermsPairRequired(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	req.LastPaymentDate = ""
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireField(t, err, "lastPaymentDate", errs.CodeIsBlank)

	req = e.createReq()
	req.Balance = vo.FlexString{}
	_, err = e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireField(t, err, "balance", errs.CodeIsBlank)
}

func TestCreateDebt_WithoutTermsNeedsAPositivePayment(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	withoutTerms(&req)
	req.Payment = vo.NewFlexString("0.001")
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireField(t, err, "payment", errs.CodeOutOfRange)
}

func TestCreateDebt_LabelMustBeTheOwners(t *testing.T) {
	e := newEnv(t)
	other := e.f.User(fixture.User{Name: "other", Email: "other@example.test"})
	foreign := e.f.Label(fixture.Label{UserID: other})
	e.class.labels[foreign] = other
	req := e.createReq()
	req.LabelId = &foreign
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireNotFound(t, err)

	own := e.newLabel()
	res := e.create(t, func(r *model.CreateDebtRequest) { r.LabelId = &own })
	if res.LabelId == nil || *res.LabelId != own {
		t.Fatalf("labelId = %v, want %s", res.LabelId, own)
	}
	// a labelled debt is not whole-category: the category keeps its plans
	if len(e.budget.transfers) != 0 {
		t.Fatalf("transfers = %+v, want none", e.budget.transfers)
	}
}

func TestCheckKeys_LabelsTellDebtsApart(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	car, student := e.newLabel(), e.newLabel()
	e.create(t, func(r *model.CreateDebtRequest) { r.LabelId = &car })
	e.create(t, func(r *model.CreateDebtRequest) { r.LabelId = &student })

	req := e.createReq()
	_, err := e.svc.CreateDebt(ctx, e.uid(), req)
	requireCode(t, err, errs.CodeDebtKeyConflict)

	req = e.createReq()
	req.LabelId = &car
	_, err = e.svc.CreateDebt(ctx, e.uid(), req)
	requireCode(t, err, errs.CodeDebtStartDateTaken)

	req.StartDate = ptr("2026-06-01")
	if _, err := e.svc.CreateDebt(ctx, e.uid(), req); err != nil {
		t.Fatalf("same label with a distinct start date: %v", err)
	}
}

func TestCreateDebt_ReportsSetUpAndLabel(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	if res.IsSetUp != 1 || res.LabelId != nil || res.PeriodsPaid != 0 {
		t.Errorf("isSetUp/labelId/periodsPaid = %d %v %d", res.IsSetUp, res.LabelId, res.PeriodsPaid)
	}
}

func TestCreateDebt_RefusesIncomeCategory(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	req.CategoryId = e.newCategory(e.user, false)
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeDebtCategoryNotExpense)
}

func TestCreateDebt_RefusesForeignCategory(t *testing.T) {
	e := newEnv(t)
	other := e.f.User(fixture.User{Name: "other", Email: "other@example.test"})
	req := e.createReq()
	req.CategoryId = e.newCategory(other, true)
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireNotFound(t, err)
}

func TestCreateDebt_RefusesUnusableCurrency(t *testing.T) {
	e := newEnv(t)
	e.currency.unusable[fixture.USD] = true
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), e.createReq())
	requireCode(t, err, errs.CodeCurrencyNotAvailable)
	if got, err := e.repo.ListByCategory(context.Background(), vo.MustParseId(e.category)); err != nil || len(got) != 0 {
		t.Errorf("debts after a refused create = %v, %v", got, err)
	}
}

func TestCreateDebt_RefusesPaymentsDontCover(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	req.Balance = vo.NewFlexString("20000")
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeDebtPaymentsDontCover)
}

// In an 8-digit currency nine payments of 0.001 are short of 0.01: the
// tolerance is half that currency's smallest unit, not half a cent.
func TestCreateDebt_RefusesPaymentsDontCoverInAPreciseCurrency(t *testing.T) {
	e := newEnv(t)
	e.rates.digits = 8
	req := e.createReq()
	// nine monthly dues after Mar 10, 2026: Apr 1 to Dec 1
	req.Balance, req.Payment, req.LastPaymentDate = vo.NewFlexString("0.01"), vo.NewFlexString("0.001"), "2026-12-01"
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeDebtPaymentsDontCover)

	req.LastPaymentDate = "2027-01-01"
	if _, err := e.svc.CreateDebt(context.Background(), e.uid(), req); err != nil {
		t.Fatalf("ten payments of 0.001 cover 0.01: %v", err)
	}
}

func TestCreateDebt_KeyConflict(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	payee := e.newPayee()
	e.create(t, nil)
	req := e.createReq()
	req.PayeeId = &payee
	_, err := e.svc.CreateDebt(ctx, e.uid(), req)
	requireCode(t, err, errs.CodeDebtKeyConflict)

	// A payee-only debt blocks a category-only one, and a tag-only debt
	// overlaps a payee-only one.
	cat2 := e.newCategory(e.user, true)
	payee2, tag2 := e.newPayee(), e.newTag()
	e.create(t, func(r *model.CreateDebtRequest) { r.CategoryId = cat2; r.PayeeId = &payee2 })
	plain := e.createReq()
	plain.CategoryId = cat2
	_, err = e.svc.CreateDebt(ctx, e.uid(), plain)
	requireCode(t, err, errs.CodeDebtKeyConflict)
	tagReq := e.createReq()
	tagReq.CategoryId, tagReq.TagId = cat2, &tag2
	_, err = e.svc.CreateDebt(ctx, e.uid(), tagReq)
	requireCode(t, err, errs.CodeDebtKeyConflict)

	// Disjoint payees coexist.
	other := e.newPayee()
	e.create(t, func(r *model.CreateDebtRequest) { r.CategoryId = cat2; r.PayeeId = &other })
}

func TestCreateDebt_SameKeyNeedsDistinctStartDates(t *testing.T) {
	e := newEnv(t)
	e.create(t, nil)
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), e.createReq())
	requireCode(t, err, errs.CodeDebtStartDateTaken)

	e.create(t, func(r *model.CreateDebtRequest) { r.StartDate = ptr("2026-01-01") })
	dup := e.createReq()
	dup.StartDate = ptr("2026-01-01")
	_, err = e.svc.CreateDebt(context.Background(), e.uid(), dup)
	requireCode(t, err, errs.CodeDebtStartDateTaken)

	e.create(t, func(r *model.CreateDebtRequest) { r.StartDate = ptr("2026-02-01") })
}

func TestCreateDebt_WholeCategoryTransfersPlans(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	// only the debt's life moves: from its first snapshot to its last payment
	want := transferCall{from: e.category, fromType: model.ElementCategory, to: res.Id, toType: model.ElementDebt, currency: fixture.USD,
		sum: true, first: "2026-03-10", last: "2027-03-01"}
	if len(e.budget.transfers) != 1 || e.budget.transfers[0] != want {
		t.Fatalf("transfers = %+v, want [%+v]", e.budget.transfers, want)
	}

	payee := e.newPayee()
	e.budget.transfers = nil
	e.create(t, func(r *model.CreateDebtRequest) { r.CategoryId = e.newCategory(e.user, true); r.PayeeId = &payee })
	if len(e.budget.transfers) != 0 {
		t.Fatalf("a payee-narrowed debt must not move the category's plans: %+v", e.budget.transfers)
	}
}

func TestCreateDebt_WorksWithoutBudgetPeer(t *testing.T) {
	e := newEnv(t)
	e.svc.SetBudgetElements(nil)
	e.create(t, nil)
}

// A blank start date passes the request's validation, so the service must
// read it as none rather than fail to parse it.
func TestCreateDebt_BlankStartDateIsNone(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, func(r *model.CreateDebtRequest) { r.StartDate = ptr(" ") })
	if res.StartDate != "" {
		t.Fatalf("start date = %q, want none", res.StartDate)
	}
}

// The projection covers 100 years; a longer term (a mistyped year) is refused
// rather than projected partially.
func TestCreateDebt_TermBeyondTheProjectionIsRefused(t *testing.T) {
	e := newEnv(t)
	req := e.createReq()
	req.LastPaymentDate = "2126-04-01"
	_, err := e.svc.CreateDebt(context.Background(), e.uid(), req)
	requireCode(t, err, errs.CodeOutOfRange)
	req.LastPaymentDate = "2126-03-01"
	req.Balance = vo.NewFlexString("1200000")
	if _, err := e.svc.CreateDebt(context.Background(), e.uid(), req); err != nil {
		t.Fatalf("a 100-year term is allowed: %v", err)
	}
}
