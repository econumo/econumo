package budget_test

import (
	"errors"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func (h *attrHarness) txList(t *testing.T, req model.BudgetTransactionListRequest) *model.GetBudgetTransactionListResult {
	t.Helper()
	req.BudgetId, req.PeriodStart = h.budgetID, "2026-06-01"
	res, err := h.svc.GetTransactionList(h.ctx, h.owner, req)
	if err != nil {
		t.Fatalf("GetTransactionList %+v: %v", req, err)
	}
	return res
}

func itemIDs(res *model.GetBudgetTransactionListResult) []string {
	out := []string{}
	for _, it := range res.Items {
		out = append(out, it.Id)
	}
	return out
}

func upcomingIDs(res *model.GetBudgetTransactionListResult) []string {
	out := []string{}
	for _, it := range res.Upcoming {
		out = append(out, it.Id)
	}
	return out
}

func sameIDs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]int{}
	for _, id := range got {
		seen[id]++
	}
	for _, id := range want {
		if seen[id] == 0 {
			return false
		}
		seen[id]--
	}
	return true
}

func TestTransactionList_DrillDownsLeaveDebtPaymentsOut(t *testing.T) {
	h := newAttrHarness(t)
	env := h.f.BudgetEnvelope(fixture.BudgetEnvelope{BudgetID: h.budgetID, Name: "Home", Icon: "home"})
	h.f.BudgetElement(fixture.BudgetElement{BudgetID: h.budgetID, ExternalID: env, Type: int(model.ElementEnvelope)})
	h.f.EnvelopeCategory(env, h.loan)
	h.f.EnvelopeCategory(env, h.food)
	untagged := h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	h.spend(h.usd, h.loan, h.tag, "1000", time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC))
	food := h.spend(h.usd, h.food, "", "50", time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC))
	foodTagged := h.spend(h.usd, h.food, h.tag, "20", time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC))
	loan, tag, envID := h.loan, h.tag, env

	if got := itemIDs(h.txList(t, model.BudgetTransactionListRequest{CategoryId: &loan})); !sameIDs(got, []string{untagged}) {
		t.Fatalf("category list without a debt = %v", got)
	}

	h.createDebt(t)
	cases := []struct {
		name string
		req  model.BudgetTransactionListRequest
		want []string
	}{
		{"category", model.BudgetTransactionListRequest{CategoryId: &loan}, []string{}},
		{"tag", model.BudgetTransactionListRequest{TagId: &tag}, []string{foodTagged}},
		{"tag child", model.BudgetTransactionListRequest{TagId: &tag, CategoryId: &loan}, []string{}},
		{"envelope", model.BudgetTransactionListRequest{EnvelopeId: &envID}, []string{food}},
	}
	for _, c := range cases {
		if got := itemIDs(h.txList(t, c.req)); !sameIDs(got, c.want) {
			t.Errorf("%s list = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestTransactionList_DebtRow(t *testing.T) {
	h := newAttrHarness(t)
	outside := h.f.Account(fixture.Account{UserID: h.owner.String(), Name: "Not in the budget"})
	paid := h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC))
	paidEUR := h.spend(h.eur, h.loan, h.tag, "100", time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC))
	h.spend(outside, h.loan, "", "1000", time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC))
	h.spend(h.usd, h.food, "", "50", time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC))
	h.spend(h.usd, h.loan, "", "1000", time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC))
	loan := vo.MustParseId(h.loan)
	template := vo.NewId()
	h.recurring.templates = []*model.RecurringTransaction{{
		ID: template, UserID: h.owner, Type: model.TransactionTypeExpense, AccountID: vo.MustParseId(h.usd),
		Amount: "1000", CategoryID: &loan, Schedule: model.RecurringScheduleMonthly,
		NextPaymentAt: time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC), ScheduledDay: 20,
	}}

	debtID := h.createDebt(t)
	res := h.txList(t, model.BudgetTransactionListRequest{DebtId: &debtID})
	if got := itemIDs(res); !sameIDs(got, []string{paid, paidEUR}) {
		t.Errorf("debt list = %v, want the two June payments from member accounts", got)
	}
	if got := upcomingIDs(res); !sameIDs(got, []string{template.String()}) {
		t.Errorf("debt upcoming = %v, want the matching template", got)
	}

	loanID := h.loan
	if got := upcomingIDs(h.txList(t, model.BudgetTransactionListRequest{CategoryId: &loanID})); len(got) != 0 {
		t.Errorf("category upcoming = %v, want the template on the debt row only", got)
	}

	// The list adds up to the row's Spent. Its Upcoming is the schedule's
	// (its next due is July 1), not the template's 1000.
	row := debtRow(h.budgetAt(t, h.owner, "2026-06-01"), debtID)
	if row == nil || row.Spent != "1200" || row.Upcoming != "0" {
		t.Errorf("debt row = %+v, want spent 1200 (1000 USD + 100 EUR), nothing still due", row)
	}
}

func TestTransactionList_DebtIdValidation(t *testing.T) {
	h := newAttrHarness(t)
	h.createDebt(t)
	unknown := vo.NewId().String()
	loan := h.loan
	debtID := h.createDebtOn(t, h.food)
	for name, req := range map[string]model.BudgetTransactionListRequest{
		"unknown":         {DebtId: &unknown},
		"with a category": {DebtId: &debtID, CategoryId: &loan},
	} {
		req.BudgetId, req.PeriodStart = h.budgetID, "2026-06-01"
		_, err := h.svc.GetTransactionList(h.ctx, h.owner, req)
		var v *errs.ValidationError
		if !errors.As(err, &v) {
			t.Errorf("%s: err = %v, want a validation error", name, err)
		}
	}
}

func (h *attrHarness) createDebtOn(t *testing.T, categoryID string) string {
	t.Helper()
	res, err := h.debts.CreateDebt(h.ctx, h.owner, model.CreateDebtRequest{
		Name: "Other", CurrencyId: fixture.USD, CategoryId: categoryID,
		StartDate: &attrStart, AmountBorrowed: vo.NewFlexString("1000"), Balance: vo.NewFlexString("1000"), Payment: vo.NewFlexString("100"),
		Schedule: "monthly", LastPaymentDate: "2027-06-01",
	})
	if err != nil {
		t.Fatalf("CreateDebt: %v", err)
	}
	return res.Id
}
