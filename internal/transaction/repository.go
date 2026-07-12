package transaction

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Repository is the transaction aggregate's persistence port. The app service
// depends only on this interface. A missing transaction returns
// *errs.NotFoundError.
type Repository interface {
	// NextIdentity allocates a fresh transaction id.
	NextIdentity() vo.Id

	GetByID(ctx context.Context, id vo.Id) (*model.Transaction, error)

	Save(ctx context.Context, t *model.Transaction) error

	Delete(ctx context.Context, id vo.Id) error

	// ListByAccount returns transactions where the account is source or recipient,
	// newest first (spent_at DESC).
	ListByAccount(ctx context.Context, accountID vo.Id) ([]*model.Transaction, error)

	// ListByAccountIDs returns transactions whose source OR recipient account is in
	// accountIDs, optionally bounded by [periodStart, periodEnd). With no period,
	// pass zero times. Used for the user-wide visible-accounts list.
	ListByAccountIDs(ctx context.Context, accountIDs []vo.Id, periodStart, periodEnd time.Time) ([]*model.Transaction, error)

	// ListPageByAccount returns up to limit transactions where the account is
	// source or recipient, strictly older than after in the (spent_at DESC, id)
	// order (nil after = from the newest). Callers pass limit+1 to detect a
	// further page.
	ListPageByAccount(ctx context.Context, accountID vo.Id, after *PageCursor, limit int) ([]*model.Transaction, error)

	// ListRecentByAccountIDs returns, per account id, its newest transactions
	// (source or recipient) in (spent_at DESC, id) order, at most perAccountLimit
	// each. A transfer between two requested accounts appears in both windows.
	ListRecentByAccountIDs(ctx context.Context, accountIDs []vo.Id, perAccountLimit int) (map[string][]*model.Transaction, error)
}
