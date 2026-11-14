package server

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type recordingBudgetElements struct{ calls []string }

func (r *recordingBudgetElements) DebtElement(_ context.Context, _, elementID vo.Id) (model.DebtElementInfo, error) {
	r.calls = append(r.calls, "element "+elementID.String())
	return model.DebtElementInfo{ID: elementID, Type: model.DebtElementEnvelope, Eligible: true}, nil
}

func (r *recordingBudgetElements) DebtElementPlans(_ context.Context, _, elementID vo.Id) (map[string]float64, error) {
	r.calls = append(r.calls, "plans "+elementID.String())
	return map[string]float64{"2026-05-01": 3000}, nil
}

func TestDebtElements_PassesThrough(t *testing.T) {
	ctx := context.Background()
	rec := &recordingBudgetElements{}
	a := NewDebtElements(rec)
	budget, element := vo.NewId(), vo.NewId()
	if el, err := a.Element(ctx, budget, element); err != nil || !el.ID.Equal(element) || !el.Eligible {
		t.Fatalf("Element = %+v, %v", el, err)
	}
	if p, err := a.Plans(ctx, budget, element); err != nil || p["2026-05-01"] != 3000 {
		t.Fatalf("Plans = %+v, %v", p, err)
	}
	if len(rec.calls) != 2 || rec.calls[0] != "element "+element.String() || rec.calls[1] != "plans "+element.String() {
		t.Fatalf("calls = %v", rec.calls)
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

func TestDebtBudgets_PassesThrough(t *testing.T) {
	ctx := context.Background()
	rec := &recordingDebtBudgetAccess{}
	a := NewDebtBudgets(rec)
	budget := vo.NewId()
	if b, err := a.Access(ctx, vo.NewId(), budget); err != nil || !b.ID.Equal(budget) {
		t.Fatalf("Access = %+v, %v", b, err)
	}
	if list, err := a.Readable(ctx, vo.NewId()); err != nil || len(list) != 1 {
		t.Fatalf("Readable = %+v, %v", list, err)
	}
	if s, err := a.Scope(ctx, budget); err != nil || !s.BudgetID.Equal(budget) {
		t.Fatalf("Scope = %+v, %v", s, err)
	}
	want := []string{"budget", "budgets", "scope"}
	if len(rec.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", rec.calls, want)
	}
	for i := range want {
		if rec.calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", rec.calls, want)
		}
	}
}
