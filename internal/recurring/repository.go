package recurring

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

type Repository interface {
	NextIdentity() vo.Id
	GetByID(ctx context.Context, id vo.Id) (*model.RecurringTransaction, error)
	ListByAccountIDs(ctx context.Context, accountIDs []vo.Id) ([]*model.RecurringTransaction, error)
	Save(ctx context.Context, rt *model.RecurringTransaction) error
	Delete(ctx context.Context, id vo.Id) error
}
