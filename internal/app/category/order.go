// Order use case: apply position changes to the user's categories.
package category

import (
	"context"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// OrderCategoryList applies each {id, position} change to the matching category,
// then returns the full available list.
//
// IMPORTANT asymmetry vs tag/payee: PHP's CategoryService::orderCategories
// iterates findByOwnerId (OWNER-ONLY), so a SHARED category's position is NOT
// updated — only the user's own categories are. (Tag/payee order, by contrast,
// iterate findAvailableForUserId and DO update shared.) The RESPONSE, however,
// is the full available list (own + shared) via the read view.
func (s *Service) OrderCategoryList(ctx context.Context, userID vo.Id, req OrderCategoryListRequest) (*OrderCategoryListResult, error) {
	positions := make(map[string]int16, len(req.Changes))
	for _, ch := range req.Changes {
		id, err := vo.ParseId(ch.Id)
		if err != nil {
			return nil, err
		}
		positions[id.String()] = int16(ch.Position)
	}

	var items []CategoryResult
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		// Owner-only set: shared categories are not reordered (matches PHP).
		cats, err := s.repo.ListByOwner(ctx, userID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		for _, c := range cats {
			pos, ok := positions[c.Id().String()]
			if !ok {
				continue
			}
			before := c.Position()
			c.UpdatePosition(pos, now)
			if c.Position() != before {
				if serr := s.repo.Save(ctx, c); serr != nil {
					return serr
				}
			}
		}
		// Response is the full available list (own + shared).
		built, berr := s.listResults(ctx, userID)
		if berr != nil {
			return berr
		}
		items = built
		return nil
	}); err != nil {
		return nil, err
	}

	return &OrderCategoryListResult{Items: items}, nil
}
