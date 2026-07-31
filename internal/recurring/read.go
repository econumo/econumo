package recurring

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) GetRecurringTransactionList(ctx context.Context, userID vo.Id) (*model.GetRecurringTransactionListResult, error) {
	accountIDs, err := s.visible.VisibleAccountIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListByAccountIDs(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	out := make([]model.RecurringTransactionResult, 0, len(items))
	for _, rt := range items {
		out = append(out, toResult(rt))
	}
	return &model.GetRecurringTransactionListResult{Items: out}, nil
}
