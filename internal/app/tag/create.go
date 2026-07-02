package tag

import (
	"context"

	domtag "github.com/econumo/econumo/internal/domain/tag"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateTag is idempotent on the request id: inside the tx we Claim the id in
// operation_requests_ids; a second request with the same id finds the row
// already present and is rejected ("Operation is locked"). The new tag's
// position is count(user's existing tags).
func (s *Service) CreateTag(ctx context.Context, userID vo.Id, req CreateTagRequest) (*CreateTagResult, error) {
	// The request id is the OPERATION id (idempotency key), not the entity id;
	// the entity gets a fresh UUIDv7.
	opID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	id := vo.NewId()
	name, err := newTagName(req.Name)
	if err != nil {
		return nil, err
	}

	// accountId, when present, selects which user owns the new tag: a tag added
	// in the context of a shared account is owned by the ACCOUNT OWNER, gated by
	// an owner/admin access check. Absent accountId -> owned by the caller.
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

	var created *domtag.Tag
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		already, cerr := s.ops.Claim(ctx, opID, s.clock.Now())
		if cerr != nil {
			return cerr
		}
		if already {
			return errs.NewValidation("Operation is locked")
		}

		if uerr := s.ensureNameUnique(ctx, ownerID, name, vo.Id{}); uerr != nil {
			return uerr
		}

		count, cerr := s.repo.CountByOwner(ctx, ownerID)
		if cerr != nil {
			return cerr
		}
		now := s.clock.Now()
		t := domtag.NewTag(id, ownerID, name, now)
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
