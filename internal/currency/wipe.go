package currency

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// MoveSharedCustomCurrencies hands every custom currency of userID that
// another owner's account, budget, budget element or debt uses to the lowest
// such owner id, so the user's cascade neither deletes the partner's account
// nor fails on the budget and debt references. A currency whose code the new
// owner already has live arrives soft-deleted: one live custom per code and
// owner. It runs on the caller's transaction context.
func (s *ManageService) MoveSharedCustomCurrencies(ctx context.Context, userID vo.Id) error {
	uid := userID.String()
	customs, err := s.repo.ListCustomCurrenciesByOwner(ctx, uid)
	if err != nil {
		return err
	}
	for _, c := range customs {
		owner, ok, err := s.repo.FirstForeignOwner(ctx, c.ID, uid)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		deleted := c.IsDeleted
		if !deleted {
			taken, err := s.repo.OwnerCodeExists(ctx, owner, c.Code)
			if err != nil {
				return err
			}
			deleted = taken
		}
		if err := s.repo.MoveCurrency(ctx, c.ID, owner, deleted); err != nil {
			return err
		}
	}
	return nil
}
