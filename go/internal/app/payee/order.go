// Order use case: apply position changes to the user's payees.
package payee

import (
	"context"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// OrderPayeeList applies each {id, position} change to the matching payee owned
// by the user, saving only the ones that actually changed, then returns the
// full ordered list.
//
// Changes referencing an id the user does not own are ignored (only payees
// found among the owner's are mutated).
func (s *Service) OrderPayeeList(ctx context.Context, userID vo.Id, req OrderPayeeListRequest) (*OrderPayeeListResult, error) {
	// Build an id -> position map from the changes.
	positions := make(map[string]int16, len(req.Changes))
	for _, ch := range req.Changes {
		id, err := vo.ParseId(ch.Id)
		if err != nil {
			return nil, err
		}
		positions[id.String()] = int16(ch.Position)
	}

	var items []PayeeResult
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		payees, err := s.repo.ListByOwner(txCtx, userID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		for _, p := range payees {
			pos, ok := positions[p.Id().String()]
			if !ok {
				continue
			}
			before := p.Position()
			p.UpdatePosition(pos, now)
			if p.Position() != before {
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

	return &OrderPayeeListResult{Items: items}, nil
}
