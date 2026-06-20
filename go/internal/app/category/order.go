// Order use case: apply position changes to the user's categories.
package category

import (
	"context"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// OrderCategoryList applies each {id, position} change to the matching category
// owned by the user, saving only the ones that actually changed, then returns
// the full ordered list.
//
// Changes referencing an id the user does not own are ignored (only categories
// found among the owner's are mutated).
func (s *Service) OrderCategoryList(ctx context.Context, userID vo.Id, req OrderCategoryListRequest) (*OrderCategoryListResult, error) {
	// Build an id -> position map from the changes.
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
