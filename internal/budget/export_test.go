package budget

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) SyncElementsForTest(ctx context.Context, budgetID vo.Id) error {
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		return s.syncElements(ctx, budgetID, s.clock.Now())
	})
}
