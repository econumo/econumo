// Delete use case: remove a payee (unconditional — no replace mode).
package payee

import (
	"context"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// DeletePayee deletes the payee. The user must own it; an ownership failure
// surfaces as an AccessDenied (HTTP 403), mirroring the PHP
// PayeeService::deletePayee flow (the application service checks ownership
// before delegating). Transactions referencing the payee have payee_id set to
// NULL via the ON DELETE SET NULL FK.
//
// Like tag-delete, there is no mode/replaceId — payee delete is unconditional.
// Returns an empty result ({}).
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
		if !p.UserId().Equal(userID) {
			return errs.NewAccessDenied("Access denied")
		}
		return s.repo.Delete(txCtx, id)
	}); err != nil {
		return nil, err
	}

	return &DeletePayeeResult{}, nil
}
