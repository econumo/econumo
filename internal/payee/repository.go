package payee

import (
	"context"
	"time"

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

	// CountByOwner returns the number of payees the owner has (used to seed a new
	// payee's position).
	CountByOwner(ctx context.Context, userID vo.Id) (int, error)

	Save(ctx context.Context, p *model.Payee) error

	// Delete removes a payee. Transactions referencing it have payee_id set to
	// NULL via the ON DELETE SET NULL FK.
	Delete(ctx context.Context, id vo.Id) error

	// UsageCounts returns, for each of the owner's payees that has at least one
	// transaction with spent_at >= since, the count of such transactions.
	UsageCounts(ctx context.Context, userID vo.Id, since time.Time) (map[string]int, error)
}
