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

	// ReplaceLabels rewrites a template's label links; the caller runs this
	// inside the same tx as Save so the classification and the row commit or
	// roll back together.
	ReplaceLabels(ctx context.Context, recurringID vo.Id, labelIDs []vo.Id) error

	// LabelsByRecurringIDs batch-loads label ids for many templates, keyed by
	// template id (string), for attaching to already-hydrated results.
	LabelsByRecurringIDs(ctx context.Context, ids []vo.Id) (map[string][]string, error)
}
