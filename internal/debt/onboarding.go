package debt

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// CoveredCategories are the categories the owner's debts count payments in.
func (s *Service) CoveredCategories(ctx context.Context, ownerID vo.Id) ([]vo.Id, error) {
	debts, err := s.repo.ListByOwners(ctx, []vo.Id{ownerID})
	if err != nil {
		return nil, err
	}
	out := make([]vo.Id, 0, len(debts))
	for _, d := range debts {
		if d.OwnerID.Equal(ownerID) {
			out = append(out, d.CategoryID)
		}
	}
	return out, nil
}

// TakeCategoryPlans hands the category's plans and comments to the owner's
// debt that covers the whole category and is not set up yet, as creating that
// debt did: a plan made on the category after the debt appeared would
// otherwise sit on a second row. moved is false when there is no such debt.
func (s *Service) TakeCategoryPlans(ctx context.Context, ownerID, categoryID vo.Id) (moved bool, err error) {
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		moved = false
		if err := s.repo.LockOwner(ctx, ownerID); err != nil {
			return err
		}
		debts, err := s.repo.ListByCategory(ctx, categoryID)
		if err != nil {
			return err
		}
		for _, d := range debts {
			if d.OwnerID.Equal(ownerID) && !d.IsSetUp() && isWholeCategory(d, debts) {
				moved = true
				return s.moveToDebt(ctx, d)
			}
		}
		return nil
	})
	return moved, err
}
