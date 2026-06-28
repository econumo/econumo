// Create use case: create a tag, idempotent on the request id.
package tag

import (
	"context"

	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	domtag "github.com/econumo/econumo/internal/domain/tag"
)

// CreateTag creates a tag for the current user and returns it.
//
// Idempotency: the request id doubles as the operation id. Inside the tx we
// Claim the id in operation_requests_ids; a second request with the same id
// finds the row already present and is rejected with a ValidationError
// ("Operation is locked").
//
// Uniqueness: a tag name must be unique among the owner's tags; a duplicate is
// rejected with "Tag already exists." (mirrors PHP TagAlreadyExistsException).
//
// New-tag position = count(user's existing tags); the new tag is active with
// created/updated = now.
func (s *Service) CreateTag(ctx context.Context, userID vo.Id, req CreateTagRequest) (*CreateTagResult, error) {
	// The request id is the OPERATION id (idempotency key), not the entity id.
	// PHP mints a fresh UUIDv7 via tagFactory->create (getNextIdentity); the dto
	// id is consumed only by the operation-id middleware. Mirror that.
	opID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	id := vo.NewId()
	name, err := newTagName(req.Name)
	if err != nil {
		return nil, err
	}

	// accountId, when present, selects which user owns the new tag in the full
	// app (an account may belong to a connected user). The connection module is
	// not ported yet, so we always create for the current user and ignore
	// accountId beyond validating its shape.
	if req.AccountId != nil && *req.AccountId != "" {
		if _, perr := vo.ParseId(*req.AccountId); perr != nil {
			return nil, perr
		}
	}

	var created *domtag.Tag
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		already, cerr := s.ops.Claim(ctx, opID, s.clock.Now())
		if cerr != nil {
			return cerr
		}
		if already {
			return errs.NewValidation("Operation is locked")
		}

		if uerr := s.ensureNameUnique(ctx, userID, name, vo.Id{}); uerr != nil {
			return uerr
		}

		count, cerr := s.repo.CountByOwner(ctx, userID)
		if cerr != nil {
			return cerr
		}
		now := s.clock.Now()
		t := domtag.NewTag(id, userID, name, now)
		t.SetPosition(int16(count))
		if serr := s.repo.Save(ctx, t); serr != nil {
			return serr
		}
		if merr := s.ops.MarkHandled(ctx, opID, now); merr != nil {
			return merr
		}
		created = t
		return nil
	}); err != nil {
		return nil, err
	}

	return &CreateTagResult{Item: toResult(created)}, nil
}
