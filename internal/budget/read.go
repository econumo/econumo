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
	result, _, err := s.getBudget(ctx, userID, req)
	return result, err
}

// getBudget is GetBudget plus what each debt's matched payments add up to.
func (s *Service) getBudget(ctx context.Context, userID vo.Id, req model.GetBudgetRequest) (*model.GetBudgetResult, map[string]debtPaid, error) {
	budgetID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	periodStart, err := parsePeriodDate(req.Date, localMonth(s.clock.Now(), reqctx.Location(ctx)))
	if err != nil {
		return nil, nil, err
	}
	b, err := s.requireBudget(ctx, userID, budgetID)
	if err != nil {
		return nil, nil, err
	}
	periodStart = b.clampPeriod(periodStart)
	now := s.clock.Now()
	f, err := s.filtersWithDebts(ctx, userID, b, periodStart, periodStart.AddDate(0, 1, 0))
	if err != nil {
		return nil, nil, err
	}
	pr, err := s.upcomingRead(ctx, f, now)
	if err != nil {
		return nil, nil, err
	}
	result, debts, err := s.asOfTomorrow(ctx).buildBudget(ctx, b, f, now)
	if err != nil {
		return nil, nil, err
	}
	if pr != nil {
		if err := s.addUpcoming(ctx, b, now, pr, f, &result); err != nil {
			return nil, nil, err
		}
	}
	return &model.GetBudgetResult{Item: result}, debts, nil
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
