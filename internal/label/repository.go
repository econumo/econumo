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

	// ListByOwner returns the owner's labels ordered by position.
	ListByOwner(ctx context.Context, userID vo.Id) ([]*model.Label, error)

	// CountByOwner returns the number of labels the owner has (used to seed a
	// new label's position).
	CountByOwner(ctx context.Context, userID vo.Id) (int, error)

	Save(ctx context.Context, l *model.Label) error

	// Delete removes a label; its transactions_labels rows go with it via the
	// ON DELETE CASCADE FK.
	Delete(ctx context.Context, id vo.Id) error
}

// ReadModel is the label read side (CQRS): available labels for a user are
// their own plus every user's who has shared an account with them.
type ReadModel interface {
	LabelListView(ctx context.Context, userID string) ([]model.LabelViewRow, error)
}
