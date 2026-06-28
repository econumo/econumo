// Create use case: create a category, idempotent on the request id.
package category

import (
	"context"

	domcategory "github.com/econumo/econumo/internal/domain/category"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// CreateCategory creates a category for the current user and returns it.
//
// Idempotency: the request id doubles as the operation id. Inside the tx we
// Claim the id in operation_requests_ids; a second request with the same id
// finds the row already present and is rejected with a ValidationError
// ("Operation is locked"). See repo/category for the row semantics and the
// package README for the rationale.
//
// New-category position = count(user's existing categories); the new category is
// active with created/updated = now.
func (s *Service) CreateCategory(ctx context.Context, userID vo.Id, req CreateCategoryRequest) (*CreateCategoryResult, error) {
	// The request id is the OPERATION id (idempotency key), NOT the new entity's
	// id. PHP ignores $dto->id for the entity and mints a fresh UUIDv7 via
	// getNextIdentity() (CategoryFactory::create); the dto id is consumed only by
	// the operation-id middleware. Mirror that: claim opID, generate a new entity id.
	opID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	id := vo.NewId()
	name, err := newCategoryName(req.Name)
	if err != nil {
		return nil, err
	}
	typ, err := newCategoryType(req.Type)
	if err != nil {
		return nil, err
	}
	iconVal := defaultIcon
	if req.Icon != nil && *req.Icon != "" {
		iconVal = *req.Icon
	}
	icon, err := newIcon(iconVal)
	if err != nil {
		return nil, err
	}

	// accountId, when present, selects which user owns the new category in the
	// full app (an account may belong to a connected user). The connection module
	// is not ported yet, so we always create for the current user and ignore
	// accountId beyond validating its shape. See notes/README.
	if req.AccountId != nil && *req.AccountId != "" {
		if _, perr := vo.ParseId(*req.AccountId); perr != nil {
			return nil, perr
		}
	}

	var created *domcategory.Category
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		already, cerr := s.ops.Claim(ctx, opID, s.clock.Now())
		if cerr != nil {
			return cerr
		}
		if already {
			return errs.NewValidation("Operation is locked")
		}

		count, cerr := s.repo.CountByOwner(ctx, userID)
		if cerr != nil {
			return cerr
		}
		now := s.clock.Now()
		c := domcategory.NewCategory(id, userID, name, typ, icon, now)
		c.SetPosition(int16(count))
		if serr := s.repo.Save(ctx, c); serr != nil {
			return serr
		}
		if merr := s.ops.MarkHandled(ctx, opID, now); merr != nil {
			return merr
		}
		created = c
		return nil
	}); err != nil {
		return nil, err
	}

	return &CreateCategoryResult{Item: toResult(created)}, nil
}
