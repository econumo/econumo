// Move use case: place one tag relative to a sibling.
package tag

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MoveTag places the tag immediately after req.AfterId (nil = first) and
// returns the full available list.
//
// Only the OWNER's tags are repositioned: a shared tag belongs to another
// user's list, so a sharee's move silently no-ops (issue #108). The RESPONSE is
// still the full available list (own + shared) via the read view.
func (s *Service) MoveTag(ctx context.Context, userID vo.Id, req model.MoveTagRequest) (*model.MoveTagResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	afterID := ""
	if req.AfterId != nil {
		afterID = *req.AfterId
	}

	var items []model.TagResult
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rows, lerr := s.repo.ListByOwner(ctx, userID)
		if lerr != nil {
			return lerr
		}
		moved, key, ok, kerr := sortkey.MoveWithin(rows, id.String(), afterID, tagItem, sortkey.GrowsUp)
		if kerr != nil {
			return kerr
		}
		if ok {
			moved.UpdateSortKey(key, s.clock.Now())
			if serr := s.repo.Save(ctx, moved); serr != nil {
				return serr
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
	return &model.MoveTagResult{Items: items}, nil
}

func tagItem(t *model.Tag) sortkey.Item {
	return sortkey.Item{ID: t.ID.String(), Key: t.SortKey}
}
