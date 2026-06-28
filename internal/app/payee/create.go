// Create use case: create a payee, idempotent on the request id.
package payee

import (
	"context"

	dompayee "github.com/econumo/econumo/internal/domain/payee"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// CreatePayee creates a payee for the current user and returns it.
//
// Idempotency: the request id doubles as the operation id. Inside the tx we
// Claim the id in operation_requests_ids; a second request with the same id
// finds the row already present and is rejected with a ValidationError
// ("Operation is locked").
//
// Uniqueness: a payee name must be unique among the owner's payees; a duplicate
// is rejected with "Payee already exists." (mirrors PHP
// PayeeAlreadyExistsException).
//
// New-payee position = count(user's existing payees); the new payee is active
// with created/updated = now.
func (s *Service) CreatePayee(ctx context.Context, userID vo.Id, req CreatePayeeRequest) (*CreatePayeeResult, error) {
	// Request id is the OPERATION id; PHP mints a fresh entity UUIDv7 via
	// payeeFactory->create (getNextIdentity). Mirror that.
	opID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	id := vo.NewId()
	name, err := newPayeeName(req.Name)
	if err != nil {
		return nil, err
	}

	// accountId, when present, selects which user owns the new payee: an account
	// may belong to a connected user, and a payee added in the context of a shared
	// account is owned by the ACCOUNT OWNER (PHP createPayeeForAccount), gated by
	// an owner/admin access check (canAddPayee == isAdmin). Absent accountId ->
	// owned by the caller.
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

	var created *dompayee.Payee
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		already, cerr := s.ops.Claim(txCtx, opID, s.clock.Now())
		if cerr != nil {
			return cerr
		}
		if already {
			return errs.NewValidation("Operation is locked")
		}

		if uerr := s.ensureNameUnique(txCtx, ownerID, name, vo.Id{}); uerr != nil {
			return uerr
		}

		count, cerr := s.repo.CountByOwner(txCtx, ownerID)
		if cerr != nil {
			return cerr
		}
		now := s.clock.Now()
		p := dompayee.NewPayee(id, ownerID, name, now)
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

	return &CreatePayeeResult{Item: toResult(created)}, nil
}
