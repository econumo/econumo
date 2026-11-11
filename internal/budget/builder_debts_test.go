package budget_test

import (
	"errors"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// The reference loan: 300000 at 2000 a month, taken on 2026-06-01, 240 dues
// on the 20th from 2026-06-20 (five days after the harness's today).
func (h *attrHarness) createRefDebt(t *testing.T, extra func(*model.CreateDebtRequest)) string {
	t.Helper()
	start := "2026-06-01"
	req := model.CreateDebtRequest{
		Name: "Mortgage", CurrencyId: fixture.USD, CategoryId: h.loan,
		StartDate: &start, AmountBorrowed: vo.NewFlexString("300000"), Balance: vo.NewFlexString("300000"), Payment: vo.NewFlexString("2000"),
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

	r, err := model.ImpliedRate(300000, 2000, 240, 2)
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

// Plans stored for months the debt no longer reaches after an early payoff
// do not carry into Available.
func TestGetBudget_DebtRowIgnoresPlansPastEarlyPayoff(t *testing.T) {
	h := newAttrHarness(t)
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	debtID := h.createDebt(t)
	h.setLimit(t, debtID, "2026-06-01", "1000")
	h.setLimit(t, debtID, "2026-07-01", "1000")
	h.setLimit(t, debtID, "2026-08-01", "1000")
	if _, err := h.debts.MarkDebtPaidOff(h.ctx, h.owner, model.MarkDebtPaidOffRequest{Id: debtID}); err != nil {
		t.Fatalf("MarkDebtPaidOff: %v", err)
	}

	for _, month := range []string{"2026-07-01", "2026-08-01", "2026-09-01"} {
		if row := debtRow(h.budgetAt(t, h.owner, month), debtID); row != nil {
			t.Errorf("%s row after payoff = %+v, want none (later plans are outside the debt's life)", month, row)
		}
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
		StartDate: &attrStart, AmountBorrowed: vo.NewFlexString("6000"), Balance: vo.NewFlexString("6000"), Payment: vo.NewFlexString("500"),
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
	debtID := h.createRefDebt(t, nil) // taken on 2026-06-01
	element := h.f.BudgetElement(fixture.BudgetElement{BudgetID: h.budgetID, ExternalID: debtID, Type: int(model.ElementDebt), CurrencyID: fixture.USD})
	h.f.BudgetLimit(fixture.BudgetLimit{ElementID: element, Period: "2026-05-01 00:00:00", Amount: "50000"})
	h.setLimit(t, debtID, "2026-06-01", "2000")

	row := planDebtRow(h.plan(t, "2026-04-01", "4"), debtID)
	if row == nil {
		t.Fatal("no plan debt row")
	}
	for i, want := range []int{0, 0, 1, 1} {
		if row.Cells[i].IsPlannable != want {
			t.Errorf("cell %d plannable = %d, want %d", i, row.Cells[i].IsPlannable, want)
		}
	}
	// April's and May's dues fall before the loan's start: nothing
	// scheduled, due or owed is shown there.
	for i := 0; i < 2; i++ {
		if c := row.Cells[i]; c.ScheduledTotal != "0" || c.Upcoming != "0" || c.ClosingBalance != "0" {
			t.Errorf("cell %d before the start = %+v, want nothing scheduled, due or owed", i, c)
		}
	}
	if c := row.Cells[2]; c.ScheduledTotal != "2000" || c.Upcoming != "2000" {
		t.Errorf("June cell = %+v, want 2000 scheduled and due", c)
	}
	if row.Cells[1].Planned != "" {
		t.Errorf("May's stored plan is out of range, planned = %q", row.Cells[1].Planned)
	}
	if row.Cells[2].Planned != "2000" {
		t.Errorf("June planned = %q, want 2000", row.Cells[2].Planned)
	}
	if row.PayoffDate != row.BasePayoffDate {
		t.Errorf("an out-of-range plan moved the payoff: %s vs %s", row.PayoffDate, row.BasePayoffDate)
	}
	if may := debtRow(h.budgetAt(t, h.owner, "2026-05-01"), debtID); may != nil {
		t.Errorf("May is before the debt's start, row = %+v", may)
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

// createLooseDebt makes a debt with no terms on the loan category: payment
// and schedule only, so it is not set up.
func (h *attrHarness) createLooseDebt(t *testing.T, extra func(*model.CreateDebtRequest)) string {
	t.Helper()
	req := model.CreateDebtRequest{
		Name: "Loose", CurrencyId: fixture.USD, CategoryId: h.loan,
		Payment: vo.NewFlexString("1000"), Schedule: "monthly",
	}
	if extra != nil {
		extra(&req)
	}
	res, err := h.debts.CreateDebt(h.ctx, h.owner, req)
	if err != nil {
		t.Fatalf("CreateDebt: %v", err)
	}
	if res.IsSetUp != 0 {
		t.Fatalf("debt without terms reads set up: %+v", res)
	}
	return res.Id
}

func (h *attrHarness) labelTx(t *testing.T, txID string, labelIDs ...string) {
	t.Helper()
	for _, l := range labelIDs {
		if _, err := h.tdb.Raw.ExecContext(h.ctx, h.tdb.Rebind("INSERT INTO transactions_labels (transaction_id, label_id) VALUES (?, ?)"), txID, l); err != nil {
			t.Fatalf("label %s: %v", txID, err)
		}
	}
}

// backdateDebt moves a debt's creation instant, which decides which of two
// debts matching one payment is the older.
func (h *attrHarness) backdateDebt(t *testing.T, debtID string, at time.Time) {
	t.Helper()
	if _, err := h.tdb.Raw.ExecContext(h.ctx, h.tdb.Rebind("UPDATE debts SET created_at = ? WHERE id = ?"), at.UTC().Format("2006-01-02 15:04:05"), debtID); err != nil {
		t.Fatalf("backdate %s: %v", debtID, err)
	}
}

func TestGetBudget_NotSetUpDebtRow(t *testing.T) {
	h := newAttrHarness(t)
	h.spend(h.usd, h.loan, "", "800", time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC))
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	debtID := h.createLooseDebt(t, nil)
	h.setLimit(t, debtID, "2026-06-01", "1200")

	res := h.budgetAt(t, h.owner, "2026-06-01")
	if got := spentOf(t, res, h.loan, model.ElementCategory); got != "0" {
		t.Errorf("loan spent = %s, want 0 (the payments moved to the debt)", got)
	}
	june := debtRow(res, debtID)
	if june == nil {
		t.Fatal("no row for the debt without terms")
	}
	if june.IsSetUp != 0 || june.IsPlannable != 1 || june.IsPaidOff != 0 {
		t.Errorf("June flags = %+v, want not set up, plannable", june)
	}
	if june.Spent != "1000" || june.BudgetSpent != "1000" || june.Budgeted != "1200" || june.IsBudgeted != 1 || june.Available != "-600" {
		t.Errorf("June planned/paid = %+v, want 1200 planned, 1000 paid, -600 available (May's 800 unplanned)", june)
	}
	if june.ClosingBalance != "" || june.PaymentsLeft != 0 || june.ScheduledTotal != "0" || june.Upcoming != "0" {
		t.Errorf("June schedule = %+v, want no balance, no payments left, nothing scheduled or due", june)
	}
	if june.PayoffDate != "" || june.BasePayoffDate != "" || june.OverpaymentLeft != "" || june.BaseOverpaymentLeft != "" {
		t.Errorf("June payoff = %+v, want every payoff figure empty", june)
	}

	// Plannable from the first payment's month, with no end.
	if may := debtRow(h.budgetAt(t, h.owner, "2026-05-01"), debtID); may == nil || may.IsPlannable != 1 || may.Spent != "800" {
		t.Errorf("May row = %+v, want plannable with 800 paid", may)
	}
	if april := debtRow(h.budgetAt(t, h.owner, "2026-04-01"), debtID); april != nil {
		t.Errorf("April is before the first payment, row = %+v", april)
	}
	a := vo.NewFlexString("10")
	_, err := h.svc.SetLimit(h.ctx, h.owner, model.SetLimitRequest{BudgetId: h.budgetID, ElementId: debtID, Period: "2026-04-01", Amount: &a})
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || len(ve.Fields) == 0 || ve.Fields[0].Code != errs.CodeBudgetDebtPeriodOutOfRange {
		t.Errorf("SetLimit April = %v, want %s", err, errs.CodeBudgetDebtPeriodOutOfRange)
	}
	h.setLimit(t, debtID, "2031-01-01", "500")
	if _, err := h.svc.CreateComment(h.ctx, h.owner, model.CreateCommentRequest{
		Id: vo.NewId().String(), BudgetId: h.budgetID, ElementId: debtID, Period: "2031-01-01", Comment: "later",
	}); err != nil {
		t.Errorf("CreateComment 2031: %v, want accepted (no end until set up)", err)
	}
	if far := debtRow(h.budgetAt(t, h.owner, "2031-01-01"), debtID); far == nil || far.IsPlannable != 1 || far.Budgeted != "500" {
		t.Errorf("2031 row = %+v, want plannable with the 500 plan (no end until set up)", far)
	}

	row := planDebtRow(h.plan(t, "2026-04-01", "4"), debtID)
	if row == nil {
		t.Fatal("no plan row for the debt without terms")
	}
	if row.IsSetUp != 0 || row.PayoffDate != "" || row.BasePayoffDate != "" || row.OverpaymentLeft != "" || row.BaseOverpaymentLeft != "" {
		t.Errorf("plan row = %+v, want not set up, payoff figures empty", row)
	}
	for i, want := range []struct {
		plannable       int
		actual, planned string
	}{{0, "0", ""}, {1, "800", ""}, {1, "1000", "1200"}, {1, "0", ""}} {
		c := row.Cells[i]
		if c.IsPlannable != want.plannable || c.Actual != want.actual || c.Planned != want.planned {
			t.Errorf("cell %d = %+v, want plannable %d, actual %s, planned %q", i, c, want.plannable, want.actual, want.planned)
		}
		if c.ClosingBalance != "" || c.PaymentsLeft != 0 || c.ScheduledTotal != "0" || c.Upcoming != "0" {
			t.Errorf("cell %d schedule = %+v, want empty balance, nothing scheduled or due", i, c)
		}
	}
}

func TestGetBudget_SharedLabelPaymentOnOlderDebtRow(t *testing.T) {
	h := newAttrHarness(t)
	car := h.f.Label(fixture.Label{UserID: h.owner.String(), Name: "Car"})
	student := h.f.Label(fixture.Label{UserID: h.owner.String(), Name: "Student"})
	carOnly := h.spend(h.usd, h.loan, "", "300", time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC))
	h.labelTx(t, carOnly, car)
	both := h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	h.labelTx(t, both, car, student)
	h.spend(h.usd, h.loan, "", "40", time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC))

	carDebt := h.createLooseDebt(t, func(r *model.CreateDebtRequest) { r.Name, r.LabelId = "Car", &car })
	studentDebt := h.createRefDebt(t, func(r *model.CreateDebtRequest) { r.Name, r.LabelId = "Student", &student })
	// The student loan is the older debt, though created second.
	h.backdateDebt(t, studentDebt, attrToday.AddDate(0, -1, 0))

	res := h.budgetAt(t, h.owner, "2026-06-01")
	if got := spentOf(t, res, h.loan, model.ElementCategory); got != "40" {
		t.Errorf("loan spent = %s, want 40 (the unlabelled payment only)", got)
	}
	if r := debtRow(res, studentDebt); r == nil || r.Spent != "1000" || r.IsSetUp != 1 {
		t.Errorf("student row = %+v, want the shared payment's 1000, set up", r)
	}
	if r := debtRow(res, carDebt); r == nil || r.Spent != "300" {
		t.Errorf("car row = %+v, want only its own 300", r)
	}

	plan := h.plan(t, "2026-06-01", "1")
	if r := planDebtRow(plan, studentDebt); r == nil || r.Cells[0].Actual != "1000" || r.IsSetUp != 1 {
		t.Errorf("student plan row = %+v, want 1000", r)
	}
	if r := planDebtRow(plan, carDebt); r == nil || r.Cells[0].Actual != "300" {
		t.Errorf("car plan row = %+v, want 300", r)
	}
}

// planAndComments reads the plans ("2006-01" -> amount) and the comment
// months of the external id's row in the harness budget.
func (h *attrHarness) planAndComments(t *testing.T, externalID string) (map[string]string, []string) {
	t.Helper()
	plans := map[string]string{}
	rows, err := h.tdb.Raw.QueryContext(h.ctx, h.tdb.Rebind(`SELECT l.period, l.amount FROM budgets_elements_limits l
		JOIN budgets_elements e ON e.id = l.element_id WHERE e.budget_id = ? AND e.external_id = ?`), h.budgetID, externalID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var p time.Time
		var a string
		if err := rows.Scan(&p, &a); err != nil {
			t.Fatal(err)
		}
		plans[p.Format("2006-01")] = vo.NewDecimal(a).String()
	}
	rows.Close()
	var comments []string
	rows, err = h.tdb.Raw.QueryContext(h.ctx, h.tdb.Rebind(`SELECT c.period FROM budgets_elements_comments c
		JOIN budgets_elements e ON e.id = c.element_id WHERE e.budget_id = ? AND e.external_id = ? ORDER BY c.period`), h.budgetID, externalID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var p time.Time
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		comments = append(comments, p.Format("2006-01"))
	}
	rows.Close()
	return plans, comments
}

// A whole-category debt created without terms takes the category's plans
// with no end; finishing setup with an end hands the months after it back to
// the category, where they show again, with their comments.
func TestFinishSetup_PlansPastTheEndReturnToTheCategory(t *testing.T) {
	h := newAttrHarness(t)
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i <= 24; i++ {
		h.setLimit(t, h.loan, start.AddDate(0, i, 0).Format("2006-01-02"), "100")
	}
	for _, m := range []string{"2026-09-01", "2027-03-01"} {
		if _, err := h.svc.CreateComment(h.ctx, h.owner, model.CreateCommentRequest{
			Id: vo.NewId().String(), BudgetId: h.budgetID, ElementId: h.loan, Period: m, Comment: "note",
		}); err != nil {
			t.Fatal(err)
		}
	}
	debtID := h.createLooseDebt(t, nil)
	if plans, _ := h.planAndComments(t, debtID); len(plans) != 25 {
		t.Fatalf("debt plans after conversion = %d months, want all 25", len(plans))
	}

	if _, err := h.debts.UpdateDebt(h.ctx, h.owner, model.UpdateDebtRequest{
		Id: debtID, Name: "Loose", CurrencyId: fixture.USD, CategoryId: h.loan,
		StartDate: &attrStart, AmountBorrowed: vo.NewFlexString("5800"), Balance: vo.NewFlexString("5800"), Payment: vo.NewFlexString("1000"), Schedule: "monthly", LastPaymentDate: "2026-12-01",
	}); err != nil {
		t.Fatal(err)
	}
	debtPlans, debtComments := h.planAndComments(t, debtID)
	catPlans, catComments := h.planAndComments(t, h.loan)
	if len(debtPlans) != 7 || debtPlans["2026-06"] != "100" || debtPlans["2026-12"] != "100" {
		t.Errorf("debt plans = %v, want June to December 2026", debtPlans)
	}
	if len(catPlans) != 18 || catPlans["2027-01"] != "100" || catPlans["2028-06"] != "100" {
		t.Errorf("category plans = %v, want January 2027 to June 2028", catPlans)
	}
	if len(debtComments) != 1 || debtComments[0] != "2026-09" || len(catComments) != 1 || catComments[0] != "2027-03" {
		t.Errorf("comments: debt %v, category %v; want September on the debt, March 2027 back on the category", debtComments, catComments)
	}
	if el := findElement(h.budgetAt(t, h.owner, "2027-01-01"), h.loan, model.ElementCategory); el == nil || el.Budgeted != "100" {
		t.Errorf("January 2027 category row = %+v, want its 100 plan shown again", el)
	}
}

func plannedTotal(t *testing.T, res *model.GetBudgetResult) float64 {
	t.Helper()
	var total float64
	add := func(s string) {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			t.Fatalf("budgeted %q: %v", s, err)
		}
		total += v
	}
	for _, el := range res.Item.Structure.Elements {
		add(el.Budgeted)
	}
	for _, d := range res.Item.Structure.Debts {
		add(d.Budgeted)
	}
	return total
}

// A category inside an envelope never counts its own plans: converting it to
// a whole-category debt leaves those hidden plans where they are instead of
// surfacing them on the debt row, so the budget's planned total is unchanged.
func TestConvert_EnvelopeChildKeepsItsHiddenPlans(t *testing.T) {
	h := newAttrHarness(t)
	h.setLimit(t, h.loan, "2026-06-01", "300")
	envelope := vo.NewId().String()
	if _, err := h.svc.CreateEnvelope(h.ctx, h.owner, model.CreateEnvelopeRequest{BudgetId: h.budgetID, Id: envelope,
		Name: "Housing", Icon: "home", CurrencyId: fixture.USD, Categories: []string{h.loan}}); err != nil {
		t.Fatal(err)
	}
	h.setLimit(t, envelope, "2026-06-01", "500")
	before := plannedTotal(t, h.budgetAt(t, h.owner, "2026-06-01"))

	debtID := h.createRefDebt(t, nil)
	res := h.budgetAt(t, h.owner, "2026-06-01")
	if after := plannedTotal(t, res); after != before {
		t.Fatalf("planned total = %v after converting, want %v", after, before)
	}
	if row := debtRow(res, debtID); row == nil || row.IsBudgeted != 0 {
		t.Fatalf("debt row = %+v, want it without the category's hidden plan", row)
	}
}
