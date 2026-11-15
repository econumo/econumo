package debt_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func (e *env) forecast(t *testing.T, id string) *model.DebtForecastResult {
	t.Helper()
	got, err := e.svc.GetDebtForecast(context.Background(), e.uid(), model.GetDebtForecastRequest{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// planOf sets the plans of the element the debt is attached to.
func (e *env) planOf(t *testing.T, debtID string, plans map[string]string) {
	t.Helper()
	d, err := e.repo.GetByID(context.Background(), vo.MustParseId(debtID))
	if err != nil {
		t.Fatal(err)
	}
	for period, amount := range plans {
		f, err := strconv.ParseFloat(amount, 64)
		if err != nil {
			t.Fatal(err)
		}
		e.setPlan(d.BudgetID.String(), d.ElementID.String(), period, f)
	}
}

func TestGetDebtForecast_ScheduleOnlyRunsToPayoff(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)

	got := e.forecast(t, res.Id)
	if got.BudgetId != "" || got.BudgetName != "" {
		t.Fatalf("budget = %q %q, want none", got.BudgetId, got.BudgetName)
	}
	if len(got.Items) != res.PaymentsLeft || len(got.Items) == 0 {
		t.Fatalf("items = %d, want the %d payments left", len(got.Items), res.PaymentsLeft)
	}
	first, last := got.Items[0], got.Items[len(got.Items)-1]
	if first.Date != res.NextDueDate || first.Payment != "1000" {
		t.Fatalf("first = %+v, want the next due %s at 1000", first, res.NextDueDate)
	}
	if last.Balance != "0" || last.Date != res.PayoffDate {
		t.Fatalf("last = %+v, want balance 0 on %s", last, res.PayoffDate)
	}
	balance := vo.NewDecimal(res.CurrentBalance)
	for _, it := range got.Items {
		if !vo.NewDecimal(it.Payment).Sub(vo.NewDecimal(it.Interest)).Equals(vo.NewDecimal(it.Principal)) {
			t.Fatalf("item %+v: principal is not payment less interest", it)
		}
		balance = balance.Sub(vo.NewDecimal(it.Principal))
		if balance.Sub(vo.NewDecimal(it.Balance)).Abs().IsGreaterThan(vo.NewDecimal("0.02")) {
			t.Fatalf("item %+v: balance drifts from %s", it, balance)
		}
	}
}

func TestGetDebtForecast_NotSetUpHasNoItems(t *testing.T) {
	e := newEnv(t)
	res := e.createWithoutTerms(t, nil)

	got := e.forecast(t, res.Id)
	if got.Items == nil || len(got.Items) != 0 || got.BudgetId != "" {
		t.Fatalf("forecast = %+v, want empty items", got)
	}
}

func TestGetDebtForecast_OtherUserNotFound(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	other := vo.MustParseId(e.f.User(fixture.User{Name: "other", Email: "other@example.test"}))

	_, err := e.svc.GetDebtForecast(context.Background(), other, model.GetDebtForecastRequest{Id: res.Id})
	requireNotFound(t, err)
}

// The debt's own budget adds its planned extras; the note names it. A guest
// of the budget reads the same forecast.
func TestGetDebtForecast_UsesTheDebtsBudget(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	base := e.forecast(t, res.Id)
	e.planOf(t, res.Id, map[string]string{"2026-05-01": "3000"})

	got := e.forecast(t, res.Id)
	if got.BudgetId != e.budget || got.BudgetName != "Family" {
		t.Fatalf("budget = %q %q", got.BudgetId, got.BudgetName)
	}
	if len(got.Items) >= len(base.Items) {
		t.Fatalf("items = %d, want fewer than the schedule's %d", len(got.Items), len(base.Items))
	}
	var may *model.DebtForecastItem
	for i := range got.Items {
		if got.Items[i].Date[:7] == "2026-05" {
			may = &got.Items[i]
		}
	}
	if may == nil || may.Payment != "3000" {
		t.Fatalf("May = %+v, want the regular 1000 and the 2000 extra", may)
	}
	if got.Items[len(got.Items)-1].Balance != "0" {
		t.Fatalf("last = %+v", got.Items[len(got.Items)-1])
	}
	guest := e.partner(t, "guest", roleGuest)
	byGuest, err := e.svc.GetDebtForecast(context.Background(), uidOf(guest), model.GetDebtForecastRequest{Id: res.Id})
	if err != nil || len(byGuest.Items) != len(got.Items) {
		t.Fatalf("guest forecast = %+v, %v", byGuest, err)
	}
}

// Plans at the regular payment add no extra: the forecast names no budget.
func TestGetDebtForecast_PlansWithoutExtrasNameNoBudget(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	e.planOf(t, res.Id, map[string]string{"2026-06-01": "1000"})
	got := e.forecast(t, res.Id)
	if got.BudgetId != "" || got.BudgetName != "" || len(got.Items) != res.PaymentsLeft {
		t.Fatalf("forecast = %q %q with %d items, want the schedule's %d", got.BudgetId, got.BudgetName, len(got.Items), res.PaymentsLeft)
	}
}

// A loan whose term ended in December, still owing what was never recorded,
// keeps being projected, and its planning life runs to that payoff: a May
// plan is an extra.
func TestGetDebtForecast_PlansPastAnEndedTermWhileStillOwing(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, func(r *createSpec) { r.StartDate, r.LastPaymentDate = ptr("2025-01-01"), "2025-12-01" })
	e.planOf(t, res.Id, map[string]string{"2026-05-01": "5000"})

	got := e.forecast(t, res.Id)
	if got.BudgetId != e.budget || len(got.Items) >= res.PaymentsLeft {
		t.Fatalf("forecast = %q with %d items, want fewer than the schedule's %d", got.BudgetId, len(got.Items), res.PaymentsLeft)
	}
}

// A payment recorded ahead of today is not projected again: the forecast starts
// after the last recorded payment, from the balance that payment leaves.
func TestGetDebtForecast_StartsAfterAFutureDatedPayment(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	e.expense(t, e.account, ymd(2026, 4, 1), "1000", "")

	details, err := e.svc.GetDebt(context.Background(), e.uid(), model.GetDebtRequest{Id: res.Id})
	if err != nil {
		t.Fatal(err)
	}
	if details.Item.NextDueDate != "2026-04-01" {
		t.Fatalf("next due = %s: the figures still count the April due as unpaid today", details.Item.NextDueDate)
	}
	got := e.forecast(t, res.Id)
	first := got.Items[0]
	if first.Date != "2026-05-01" {
		t.Fatalf("first = %+v, want the May due: April is recorded", first)
	}
	top := vo.NewDecimal(details.Payments[0].Balance)
	if want := top.Sub(vo.NewDecimal(first.Principal)); !vo.NewDecimal(first.Balance).Equals(want) {
		t.Fatalf("first balance %s, want %s (the recorded payment's %s less %s)", first.Balance, want, top, first.Principal)
	}
	if last := got.Items[len(got.Items)-1]; last.Balance != "0" {
		t.Fatalf("last = %+v", last)
	}
}

// On a budget whose periods start on the 15th, the period keyed May runs from
// May 15 to June 14: its extra rides on the June 1 payment, not May's.
func TestGetDebtForecast_FollowsTheBudgetsPeriods(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	e.fakeBudget(e.budget).startDay = 15
	e.planOf(t, res.Id, map[string]string{"2026-05-01": "3000"})

	got := e.forecast(t, res.Id)
	if got.BudgetName != "Family" {
		t.Fatalf("budget = %q", got.BudgetName)
	}
	byDate := map[string]string{}
	for _, it := range got.Items {
		byDate[it.Date] = it.Payment
	}
	if byDate["2026-05-01"] != "1000" || byDate["2026-06-01"] != "3000" {
		t.Fatalf("May %q, June %q; want the extra on June 1", byDate["2026-05-01"], byDate["2026-06-01"])
	}
}

// With no plans to follow, the summary is get-debt's own figures; a budget's
// plans change it.
func TestGetDebtForecast_SummaryMatchesTheDebtWithoutPlans(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	for _, flat := range []bool{false, true} {
		if flat {
			e.fakeBudget(e.budget).startDay = 15
			e.planOf(t, res.Id, map[string]string{"2026-06-01": "1000"})
		}
		got := e.forecast(t, res.Id)
		if got.PaymentsLeft != res.PaymentsLeft || got.PayoffDate != res.PayoffDate || got.OverpaymentLeft != res.OverpaymentLeft {
			t.Fatalf("flat plans %v: summary = %d %q %q, want the debt's %d %q %q", flat, got.PaymentsLeft, got.PayoffDate, got.OverpaymentLeft,
				res.PaymentsLeft, res.PayoffDate, res.OverpaymentLeft)
		}
	}
	if res.PaymentsLeft == 0 || res.PayoffDate == "" || !vo.NewDecimal(res.OverpaymentLeft).IsGreaterThan(vo.NewDecimal("0")) {
		t.Fatalf("debt = %+v: the loan should carry interest", res)
	}
}

// A plan above the regular payment pays the loan off sooner: fewer payments,
// less interest, an earlier payoff.
func TestGetDebtForecast_SummaryFollowsThePlans(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	e.planOf(t, res.Id, map[string]string{"2026-05-01": "4000"})

	got := e.forecast(t, res.Id)
	if got.PaymentsLeft >= res.PaymentsLeft {
		t.Fatalf("payments left = %d, want fewer than the schedule's %d", got.PaymentsLeft, res.PaymentsLeft)
	}
	if !vo.NewDecimal(got.OverpaymentLeft).IsLessThan(vo.NewDecimal(res.OverpaymentLeft)) {
		t.Fatalf("overpayment left = %s, want less than the schedule's %s", got.OverpaymentLeft, res.OverpaymentLeft)
	}
	if got.PayoffDate == "" || got.PayoffDate >= res.PayoffDate {
		t.Fatalf("payoff = %q, want before the schedule's %q", got.PayoffDate, res.PayoffDate)
	}
	if last := got.Items[len(got.Items)-1]; last.Date != got.PayoffDate {
		t.Fatalf("payoff = %q, want the last payment's %q", got.PayoffDate, last.Date)
	}
}

func TestGetDebtForecast_SummaryEmptyWhileNotSetUp(t *testing.T) {
	e := newEnv(t)
	res := e.createWithoutTerms(t, nil)

	got := e.forecast(t, res.Id)
	if got.PaymentsLeft != 0 || got.PayoffDate != "" || got.OverpaymentLeft != "" {
		t.Fatalf("summary = %d %q %q, want none", got.PaymentsLeft, got.PayoffDate, got.OverpaymentLeft)
	}
}

// extraPlanned sums the planned extras the forecast applies: a plan of 5000
// in a month due 450 adds 4550; with no plans it is "0".
func TestGetDebtForecast_ExtraPlanned(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, func(r *createSpec) {
		r.AmountBorrowed, r.Payment, r.LastPaymentDate = vo.NewFlexString("16500"), vo.NewFlexString("450"), "2030-03-01"
	})
	if got := e.forecast(t, res.Id); got.ExtraPlanned != "0" {
		t.Fatalf("extra planned with no plans = %q, want 0", got.ExtraPlanned)
	}
	e.planOf(t, res.Id, map[string]string{"2026-05-01": "5000"})
	if got := e.forecast(t, res.Id); got.ExtraPlanned != "4550" {
		t.Fatalf("extra planned = %q, want 4550", got.ExtraPlanned)
	}

	bare := e.createWithoutTerms(t, func(r *createSpec) { r.ElementId = e.newCategory(e.user, true) })
	if got := e.forecast(t, bare.Id); got.ExtraPlanned != "" {
		t.Fatalf("not set up extra planned = %q, want empty", got.ExtraPlanned)
	}
}

// A period's planned extra is the element's plan above the scheduled payment:
// a plan of 3000 against a payment of 2647.82 pays 352.18 extra that month.
func TestProjection_ExtraFromTheElementsPlan(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, func(r *createSpec) {
		r.AmountBorrowed, r.Payment, r.LastPaymentDate = vo.NewFlexString("85000"), vo.NewFlexString("2647.82"), "2029-03-01"
	})
	e.planOf(t, res.Id, map[string]string{"2026-05-01": "3000"})

	got := e.forecast(t, res.Id)
	if got.ExtraPlanned != "352.18" || got.BudgetId != e.budget {
		t.Fatalf("forecast = extra %q, budget %q; want 352.18 from the element's plan", got.ExtraPlanned, got.BudgetId)
	}
	var may string
	for _, it := range got.Items {
		if it.Date == "2026-05-01" {
			may = it.Payment
		}
	}
	if may != "3000" {
		t.Fatalf("May's payment = %q, want 3000 (2647.82 + 352.18)", may)
	}
}

// A rate raised under the same payment repays past the contract's last
// payment date: the planning life runs to the projected payoff, so a plan
// after the contract date is an extra.
func TestGetDebtForecast_PlansPastTheContractWhileStillOwing(t *testing.T) {
	e := newEnv(t)
	for _, m := range []time.Month{time.January, time.February, time.March} {
		e.expense(t, e.account, ymd(2026, m, 1), "1000", "")
	}
	res := e.create(t, func(r *createSpec) {
		r.StartDate, r.AmountBorrowed, r.LastPaymentDate = ptr("2025-12-01"), vo.NewFlexString("6000"), "2026-06-01"
	})
	res = e.mustAddRate(t, res.Id, "2025-12-01", "60", "1000")
	if res.LastPaymentDate != "2026-06-01" || res.PayoffDate <= "2026-07-01" {
		t.Fatalf("debt = last %s payoff %s, want a payoff after July", res.LastPaymentDate, res.PayoffDate)
	}
	views, err := e.svc.Views(context.Background(), e.scope(e.budget))
	if err != nil || len(views) != 1 {
		t.Fatalf("views = %+v, %v", views, err)
	}
	if got := monthString(views[0].LastDay); got != res.PayoffDate {
		t.Fatalf("last planning day = %s, want the projected payoff %s", got, res.PayoffDate)
	}

	e.planOf(t, res.Id, map[string]string{"2026-07-01": "1500"})
	got := e.forecast(t, res.Id)
	if got.ExtraPlanned == "0" || got.PayoffDate >= res.PayoffDate {
		t.Fatalf("forecast = extra %s payoff %s, want July's plan to bring %s forward", got.ExtraPlanned, got.PayoffDate, res.PayoffDate)
	}
}

// A loan refinanced within the period: the element's plan covered the old
// debt's payment too, so the new debt's still-due payment leaves no extra.
func TestGetDebtForecast_RefinancingCountsTheOldDebtsPayments(t *testing.T) {
	e := newEnv(t)
	e.expense(t, e.account, ymd(2026, 3, 1), "600", "")
	old := e.create(t, func(r *createSpec) {
		r.StartDate, r.AmountBorrowed, r.Payment, r.LastPaymentDate = ptr("2025-06-01"), vo.NewFlexString("12000"), vo.NewFlexString("600"), "2027-06-01"
	})
	if _, err := e.svc.MarkDebtPaidOff(context.Background(), e.uid(), model.MarkDebtPaidOffRequest{Id: old.Id, Date: "2026-03-05"}); err != nil {
		t.Fatalf("MarkDebtPaidOff: %v", err)
	}
	newer := e.create(t, func(r *createSpec) {
		r.StartDate, r.AmountBorrowed, r.Payment, r.LastPaymentDate = ptr("2026-03-06"), vo.NewFlexString("6000"), vo.NewFlexString("600"), "2027-02-25"
	})
	e.planOf(t, newer.Id, map[string]string{"2026-03-01": "1200"})

	got := e.forecast(t, newer.Id)
	if got.ExtraPlanned != "0" || got.BudgetId != "" {
		t.Fatalf("forecast = extra %s budget %q, want no extra: the plan covered both debts' payments", got.ExtraPlanned, got.BudgetId)
	}
}

// A category that joined an envelope this period keeps its own paid-off
// debt's payments, which count in the envelope's Spent: the envelope debt's
// forecast does not project them again as extra.
func TestGetDebtForecast_EnvelopePlanCountsAChildCategorysDebtPayments(t *testing.T) {
	e := newEnv(t)
	e.expense(t, e.account, ymd(2026, 3, 1), "1000", "")
	old := e.create(t, func(r *createSpec) {
		r.StartDate, r.AmountBorrowed, r.Payment, r.LastPaymentDate = ptr("2025-06-01"), vo.NewFlexString("12000"), vo.NewFlexString("1000"), "2027-06-01"
	})
	if _, err := e.svc.MarkDebtPaidOff(context.Background(), e.uid(), model.MarkDebtPaidOffRequest{Id: old.Id, Date: "2026-03-05"}); err != nil {
		t.Fatalf("MarkDebtPaidOff: %v", err)
	}
	envelope := e.newEnvelope("Housing", e.category)
	newer := e.create(t, func(r *createSpec) {
		r.ElementId = envelope
		r.StartDate, r.AmountBorrowed, r.Payment, r.LastPaymentDate = ptr("2026-03-06"), vo.NewFlexString("12000"), vo.NewFlexString("1000"), "2027-02-25"
	})
	e.planOf(t, newer.Id, map[string]string{"2026-03-01": "3000"})

	if got := e.forecast(t, newer.Id); got.ExtraPlanned != "1000" {
		t.Fatalf("extra planned = %s, want 1000: the child's 1000 paid and the 1000 still due use 2000 of the 3000 plan", got.ExtraPlanned)
	}
}

// On the 31st the lookback must still reach the 1st: subtracting a month and
// a day from March 31 lands on March 2.
func TestGetDebtForecast_LookbackCoversTheWholePeriodOnTheThirtyFirst(t *testing.T) {
	e := newEnv(t)
	e.clock.now = time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	e.expense(t, e.account, ymd(2026, 3, 1), "600", "")
	old := e.create(t, func(r *createSpec) {
		r.StartDate, r.AmountBorrowed, r.Payment, r.LastPaymentDate = ptr("2025-06-01"), vo.NewFlexString("12000"), vo.NewFlexString("600"), "2027-06-01"
	})
	if _, err := e.svc.MarkDebtPaidOff(context.Background(), e.uid(), model.MarkDebtPaidOffRequest{Id: old.Id, Date: "2026-03-05"}); err != nil {
		t.Fatalf("MarkDebtPaidOff: %v", err)
	}
	newer := e.create(t, func(r *createSpec) {
		r.StartDate, r.AmountBorrowed, r.Payment, r.LastPaymentDate = ptr("2026-03-06"), vo.NewFlexString("6000"), vo.NewFlexString("600"), "2027-03-31"
	})
	e.planOf(t, newer.Id, map[string]string{"2026-03-01": "1200"})

	if got := e.forecast(t, newer.Id); got.ExtraPlanned != "0" {
		t.Fatalf("extra planned = %s, want 0: the March 1 payment still counts", got.ExtraPlanned)
	}
}
