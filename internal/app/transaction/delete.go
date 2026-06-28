// Delete use case: remove a transaction the user has access to.
package transaction

import (
	"context"

	"github.com/econumo/econumo/internal/domain/shared/vo"
	domtransaction "github.com/econumo/econumo/internal/domain/transaction"
)

// DeleteTransaction deletes the transaction and returns it (the deleted item)
// plus the refreshed account list. Access is checked on the transaction's
// account (PHP checks canDeleteTransaction on the transaction's accountId);
// non-access yields a ValidationError ("transaction.transaction.not_available").
func (s *Service) DeleteTransaction(ctx context.Context, userID vo.Id, req DeleteTransactionRequest) (*DeleteTransactionResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}

	var deleted *domtransaction.Transaction
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		t, gerr := s.repo.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if aerr := s.checkWriteAccess(ctx, userID, t.AccountId(), "transaction.transaction.not_available"); aerr != nil {
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
