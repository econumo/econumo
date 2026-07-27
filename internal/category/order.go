// Order use case: apply position changes to the user's categories.
package category

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// OrderCategoryList applies each {id, position} change to the matching category,
// then returns the full available list.
//
// Reordering iterates the OWNER-ONLY set, so a SHARED category's position is
// NOT updated — only the user's own categories are (tag/payee order works the
// same way, see issue #108). The RESPONSE, however, is the full available list
// (own + shared) via the read view.
func (s *Service) OrderCategoryList(ctx context.Context, userID vo.Id, req model.OrderCategoryListRequest) (*model.OrderCategoryListResult, error) {
	positions := make(map[string]int16, len(req.Changes))
	for _, ch := range req.Changes {
		id, err := vo.ParseId(ch.Id)
		if err != nil {
			return nil, err
		}
		positions[id.String()] = int16(ch.Position)
	}

	var items []model.CategoryResult
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		// Owner-only set: shared categories are not reordered.
		cats, err := s.repo.ListByOwner(ctx, userID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		for _, c := range cats {
			pos, ok := positions[c.ID.String()]
			if !ok {
				continue
			}
			before := c.Position
			c.UpdatePosition(pos, now)
			if c.Position != before {
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

	return &model.OrderCategoryListResult{Items: items}, nil
}
