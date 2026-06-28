package budget

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// GetBudgetList returns the user's budgets as meta entries (BudgetListService +
// BudgetMetaBuilder).
func (s *Service) GetBudgetList(ctx context.Context, userID vo.Id) (*GetBudgetListResult, error) {
	budgets, err := s.repo.ListForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]MetaResult, 0, len(budgets))
	for _, b := range budgets {
		agg, lerr := s.loadAggregate(ctx, b.Id())
		if lerr != nil {
			return nil, lerr
		}
		meta, merr := s.buildMeta(ctx, agg)
		if merr != nil {
			return nil, merr
		}
		items = append(items, meta)
	}
	return &GetBudgetListResult{Items: items}, nil
}

// GetBudget returns the full budget for the period containing `date` (snapped to
// first-of-month). Requires read access.
func (s *Service) GetBudget(ctx context.Context, userID vo.Id, req GetBudgetRequest) (*GetBudgetResult, error) {
	budgetID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, validateBlank(map[string]string{"id": ""})
	}
	periodStart, err := parsePeriodDate(req.Date, s.clock.Now())
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
	return &GetBudgetResult{Item: result}, nil
}

// parsePeriodDate parses the get-budget date and snaps it to first-of-month. An
// empty/invalid date falls back to the current month (PHP new DateTimeImmutable
// would throw on garbage, but the frontend always sends a valid date; we default
// to now to be tolerant).
func parsePeriodDate(s string, now time.Time) (time.Time, error) {
	if s == "" {
		return firstOfMonth(now), nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return firstOfMonth(t), nil
		}
	}
	return firstOfMonth(now), nil
}
