package category

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Repository is the category aggregate's persistence port; the application
// service depends only on this interface. A missing category returns an
// *errs.NotFoundError so the HTTP layer maps it consistently.
type Repository interface {
	// NextIdentity allocates a fresh aggregate id (no DB round-trip).
	NextIdentity() vo.Id

	GetByID(ctx context.Context, id vo.Id) (*model.Category, error)

	// ListByOwner returns the owner's categories ordered by position.
	ListByOwner(ctx context.Context, userID vo.Id) ([]*model.Category, error)

	// CountByOwner returns the number of categories the owner has (used to seed
	// a new category's position).
	CountByOwner(ctx context.Context, userID vo.Id) (int, error)

	Save(ctx context.Context, c *model.Category) error

	// Delete removes a category. Transactions referencing it have category_id set
	// to NULL via the ON DELETE SET NULL FK.
	Delete(ctx context.Context, id vo.Id) error

	// ReassignTransactions points every transaction on oldID at newID (replace
	// mode), before the old category is deleted.
	ReassignTransactions(ctx context.Context, oldID, newID vo.Id) error
}
