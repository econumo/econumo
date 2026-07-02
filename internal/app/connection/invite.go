// Connection invite + delete-connection use cases — the endpoints enabled in the
// cloud edition (generate/accept/delete-invite and delete-connection).
package connection

import (
	"context"
	"errors"

	domconnection "github.com/econumo/econumo/internal/domain/connection"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// BudgetAccessRevoker is the slice of budget-sharing behavior delete-connection
// needs: drop any budget access SHARED BETWEEN the two users (both directions).
// Backed by the budget module via an adapter in main. A nil revoker is tolerated
// (delete-connection then only unwinds account access + the connection link) so
// test harnesses without the budget module still work.
type BudgetAccessRevoker interface {
	// RevokeBetween removes any budget-access grants where one user owns a budget
	// the other can access, in BOTH directions, for the given user pair.
	RevokeBetween(ctx context.Context, a, b vo.Id) error
}

// GenerateInvite creates (or refreshes) the user's outstanding invite code and
// returns {code, expiredAt}.
func (s *Service) GenerateInvite(ctx context.Context, userID vo.Id, _ GenerateInviteRequest) (*GenerateInviteResult, error) {
	var inv *domconnection.ConnectionInvite
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		existing, err := s.invites.GetByUser(txCtx, userID)
		if err != nil {
			return err
		}
		if existing == nil {
			existing = domconnection.NewConnectionInvite(userID)
		}
		existing.GenerateNewCode(s.clock.Now())
		if serr := s.invites.Save(txCtx, existing); serr != nil {
			return serr
		}
		inv = existing
		return nil
	}); err != nil {
		return nil, err
	}
	return &GenerateInviteResult{Item: ConnectionInviteResult{
		Code:      inv.Code().Value(),
		ExpiredAt: inv.ExpiredAt().Format(datetime.Layout),
	}}, nil
}

// DeleteInvite clears the user's outstanding invite (no-op if none).
func (s *Service) DeleteInvite(ctx context.Context, userID vo.Id, _ DeleteInviteRequest) (*DeleteInviteResult, error) {
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		inv, err := s.invites.GetByUser(txCtx, userID)
		if err != nil {
			return err
		}
		if inv == nil {
			return nil // no invite — nothing to clear
		}
		inv.ClearCode()
		return s.invites.Save(txCtx, inv)
	}); err != nil {
		return nil, err
	}
	return &DeleteInviteResult{}, nil
}

// AcceptInvite redeems a code: it connects the redeeming user with the invite's
// owner (symmetric users_connections link), clears the code, and returns the
// redeeming user's full connection list.
func (s *Service) AcceptInvite(ctx context.Context, userID vo.Id, req AcceptInviteRequest) (*AcceptInviteResult, error) {
	code, err := domconnection.NewConnectionCode(req.Code)
	if err != nil {
		return nil, err
	}
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		inv, gerr := s.invites.GetByCode(txCtx, code, s.clock.Now())
		if gerr != nil {
			return gerr
		}
		if inv.UserId().Equal(userID) {
			return errs.NewValidation("Inviting yourself?")
		}
		if cerr := s.access.ConnectUsers(txCtx, inv.UserId(), userID); cerr != nil {
			return cerr
		}
		inv.ClearCode()
		return s.invites.Save(txCtx, inv)
	}); err != nil {
		return nil, err
	}
	// Build the connection-list response (same shape as get-connection-list).
	list, err := s.GetConnectionList(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &AcceptInviteResult{Items: list.Items}, nil
}

// DeleteConnection disconnects the requesting user from a connected user: it
// revokes every account-access grant shared between them (both directions),
// drops any budget access between them (both directions), and removes the
// symmetric users_connections link.
func (s *Service) DeleteConnection(ctx context.Context, userID vo.Id, req DeleteConnectionRequest) (*DeleteConnectionResult, error) {
	connectedID, err := parseID("id", req.Id)
	if err != nil {
		return nil, err
	}
	if connectedID.Equal(userID) {
		return nil, errs.NewValidation("Deleting yourself?")
	}

	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		// Revoke account access where the connected user owns an account shared TO
		// me (received), and where I own an account shared TO the connected user
		// (issued).
		received, rerr := s.access.ListReceived(txCtx, userID)
		if rerr != nil {
			return rerr
		}
		for _, a := range received {
			owner, oerr := s.access.AccountOwner(txCtx, a.AccountId())
			if oerr != nil {
				return oerr
			}
			if owner.Equal(connectedID) {
				if rg := s.revokeGrantTx(txCtx, a.AccountId(), a.UserId()); rg != nil {
					return rg
				}
			}
		}
		issued, ierr := s.access.ListIssued(txCtx, userID)
		if ierr != nil {
			return ierr
		}
		for _, a := range issued {
			if a.UserId().Equal(connectedID) {
				if rg := s.revokeGrantTx(txCtx, a.AccountId(), a.UserId()); rg != nil {
					return rg
				}
			}
		}

		// Drop budget access between the two users (both directions), if the budget
		// module is wired.
		if s.budgetAccess != nil {
			if berr := s.budgetAccess.RevokeBetween(txCtx, userID, connectedID); berr != nil {
				return berr
			}
		}

		// Remove the symmetric connection link.
		return s.access.DeleteConnection(txCtx, userID, connectedID)
	}); err != nil {
		return nil, err
	}
	return &DeleteConnectionResult{}, nil
}

// revokeGrantTx unwinds one grant (folders + options + the grant row) on the
// CURRENT transaction context. It is revokeGrant's body without opening its own
// tx (delete-connection already holds one; nesting would savepoint, so reusing
// the same tx keeps it all in a single transaction).
func (s *Service) revokeGrantTx(txCtx context.Context, accountID, affectedUserID vo.Id) error {
	if _, gerr := s.access.Get(txCtx, accountID, affectedUserID); gerr != nil {
		var nf *errs.NotFoundError
		if errors.As(gerr, &nf) {
			return nil // already gone
		}
		return gerr
	}
	folderIDs, ferr := s.folders.FoldersContaining(txCtx, affectedUserID, accountID)
	if ferr != nil {
		return ferr
	}
	for _, fid := range folderIDs {
		if rerr := s.folders.RemoveAccount(txCtx, fid, accountID); rerr != nil {
			return rerr
		}
	}
	if oerr := s.access.DeleteOption(txCtx, accountID, affectedUserID); oerr != nil {
		return oerr
	}
	return s.access.Delete(txCtx, accountID, affectedUserID)
}
