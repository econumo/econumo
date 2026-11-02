package budget

import (
	"context"
	"strconv"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	planMonthsDefault = 12
	planMonthsMax     = 24
)

// GetBudgetPlan returns the multi-month plan sheet for [from, from+months).
// Readable by any accepted member (same visibility as get-budget). `from`
// snaps to first-of-month with the same tolerant fallback as get-budget's
// date param; `months` is strict (see parsePlanMonths).
func (s *Service) GetBudgetPlan(ctx context.Context, userID vo.Id, req model.GetBudgetPlanRequest) (*model.GetBudgetPlanResult, error) {
	result, _, err := s.getBudgetPlan(ctx, userID, req)
	return result, err
}

// getBudgetPlan is GetBudgetPlan plus each debt's matched payments per window
// month.
func (s *Service) getBudgetPlan(ctx context.Context, userID vo.Id, req model.GetBudgetPlanRequest) (*model.GetBudgetPlanResult, map[string][]vo.DecimalNumber, error) {
	budgetID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	from, err := parsePeriodDate(req.From, localMonth(s.clock.Now(), reqctx.Location(ctx)))
	if err != nil {
		return nil, nil, err
	}
	months, err := parsePlanMonths(req.Months)
	if err != nil {
		return nil, nil, err
	}
	b, err := s.requireBudget(ctx, userID, budgetID)
	if err != nil {
		return nil, nil, err
	}
	from = b.clampPeriod(from)
	f, err := s.filtersWithDebts(ctx, userID, b, from, from.AddDate(0, months, 0))
	if err != nil {
		return nil, nil, err
	}
	result, debts, err := s.asOfTomorrow(ctx).buildBudgetPlan(ctx, b, f, months)
	if err != nil {
		return nil, nil, err
	}
	if err := s.addPlanUpcoming(ctx, b, f, months, &result); err != nil {
		return nil, nil, err
	}
	return &model.GetBudgetPlanResult{Item: result}, debts, nil
}

// parsePlanMonths parses the months query param: empty defaults to 12. Unlike
// the tolerant date fallback, a malformed or out-of-range (1..24) value is a
// hard validation error — silently rendering a different window size than the
// sheet asked for would read as data loss.
func parsePlanMonths(raw string) (int, error) {
	if raw == "" {
		return planMonthsDefault, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > planMonthsMax {
		return 0, errs.NewValidation("Validation failed", errs.FieldError{
			Key: "months", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice,
		})
	}
	return n, nil
}
