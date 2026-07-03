package payee

import (
	"context"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DeletePayee deletes the payee. The user must own it; an ownership failure
// surfaces as AccessDenied (HTTP 403). Transactions referencing the payee have
// payee_id set to NULL via the ON DELETE SET NULL FK. Delete is unconditional —
// there is no mode/replaceId.
func (s *Service) DeletePayee(ctx context.Context, userID vo.Id, req DeletePayeeRequest) (*DeletePayeeResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}

	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		p, gerr := s.repo.GetByID(txCtx, id)
		if gerr != nil {
			return gerr
		}
		if !p.UserID.Equal(userID) {
			return errs.NewAccessDenied("")
		}
		return s.repo.Delete(txCtx, id)
	}); err != nil {
		return nil, err
	}

	return &DeletePayeeResult{}, nil
}
