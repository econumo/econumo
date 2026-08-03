// Move use case: place one payee relative to a sibling.
package payee

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MovePayee places the payee immediately after req.AfterId (nil = first) and
// returns the full available list.
//
// Only the OWNER's payees are repositioned: a shared payee belongs to another
// user's list, so a sharee's move silently no-ops (issue #108). The RESPONSE is
// still the full available list (own + shared) via the read view.
func (s *Service) MovePayee(ctx context.Context, userID vo.Id, req model.MovePayeeRequest) (*model.MovePayeeResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	afterID := ""
	if req.AfterId != nil {
		afterID = *req.AfterId
	}

	var items []model.PayeeResult
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		rows, lerr := s.repo.ListByOwner(txCtx, userID)
		if lerr != nil {
			return lerr
		}
		key, found, kerr := sortkey.Relocate(rows, id.String(), afterID, payeeItem, sortkey.GrowsUp)
		if kerr != nil {
			return kerr
		}
		if found {
			for _, p := range rows {
				if !p.ID.Equal(id) {
					continue
				}
				p.UpdateSortKey(key, s.clock.Now())
				if serr := s.repo.Save(txCtx, p); serr != nil {
					return serr
				}
				break
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
	return &model.MovePayeeResult{Items: items}, nil
}

func payeeItem(p *model.Payee) sortkey.Item {
	return sortkey.Item{ID: p.ID.String(), Key: p.SortKey}
}
