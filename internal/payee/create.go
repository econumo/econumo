package payee

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreatePayee is idempotent on the request id: inside the tx we Claim the id in
// operation_requests_ids; a second request with the same id finds the row
// already present and is rejected ("Operation is locked"). The new payee's
// position is count(user's existing payees).
func (s *Service) CreatePayee(ctx context.Context, userID vo.Id, req model.CreatePayeeRequest) (*model.CreatePayeeResult, error) {
	// The request id is the OPERATION id; the entity gets a fresh UUIDv7.
	opID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	id := vo.NewId()
	name, err := newPayeeName(req.Name)
	if err != nil {
		return nil, err
	}

	// accountId, when present, selects which user owns the new payee: a payee
	// added in the context of a shared account is owned by the ACCOUNT OWNER,
	// gated by an owner/admin access check. Absent accountId -> owned by the
	// caller.
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

	var created *model.Payee
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		already, cerr := s.ops.Claim(txCtx, opID, s.clock.Now())
		if cerr != nil {
			return cerr
		}
		if already {
			return &errs.ValidationError{Msg: "Operation is locked", MsgCode: errs.CodeOperationLocked}
		}

		if uerr := s.ensureNameUnique(txCtx, ownerID, name, vo.Id{}); uerr != nil {
			return uerr
		}

		count, cerr := s.repo.CountByOwner(txCtx, ownerID)
		if cerr != nil {
			return cerr
		}
		now := s.clock.Now()
		p := model.NewPayee(id, ownerID, name, now)
		p.SetPosition(int16(count))
		if serr := s.repo.Save(txCtx, p); serr != nil {
			return serr
		}
		if merr := s.ops.MarkHandled(txCtx, opID, now); merr != nil {
			return merr
		}
		created = p
		return nil
	}); err != nil {
		return nil, err
	}

	return &model.CreatePayeeResult{Item: toResult(created)}, nil
}
