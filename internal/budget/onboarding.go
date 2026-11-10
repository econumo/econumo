// Reads the onboarding steps run against the user's budgets.
package budget

import (
	"context"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/shared/reqctx"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DefaultBudget picks the budget onboarding works in: preferred (the user's
// active-budget option) when the user can read it, else the first budget they
// own or accepted. writable reports whether the user may change it (an editing
// role on an unarchived budget). found=false when the user has no such budget;
// a pending invitation does not count.
func (s *Service) DefaultBudget(ctx context.Context, userID vo.Id, preferred string) (id vo.Id, writable, found bool, err error) {
	budgets, err := s.budgets.ListForUser(ctx, userID)
	if err != nil {
		return vo.Id{}, false, false, err
	}
	var first *budgetAggregate
	for _, b := range budgets {
		agg, lerr := s.loadAggregate(ctx, b.ID)
		if lerr != nil {
			return vo.Id{}, false, false, lerr
		}
		if !s.canRead(agg, userID) {
			continue
		}
		if strings.EqualFold(agg.budget.ID.String(), strings.TrimSpace(preferred)) {
			first = agg
			break
		}
		if first == nil {
			first = agg
		}
	}
	if first == nil {
		return vo.Id{}, false, false, nil
	}
	return first.budget.ID, s.canUpdate(first, userID) && !first.budget.IsArchived, true, nil
}

// CurrentPeriod is the first day of the budget's period holding today in the
// caller's timezone, never before the budget's first period. The caller must
// be able to update the budget, and it must not be archived.
func (s *Service) CurrentPeriod(ctx context.Context, userID, budgetID vo.Id) (time.Time, error) {
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return time.Time{}, err
	}
	if !s.canUpdate(b, userID) {
		return time.Time{}, accessDenied()
	}
	if err := s.requireNotArchived(b); err != nil {
		return time.Time{}, err
	}
	return openingPeriod(b.budget, currentPeriod(s.clock.Now(), reqctx.Location(ctx), b.budget.PeriodStartDay)), nil
}
