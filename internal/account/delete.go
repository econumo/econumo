// Delete use case: soft-delete an account the user owns.
package account

import (
	"context"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DeleteAccount handles delete-account. Access requires the owner OR any
// accounts_access grant. The OWNER soft-deletes the account (sets is_deleted); a
// NON-owner with a grant instead drops their own access via the connection module
// (RevokeOwnAccess), unwinding their folder memberships + accounts_options.
// Without a connection module wired, a non-owner gets AccessDenied (403). Returns
// an empty result ({}).
func (s *Service) DeleteAccount(ctx context.Context, userID vo.Id, req DeleteAccountRequest) (*DeleteAccountResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}

	acct, gerr := s.repo.GetByID(ctx, id)
	if gerr != nil {
		return nil, gerr
	}

	if acct.UserId().Equal(userID) {
		if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
			acct.Delete(s.clock.Now())
			return s.repo.Save(ctx, acct)
		}); err != nil {
			return nil, err
		}
		return &DeleteAccountResult{}, nil
	}

	// Non-owner: must have a grant, then revoke their own access. No connection
	// module -> AccessDenied.
	if s.revoker == nil {
		return nil, errs.NewAccessDenied("Access denied")
	}
	hasAccess, herr := s.revoker.HasAccess(ctx, userID, id)
	if herr != nil {
		return nil, herr
	}
	if !hasAccess {
		return nil, errs.NewAccessDenied("Access denied")
	}
	if rerr := s.revoker.RevokeOwnAccess(ctx, userID, id); rerr != nil {
		return nil, rerr
	}
	return &DeleteAccountResult{}, nil
}
