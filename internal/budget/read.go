package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// GetBudgetList returns the user's budgets as meta entries.
func (s *Service) GetBudgetList(ctx context.Context, userID vo.Id) (*model.GetBudgetListResult, error) {
	budgets, err := s.budgets.ListForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]model.MetaResult, 0, len(budgets))
	for _, b := range budgets {
		agg, lerr := s.loadAggregate(ctx, b.ID)
		if lerr != nil {
			return nil, lerr
		}
		meta, merr := s.buildMeta(ctx, agg)
		if merr != nil {
			return nil, merr
		}
		items = append(items, meta)
	}
	return &model.GetBudgetListResult{Items: items}, nil
}

// GetBudget returns the full budget for the period containing `date` (snapped to
// first-of-month). Requires read access.
func (s *Service) GetBudget(ctx context.Context, userID vo.Id, req model.GetBudgetRequest) (*model.GetBudgetResult, error) {
	budgetID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	periodStart, err := parsePeriodDate(req.Date, localMonth(s.clock.Now(), reqctx.Location(ctx)))
	if err != nil {
		return nil, err
	}
	b, err := s.requireBudget(ctx, userID, budgetID)
	if err != nil {
		return nil, err
	}
	result, err := s.BuildBudget(ctx, userID, b, periodStart, s.clock.Now())
	if err != nil {
		return nil, err
	}
	return &model.GetBudgetResult{Item: result}, nil
}

// parsePeriodDate parses the get-budget date and snaps it to first-of-month. An
// empty/invalid date falls back to `fallback` (the caller-local current month):
// the frontend always sends a valid date, so defaulting is the tolerant choice
// for garbage input.
func parsePeriodDate(s string, fallback time.Time) (time.Time, error) {
	if s == "" {
		return fallback, nil
	}
	for _, layout := range []string{datetime.Layout, datetime.DateLayout, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return model.FirstOfMonth(t), nil
		}
	}
	return fallback, nil
}
