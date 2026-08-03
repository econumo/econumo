// Move use case: place one category relative to a sibling.
package category

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MoveCategory places the category immediately after req.AfterId (nil = first)
// and returns the full available list.
//
// Only the OWNER's categories are repositioned: a shared category belongs to
// another user's list, so a sharee's move silently no-ops (issue #108). The
// RESPONSE is still the full available list (own + shared) via the read view.
func (s *Service) MoveCategory(ctx context.Context, userID vo.Id, req model.MoveCategoryRequest) (*model.MoveCategoryResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	afterID := ""
	if req.AfterId != nil {
		afterID = *req.AfterId
	}

	var items []model.CategoryResult
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		cats, lerr := s.repo.ListByOwner(ctx, userID)
		if lerr != nil {
			return lerr
		}
		key, found, kerr := sortkey.Relocate(cats, id.String(), afterID, categoryItem, sortkey.GrowsUp)
		if kerr != nil {
			return kerr
		}
		if found {
			for _, c := range cats {
				if !c.ID.Equal(id) {
					continue
				}
				c.UpdateSortKey(key, s.clock.Now())
				if serr := s.repo.Save(ctx, c); serr != nil {
					return serr
				}
				break
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
	return &model.MoveCategoryResult{Items: items}, nil
}

func categoryItem(c *model.Category) sortkey.Item {
	return sortkey.Item{ID: c.ID.String(), Key: c.SortKey}
}
