// Profile use cases: update the display name, currency, and report period.
package user

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UpdateName changes the display name and returns the refreshed current user.
func (s *Service) UpdateName(ctx context.Context, userID vo.Id, req model.UpdateNameRequest) (*model.UpdateNameResult, error) {
	u, err := s.mutate(ctx, userID, func(u *model.User, now time.Time) error {
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
	return &model.UpdateNameResult{User: cur}, nil
}

// UpdateCurrency validates the code (tier-2 VO), confirms it resolves to a real
// currency, sets the option, and returns the refreshed current user.
func (s *Service) UpdateCurrency(ctx context.Context, userID vo.Id, req model.UpdateCurrencyRequest) (*model.UpdateCurrencyResult, error) {
	code, err := newCurrencyCode(req.Currency)
	if err != nil {
		return nil, err
	}
	if _, err := s.currency.GetIDByCode(ctx, code); err != nil {
		return nil, errs.NewNotFound(fmt.Sprintf("Currency %s not found", code))
	}
	u, err := s.mutate(ctx, userID, func(u *model.User, now time.Time) error {
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
	return &model.UpdateCurrencyResult{User: cur}, nil
}

// UpdateReportPeriod validates the period (tier-2 VO), sets the option, and
// returns the refreshed current user.
func (s *Service) UpdateReportPeriod(ctx context.Context, userID vo.Id, req model.UpdateReportPeriodRequest) (*model.UpdateReportPeriodResult, error) {
	period, err := newReportPeriod(req.Value)
	if err != nil {
		return nil, err
	}
	u, err := s.mutate(ctx, userID, func(u *model.User, now time.Time) error {
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
	return &model.UpdateReportPeriodResult{User: cur}, nil
}

// UpdateLanguage persists the caller's UI language. Write-only: nothing reads
// it yet; it exists so future background emails can render in the user's
// language. Deliberately not via mutate/Save (a dedicated UPDATE keeps the
// column out of the whole-row upsert).
func (s *Service) UpdateLanguage(ctx context.Context, userID vo.Id, req model.UpdateLanguageRequest) (*model.UpdateLanguageResult, error) {
	lang, err := newLanguage(req.Language)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateLanguage(ctx, userID, lang); err != nil {
		return nil, err
	}
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	cur, err := s.toCurrentUser(ctx, u)
	if err != nil {
		return nil, err
	}
	return &model.UpdateLanguageResult{User: cur}, nil
}

// UpdateBudget sets the user's default budget option to the given budget id and
// returns the refreshed current user. The budget must already exist
// (existence-only, no ownership/access check); a miss maps to the "Plan not
// found" validation error (HTTP 400). The id format is validated tier-1
// (NotBlank + Uuid).
func (s *Service) UpdateBudget(ctx context.Context, userID vo.Id, req model.UpdateActiveBudgetRequest) (*model.UpdateActiveBudgetResult, error) {
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
	u, err := s.mutate(ctx, userID, func(u *model.User, now time.Time) error {
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
	return &model.UpdateActiveBudgetResult{User: cur}, nil
}

// UpdateAvatar validates the icon format and color choice (tier-2), stores the
// joined "<icon>:<color>" value, and returns the refreshed current user.
func (s *Service) UpdateAvatar(ctx context.Context, userID vo.Id, req model.UpdateAvatarRequest) (*model.UpdateAvatarResult, error) {
	icon := strings.TrimSpace(req.Icon)
	color := strings.TrimSpace(req.Color)
	var fields []errs.FieldError
	if !IsValidAvatarIcon(icon) {
		fields = append(fields, errs.FieldError{Key: "icon", Message: "This value is not valid.", Code: errs.CodeInvalidFormat})
	}
	if !IsValidAvatarColor(color) {
		fields = append(fields, errs.FieldError{Key: "color", Message: "The value you selected is not a valid choice.", Code: errs.CodeInvalidChoice})
	}
	if len(fields) > 0 {
		return nil, errs.NewValidation("Validation failed", fields...)
	}
	u, err := s.mutate(ctx, userID, func(u *model.User, now time.Time) error {
		u.UpdateAvatar(JoinAvatar(icon, color), now)
		return nil
	})
	if err != nil {
		return nil, err
	}
	cur, err := s.toCurrentUser(ctx, u)
	if err != nil {
		return nil, err
	}
	return &model.UpdateAvatarResult{User: cur}, nil
}
