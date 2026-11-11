package onboarding

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// Schedule exposes the background savings classification to the external tests;
// in production only SaveAccounts starts it, after its transaction commits.
func (s *Service) Schedule(ctx context.Context, userID, budgetID vo.Id, accountIDs []vo.Id) {
	s.schedule(ctx, userID, budgetID, accountIDs)
}

// StartPlanDebts exposes save-plan's debt detection to the external tests.
func (s *Service) StartPlanDebts(ctx context.Context, userID vo.Id, budgetID string, planned map[string]vo.DecimalNumber) {
	s.startPlanDebts(ctx, userID, budgetID, planned)
}
