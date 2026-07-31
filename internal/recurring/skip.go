package recurring

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// SkipRecurringTransaction advances the schedule without posting a
// transaction — the caller acknowledges the occurrence but doesn't want it
// recorded.
func (s *Service) SkipRecurringTransaction(ctx context.Context, userID vo.Id, req model.SkipRecurringTransactionRequest) (*model.SkipRecurringTransactionResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	var rt *model.RecurringTransaction
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		var gerr error
		rt, gerr = s.repo.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if aerr := s.checkWriteAccess(ctx, userID, rt.AccountID); aerr != nil {
			return aerr
		}
		rt.Advance(s.clock.Now())
		return s.repo.Save(ctx, rt)
	}); err != nil {
		return nil, err
	}
	return &model.SkipRecurringTransactionResult{Item: toResult(rt)}, nil
}
