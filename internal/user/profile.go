// Profile use cases: update the display name, currency, and report period.
package user

import (
	"context"
	"fmt"
	"time"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UpdateName changes the display name and returns the refreshed current user.
func (s *Service) UpdateName(ctx context.Context, userID vo.Id, req UpdateNameRequest) (*UpdateNameResult, error) {
	u, err := s.mutate(ctx, userID, func(u *User, now time.Time) error {
		u.UpdateName(req.Name, now)
		return nil
	})
	if err != nil {
		return nil, err
	}
	cur, err := s.toCurrentUser(ctx, u)
	if err != nil {
		return nil, err
	}
	return &UpdateNameResult{User: cur}, nil
}

// UpdateCurrency validates the code (tier-2 VO), confirms it resolves to a real
// currency, sets the option, and returns the refreshed current user.
func (s *Service) UpdateCurrency(ctx context.Context, userID vo.Id, req UpdateCurrencyRequest) (*UpdateCurrencyResult, error) {
	code, err := newCurrencyCode(req.Currency)
	if err != nil {
		return nil, err
	}
	if _, err := s.currency.GetIDByCode(ctx, code); err != nil {
		return nil, errs.NewNotFound(fmt.Sprintf("Currency %s not found", code))
	}
	u, err := s.mutate(ctx, userID, func(u *User, now time.Time) error {
		u.UpdateCurrency(code, now)
		return nil
	})
	if err != nil {
		return nil, err
	}
	cur, err := s.toCurrentUser(ctx, u)
	if err != nil {
		return nil, err
	}
	return &UpdateCurrencyResult{User: cur}, nil
}

// UpdateReportPeriod validates the period (tier-2 VO), sets the option, and
// returns the refreshed current user.
func (s *Service) UpdateReportPeriod(ctx context.Context, userID vo.Id, req UpdateReportPeriodRequest) (*UpdateReportPeriodResult, error) {
	period, err := newReportPeriod(req.Value)
	if err != nil {
		return nil, err
	}
	u, err := s.mutate(ctx, userID, func(u *User, now time.Time) error {
		u.UpdateReportPeriod(period, now)
		return nil
	})
	if err != nil {
		return nil, err
	}
	cur, err := s.toCurrentUser(ctx, u)
	if err != nil {
		return nil, err
	}
	return &UpdateReportPeriodResult{User: cur}, nil
}

// UpdateBudget sets the user's default budget option to the given budget id and
// returns the refreshed current user. The budget must already exist
// (existence-only, no ownership/access check); a miss maps to the "Plan not
// found" validation error (HTTP 400). The id format is validated tier-1
// (NotBlank + Uuid).
func (s *Service) UpdateBudget(ctx context.Context, userID vo.Id, req UpdateBudgetRequest) (*UpdateBudgetResult, error) {
	exists, err := s.budgets.Exists(ctx, req.Value)
	if err != nil {
		return nil, err
	}
	if !exists {
		// NewNotFound renders "Plan not found" verbatim at HTTP 400; the errors
		// {} vs [] shape is a pre-existing cross-cutting envelope divergence, not
		// specific to this endpoint.
		return nil, errs.NewNotFound("Plan not found")
	}
	u, err := s.mutate(ctx, userID, func(u *User, now time.Time) error {
		u.UpdateBudget(req.Value, now)
		return nil
	})
	if err != nil {
		return nil, err
	}
	cur, err := s.toCurrentUser(ctx, u)
	if err != nil {
		return nil, err
	}
	return &UpdateBudgetResult{User: cur}, nil
}
