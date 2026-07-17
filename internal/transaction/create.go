package transaction

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateTransaction creates a transaction for the current user and returns it
// plus the refreshed account list (balances updated). The user must have write
// access to the account: they own it, or hold an admin/user grant on it (see
// checkWriteAccess).
func (s *Service) CreateTransaction(ctx context.Context, userID vo.Id, req model.CreateTransactionRequest) (*model.CreateTransactionResult, error) {
	// req.Id is the operation/idempotency id; the entity id is freshly minted.
	opID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	id := vo.NewId()
	typ, err := parseType(req.Type)
	if err != nil {
		return nil, err
	}
	accountID, err := vo.ParseId(req.AccountId)
	if err != nil {
		return nil, err
	}
	spentAt, err := parseSpentAt(req.Date)
	if err != nil {
		return nil, err
	}
	description := ""
	if req.Description != nil {
		description = *req.Description
	}

	var created *model.Transaction
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if aerr := s.checkWriteAccess(ctx, userID, accountID, "account.account.not_available"); aerr != nil {
			return aerr
		}
		already, cerr := s.ops.Claim(ctx, opID, s.clock.Now())
		if cerr != nil {
			return cerr
		}
		if already {
			return &errs.ValidationError{Msg: "Operation is locked", MsgCode: errs.CodeOperationLocked}
		}
		now := s.clock.Now()
		st, berr := buildState(id, userID, typ, accountID, req.Amount.String(),
			req.AmountRecipient.StrPtr(), req.AccountRecipientId, req.CategoryId, req.PayeeId, req.TagId,
			description, spentAt, now)
		if berr != nil {
			return berr
		}
		if nerr := s.normalizeTransferAmounts(ctx, &st); nerr != nil {
			return nerr
		}
		t := model.New(st)
		if serr := s.repo.Save(ctx, t); serr != nil {
			return serr
		}
		if merr := s.ops.MarkHandled(ctx, opID, now); merr != nil {
			return merr
		}
		created = t
		return nil
	}); err != nil {
		return nil, err
	}

	item, err := s.toResult(ctx, created)
	if err != nil {
		return nil, err
	}
	accounts, err := s.accountListEmbed(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &model.CreateTransactionResult{Item: item, Accounts: accounts}, nil
}
