package budget_test

import (
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func (h *flowHarness) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := h.tdb.Raw.ExecContext(h.ctx, h.tdb.Rebind(query), args...); err != nil {
		t.Fatal(err)
	}
}

func (h *flowHarness) budget(t *testing.T, startedAt string, accounts ...string) string {
	t.Helper()
	id := h.f.Budget(fixture.Budget{UserID: h.user, StartedAt: startedAt})
	for _, a := range accounts {
		h.f.BudgetAccount(id, a)
	}
	return id
}

func decEq(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	return vo.NewDecimal(a).Equals(vo.NewDecimal(b))
}

func findPlanElement(plan model.BudgetPlanResult, row model.ReportRow, income bool) (model.PlanElementResult, bool) {
	for _, el := range plan.Structure.Elements {
		isIncome := el.Type == 3 || el.Type == 4
		if el.Id == row.Id && isIncome == income {
			return el, true
		}
	}
	return model.PlanElementResult{}, false
}

func TestFlowReport_Budget_MatchesPlan(t *testing.T) {
	h := newFlowHarness(t)
	sav := h.f.Account(fixture.Account{UserID: h.user, Name: "Savings"})
	ext := h.f.Account(fixture.Account{UserID: h.user, Name: "Outside"})
	b := h.budget(t, "2024-01-01 00:00:00", h.acct, sav)
	h.exec(t, "UPDATE budgets_accounts SET is_savings = ? WHERE budget_id = ? AND account_id = ?", true, b, sav)

	food := h.f.Category(fixture.Category{UserID: h.user, Name: "Food", Type: 0})
	fun := h.f.Category(fixture.Category{UserID: h.user, Name: "Fun", Type: 0})
	rent := h.f.Category(fixture.Category{UserID: h.user, Name: "Rent", Type: 0})
	idle := h.f.Category(fixture.Category{UserID: h.user, Name: "Idle", Type: 0})
	salary := h.f.Category(fixture.Category{UserID: h.user, Name: "Salary", Type: 1})
	trip := h.f.Tag(fixture.Tag{UserID: h.user, Name: "Trip"})
	env := h.f.BudgetEnvelope(fixture.BudgetEnvelope{BudgetID: b, Name: "Leisure", Icon: "star"})
	h.f.EnvelopeCategory(env, fun)
	h.f.EnvelopeCategory(env, rent)
	envEl := h.f.BudgetElement(fixture.BudgetElement{BudgetID: b, ExternalID: env, Type: 0})
	foodEl := h.f.BudgetElement(fixture.BudgetElement{BudgetID: b, ExternalID: food, Type: 1})
	savEl := h.f.BudgetElement(fixture.BudgetElement{BudgetID: b, ExternalID: sav, Type: 5})
	h.f.BudgetLimit(fixture.BudgetLimit{ElementID: foodEl, Period: "2024-04-01 00:00:00", Amount: "100"})
	h.f.BudgetLimit(fixture.BudgetLimit{ElementID: envEl, Period: "2024-03-01 00:00:00", Amount: "50"})
	h.f.BudgetLimit(fixture.BudgetLimit{ElementID: savEl, Period: "2024-05-01 00:00:00", Amount: "30"})

	h.tx(0, h.acct, food, "", "", "12", "2024-03-03 10:00:00")
	h.tx(0, h.acct, food, "", "", "40", "2024-04-03 10:00:00")
	h.tx(0, h.acct, fun, "", "", "7", "2024-03-05 10:00:00")
	h.tx(0, h.acct, rent, "", "", "300", "2024-05-01 00:00:00")
	h.tx(0, h.acct, food, "", trip, "25", "2024-04-10 10:00:00")
	h.tx(0, h.acct, "", "", trip, "5", "2024-05-10 10:00:00")
	h.tx(0, h.acct, "", "", "", "3", "2024-04-11 10:00:00")
	h.tx(1, h.acct, salary, "", "", "1000", "2024-03-01 09:00:00")
	h.tx(1, h.acct, "", "", "", "20", "2024-05-15 09:00:00")
	h.f.Transaction(fixture.Transaction{UserID: h.user, AccountID: h.acct, AccountRecipientID: sav, Type: 2, Amount: "200", AmountRecipient: "200", SpentAt: "2024-04-20 10:00:00"})
	h.f.Transaction(fixture.Transaction{UserID: h.user, AccountID: ext, AccountRecipientID: h.acct, Type: 2, Amount: "60", AmountRecipient: "60", SpentAt: "2024-05-20 10:00:00"})

	planRes, err := h.svc.GetBudgetPlan(h.ctx, vo.MustParseId(h.user), model.GetBudgetPlanRequest{Id: b, From: "2024-03-01", Months: "3"})
	if err != nil {
		t.Fatal(err)
	}
	plan := planRes.Item
	r := h.report(t, model.GetFlowReportRequest{BudgetId: b, From: "2024-03-01", To: "2024-05-01"})

	if r.CurrencyId != fixture.USD || len(r.Months) != 3 || r.Months[0] != "2024-03-01" || r.Months[2] != "2024-05-01" {
		t.Fatalf("currency/months = %s %v", r.CurrencyId, r.Months)
	}
	check := func(rows []model.ReportRow, income bool) map[string]bool {
		seen := map[string]bool{}
		for _, row := range rows {
			el, ok := findPlanElement(plan, row, income)
			if !ok {
				t.Fatalf("report row %s/%s has no plan element", row.Kind, row.Name)
			}
			seen[el.Id] = true
			if len(row.Amounts) != 3 || len(row.Planned) != 3 {
				t.Fatalf("row %s lengths: %+v", row.Name, row)
			}
			for i, c := range el.Cells {
				if !decEq(row.Amounts[i], c.Actual) || !decEq(row.Planned[i], c.Planned) {
					t.Fatalf("row %s month %d: report (%s, %q) plan (%s, %q)", row.Name, i, row.Amounts[i], row.Planned[i], c.Actual, c.Planned)
				}
			}
			for _, ch := range row.Children {
				var pc *model.PlanChildResult
				for k := range el.Children {
					if el.Children[k].Id == ch.Id {
						pc = &el.Children[k]
					}
				}
				if pc == nil {
					t.Fatalf("child %s of %s missing in plan", ch.Name, row.Name)
				}
				for i, c := range pc.Cells {
					if !decEq(ch.Amounts[i], c.Actual) {
						t.Fatalf("child %s month %d: %s vs %s", ch.Name, i, ch.Amounts[i], c.Actual)
					}
				}
			}
		}
		return seen
	}
	seenExp := check(r.Expense, false)
	seenInc := check(r.Income, true)
	for _, el := range plan.Structure.Elements {
		active := false
		for _, c := range el.Cells {
			if !vo.NewDecimal(c.Actual).IsZero() || c.Planned != "" {
				active = true
			}
		}
		seen := seenExp
		if el.Type == 3 || el.Type == 4 {
			seen = seenInc
		}
		if active && !seen[el.Id] {
			t.Fatalf("plan element %s (type %d) missing from report", el.Name, el.Type)
		}
	}
	for _, row := range r.Expense {
		if row.Id == idle {
			t.Fatal("inactive category must be omitted")
		}
	}

	env0 := rowByName(t, r.Expense, "Leisure")
	if env0.Kind != "envelope" || len(env0.Children) != 2 {
		t.Fatalf("envelope row = %+v", env0)
	}
	eqAmounts(t, env0.Amounts, "7", "0", "300")
	tag0 := rowByName(t, r.Expense, "Trip")
	if tag0.Kind != "tag" || len(tag0.Children) != 2 {
		t.Fatalf("tag row = %+v", tag0)
	}
	foodRow := rowByName(t, r.Expense, "Food")
	if foodRow.Planned[0] != "" || !decEq(foodRow.Planned[1], "100") || !decEq(env0.Planned[0], "50") {
		t.Fatalf("planned food %q envelope %q", foodRow.Planned, env0.Planned)
	}
	if foodRow.Kind != "category" || rowByName(t, r.Income, "Salary").Kind != "income_category" {
		t.Fatal("kinds")
	}
	eqAmounts(t, rowByName(t, r.Expense, model.UncategorizedName).Amounts, "0", "3", "0")
	eqAmounts(t, rowByName(t, r.Income, model.UncategorizedName).Amounts, "0", "0", "20")

	if len(r.Savings) != 1 || len(plan.Structure.Savings) != 1 {
		t.Fatalf("savings = %+v", r.Savings)
	}
	s0, p0 := r.Savings[0], plan.Structure.Savings[0]
	if s0.Kind != "savings" || s0.Id != sav {
		t.Fatalf("savings row = %+v", s0)
	}
	for i, c := range p0.Cells {
		if !decEq(s0.Amounts[i], c.Actual) || !decEq(s0.Planned[i], c.Planned) {
			t.Fatalf("savings month %d: (%s, %q) vs (%s, %q)", i, s0.Amounts[i], s0.Planned[i], c.Actual, c.Planned)
		}
	}
	eqAmounts(t, s0.Amounts, "0", "200", "0")
	if !decEq(s0.Planned[2], "30") {
		t.Fatalf("savings planned = %q", s0.Planned)
	}
	eqAmounts(t, r.TransfersIn, "0", "0", "60")
	eqAmounts(t, r.TransfersOut, "0", "0", "0")
}

func TestFlowReport_Budget_IncludesMonthsBeforeStart(t *testing.T) {
	h := newFlowHarness(t)
	b := h.budget(t, "2024-05-01 00:00:00", h.acct)
	food := h.f.Category(fixture.Category{UserID: h.user, Name: "Food", Type: 0})
	h.tx(0, h.acct, food, "", "", "9", "2023-11-04 10:00:00")
	h.tx(0, h.acct, food, "", "", "4", "2024-05-04 10:00:00")
	r := h.report(t, model.GetFlowReportRequest{BudgetId: b, To: "2024-05-01"})
	if len(r.Months) != 7 || r.Months[0] != "2023-11-01" {
		t.Fatalf("months = %v", r.Months)
	}
	eqAmounts(t, rowByName(t, r.Expense, "Food").Amounts, "9", "0", "0", "0", "0", "0", "4")
}

func TestFlowReport_Budget_EndedBudgetCapped(t *testing.T) {
	h := newFlowHarness(t)
	b := h.budget(t, "2024-01-01 00:00:00", h.acct)
	h.exec(t, "UPDATE budgets SET ended_at = ? WHERE id = ?", "2024-04-01 00:00:00", b)
	food := h.f.Category(fixture.Category{UserID: h.user, Name: "Food", Type: 0})
	h.tx(0, h.acct, food, "", "", "9", "2024-02-04 10:00:00")
	h.tx(0, h.acct, food, "", "", "4", "2024-06-04 10:00:00")
	r := h.report(t, model.GetFlowReportRequest{BudgetId: b})
	if len(r.Months) != 3 || r.Months[0] != "2024-02-01" || r.Months[2] != "2024-04-01" {
		t.Fatalf("months = %v", r.Months)
	}
}

func TestFlowReport_Budget_ElementCurrencyConverted(t *testing.T) {
	h := newFlowHarness(t)
	eur := h.f.Currency(fixture.Currency{Code: "EUR", Symbol: "E"})
	h.f.Rate(fixture.Rate{CurrencyID: eur, BaseCurrencyID: fixture.USD, Rate: "0.5", PublishedAt: "2024-03-01"})
	b := h.budget(t, "2024-01-01 00:00:00", h.acct)
	food := h.f.Category(fixture.Category{UserID: h.user, Name: "Food", Type: 0})
	el := h.f.BudgetElement(fixture.BudgetElement{BudgetID: b, ExternalID: food, Type: 1, CurrencyID: eur})
	h.f.BudgetLimit(fixture.BudgetLimit{ElementID: el, Period: "2024-03-01 00:00:00", Amount: "20"})
	h.tx(0, h.acct, food, "", "", "10", "2024-03-04 10:00:00")

	planRes, err := h.svc.GetBudgetPlan(h.ctx, vo.MustParseId(h.user), model.GetBudgetPlanRequest{Id: b, From: "2024-03-01", Months: "1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, pe := range planRes.Item.Structure.Elements {
		if pe.Id == food && !decEq(pe.Cells[0].Actual, "5") {
			t.Fatalf("plan actual (EUR) = %s", pe.Cells[0].Actual)
		}
	}
	r := h.report(t, model.GetFlowReportRequest{BudgetId: b, From: "2024-03-01", To: "2024-03-01"})
	row := rowByName(t, r.Expense, "Food")
	eqAmounts(t, row.Amounts, "10")
	eqAmounts(t, row.Planned, "40")
}

func TestFlowReport_Budget_RejectsPayeeGrouping(t *testing.T) {
	h := newFlowHarness(t)
	b := h.budget(t, "2024-01-01 00:00:00", h.acct)
	for _, g := range []string{"payee", "tag"} {
		_, err := h.svc.GetFlowReport(h.ctx, vo.MustParseId(h.user), model.GetFlowReportRequest{BudgetId: b, GroupBy: g})
		var ve *errs.ValidationError
		if !errors.As(err, &ve) || len(ve.Fields) != 1 || ve.Fields[0].Key != "groupBy" {
			t.Fatalf("groupBy=%s: err = %v", g, err)
		}
	}
	if _, err := h.svc.GetFlowReport(h.ctx, vo.MustParseId(h.user), model.GetFlowReportRequest{BudgetId: b, GroupBy: "category"}); err != nil {
		t.Fatal(err)
	}
}

func TestFlowReport_Budget_NoAccess(t *testing.T) {
	h := newFlowHarness(t)
	other := h.f.User(fixture.User{ID: "ffff0000-0000-7000-8000-0000000000bb", Name: "Other", Email: "other@flow.test"})
	h.f.DefaultOptions(other)
	b := h.f.Budget(fixture.Budget{UserID: other})
	_, reportErr := h.svc.GetFlowReport(h.ctx, vo.MustParseId(h.user), model.GetFlowReportRequest{BudgetId: b})
	_, planErr := h.svc.GetBudgetPlan(h.ctx, vo.MustParseId(h.user), model.GetBudgetPlanRequest{Id: b})
	if reportErr == nil || planErr == nil || reportErr.Error() != planErr.Error() {
		t.Fatalf("report err %v, plan err %v", reportErr, planErr)
	}
	var ad *errs.AccessDeniedError
	if !errors.As(reportErr, &ad) {
		t.Fatalf("want access denied, got %T", reportErr)
	}
}
