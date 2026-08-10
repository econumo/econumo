package recurring

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
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
		if !st.Type.IsTransfer() {
			ownerID, operr := s.accounts.AccountOwner(ctx, st.AccountID)
			if operr != nil {
				return &errs.ValidationError{Msg: "account.account.not_available", MsgCode: errs.CodeTransactionAccountNotAvailable}
			}
			ids, lerr := s.resolveLabels(ctx, ownerID, req.LabelIds)
			if lerr != nil {
				return lerr
			}
			st.LabelIDs = ids
		}
		rt.Update(st, s.clock.Now())
		updated = rt
		if serr := s.repo.Save(ctx, rt); serr != nil {
			return serr
		}
		return s.repo.ReplaceLabels(ctx, rt.ID, rt.LabelIDs)
	}); err != nil {
		return nil, err
	}
	return &model.UpdateRecurringTransactionResult{Item: toResult(updated, labelIDStrings(updated.LabelIDs))}, nil
}
