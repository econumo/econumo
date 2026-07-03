package transaction

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// DeleteTransaction deletes the transaction and returns it (the deleted item)
// plus the refreshed account list. Write access is checked on the transaction's
// account; non-access yields a ValidationError
// ("transaction.transaction.not_available").
func (s *Service) DeleteTransaction(ctx context.Context, userID vo.Id, req DeleteTransactionRequest) (*DeleteTransactionResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}

	var deleted *Transaction
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		t, gerr := s.repo.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if aerr := s.checkWriteAccess(ctx, userID, t.AccountID, "transaction.transaction.not_available"); aerr != nil {
			return aerr
		}
		if derr := s.repo.Delete(ctx, id); derr != nil {
			return derr
		}
		deleted = t
		return nil
	}); err != nil {
		return nil, err
	}

	item, err := s.toResult(ctx, deleted)
	if err != nil {
		return nil, err
	}
	accounts, err := s.accountListEmbed(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &DeleteTransactionResult{Item: item, Accounts: accounts}, nil
}
