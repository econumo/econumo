// Onboarding use cases: complete onboarding, and the row lock plus step
// marking the onboarding feature's step writes run inside their transaction.
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

// LockForOnboarding takes the user row lock inside the caller's transaction
// (it holds to that transaction's end) and returns the current user. An
// onboarding step locks first, as every write to the user does, so two calls
// for one user serialize before either reads the step state.
func (s *Service) LockForOnboarding(ctx context.Context, userID vo.Id) (*model.CurrentUserResult, error) {
	if err := s.repo.LockRow(ctx, userID); err != nil {
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
	return &cur, nil
}

// MarkOnboardingStep records step as finished (see model.User.MarkOnboardingStep)
// and returns the refreshed current user.
func (s *Service) MarkOnboardingStep(ctx context.Context, userID vo.Id, step string) (*model.CurrentUserResult, error) {
	u, err := s.mutate(ctx, userID, func(u *model.User, now time.Time) error {
		u.MarkOnboardingStep(step, now)
		return nil
	})
	if err != nil {
		return nil, err
	}
	cur, err := s.toCurrentUser(ctx, u)
	if err != nil {
		return nil, err
	}
	return &cur, nil
}

// OnboardingUser returns the current user without taking the row lock, for the
// onboarding reads.
func (s *Service) OnboardingUser(ctx context.Context, userID vo.Id) (*model.CurrentUserResult, error) {
	u, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	cur, err := s.toCurrentUser(ctx, u)
	if err != nil {
		return nil, err
	}
	return &cur, nil
}
