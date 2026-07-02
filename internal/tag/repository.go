package tag

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// Repository is the tag aggregate's persistence port; the application service
// depends only on this interface. A missing tag returns an *errs.NotFoundError
// so the HTTP layer maps it consistently.
type Repository interface {
	// NextIdentity allocates a fresh aggregate id (no DB round-trip).
	NextIdentity() vo.Id

	GetByID(ctx context.Context, id vo.Id) (*Tag, error)

	// ListByOwner returns the owner's tags ordered by position.
	ListByOwner(ctx context.Context, userID vo.Id) ([]*Tag, error)

	// CountByOwner returns the number of tags the owner has (used to seed a new
	// tag's position).
	CountByOwner(ctx context.Context, userID vo.Id) (int, error)

	Save(ctx context.Context, t *Tag) error

	// Delete removes a tag. Transactions referencing it have tag_id set to NULL
	// via the ON DELETE SET NULL FK.
	Delete(ctx context.Context, id vo.Id) error
}
