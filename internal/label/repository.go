package label

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Repository is the label aggregate's persistence port; the application
// service depends only on this interface. A missing label returns an
// *errs.NotFoundError so the HTTP layer maps it consistently.
type Repository interface {
	// NextIdentity allocates a fresh aggregate id (no DB round-trip).
	NextIdentity() vo.Id

	GetByID(ctx context.Context, id vo.Id) (*model.Label, error)

	// ListByOwner returns the owner's labels ordered by sort key.
	ListByOwner(ctx context.Context, userID vo.Id) ([]*model.Label, error)

	Save(ctx context.Context, l *model.Label) error

	// Delete removes a label; its transactions_labels and
	// recurring_transactions_labels rows go with it via the ON DELETE CASCADE
	// FKs.
	Delete(ctx context.Context, id vo.Id) error

	// ReassignTransactions / ReassignRecurring point every row on oldID at newID
	// (merge), before the old row is deleted.
	ReassignTransactions(ctx context.Context, oldID, newID vo.Id) error
	ReassignRecurring(ctx context.Context, oldID, newID vo.Id) error
}

// ReadModel is the label read side (CQRS): available labels for a user are
// their own plus every user's who has shared an account with them.
type ReadModel interface {
	LabelListView(ctx context.Context, userID string) ([]model.LabelViewRow, error)
}
