package server

import (
	"context"
	"errors"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type stubOnboardingDebtService struct {
	err error
	req model.CreateDebtRequest
}

func (s *stubOnboardingDebtService) CreateDebt(_ context.Context, _ vo.Id, req model.CreateDebtRequest) (*model.DebtResult, error) {
	s.req = req
	if s.err != nil {
		return nil, s.err
	}
	return &model.DebtResult{}, nil
}

func (s *stubOnboardingDebtService) CoveredCategories(context.Context, vo.Id) ([]vo.Id, error) {
	return nil, nil
}

func (s *stubOnboardingDebtService) TakeCategoryPlans(context.Context, vo.Id, vo.Id) (bool, error) {
	return false, nil
}

// A detected debt the caller may not create in the budget (a guest, or an
// archived budget) is skipped like any other refusal; only a real failure
// fails the job.
func TestOnboardingDebts_RefusalsAreSkipped(t *testing.T) {
	ctx := context.Background()
	budget, category, currency := vo.NewId(), vo.NewId(), vo.NewId()
	create := func(err error) (bool, error, model.CreateDebtRequest) {
		svc := &stubOnboardingDebtService{err: err}
		ok, cerr := NewOnboardingDebts(svc).CreateDebt(ctx, vo.NewId(), budget, "Car", category, currency, vo.NewDecimal("250"), model.RecurringScheduleMonthly)
		return ok, cerr, svc.req
	}

	ok, err, req := create(nil)
	if !ok || err != nil {
		t.Fatalf("created = %v, %v", ok, err)
	}
	if req.BudgetId != budget.String() || len(req.Rules) != 1 || req.Rules[0].CategoryId != category.String() ||
		req.Rules[0].PayeeId != nil || req.Rules[0].TagId != nil || req.Rules[0].LabelId != nil || req.CurrencyId != currency.String() {
		t.Errorf("request = %+v, want one whole-category rule in the budget", req)
	}

	for name, refusal := range map[string]error{
		"guest":     errs.NewAccessDenied("Access denied"),
		"archived":  &errs.AccessDeniedError{Msg: "This budget is archived", Code: errs.CodeBudgetArchived},
		"not found": errs.NewNotFound("Budget not found"),
		"covered":   errs.NewValidation("Validation failed", errs.FieldError{Key: "rules", Code: errs.CodeDebtKeyConflict}),
	} {
		if ok, err, _ := create(refusal); ok || err != nil {
			t.Errorf("%s: created = %v, %v; want skipped", name, ok, err)
		}
	}

	boom := errors.New("database down")
	if ok, err, _ := create(boom); ok || !errors.Is(err, boom) {
		t.Errorf("failure: created = %v, %v; want the error", ok, err)
	}
}
