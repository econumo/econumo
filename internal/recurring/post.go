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
	// An explicit list (empty included) is the caller's own set; omitting the
	// field means "inherit the template's labels", resolved inside the tx below.
	// Either way the ids ride the create request, so the create path's reference
	// checks apply to both: labels must belong to the posted account's owner
	// (req.AccountId may differ from the template's account), and a transfer
	// ignores them — inherited labels must not leak onto a transfer either.
	if req.LabelIds != nil {
		createReq.LabelIds = *req.LabelIds
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
		if req.LabelIds == nil {
			labelIDs, lerr := s.labelIDsFor(ctx, rtID)
			if lerr != nil {
				return lerr
			}
			createReq.LabelIds = labelIDStrings(labelIDs)
		}
		created, gerr = s.creator.CreateTransactionFromRecurring(ctx, userID, createReq, rtID)
		if gerr != nil {
			return gerr
		}
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
