// Account-access use cases: the grant -> pending -> accept/decline handshake
// plus revocation. The wire semantics mirror the budget feature's
// grant/accept/decline/revoke contract.
package account

import (
	"context"
	"errors"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// parseAccessID converts a primitive id string to a vo.Id, surfacing the
// standard invalid-UUID validation error.
func parseAccessID(field, s string) (vo.Id, error) {
	id, err := vo.ParseId(s)
	if err != nil {
		return vo.Id{}, errs.NewValidation("Validation failed", errs.FieldError{
			Key: field, Message: "This is not a valid UUID.", Code: "INVALID_UUID_ERROR",
		})
	}
	return id, nil
}

// requireOwnerAdmin gates grant/revoke: the caller owns the account or holds
// an ACCEPTED admin grant on it.
func (s *Service) requireOwnerAdmin(ctx context.Context, userID, accountID vo.Id) error {
	acct, err := s.accounts.GetByID(ctx, accountID)
	if err != nil {
		return err
	}
	if acct.UserID.Equal(userID) {
		return nil
	}
	grant, err := s.access.Get(ctx, accountID, userID)
	if err != nil {
		var nf *errs.NotFoundError
		if errors.As(err, &nf) {
			return errs.NewAccessDenied("Access denied")
		}
		return err
	}
	if grant.IsAccepted && grant.Role == model.RoleAdmin {
		return nil
	}
	return errs.NewAccessDenied("Access denied")
}

func (s *Service) GrantAccess(ctx context.Context, userID vo.Id, req model.GrantAccountAccessRequest) (*model.GrantAccountAccessResult, error) {
	accountID, err := parseAccessID("accountId", req.AccountId)
	if err != nil {
		return nil, err
	}
	affectedUserID, err := parseAccessID("userId", req.UserId)
	if err != nil {
		return nil, err
	}
	role, err := model.RoleFromAlias(req.Role)
	if err != nil {
		return nil, err
	}
	if err := s.requireOwnerAdmin(ctx, userID, accountID); err != nil {
		return nil, err
	}
	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		grant, gerr := s.access.Get(txCtx, accountID, affectedUserID)
		if gerr != nil {
			var nf *errs.NotFoundError
			if !errors.As(gerr, &nf) {
				return gerr
			}
			// New grant: pending, no placement -- the recipient places the
			// account when they accept.
			return s.access.Save(txCtx, model.NewAccountAccess(accountID, affectedUserID, role, now))
		}
		grant.UpdateRole(role, now)
		return s.access.Save(txCtx, grant)
	})
	if err != nil {
		return nil, err
	}
	return &model.GrantAccountAccessResult{}, nil
}

func (s *Service) AcceptAccess(ctx context.Context, userID vo.Id, req model.AcceptAccountAccessRequest) (*model.AcceptAccountAccessResult, error) {
	accountID, err := parseAccessID("accountId", req.AccountId)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		grant, gerr := s.access.Get(txCtx, accountID, userID)
		if gerr != nil {
			var nf *errs.NotFoundError
			if errors.As(gerr, &nf) {
				return errs.NewAccessDenied("Access denied")
			}
			return gerr
		}
		if grant.IsAccepted {
			return errs.NewAccessDenied("Access denied")
		}
		folderID, ferr := s.resolveAccountFolder(txCtx, userID, req.FolderId)
		if ferr != nil {
			return ferr
		}
		grant.Accept(now)
		if serr := s.access.Save(txCtx, grant); serr != nil {
			return serr
		}
		max, perr := s.positions.MaxPosition(txCtx, userID)
		if perr != nil {
			return perr
		}
		if perr := s.positions.SavePosition(txCtx, accountID, userID, max+1, now); perr != nil {
			return perr
		}
		return s.memberships.AddAccount(txCtx, folderID, accountID)
	})
	if err != nil {
		return nil, err
	}
	return &model.AcceptAccountAccessResult{}, nil
}

func (s *Service) DeclineAccess(ctx context.Context, userID vo.Id, req model.DeclineAccountAccessRequest) (*model.DeclineAccountAccessResult, error) {
	accountID, err := parseAccessID("accountId", req.AccountId)
	if err != nil {
		return nil, err
	}
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if _, gerr := s.access.Get(txCtx, accountID, userID); gerr != nil {
			var nf *errs.NotFoundError
			if errors.As(gerr, &nf) {
				return errs.NewAccessDenied("Access denied")
			}
			return gerr
		}
		return s.unwindGrant(txCtx, accountID, userID)
	}); err != nil {
		return nil, err
	}
	return &model.DeclineAccountAccessResult{}, nil
}

func (s *Service) RevokeAccess(ctx context.Context, userID vo.Id, req model.RevokeAccountAccessRequest) (*model.RevokeAccountAccessResult, error) {
	accountID, err := parseAccessID("accountId", req.AccountId)
	if err != nil {
		return nil, err
	}
	affectedUserID, err := parseAccessID("userId", req.UserId)
	if err != nil {
		return nil, err
	}
	if err := s.requireOwnerAdmin(ctx, userID, accountID); err != nil {
		return nil, err
	}
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		// Load first so a missing grant surfaces NotFound before cleanup.
		if _, gerr := s.access.Get(txCtx, accountID, affectedUserID); gerr != nil {
			return gerr
		}
		return s.unwindGrant(txCtx, accountID, affectedUserID)
	}); err != nil {
		return nil, err
	}
	return &model.RevokeAccountAccessResult{}, nil
}

// RevokeOwnAccess drops the caller's own grant (the delete-account non-owner
// branch).
func (s *Service) RevokeOwnAccess(ctx context.Context, userID, accountID vo.Id) error {
	return s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if _, gerr := s.access.Get(txCtx, accountID, userID); gerr != nil {
			return gerr
		}
		return s.unwindGrant(txCtx, accountID, userID)
	})
}

// RevokeAccessBetween removes every grant shared between the two users, both
// directions. It runs on the CALLER's transaction context (delete-connection
// already holds one; opening another would savepoint).
func (s *Service) RevokeAccessBetween(ctx context.Context, a, b vo.Id) error {
	received, err := s.access.ListReceived(ctx, a)
	if err != nil {
		return err
	}
	for _, g := range received {
		acct, gerr := s.accounts.GetByID(ctx, g.AccountID)
		if gerr != nil {
			return gerr
		}
		if acct.UserID.Equal(b) {
			if uerr := s.unwindGrant(ctx, g.AccountID, g.UserID); uerr != nil {
				return uerr
			}
		}
	}
	issued, err := s.access.ListIssued(ctx, a)
	if err != nil {
		return err
	}
	for _, g := range issued {
		if g.UserID.Equal(b) {
			if uerr := s.unwindGrant(ctx, g.AccountID, g.UserID); uerr != nil {
				return uerr
			}
		}
	}
	return nil
}

// unwindGrant removes affectedUserID's view of the account (folder memberships
// + accounts_options) and the grant row, on the current (tx) context.
func (s *Service) unwindGrant(ctx context.Context, accountID, affectedUserID vo.Id) error {
	memberships, err := s.memberships.MembershipsByUser(ctx, affectedUserID)
	if err != nil {
		return err
	}
	for folderID, accountIDs := range memberships {
		for _, aid := range accountIDs {
			if aid == accountID.String() {
				fid, perr := vo.ParseId(folderID)
				if perr != nil {
					return perr
				}
				if rerr := s.memberships.RemoveAccount(ctx, fid, accountID); rerr != nil {
					return rerr
				}
				break
			}
		}
	}
	if oerr := s.access.DeleteOption(ctx, accountID, affectedUserID); oerr != nil {
		return oerr
	}
	return s.access.Delete(ctx, accountID, affectedUserID)
}
