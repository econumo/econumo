// Onboarding use case: mark the user's onboarding complete.
package user

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CompleteOnboarding marks onboarding complete and returns the current user.
func (s *Service) CompleteOnboarding(ctx context.Context, userID vo.Id) (*model.CompleteOnboardingResult, error) {
	u, err := s.mutate(ctx, userID, func(u *model.User, now time.Time) error {
		u.CompleteOnboarding(now)
		return nil
	})
	if err != nil {
		return nil, err
	}
	cur, err := s.toCurrentUser(ctx, u)
	if err != nil {
		return nil, err
	}
	return &model.CompleteOnboardingResult{User: cur}, nil
}
