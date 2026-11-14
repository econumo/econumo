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

func (s *stubOnboardingDebtService) ElementIDs(context.Context, vo.Id) ([]vo.Id, error) {
	return nil, nil
}

// A detected debt the caller may not create in the budget (a guest, or an
// archived budget) is skipped like any other refusal; only a real failure
// fails the job.
func TestOnboardingDebts_RefusalsAreSkipped(t *testing.T) {
	ctx := context.Background()
	budget, category := vo.NewId(), vo.NewId()
	create := func(err error) (bool, error, model.CreateDebtRequest) {
		svc := &stubOnboardingDebtService{err: err}
		ok, cerr := NewOnboardingDebts(svc).CreateDebt(ctx, vo.NewId(), budget, category, vo.NewDecimal("250"), model.RecurringScheduleMonthly)
		return ok, cerr, svc.req
	}

	ok, err, req := create(nil)
	if !ok || err != nil {
		t.Fatalf("created = %v, %v", ok, err)
	}
	if req.BudgetId != budget.String() || req.ElementId != category.String() || req.Payment.String() != "250" || req.Schedule != "monthly" {
		t.Errorf("request = %+v, want a debt on the category in the budget", req)
	}

	for name, refusal := range map[string]error{
		"guest":     errs.NewAccessDenied("Access denied"),
		"archived":  &errs.AccessDeniedError{Msg: "This budget is archived", Code: errs.CodeBudgetArchived},
		"not found": errs.NewNotFound("Budget not found"),
		"has debt":  errs.NewValidation("Validation failed", errs.FieldError{Key: "elementId", Code: errs.CodeDebtElementHasDebt}),
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
