package budget_test

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// The reference loan: 300000 at 2000 a month, 240 dues on the 20th from
// 2026-06-20 (five days after the harness's today).
func (h *attrHarness) createRefDebt(t *testing.T, extra func(*model.CreateDebtRequest)) string {
	t.Helper()
	req := model.CreateDebtRequest{
		Name: "Mortgage", CurrencyId: fixture.USD, CategoryId: h.loan,
		Balance: vo.NewFlexString("300000"), Payment: vo.NewFlexString("2000"),
		Schedule: "monthly", LastPaymentDate: "2046-05-20",
	}
	if extra != nil {
		extra(&req)
	}
	res, err := h.debts.CreateDebt(h.ctx, h.owner, req)
	if err != nil {
		t.Fatalf("CreateDebt: %v", err)
	}
	return res.Id
}

func (h *attrHarness) setLimit(t *testing.T, elementID, period, amount string) {
	t.Helper()
	a := vo.NewFlexString(amount)
	if _, err := h.svc.SetLimit(h.ctx, h.owner, model.SetLimitRequest{
		BudgetId: h.budgetID, ElementId: elementID, Period: period, Amount: &a,
	}); err != nil {
		t.Fatalf("SetLimit %s: %v", period, err)
	}
}

func (h *attrHarness) budgetAt(t *testing.T, viewer vo.Id, date string) *model.GetBudgetResult {
	t.Helper()
	res, err := h.svc.GetBudget(h.ctx, viewer, model.GetBudgetRequest{Id: h.budgetID, Date: date})
	if err != nil {
		t.Fatalf("GetBudget %s: %v", date, err)
	}
	return res
}

func (h *attrHarness) plan(t *testing.T, from, months string) *model.GetBudgetPlanResult {
	t.Helper()
	res, err := h.svc.GetBudgetPlan(h.ctx, h.owner, model.GetBudgetPlanRequest{Id: h.budgetID, From: from, Months: months})
	if err != nil {
		t.Fatalf("GetBudgetPlan: %v", err)
	}
	return res
}

func debtRow(res *model.GetBudgetResult, id string) *model.DebtElementResult {
	for i := range res.Item.Structure.Debts {
		if res.Item.Structure.Debts[i].Id == id {
			return &res.Item.Structure.Debts[i]
		}
	}
	return nil
}

func planDebtRow(res *model.GetBudgetPlanResult, id string) *model.PlanDebtElementResult {
	for i := range res.Item.Structure.Debts {
		if res.Item.Structure.Debts[i].Id == id {
			return &res.Item.Structure.Debts[i]
		}
	}
	return nil
}

func decimalOf(t *testing.T, s string) float64 {
	t.Helper()
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("amount %q: %v", s, err)
	}
	return f
}

func nearAmount(t *testing.T, name, got string, want, tol float64) {
	t.Helper()
	if math.Abs(decimalOf(t, got)-want) > tol {
		t.Errorf("%s = %s, want %.2f (±%v)", name, got, want, tol)
	}
}

func TestGetBudget_DebtRowFigures(t *testing.T) {
	h := newAttrHarness(t)
	h.spend(h.usd, h.loan, "", "2000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	debtID := h.createRefDebt(t, nil)
	h.setLimit(t, debtID, "2026-06-01", "2000")

	r, err := model.ImpliedRate(300000, 2000, 240)
	if err != nil {
		t.Fatal(err)
	}
	juneClosing := 300000*(1+r) - 2000
	julyClosing := juneClosing*(1+r) - 2000

	june := debtRow(h.budgetAt(t, h.owner, "2026-06-01"), debtID)
	if june == nil {
		t.Fatal("no debt row in June")
	}
	if june.Type != 6 || june.Name != "Mortgage" || june.CurrencyId != fixture.USD || june.OwnerUserId != h.owner.String() {
		t.Errorf("June row meta = %+v", june)
	}
	if june.Budgeted != "2000" || june.IsBudgeted != 1 || june.Spent != "2000" || june.BudgetSpent != "2000" || june.Available != "0" {
		t.Errorf("June planned/paid = %+v, want 2000 / 2000 / available 0", june)
	}
	nearAmount(t, "June closing", june.ClosingBalance, juneClosing, 0.02)
	if june.PaymentsLeft != 239 || june.ScheduledTotal != "2000" || june.Upcoming != "2000" || june.IsPlannable != 1 || june.IsPaidOff != 0 {
		t.Errorf("June schedule = %+v, want 239 left, 2000 scheduled and still due", june)
	}
	if june.PayoffDate != "2046-05-20" || june.BasePayoffDate != "2046-05-20" {
		t.Errorf("payoff = %s / base %s, want 2046-05-20", june.PayoffDate, june.BasePayoffDate)
	}
	nearAmount(t, "overpayment left", june.OverpaymentLeft, 180000, 1)

	july := debtRow(h.budgetAt(t, h.owner, "2026-07-01"), debtID)
	if july == nil {
		t.Fatal("no debt row in July")
	}
	if july.Budgeted != "0" || july.IsBudgeted != 0 || july.Spent != "0" || july.Available != "0" {
		t.Errorf("July planned/paid = %+v, want nothing (June's plan was paid)", july)
	}
	nearAmount(t, "July closing", july.ClosingBalance, julyClosing, 0.02)
	if july.PaymentsLeft != 238 || july.Upcoming != "2000" {
		t.Errorf("July = %d left, %s still due; want 238, 2000", july.PaymentsLeft, july.Upcoming)
	}

	if may := debtRow(h.budgetAt(t, h.owner, "2026-05-01"), debtID); may != nil {
		t.Errorf("May is before the debt's first payment, row = %+v", may)
	}
}

func TestGetBudget_DebtRowCarriesOver(t *testing.T) {
	h := newAttrHarness(t)
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC))
	debtID := h.createDebt(t)
	h.setLimit(t, debtID, "2026-05-01", "1500")

	june := debtRow(h.budgetAt(t, h.owner, "2026-06-01"), debtID)
	if june == nil || june.Available != "500" || june.Spent != "0" {
		t.Fatalf("June row = %+v, want May's unspent 500 carried", june)
	}
}

func TestGetBudget_DebtRowHiddenAfterPayoff(t *testing.T) {
	h := newAttrHarness(t)
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	debtID := h.createDebt(t)
	// the payment was planned, so nothing carries past the payoff month
	h.setLimit(t, debtID, "2026-06-01", "1000")
	if _, err := h.debts.MarkDebtPaidOff(h.ctx, h.owner, model.MarkDebtPaidOffRequest{Id: debtID}); err != nil {
		t.Fatalf("MarkDebtPaidOff: %v", err)
	}

	june := debtRow(h.budgetAt(t, h.owner, "2026-06-01"), debtID)
	if june == nil {
		t.Fatal("the payoff month keeps the row")
	}
	if june.IsPaidOff != 1 || june.ClosingBalance != "0" || june.PaymentsLeft != 0 || june.Spent != "1000" {
		t.Errorf("June = %+v, want paid off, nothing left, 1000 paid", june)
	}
	if july := debtRow(h.budgetAt(t, h.owner, "2026-07-01"), debtID); july != nil {
		t.Errorf("July row after payoff = %+v, want none", july)
	}
}

func TestGetBudget_DebtRowWithOverspendStaysAfterPayoff(t *testing.T) {
	h := newAttrHarness(t)
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	debtID := h.createDebt(t)
	if _, err := h.debts.MarkDebtPaidOff(h.ctx, h.owner, model.MarkDebtPaidOffRequest{Id: debtID}); err != nil {
		t.Fatalf("MarkDebtPaidOff: %v", err)
	}
	// an unplanned payment carries, like any expense row's overspend
	july := debtRow(h.budgetAt(t, h.owner, "2026-07-01"), debtID)
	if july == nil || july.Available != "-1000" || july.IsPlannable != 0 {
		t.Errorf("July row = %+v, want -1000 carried, not plannable", july)
	}
}

func TestGetBudget_DebtRowWithCarryStaysAfterPayoff(t *testing.T) {
	h := newAttrHarness(t)
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	debtID := h.createDebt(t)
	h.setLimit(t, debtID, "2026-06-01", "1500")
	if _, err := h.debts.MarkDebtPaidOff(h.ctx, h.owner, model.MarkDebtPaidOffRequest{Id: debtID}); err != nil {
		t.Fatalf("MarkDebtPaidOff: %v", err)
	}

	july := debtRow(h.budgetAt(t, h.owner, "2026-07-01"), debtID)
	if july == nil {
		t.Fatal("July after payoff dropped the row while June's unspent plan still counts in Available")
	}
	if july.Available != "500" || july.IsPlannable != 0 || july.Budgeted != "0" || july.IsBudgeted != 0 || july.Spent != "0" {
		t.Errorf("July row = %+v, want 500 carried, not plannable, no plan, nothing paid", july)
	}
}

func TestGetBudget_PartnerDebtRowVisible(t *testing.T) {
	h := newAttrHarness(t)
	partner := h.f.User(fixture.User{Name: "Partner", Email: "partner@attr.test"})
	guest := h.f.User(fixture.User{Name: "Guest", Email: "guest@attr.test"})
	h.f.BudgetAccess(h.budgetID, partner, int(model.BudgetRoleUser), true)
	h.f.BudgetAccess(h.budgetID, guest, int(model.BudgetRoleGuest), true)
	car := h.f.Category(fixture.Category{UserID: partner, Name: "Car loan"})
	res, err := h.debts.CreateDebt(h.ctx, vo.MustParseId(partner), model.CreateDebtRequest{
		Name: "Car", CurrencyId: fixture.USD, CategoryId: car,
		Balance: vo.NewFlexString("6000"), Payment: vo.NewFlexString("500"),
		Schedule: "monthly", LastPaymentDate: "2027-05-20",
	})
	if err != nil {
		t.Fatalf("CreateDebt: %v", err)
	}

	// June 20's payment is still due, so June closes at 5500 with 11 to go.
	for name, viewer := range map[string]vo.Id{"owner": h.owner, "guest": vo.MustParseId(guest), "partner": vo.MustParseId(partner)} {
		row := debtRow(h.budgetAt(t, viewer, "2026-06-01"), res.Id)
		if row == nil {
			t.Errorf("%s sees no partner debt row", name)
			continue
		}
		if row.OwnerUserId != partner || row.Name != "Car" || row.ClosingBalance != "5500" || row.PaymentsLeft != 11 {
			t.Errorf("%s sees %+v", name, row)
		}
	}
}

func TestGetBudgetPlan_PlannedExtraMovesPayoff(t *testing.T) {
	h := newAttrHarness(t)
	debtID := h.createRefDebt(t, nil)

	base := planDebtRow(h.plan(t, "2026-06-01", "3"), debtID)
	if base == nil {
		t.Fatal("no plan debt row")
	}
	if base.PayoffDate != base.BasePayoffDate {
		t.Fatalf("payoff %s differs from base %s without a plan", base.PayoffDate, base.BasePayoffDate)
	}

	h.setLimit(t, debtID, "2026-07-01", "7000")
	row := planDebtRow(h.plan(t, "2026-06-01", "3"), debtID)
	if row.PayoffDate >= row.BasePayoffDate || row.BasePayoffDate != base.BasePayoffDate {
		t.Errorf("payoff %s / base %s, want earlier than %s", row.PayoffDate, row.BasePayoffDate, base.BasePayoffDate)
	}
	if decimalOf(t, row.OverpaymentLeft) >= decimalOf(t, row.BaseOverpaymentLeft) {
		t.Errorf("overpayment %s not below base %s", row.OverpaymentLeft, row.BaseOverpaymentLeft)
	}
	if row.Cells[0].ClosingBalance != base.Cells[0].ClosingBalance {
		t.Errorf("June closing moved: %s -> %s", base.Cells[0].ClosingBalance, row.Cells[0].ClosingBalance)
	}
	nearAmount(t, "July closing", row.Cells[1].ClosingBalance, decimalOf(t, base.Cells[1].ClosingBalance)-5000, 0.02)
	if row.Cells[1].Planned != "7000" || row.Cells[1].ScheduledTotal != "2000" || row.Cells[1].Upcoming != "2000" {
		t.Errorf("July cell = %+v", row.Cells[1])
	}
	if row.Cells[2].PaymentsLeft >= base.Cells[2].PaymentsLeft {
		t.Errorf("August payments left %d, want fewer than %d", row.Cells[2].PaymentsLeft, base.Cells[2].PaymentsLeft)
	}

	// The extra counts whichever window is shown.
	june := debtRow(h.budgetAt(t, h.owner, "2026-06-01"), debtID)
	if june.PayoffDate != row.PayoffDate {
		t.Errorf("get-budget payoff %s, plan's %s", june.PayoffDate, row.PayoffDate)
	}
}

func TestGetBudgetPlan_OutOfBoundsCellsNotPlannable(t *testing.T) {
	h := newAttrHarness(t)
	start := "2026-08-01"
	debtID := h.createRefDebt(t, func(r *model.CreateDebtRequest) { r.StartDate = &start })
	element := h.f.BudgetElement(fixture.BudgetElement{BudgetID: h.budgetID, ExternalID: debtID, Type: int(model.ElementDebt), CurrencyID: fixture.USD})
	h.f.BudgetLimit(fixture.BudgetLimit{ElementID: element, Period: "2026-07-01 00:00:00", Amount: "50000"})
	h.setLimit(t, debtID, "2026-08-01", "2000")

	row := planDebtRow(h.plan(t, "2026-06-01", "4"), debtID)
	if row == nil {
		t.Fatal("no plan debt row")
	}
	for i, want := range []int{0, 0, 1, 1} {
		if row.Cells[i].IsPlannable != want {
			t.Errorf("cell %d plannable = %d, want %d", i, row.Cells[i].IsPlannable, want)
		}
	}
	// June's due falls before the debt's start: nothing scheduled, due or
	// owed is shown there yet.
	for i := 0; i < 2; i++ {
		if c := row.Cells[i]; c.ScheduledTotal != "0" || c.Upcoming != "0" || c.ClosingBalance != "0" {
			t.Errorf("cell %d before the start = %+v, want nothing scheduled, due or owed", i, c)
		}
	}
	if c := row.Cells[2]; c.ScheduledTotal != "2000" || c.Upcoming != "2000" {
		t.Errorf("August cell = %+v, want 2000 scheduled and due", c)
	}
	if row.Cells[1].Planned != "" {
		t.Errorf("July's stored plan is out of range, planned = %q", row.Cells[1].Planned)
	}
	if row.Cells[2].Planned != "2000" {
		t.Errorf("August planned = %q, want 2000", row.Cells[2].Planned)
	}
	if row.PayoffDate != row.BasePayoffDate {
		t.Errorf("an out-of-range plan moved the payoff: %s vs %s", row.PayoffDate, row.BasePayoffDate)
	}
	if july := debtRow(h.budgetAt(t, h.owner, "2026-07-01"), debtID); july != nil {
		t.Errorf("July is before the debt's start, row = %+v", july)
	}
}

func TestGetBudget_DebtRowsStayOutOfElements(t *testing.T) {
	h := newAttrHarness(t)
	debtID := h.createDebt(t)
	res := h.budgetAt(t, h.owner, "2026-06-01")
	if findElement(res, debtID, model.ElementDebt) != nil {
		t.Error("debt row listed in structure.elements")
	}
	if debtRow(res, debtID) == nil {
		t.Error("debt row missing from structure.debts")
	}
	plan := h.plan(t, "2026-06-01", "2")
	for _, el := range plan.Item.Structure.Elements {
		if el.Id == debtID {
			t.Error("debt row listed in the plan's elements")
		}
	}
}
