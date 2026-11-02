package payee

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DeletePayee deletes the payee. The user must own it; an ownership failure
// surfaces as AccessDenied (HTTP 403). Transactions referencing the payee have
// payee_id set to NULL via the ON DELETE SET NULL FK. Delete is unconditional —
// to keep them, use merge-payee instead.
func (s *Service) DeletePayee(ctx context.Context, userID vo.Id, req model.DeletePayeeRequest) (*model.DeletePayeeResult, error) {
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
			// Foreign-owned payee: report as not-found (matching the repo above) so
			// the response can't probe which payee ids exist.
			return errs.NewNotFound("Payee not found")
		}
		if s.debts != nil {
			used, uerr := s.debts.InUse(txCtx, id)
			if uerr != nil {
				return uerr
			}
			if used {
				return &errs.ValidationError{Msg: "A debt uses this payee. Delete or change the debt first.", MsgCode: errs.CodePayeeUsedByDebt}
			}
		}
		return s.repo.Delete(txCtx, id)
	}); err != nil {
		return nil, err
	}

	return &model.DeletePayeeResult{}, nil
}
