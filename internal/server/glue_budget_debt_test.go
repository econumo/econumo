package server

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type recordingBudgetElements struct {
	from, to, has, locked, removed, currencyType model.ElementType
	currency, ensured                            vo.Id
}

func (r *recordingBudgetElements) EnsureElements(_ context.Context, budgetID vo.Id) error {
	r.ensured = budgetID
	return nil
}

func (r *recordingBudgetElements) SetElementsCurrency(_ context.Context, _ vo.Id, typ model.ElementType, currencyID vo.Id) error {
	r.currencyType, r.currency = typ, currencyID
	return nil
}

func (r *recordingBudgetElements) TransferElementData(_ context.Context, t model.ElementDataTransfer) (bool, error) {
	r.from, r.to = t.FromType, t.ToType
	return true, nil
}

func (r *recordingBudgetElements) RemoveElements(_ context.Context, _ vo.Id, typ model.ElementType) error {
	r.removed = typ
	return nil
}

func (r *recordingBudgetElements) LockElements(_ context.Context, _ vo.Id, typ model.ElementType) error {
	r.locked = typ
	return nil
}

func (r *recordingBudgetElements) ElementHasData(_ context.Context, _ vo.Id, typ model.ElementType) (bool, error) {
	r.has = typ
	return true, nil
}

func TestDebtBudgetElements_PassesTheStoredTypes(t *testing.T) {
	rec := &recordingBudgetElements{}
	a := NewDebtBudgetElements(rec)
	left, err := a.TransferData(context.Background(), model.ElementDataTransfer{FromID: vo.NewId(), FromType: model.ElementCategory,
		ToID: vo.NewId(), ToType: model.ElementDebt, CurrencyID: vo.NewId()})
	if err != nil || !left {
		t.Fatalf("TransferData = %v, %v; want the budget's leftBehind passed through", left, err)
	}
	if rec.from != model.ElementCategory || rec.to != model.ElementDebt {
		t.Errorf("transfer types = %d -> %d, want category -> debt", rec.from, rec.to)
	}
	if has, err := a.HasData(context.Background(), vo.NewId(), model.ElementDebt); err != nil || !has {
		t.Fatalf("HasData = %v, %v", has, err)
	}
	if rec.has != model.ElementDebt {
		t.Errorf("has-data type = %d, want debt", rec.has)
	}
	if err := a.LockRows(context.Background(), vo.NewId(), model.ElementDebt); err != nil || rec.locked != model.ElementDebt {
		t.Errorf("LockRows = %v, locked type %d; want debt", err, rec.locked)
	}
	if err := a.RemoveRows(context.Background(), vo.NewId(), model.ElementDebt); err != nil || rec.removed != model.ElementDebt {
		t.Errorf("RemoveRows = %v, removed type %d; want debt", err, rec.removed)
	}
	budget := vo.NewId()
	if err := a.EnsureRows(context.Background(), budget); err != nil || !rec.ensured.Equal(budget) {
		t.Errorf("EnsureRows = %v, ensured %s; want %s", err, rec.ensured, budget)
	}
	cur := vo.NewId()
	if err := a.SetRowsCurrency(context.Background(), vo.NewId(), model.ElementDebt, cur); err != nil || rec.currencyType != model.ElementDebt || !rec.currency.Equal(cur) {
		t.Errorf("SetRowsCurrency = %v, type %d currency %s; want debt %s", err, rec.currencyType, rec.currency, cur)
	}
}

type recordingDebtBudgetAccess struct{ calls []string }

func (r *recordingDebtBudgetAccess) DebtBudget(_ context.Context, _, budgetID vo.Id) (model.DebtBudget, error) {
	r.calls = append(r.calls, "budget")
	return model.DebtBudget{ID: budgetID}, nil
}

func (r *recordingDebtBudgetAccess) DebtBudgets(context.Context, vo.Id) ([]model.DebtBudget, error) {
	r.calls = append(r.calls, "budgets")
	return []model.DebtBudget{{}}, nil
}

func (r *recordingDebtBudgetAccess) DebtScope(_ context.Context, budgetID vo.Id) (model.DebtScope, error) {
	r.calls = append(r.calls, "scope")
	return model.DebtScope{BudgetID: budgetID}, nil
}

func (r *recordingDebtBudgetAccess) DebtRuleOptions(context.Context, vo.Id, vo.Id) (*model.DebtRuleOptionsResult, error) {
	r.calls = append(r.calls, "options")
	return &model.DebtRuleOptionsResult{}, nil
}

func (r *recordingDebtBudgetAccess) DebtRowPlans(_ context.Context, _, debtID vo.Id) (model.DebtBudgetPlans, bool, error) {
	r.calls = append(r.calls, "plans "+debtID.String())
	return model.DebtBudgetPlans{Name: "Home"}, true, nil
}

func TestDebtBudgets_PassesThrough(t *testing.T) {
	ctx := context.Background()
	rec := &recordingDebtBudgetAccess{}
	a := NewDebtBudgets(rec)
	budget, debt := vo.NewId(), vo.NewId()
	if b, err := a.Access(ctx, vo.NewId(), budget); err != nil || !b.ID.Equal(budget) {
		t.Fatalf("Access = %+v, %v", b, err)
	}
	if list, err := a.Readable(ctx, vo.NewId()); err != nil || len(list) != 1 {
		t.Fatalf("Readable = %+v, %v", list, err)
	}
	if s, err := a.Scope(ctx, budget); err != nil || !s.BudgetID.Equal(budget) {
		t.Fatalf("Scope = %+v, %v", s, err)
	}
	if o, err := a.RuleOptions(ctx, vo.NewId(), budget); err != nil || o == nil {
		t.Fatalf("RuleOptions = %+v, %v", o, err)
	}
	if p, ok, err := a.DebtPlans(ctx, budget, debt); err != nil || !ok || p.Name != "Home" {
		t.Fatalf("DebtPlans = %+v, %v, %v", p, ok, err)
	}
	want := []string{"budget", "budgets", "scope", "options", "plans " + debt.String()}
	if len(rec.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", rec.calls, want)
	}
	for i := range want {
		if rec.calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", rec.calls, want)
		}
	}
}
