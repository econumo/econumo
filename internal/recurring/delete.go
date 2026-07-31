package recurring

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) DeleteRecurringTransaction(ctx context.Context, userID vo.Id, req model.DeleteRecurringTransactionRequest) (*model.DeleteRecurringTransactionResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rt, gerr := s.repo.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if aerr := s.checkWriteAccess(ctx, userID, rt.AccountID); aerr != nil {
			return aerr
		}
		return s.repo.Delete(ctx, id)
	}); err != nil {
		return nil, err
	}
	return &model.DeleteRecurringTransactionResult{}, nil
}
