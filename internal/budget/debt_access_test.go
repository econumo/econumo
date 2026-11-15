package budget_test

import (
	"slices"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func sortedIDs(ids []vo.Id) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	slices.Sort(out)
	return out
}

func sorted(ids ...vo.Id) []string { return sortedIDs(ids) }

func TestDebtBudget_RolesAndParticipants(t *testing.T) {
	h := newCommentHarness(t)
	cash := h.f.Account(fixture.Account{UserID: h.owner.String(), Name: "Cash"})
	h.f.BudgetAccount(h.budgetID.String(), cash)

	for name, tc := range map[string]struct {
		user    vo.Id
		canPlan bool
	}{
		"owner": {h.owner, true}, "admin": {h.admin, true}, "member": {h.member, true}, "guest": {h.guest, false},
	} {
		b, err := h.svc.DebtBudget(h.ctx, tc.user, h.budgetID)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if b.CanPlan != tc.canPlan {
			t.Errorf("%s: CanPlan = %v, want %v", name, b.CanPlan, tc.canPlan)
		}
		if !b.ID.Equal(h.budgetID) || b.CurrencyID.String() != fixture.USD || b.PeriodStartDay != model.MinPeriodStartDay || b.IsArchived {
			t.Errorf("%s: budget = %+v", name, b)
		}
		if len(b.AccountIDs) != 1 || b.AccountIDs[0].String() != cash {
			t.Errorf("%s: accounts = %v, want the member account", name, b.AccountIDs)
		}
	}
	for name, user := range map[string]vo.Id{"pending": h.pending, "stranger": h.stranger} {
		if _, err := h.svc.DebtBudget(h.ctx, user, h.budgetID); !isNotFound(err) {
			t.Errorf("%s: err = %v, want not found", name, err)
		}
	}
	if _, err := h.svc.DebtBudget(h.ctx, h.owner, vo.NewId()); !isNotFound(err) {
		t.Errorf("unknown budget: err = %v, want not found", err)
	}

	if _, err := h.tdb.Raw.Exec(h.tdb.Rebind("UPDATE budgets SET is_archived = 1 WHERE id = ?"), h.budgetID.String()); err != nil {
		t.Fatal(err)
	}
	if b, err := h.svc.DebtBudget(h.ctx, h.owner, h.budgetID); err != nil || !b.IsArchived || !b.CanPlan {
		t.Errorf("archived: %+v, %v; want archived, still the owner's to plan", b, err)
	}

	list, err := h.svc.DebtBudgets(h.ctx, h.guest)
	if err != nil || len(list) != 1 || !list[0].ID.Equal(h.budgetID) || list[0].CanPlan {
		t.Errorf("guest's list = %+v, %v; want the budget, read-only", list, err)
	}
	if list, err := h.svc.DebtBudgets(h.ctx, h.pending); err != nil || len(list) != 0 {
		t.Errorf("pending invite's list = %+v, %v; want none", list, err)
	}
}

func isNotFound(err error) bool {
	_, ok := errs.AsNotFound(err)
	return ok
}

// The scope is the population the budget counts: a soft-deleted member keeps
// counting, so its payments keep counting on a debt too.
func TestDebtScope_IncludesSoftDeletedMembers(t *testing.T) {
	h := newCommentHarness(t)
	live := h.f.Account(fixture.Account{UserID: h.owner.String(), Name: "Live"})
	gone := h.f.Account(fixture.Account{UserID: h.owner.String(), Name: "Gone", Deleted: true})
	h.f.Account(fixture.Account{UserID: h.owner.String(), Name: "Outside"})
	h.f.BudgetAccount(h.budgetID.String(), live)
	h.f.BudgetAccount(h.budgetID.String(), gone)

	scope, err := h.svc.DebtScope(h.ctx, h.budgetID)
	if err != nil {
		t.Fatal(err)
	}
	if !scope.BudgetID.Equal(h.budgetID) || !slices.Equal(sortedIDs(scope.AccountIDs), sorted(vo.MustParseId(live), vo.MustParseId(gone))) {
		t.Errorf("scope = %+v, want both members, the deleted one included", scope)
	}
	b, err := h.svc.DebtBudget(h.ctx, h.owner, h.budgetID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sortedIDs(b.AccountIDs), sortedIDs(scope.AccountIDs)) {
		t.Errorf("access accounts = %v, want the scope's %v", b.AccountIDs, scope.AccountIDs)
	}
}

// A debt counts what an element's Spent counts: never a savings member.
func TestDebtScope_LeavesSavingsMembersOut(t *testing.T) {
	h := newCommentHarness(t)
	everyday := h.f.Account(fixture.Account{UserID: h.owner.String(), Name: "Everyday"})
	savings := h.f.Account(fixture.Account{UserID: h.owner.String(), Name: "Savings"})
	h.f.BudgetAccount(h.budgetID.String(), everyday)
	h.f.BudgetAccount(h.budgetID.String(), savings)
	if _, err := h.tdb.Raw.ExecContext(h.ctx, h.tdb.Rebind("UPDATE budgets_accounts SET is_savings = TRUE WHERE account_id = ?"), savings); err != nil {
		t.Fatal(err)
	}
	scope, err := h.svc.DebtScope(h.ctx, h.budgetID)
	if err != nil || len(scope.AccountIDs) != 1 || scope.AccountIDs[0].String() != everyday {
		t.Fatalf("scope = %+v, %v; want the everyday member alone", scope, err)
	}
	b, err := h.svc.DebtBudget(h.ctx, h.owner, h.budgetID)
	if err != nil || len(b.AccountIDs) != 1 || b.AccountIDs[0].String() != everyday {
		t.Fatalf("access accounts = %v, %v", b.AccountIDs, err)
	}
}

// DebtElement tells which elements can carry a debt: a top-level expense
// category or an expense envelope, with the element's name, icon, currency
// and categories; DebtElementPlans reads the element row's plans.
func TestDebtElement_EligibilityAndPlans(t *testing.T) {
	h := newCommentHarness(t)
	owner, budget := h.owner.String(), h.budgetID.String()
	eur := h.f.Currency(fixture.Currency{Code: "EUR", Symbol: "€", Name: "Euro"})
	loan := h.f.Category(fixture.Category{UserID: owner, Name: "Mortgage", Icon: "house"})
	row := h.f.BudgetElement(fixture.BudgetElement{BudgetID: budget, ExternalID: loan, Type: int(model.ElementCategory), CurrencyID: eur})
	h.f.BudgetLimit(fixture.BudgetLimit{ElementID: row, Period: "2026-05-01 00:00:00", Amount: "3000"})
	child := h.f.Category(fixture.Category{UserID: owner, Name: "Insurance"})
	env := h.f.BudgetEnvelope(fixture.BudgetEnvelope{BudgetID: budget, Name: "Housing", Icon: "home"})
	h.f.BudgetElement(fixture.BudgetElement{BudgetID: budget, ExternalID: env, Type: int(model.ElementEnvelope)})
	h.f.EnvelopeCategory(env, child)
	income := h.f.Category(fixture.Category{UserID: owner, Name: "Salary", Type: 1})
	strangers := h.f.Category(fixture.Category{UserID: h.stranger.String(), Name: "Theirs"})

	el, err := h.svc.DebtElement(h.ctx, h.budgetID, vo.MustParseId(loan))
	if err != nil || el.Type != model.DebtElementCategory || !el.Eligible || el.Name != "Mortgage" || el.Icon != "house" ||
		el.CurrencyID.String() != eur || len(el.CategoryIDs) != 1 || el.CategoryIDs[0].String() != loan {
		t.Fatalf("category = %+v, %v", el, err)
	}
	el, err = h.svc.DebtElement(h.ctx, h.budgetID, vo.MustParseId(env))
	if err != nil || el.Type != model.DebtElementEnvelope || !el.Eligible || el.Name != "Housing" || el.CurrencyID.String() != fixture.USD ||
		len(el.CategoryIDs) != 1 || el.CategoryIDs[0].String() != child {
		t.Fatalf("envelope = %+v, %v", el, err)
	}
	for name, id := range map[string]string{"in an envelope": child, "income": income, "a stranger's": strangers, "unknown": vo.NewId().String()} {
		if el, err := h.svc.DebtElement(h.ctx, h.budgetID, vo.MustParseId(id)); err != nil || el.Eligible {
			t.Errorf("%s: %+v, %v; want not eligible", name, el, err)
		}
	}

	plans, err := h.svc.DebtElementPlans(h.ctx, h.budgetID, vo.MustParseId(loan))
	if err != nil || len(plans) != 1 || plans["2026-05-01"] != 3000 {
		t.Fatalf("plans = %v, %v", plans, err)
	}
	if plans, err := h.svc.DebtElementPlans(h.ctx, h.budgetID, vo.MustParseId(env)); err != nil || len(plans) != 0 {
		t.Fatalf("envelope plans = %v, %v", plans, err)
	}
}
