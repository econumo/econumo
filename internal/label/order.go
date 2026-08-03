package label

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// OrderLabelList applies each {id, position} change to the matching label,
// then returns the full available list.
//
// Reordering is OWNER-ONLY (mirrors tag/category): the changes iterate the
// caller's own labels, so a SHARED label's position is never updated — a
// sharee, guest included, cannot rewrite the owner's global ordering; shared
// ids in the changes list are silently ignored. The RESPONSE, however, is the
// full available list (own + shared) via the read view.
func (s *Service) OrderLabelList(ctx context.Context, userID vo.Id, req model.OrderLabelListRequest) (*model.OrderLabelListResult, error) {
	positions := make(map[string]int16, len(req.Changes))
	for _, ch := range req.Changes {
		id, err := vo.ParseId(ch.Id)
		if err != nil {
			return nil, err
		}
		positions[id.String()] = int16(ch.Position)
	}

	var items []model.LabelResult
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		labels, err := s.repo.ListByOwner(ctx, userID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		for _, l := range labels {
			pos, ok := positions[l.ID.String()]
			if !ok {
				continue
			}
			before := l.Position
			l.UpdatePosition(pos, now)
			if l.Position != before {
				if serr := s.repo.Save(ctx, l); serr != nil {
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

	return &model.OrderLabelListResult{Items: items}, nil
}
