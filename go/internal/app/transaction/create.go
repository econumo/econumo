// Create use case: create a transaction (idempotent on the request id).
package transaction

import (
	"context"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	domtransaction "github.com/econumo/econumo/internal/domain/transaction"
)

// CreateTransaction creates a transaction for the current user and returns it
// plus the refreshed account list (balances updated). The user must have access
// to the account (ownership, in the single-user reduction).
func (s *Service) CreateTransaction(ctx context.Context, userID vo.Id, req CreateTransactionRequest) (*CreateTransactionResult, error) {
	// req.Id is the operation/idempotency id (PHP locks on it); the entity id is
	// freshly minted (PHP transactionRepository->getNextIdentity()).
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

	var created *domtransaction.Transaction
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if aerr := s.checkAccountOwned(ctx, userID, accountID); aerr != nil {
			return aerr
		}
		already, cerr := s.ops.Claim(ctx, opID, s.clock.Now())
		if cerr != nil {
			return cerr
		}
		if already {
			return errs.NewValidation("Operation is locked")
		}
		now := s.clock.Now()
		st, berr := buildState(id, userID, typ, accountID, req.Amount,
			req.AmountRecipient, req.AccountRecipientId, req.CategoryId, req.PayeeId, req.TagId,
			description, spentAt, now)
		if berr != nil {
			return berr
		}
		t := domtransaction.New(st)
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
	return &CreateTransactionResult{Item: item, Accounts: accounts}, nil
}
