package transaction

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/shared/vo"
)

// Repository is the transaction aggregate's persistence port. The app service
// depends only on this interface. A missing transaction returns
// *errs.NotFoundError.
type Repository interface {
	// NextIdentity allocates a fresh transaction id.
	NextIdentity() vo.Id

	GetByID(ctx context.Context, id vo.Id) (*Transaction, error)

	Save(ctx context.Context, t *Transaction) error

	Delete(ctx context.Context, id vo.Id) error

	// ListByAccount returns transactions where the account is source or recipient,
	// newest first (spent_at DESC).
	ListByAccount(ctx context.Context, accountID vo.Id) ([]*Transaction, error)

	// ListByAccountIDs returns transactions whose source OR recipient account is in
	// accountIDs, optionally bounded by [periodStart, periodEnd). With no period,
	// pass zero times. Used for the user-wide visible-accounts list.
	ListByAccountIDs(ctx context.Context, accountIDs []vo.Id, periodStart, periodEnd time.Time) ([]*Transaction, error)
}
