package debt_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/test/fixture"
)

func (e *env) addRule(id string, r model.DebtRuleRequest) (*model.DebtResult, error) {
	return e.svc.AddDebtRule(context.Background(), e.uid(), model.AddDebtRuleRequest{Id: id, Rule: r})
}

// "Add to a debt": the category becomes another rule, and its row's plans
// move onto the debt's row, adding up with what is there.
func TestAddDebtRule_MovesTheCategoryPlansAddingUp(t *testing.T) {
	e := newEnv(t)
	sam := e.partner(t, "Sam", roleUser)
	hers := e.newCategory(sam, true)
	res := e.create(t, nil)
	e.elements.transfers = nil

	got, err := e.addRule(res.Id, rule(hers))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rules) != 2 || got.Rules[0].Id != res.Rules[0].Id || got.Rules[1].CategoryId != hers || got.Rules[1].OwnerName != "Sam" {
		t.Fatalf("rules = %+v", got.Rules)
	}
	want := transferCall{budget: e.budget, from: hers, fromType: model.ElementCategory, to: res.Id, toType: model.ElementDebt,
		currency: fixture.USD, sum: true, first: "2026-03-10", last: "2027-03-01"}
	if len(e.elements.transfers) != 1 || e.elements.transfers[0] != want {
		t.Fatalf("transfers = %+v, want [%+v]", e.elements.transfers, want)
	}
	// a narrowed rule moves nothing
	e.elements.transfers = nil
	payee := e.newPayee()
	if _, err := e.addRule(res.Id, model.DebtRuleRequest{CategoryId: e.newCategory(e.user, true), PayeeId: &payee}); err != nil {
		t.Fatal(err)
	}
	if len(e.elements.transfers) != 0 {
		t.Fatalf("transfers = %+v, want none", e.elements.transfers)
	}
}

func TestAddDebtRule_CategoryAlreadyCovered(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	payee := e.newPayee()
	for _, r := range []model.DebtRuleRequest{rule(e.category), {CategoryId: e.category, PayeeId: &payee}} {
		_, err := e.addRule(res.Id, r)
		requireField(t, err, "rules", errs.CodeDebtKeyConflict)
	}
	if d := e.debtOf(t, res.Id); len(d.Rules) != 1 {
		t.Fatalf("rules = %+v", d.Rules)
	}
}

func TestAddDebtRule_ConflictWithAnotherDebt(t *testing.T) {
	e := newEnv(t)
	other := e.newCategory(e.user, true)
	e.create(t, func(r *model.CreateDebtRequest) { r.Rules = []model.DebtRuleRequest{rule(other)} })
	res := e.create(t, nil)
	payee := e.newPayee()
	_, err := e.addRule(res.Id, model.DebtRuleRequest{CategoryId: other, PayeeId: &payee})
	requireField(t, err, "rules", errs.CodeDebtKeyConflict)
	// the same key on the same start date
	_, err = e.addRule(res.Id, rule(other))
	requireField(t, err, "startDate", errs.CodeDebtStartDateTaken)
}

func TestAddDebtRule_TenRulesAtMost(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	for range 9 {
		if _, err := e.addRule(res.Id, rule(e.newCategory(e.user, true))); err != nil {
			t.Fatal(err)
		}
	}
	_, err := e.addRule(res.Id, rule(e.newCategory(e.user, true)))
	requireField(t, err, "rules", errs.CodeOutOfRange)
}

func TestAddDebtRule_NeedsPlanAccess(t *testing.T) {
	e := newEnv(t)
	res := e.create(t, nil)
	guest := e.partner(t, "guest", roleGuest)
	stranger := e.f.User(fixture.User{Name: "stranger", Email: "stranger@example.test"})
	req := model.AddDebtRuleRequest{Id: res.Id, Rule: rule(e.newCategory(e.user, true))}
	_, err := e.svc.AddDebtRule(context.Background(), uidOf(guest), req)
	requireForbidden(t, err)
	_, err = e.svc.AddDebtRule(context.Background(), uidOf(stranger), req)
	requireNotFound(t, err)
	e.fakeBudget(e.budget).archived = true
	_, err = e.svc.AddDebtRule(context.Background(), e.uid(), req)
	requireArchived(t, err)
}

// A category deleted (its delete committed) between the owner check's read
// and the rule write: the write answers not found, not a foreign-key failure.
// The fake classifications still report the category, as a read taken before
// the delete would.
func TestRuleWrite_DeletedCategoryNotFound(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res := e.create(t, nil)
	gone := e.newCategory(e.user, true)
	if _, err := e.db.Raw.ExecContext(ctx, e.db.Rebind("DELETE FROM categories WHERE id = ?"), gone); err != nil {
		t.Fatal(err)
	}

	req := e.createReq()
	req.Rules = []model.DebtRuleRequest{rule(gone)}
	_, err := e.svc.CreateDebt(ctx, e.uid(), req)
	requireNotFound(t, err)
	_, err = e.addRule(res.Id, rule(gone))
	requireNotFound(t, err)
	update := e.updateReq(res)
	update.Rules = []model.DebtRuleRequest{rule(e.category), rule(gone)}
	_, err = e.svc.UpdateDebt(ctx, e.uid(), update)
	requireNotFound(t, err)

	debts, err := e.repo.ListByBudget(ctx, uidOf(e.budget))
	if err != nil || len(debts) != 1 || len(debts[0].Rules) != 1 {
		t.Fatalf("debts = %+v, %v; want the first one alone, unchanged", debts, err)
	}
}
