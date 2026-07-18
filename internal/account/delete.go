// Delete use case: soft-delete an account the user owns.
package account

import (
	"context"
	"errors"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// DeleteAccount handles delete-account. Access requires the owner OR any
// accounts_access grant. The OWNER soft-deletes the account (sets is_deleted); a
// NON-owner with a grant instead drops their own access (RevokeOwnAccess),
// unwinding their folder memberships + accounts_options. A non-owner with no
// grant gets AccessDenied (403). Returns an empty result ({}).
func (s *Service) DeleteAccount(ctx context.Context, userID vo.Id, req model.DeleteAccountRequest) (*model.DeleteAccountResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}

	acct, gerr := s.accounts.GetByID(ctx, id)
	if gerr != nil {
		return nil, gerr
	}

	if acct.UserID.Equal(userID) {
		if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
			acct.Delete(s.clock.Now())
			return s.accounts.Save(ctx, acct)
		}); err != nil {
			return nil, err
		}
		return &model.DeleteAccountResult{}, nil
	}

	// Non-owner: must hold a grant (pending counts -- deleting from their side
	// is a decline), then drop their own access. AccessStore.Get takes
	// (accountID, userID).
	if _, gerr := s.access.Get(ctx, id, userID); gerr != nil {
		var nf *errs.NotFoundError
		if errors.As(gerr, &nf) {
			return nil, errs.NewAccessDenied("Access denied")
		}
		return nil, gerr
	}
	if rerr := s.RevokeOwnAccess(ctx, userID, id); rerr != nil {
		return nil, rerr
	}
	return &model.DeleteAccountResult{}, nil
}
