package tag

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// OrderTagList applies each {id, position} change to the matching tag, then
// returns the full available list.
//
// Reordering is OWNER-ONLY (mirrors category): the changes iterate the caller's
// own tags, so a SHARED tag's position is never updated — a sharee, guest
// included, cannot rewrite the owner's global ordering (issue #108); shared ids
// in the changes list are silently ignored. The RESPONSE, however, is the full
// available list (own + shared) via the read view.
func (s *Service) OrderTagList(ctx context.Context, userID vo.Id, req model.OrderTagListRequest) (*model.OrderTagListResult, error) {
	positions := make(map[string]int16, len(req.Changes))
	for _, ch := range req.Changes {
		id, err := vo.ParseId(ch.Id)
		if err != nil {
			return nil, err
		}
		positions[id.String()] = int16(ch.Position)
	}

	var items []model.TagResult
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		tags, err := s.repo.ListByOwner(ctx, userID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		for _, t := range tags {
			pos, ok := positions[t.ID.String()]
			if !ok {
				continue
			}
			before := t.Position
			t.UpdatePosition(pos, now)
			if t.Position != before {
				if serr := s.repo.Save(ctx, t); serr != nil {
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

	return &model.OrderTagListResult{Items: items}, nil
}
