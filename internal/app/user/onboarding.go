// Onboarding use case: mark the user's onboarding complete.
package user

import (
	"context"
	"time"

	domuser "github.com/econumo/econumo/internal/domain/user"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CompleteOnboarding marks onboarding complete and returns the current user.
func (s *Service) CompleteOnboarding(ctx context.Context, userID vo.Id) (*CompleteOnboardingResult, error) {
	u, err := s.mutate(ctx, userID, func(u *domuser.User, now time.Time) error {
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
	return &CompleteOnboardingResult{User: cur}, nil
}
