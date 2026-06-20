package category

import (
	"context"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// Repository is the category aggregate's persistence port. The infra layer
// implements it over the sqlc-generated queries; the application service depends
// only on this interface. A missing category returns an *errs.NotFoundError so
// the HTTP layer maps it consistently.
//
// Persistence is whole-aggregate: Save upserts the category row (the service
// runs it inside WithTx).
type Repository interface {
	// NextIdentity allocates a fresh aggregate id (no DB round-trip).
	NextIdentity() vo.Id

	// GetByID loads a category by id. Missing -> *errs.NotFoundError.
	GetByID(ctx context.Context, id vo.Id) (*Category, error)

	// ListByOwner returns the owner's categories ordered by position.
	ListByOwner(ctx context.Context, userID vo.Id) ([]*Category, error)

	// CountByOwner returns the number of categories the owner has (used to seed
	// a new category's position).
	CountByOwner(ctx context.Context, userID vo.Id) (int, error)

	// Save upserts a category. Intended to run inside WithTx.
	Save(ctx context.Context, c *Category) error

	// Delete removes a category. Transactions referencing it have category_id set
	// to NULL via the ON DELETE SET NULL FK.
	Delete(ctx context.Context, id vo.Id) error

	// ReassignTransactions points every transaction on oldID at newID (replace
	// mode), before the old category is deleted.
	ReassignTransactions(ctx context.Context, oldID, newID vo.Id) error
}
