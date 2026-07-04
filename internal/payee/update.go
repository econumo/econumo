package payee

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UpdatePayee loads the payee, checks ownership (403 otherwise), enforces name
// uniqueness among the owner's payees (excluding itself), updates the name, and
// returns the refreshed item.
func (s *Service) UpdatePayee(ctx context.Context, userID vo.Id, req model.UpdatePayeeRequest) (*model.UpdatePayeeResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	name, err := newPayeeName(req.Name)
	if err != nil {
		return nil, err
	}
	if _, err := s.mutateChecked(ctx, id, userID, func(txCtx context.Context, p *model.Payee, now time.Time) error {
		if uerr := s.ensureNameUnique(txCtx, userID, name, id); uerr != nil {
			return uerr
		}
		p.UpdateName(name, now)
		return nil
	}); err != nil {
		return nil, err
	}
	return &model.UpdatePayeeResult{}, nil
}
