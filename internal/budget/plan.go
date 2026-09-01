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
	budgetID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	from, err := parsePeriodDate(req.From, localMonth(s.clock.Now(), reqctx.Location(ctx)))
	if err != nil {
		return nil, err
	}
	months, err := parsePlanMonths(req.Months)
	if err != nil {
		return nil, err
	}
	b, err := s.requireBudget(ctx, userID, budgetID)
	if err != nil {
		return nil, err
	}
	from = b.clampPeriod(from)
	result, err := s.BuildBudgetPlan(ctx, userID, b, from, months)
	if err != nil {
		return nil, err
	}
	return &model.GetBudgetPlanResult{Item: result}, nil
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
