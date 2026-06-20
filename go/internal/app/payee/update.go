// Update use case: change a payee's name.
package payee

import (
	"context"
	"time"

	dompayee "github.com/econumo/econumo/internal/domain/payee"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// UpdatePayee loads the payee, checks ownership (403 otherwise), enforces name
// uniqueness among the owner's payees (excluding itself), updates the name, and
// returns the refreshed item.
func (s *Service) UpdatePayee(ctx context.Context, userID vo.Id, req UpdatePayeeRequest) (*UpdatePayeeResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	name, err := newPayeeName(req.Name)
	if err != nil {
		return nil, err
	}
	p, err := s.mutateChecked(ctx, id, userID, func(txCtx context.Context, p *dompayee.Payee, now time.Time) error {
		if uerr := s.ensureNameUnique(txCtx, userID, name, id); uerr != nil {
			return uerr
		}
		p.UpdateName(name, now)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &UpdatePayeeResult{Item: toResult(p)}, nil
}
