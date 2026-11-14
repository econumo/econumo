package payee

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Repository is the payee aggregate's persistence port; the application service
// depends only on this interface. A missing payee returns an *errs.NotFoundError
// so the HTTP layer maps it consistently.
type Repository interface {
	// NextIdentity allocates a fresh aggregate id (no DB round-trip).
	NextIdentity() vo.Id

	GetByID(ctx context.Context, id vo.Id) (*model.Payee, error)

	// ListByOwner returns the owner's payees ordered by position.
	ListByOwner(ctx context.Context, userID vo.Id) ([]*model.Payee, error)

	Save(ctx context.Context, p *model.Payee) error

	// Delete removes a payee. Transactions referencing it have payee_id set to
	// NULL via the ON DELETE SET NULL FK.
	Delete(ctx context.Context, id vo.Id) error

	// ReassignTransactions / ReassignRecurring point every row on oldID at newID
	// (merge), before the old payee is deleted.
	ReassignTransactions(ctx context.Context, oldID, newID vo.Id) error
	ReassignRecurring(ctx context.Context, oldID, newID vo.Id) error
}
