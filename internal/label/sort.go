// Sort use case: apply an explicit order to the caller's own labels.
package label

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// SortLabelList reorders the caller's labels to match req.Ids and returns the
// full available list.
//
// A drag is MoveLabel; this exists because sorting a whole list -- the A-Z
// action in settings -- changes every row's neighbour, which no single relative
// move can express. Only the rows that would break the requested sequence are
// written, so re-sorting an already-sorted list writes nothing.
//
// Ids the caller does not own are skipped, exactly as MoveLabel skips them: a
// sharee cannot reorder the owner's list. Ids that name a TAG are likewise
// absent from this owner-only load and skipped, so the two lists keep
// independent sequences.
func (s *Service) SortLabelList(ctx context.Context, userID vo.Id, req model.SortLabelListRequest) (*model.SortLabelListResult, error) {
	var items []model.LabelResult
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rows, lerr := s.repo.ListByOwner(ctx, userID)
		if lerr != nil {
			return lerr
		}
		changed, kerr := sortkey.Resequence(rows, req.Ids, labelItem, sortkey.GrowsUp)
		if kerr != nil {
			return kerr
		}
		now := s.clock.Now()
		for _, row := range rows {
			key, ok := changed[row.ID.String()]
			if !ok {
				continue
			}
			row.UpdateSortKey(key, now)
			if serr := s.repo.Save(ctx, row); serr != nil {
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
	return &model.SortLabelListResult{Items: items}, nil
}
