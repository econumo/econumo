package payee

import (
	"context"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// Repository is the payee aggregate's persistence port. The infra layer
// implements it over the sqlc-generated queries; the application service depends
// only on this interface. A missing payee returns an *errs.NotFoundError so the
// HTTP layer maps it consistently.
//
// Persistence is whole-aggregate: Save upserts the payee row (the service runs
// it inside WithTx).
type Repository interface {
	// NextIdentity allocates a fresh aggregate id (no DB round-trip).
	NextIdentity() vo.Id

	// GetByID loads a payee by id. Missing -> *errs.NotFoundError.
	GetByID(ctx context.Context, id vo.Id) (*Payee, error)

	// ListByOwner returns the owner's payees ordered by position.
	ListByOwner(ctx context.Context, userID vo.Id) ([]*Payee, error)

	// CountByOwner returns the number of payees the owner has (used to seed a new
	// payee's position).
	CountByOwner(ctx context.Context, userID vo.Id) (int, error)

	// Save upserts a payee. Intended to run inside WithTx.
	Save(ctx context.Context, p *Payee) error

	// Delete removes a payee. Transactions referencing it have payee_id set to
	// NULL via the ON DELETE SET NULL FK.
	Delete(ctx context.Context, id vo.Id) error
}
