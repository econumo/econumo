package onboarding

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// Schedule exposes the save-accounts review to the external tests; in
// production only SaveAccounts starts it, after its transaction commits.
func (s *Service) Schedule(ctx context.Context, userID, budgetID vo.Id, accountIDs []vo.Id) {
	s.startAccountsReview(ctx, userID, nil, budgetID, accountIDs)
}
