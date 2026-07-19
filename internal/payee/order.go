package payee

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// OrderPayeeList applies each {id, position} change to the matching payee, then
// returns the full available list.
//
// Reordering is OWNER-ONLY (mirrors category): the changes iterate the caller's
// own payees, so a SHARED payee's position is never updated — a sharee, guest
// included, cannot rewrite the owner's global ordering (issue #108); shared ids
// in the changes list are silently ignored. The RESPONSE, however, is the full
// available list (own + shared) via the read view.
func (s *Service) OrderPayeeList(ctx context.Context, userID vo.Id, req model.OrderPayeeListRequest) (*model.OrderPayeeListResult, error) {
	positions := make(map[string]int16, len(req.Changes))
	for _, ch := range req.Changes {
		id, err := vo.ParseId(ch.Id)
		if err != nil {
			return nil, err
		}
		positions[id.String()] = int16(ch.Position)
	}

	var items []model.PayeeResult
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		payees, err := s.repo.ListByOwner(txCtx, userID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		for _, p := range payees {
			pos, ok := positions[p.ID.String()]
			if !ok {
				continue
			}
			before := p.Position
			p.UpdatePosition(pos, now)
			if p.Position != before {
				if serr := s.repo.Save(txCtx, p); serr != nil {
					return serr
				}
			}
		}
		built, berr := s.listResults(txCtx, userID)
		if berr != nil {
			return berr
		}
		items = built
		return nil
	}); err != nil {
		return nil, err
	}

	return &model.OrderPayeeListResult{Items: items}, nil
}
