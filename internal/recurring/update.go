package recurring

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) UpdateRecurringTransaction(ctx context.Context, userID vo.Id, req model.UpdateRecurringTransactionRequest) (*model.UpdateRecurringTransactionResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	st, err := s.buildState(req.Type, req.AccountId, req.AccountRecipientId, req.Amount.String(),
		req.CategoryId, req.PayeeId, req.TagId, req.Description, req.Schedule, req.NextPaymentAt)
	if err != nil {
		return nil, err
	}

	var updated *model.RecurringTransaction
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rt, gerr := s.repo.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if aerr := s.checkWriteAccess(ctx, userID, rt.AccountID); aerr != nil {
			return aerr
		}
		// moving the template to another account also needs write access there
		if !st.AccountID.Equal(rt.AccountID) {
			if aerr := s.checkWriteAccess(ctx, userID, st.AccountID); aerr != nil {
				return aerr
			}
		}
		rt.Update(st, s.clock.Now())
		updated = rt
		return s.repo.Save(ctx, rt)
	}); err != nil {
		return nil, err
	}
	return &model.UpdateRecurringTransactionResult{Item: toResult(updated)}, nil
}
