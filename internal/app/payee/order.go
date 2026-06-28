// Order use case: apply position changes to the user's payees.
package payee

import (
	"context"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// OrderPayeeList applies each {id, position} change to the matching payee in the
// user's AVAILABLE set (own + shared via account access), saving those that
// changed, then returns the full ordered list. Mirrors PHP's
// PayeeService::orderPayees, which iterates findAvailableForUserId and updates+
// saves each named payee (a SHARED payee's position is updated too).
func (s *Service) OrderPayeeList(ctx context.Context, userID vo.Id, req OrderPayeeListRequest) (*OrderPayeeListResult, error) {
	positions := make(map[string]int16, len(req.Changes))
	order := make([]string, 0, len(req.Changes))
	for _, ch := range req.Changes {
		id, err := vo.ParseId(ch.Id)
		if err != nil {
			return nil, err
		}
		if _, seen := positions[id.String()]; !seen {
			order = append(order, id.String())
		}
		positions[id.String()] = int16(ch.Position)
	}

	var items []PayeeResult
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		avail, err := s.read.PayeeListView(txCtx, userID.String())
		if err != nil {
			return err
		}
		available := make(map[string]struct{}, len(avail))
		for _, r := range avail {
			available[r.ID] = struct{}{}
		}
		now := s.clock.Now()
		for _, idStr := range order {
			if _, ok := available[idStr]; !ok {
				continue
			}
			id, _ := vo.ParseId(idStr)
			p, gerr := s.repo.GetByID(txCtx, id)
			if gerr != nil {
				return gerr
			}
			before := p.Position()
			p.UpdatePosition(positions[idStr], now)
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
