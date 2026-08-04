package recurring

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/vo"
)

// PostRecurringTransaction materializes one instance of a recurring template
// as a real transaction and advances the schedule. Both the create and the
// advance happen in one outer WithTx: CreateTransaction's own WithTx nests as
// a SAVEPOINT (see backend.TxManager), so a replayed post (idempotency lock
// on req.Id) rolls back the advance too — no double-create, no double-advance.
func (s *Service) PostRecurringTransaction(ctx context.Context, userID vo.Id, req model.PostRecurringTransactionRequest) (*model.PostRecurringTransactionResult, error) {
	rtID, err := vo.ParseId(req.RecurringId)
	if err != nil {
		return nil, err
	}
	createReq := model.CreateTransactionRequest{
		Id:                 req.Id,
		Type:               req.Type,
		Amount:             req.Amount,
		AmountRecipient:    req.AmountRecipient,
		AccountId:          req.AccountId,
		AccountRecipientId: req.AccountRecipientId,
		CategoryId:         req.CategoryId,
		Date:               req.Date,
		Description:        req.Description,
		PayeeId:            req.PayeeId,
		TagId:              req.TagId,
	}
	if verr := createReq.Validate(); verr != nil {
		return nil, verr
	}

	var created *model.CreateTransactionResult
	var rt *model.RecurringTransaction
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		var gerr error
		rt, gerr = s.repo.GetByID(ctx, rtID)
		if gerr != nil {
			return gerr
		}
		if aerr := s.checkWriteAccess(ctx, userID, rt.AccountID); aerr != nil {
			return aerr
		}
		labelIDs, lerr := s.labelIDsFor(ctx, rtID)
		if lerr != nil {
			return lerr
		}
		rt.LabelIDs = labelIDs
		created, gerr = s.creator.CreateTransactionFromRecurring(ctx, userID, createReq, rtID)
		if gerr != nil {
			return gerr
		}
		// Copy the template's labels onto the just-created transaction, inside
		// this same tx: a failure here rolls back the create too. This is a
		// second ReplaceLabels for the row (createTransaction already ran one
		// with req.LabelIds, always empty here since post never sends any) —
		// delete-then-insert makes the second call authoritative rather than
		// additive, so the two calls settle on exactly the template's set.
		txID, perr := vo.ParseId(created.Item.Id)
		if perr != nil {
			return perr
		}
		if rerr := s.creator.ReplaceLabels(ctx, txID, rt.LabelIDs); rerr != nil {
			return rerr
		}
		wireLabels := labelIDStrings(rt.LabelIDs)
		if wireLabels == nil {
			wireLabels = []string{}
		}
		created.Item.LabelIds = wireLabels
		rt.Advance(s.clock.Now())
		return s.repo.Save(ctx, rt)
	}); err != nil {
		return nil, err
	}
	return &model.PostRecurringTransactionResult{
		Item:          created.Item,
		Accounts:      created.Accounts,
		NextPaymentAt: rt.NextPaymentAt.Format(datetime.Layout),
	}, nil
}
