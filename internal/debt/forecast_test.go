package debt_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

type fakeBudgetPlans struct {
	active  string
	budgets map[string]model.DebtBudgetPlans
	asked   []string
}

func (f *fakeBudgetPlans) ActiveBudget(context.Context, vo.Id) (*vo.Id, error) {
	if f.active == "" {
		return nil, nil
	}
	id := vo.MustParseId(f.active)
	return &id, nil
}

func (f *fakeBudgetPlans) DebtPlans(_ context.Context, _, budgetID, _ vo.Id) (model.DebtBudgetPlans, bool, error) {
	f.asked = append(f.asked, budgetID.String())
	p, ok := f.budgets[budgetID.String()]
	return p, ok, nil
}

func (e *env) forecast(t *testing.T, id, budgetID string) *model.DebtForecastResult {
	t.Helper()
	got, err := e.svc.GetDebtForecast(context.Background(), e.uid(), model.GetDebtForecastRequest{Id: id, BudgetId: budgetID})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestGetDebtForecast_ScheduleOnlyRunsToPayoff(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)

	got := e.forecast(t, res.Id, "")
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

	got := e.forecast(t, res.Id, "")
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

// The budget the screen was opened from adds its planned extras; the note
// names it.
func TestGetDebtForecast_FollowsTheGivenBudgetsPlans(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	budget := fixture.NewID()
	plans := &fakeBudgetPlans{budgets: map[string]model.DebtBudgetPlans{
		budget: {Name: "Family", PeriodStartDay: 1, Plans: map[string]string{"2026-05-01": "3000"}},
	}}
	e.svc.SetBudgetPlans(plans)

	base := e.forecast(t, res.Id, "")
	got := e.forecast(t, res.Id, budget)
	if got.BudgetId != budget || got.BudgetName != "Family" {
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
}

// From Settings there is no budget: the active one is used when it plans the
// debt; a budget the caller cannot read, or one without extras, leaves the
// schedule alone and names no budget.
func TestGetDebtForecast_ActiveBudgetOrScheduleOnly(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	active, foreign, flat := fixture.NewID(), fixture.NewID(), fixture.NewID()
	plans := &fakeBudgetPlans{active: active, budgets: map[string]model.DebtBudgetPlans{
		active: {Name: "Home", PeriodStartDay: 1, Plans: map[string]string{"2026-06-01": "2500"}},
		flat:   {Name: "Flat", PeriodStartDay: 1, Plans: map[string]string{"2026-06-01": "1000"}},
	}}
	e.svc.SetBudgetPlans(plans)

	if got := e.forecast(t, res.Id, ""); got.BudgetId != active || got.BudgetName != "Home" {
		t.Fatalf("no budget given: %q %q, want the active one", got.BudgetId, got.BudgetName)
	}
	schedule := len(e.forecast(t, res.Id, flat).Items)
	if got := e.forecast(t, res.Id, flat); got.BudgetId != "" || got.BudgetName != "" {
		t.Fatalf("plans at the regular payment: %q %q, want no budget named", got.BudgetId, got.BudgetName)
	}
	got := e.forecast(t, res.Id, foreign)
	if got.BudgetId != "" || len(got.Items) != schedule || len(got.Items) != res.PaymentsLeft {
		t.Fatalf("inaccessible budget: %q, %d items, want the schedule's %d", got.BudgetId, len(got.Items), res.PaymentsLeft)
	}
	// a budget given and refused does not fall back to the active one
	if got.BudgetName != "" {
		t.Fatalf("budget name = %q", got.BudgetName)
	}
}

// A plan outside the debt's planning life never reaches the forecast: a loan
// whose term ended in December, still owing what was never recorded, keeps
// being projected, but May is past its life.
func TestGetDebtForecast_IgnoresPlansOutsideTheLife(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, func(r *model.CreateDebtRequest) { r.StartDate, r.LastPaymentDate = ptr("2025-01-01"), "2025-12-01" })
	budget := fixture.NewID()
	e.svc.SetBudgetPlans(&fakeBudgetPlans{budgets: map[string]model.DebtBudgetPlans{
		budget: {Name: "Family", PeriodStartDay: 1, Plans: map[string]string{"2026-05-01": "5000"}},
	}})

	got := e.forecast(t, res.Id, budget)
	if got.BudgetId != "" || len(got.Items) != res.PaymentsLeft {
		t.Fatalf("forecast = %q with %d items, want the schedule's %d", got.BudgetId, len(got.Items), res.PaymentsLeft)
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
	got := e.forecast(t, res.Id, "")
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
	budget := fixture.NewID()
	e.svc.SetBudgetPlans(&fakeBudgetPlans{budgets: map[string]model.DebtBudgetPlans{
		budget: {Name: "Shifted", PeriodStartDay: 15, Plans: map[string]string{"2026-05-01": "3000"}},
	}})

	got := e.forecast(t, res.Id, budget)
	if got.BudgetName != "Shifted" {
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
	flat := fixture.NewID()
	e.svc.SetBudgetPlans(&fakeBudgetPlans{budgets: map[string]model.DebtBudgetPlans{
		flat: {Name: "Flat", PeriodStartDay: 15, Plans: map[string]string{"2026-06-01": "1000"}},
	}})

	for _, budget := range []string{"", flat} {
		got := e.forecast(t, res.Id, budget)
		if got.PaymentsLeft != res.PaymentsLeft || got.PayoffDate != res.PayoffDate || got.OverpaymentLeft != res.OverpaymentLeft {
			t.Fatalf("budget %q: summary = %d %q %q, want the debt's %d %q %q", budget, got.PaymentsLeft, got.PayoffDate, got.OverpaymentLeft,
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
	budget := fixture.NewID()
	e.svc.SetBudgetPlans(&fakeBudgetPlans{budgets: map[string]model.DebtBudgetPlans{
		budget: {Name: "Family", PeriodStartDay: 1, Plans: map[string]string{"2026-05-01": "4000"}},
	}})

	got := e.forecast(t, res.Id, budget)
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

	got := e.forecast(t, res.Id, "")
	if got.PaymentsLeft != 0 || got.PayoffDate != "" || got.OverpaymentLeft != "" {
		t.Fatalf("summary = %d %q %q, want none", got.PaymentsLeft, got.PayoffDate, got.OverpaymentLeft)
	}
}
