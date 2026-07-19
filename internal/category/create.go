// Create use case: create a category, idempotent on the request id.
package category

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateCategory creates a category for the current user and returns it.
//
// Idempotency: the request id doubles as the operation id. Inside the tx we
// Claim the id in operation_requests_ids; a second request with the same id
// finds the row already present and is rejected with a ValidationError
// ("Operation is locked"). See repo/category for the row semantics.
//
// New-category position = count(user's existing categories); the new category is
// active with created/updated = now.
func (s *Service) CreateCategory(ctx context.Context, userID vo.Id, req model.CreateCategoryRequest) (*model.CreateCategoryResult, error) {
	// The request id is the OPERATION id (idempotency key), NOT the new entity's
	// id: the request id is consumed only by the operation guard, while a fresh
	// UUIDv7 is minted for the entity. So claim opID, then generate a new entity id.
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
	iconVal := model.DefaultIcon
	if req.Icon != nil && *req.Icon != "" {
		iconVal = *req.Icon
	}
	icon, err := newIcon(iconVal)
	if err != nil {
		return nil, err
	}

	// accountId, when present, selects which user owns the new category: an
	// account may belong to a connected user, and a category added in the context
	// of a shared account is owned by the ACCOUNT OWNER, gated by an owner/admin
	// access check. Absent accountId -> owned by the caller.
	ownerID := userID
	if req.AccountId != nil && *req.AccountId != "" {
		accountID, perr := vo.ParseId(*req.AccountId)
		if perr != nil {
			return nil, perr
		}
		resolved, aerr := s.resolveAccountOwner(ctx, userID, accountID)
		if aerr != nil {
			return nil, aerr
		}
		ownerID = resolved
	}

	var created *model.Category
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		already, cerr := s.ops.Claim(ctx, opID, s.clock.Now())
		if cerr != nil {
			return cerr
		}
		if already {
			return &errs.ValidationError{Msg: "Operation is locked", MsgCode: errs.CodeOperationLocked}
		}

		count, cerr := s.repo.CountByOwner(ctx, ownerID)
		if cerr != nil {
			return cerr
		}
		now := s.clock.Now()
		c := model.NewCategory(id, ownerID, name, typ, icon, now)
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

	return &model.CreateCategoryResult{Item: toResult(created)}, nil
}
