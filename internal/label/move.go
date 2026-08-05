// Move use case: place one label relative to a sibling.
package label

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MoveLabel places the label immediately after req.AfterId (nil = first) and
// returns the full available list.
//
// Only the OWNER's labels are repositioned: a shared label belongs to another
// user's list, so a sharee's move silently no-ops. The RESPONSE is still the
// full available list (own + shared) via the read view.
//
// Labels carry their own sort keys in their own table, so this never touches
// the tag ordering even when both lists are shown side by side.
func (s *Service) MoveLabel(ctx context.Context, userID vo.Id, req model.MoveLabelRequest) (*model.MoveLabelResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	afterID := ""
	if req.AfterId != nil {
		afterID = *req.AfterId
	}

	var items []model.LabelResult
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rows, lerr := s.repo.ListByOwner(ctx, userID)
		if lerr != nil {
			return lerr
		}
		moved, key, ok, kerr := sortkey.MoveWithin(rows, id.String(), afterID, labelItem, sortkey.GrowsUp)
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
	return &model.MoveLabelResult{Items: items}, nil
}

func labelItem(l *model.Label) sortkey.Item {
	return sortkey.Item{ID: l.ID.String(), Key: l.SortKey}
}
