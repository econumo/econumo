// Connection invite + delete-connection use cases — the endpoints enabled in the
// cloud edition (generate/accept/delete-invite and delete-connection).
package connection

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// GenerateInvite creates (or refreshes) the user's outstanding invite code and
// returns {code, expiredAt}.
func (s *Service) GenerateInvite(ctx context.Context, userID vo.Id, _ model.GenerateInviteRequest) (*model.GenerateInviteResult, error) {
	var inv *model.ConnectionInvite
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		existing, err := s.invites.GetByUser(txCtx, userID)
		if err != nil {
			return err
		}
		if existing == nil {
			existing = model.NewConnectionInvite(userID)
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
	return &model.GenerateInviteResult{Item: model.ConnectionInviteResult{
		Code:      inv.Code.Value(),
		ExpiredAt: inv.ExpiredAt.Format(datetime.Layout),
	}}, nil
}

// DeleteInvite clears the user's outstanding invite (no-op if none).
func (s *Service) DeleteInvite(ctx context.Context, userID vo.Id, _ model.DeleteInviteRequest) (*model.DeleteInviteResult, error) {
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
	return &model.DeleteInviteResult{}, nil
}

// AcceptInvite redeems a code: it connects the redeeming user with the invite's
// owner (symmetric users_connections link), clears the code, and returns the
// redeeming user's full connection list.
func (s *Service) AcceptInvite(ctx context.Context, userID vo.Id, req model.AcceptInviteRequest) (*model.AcceptInviteResult, error) {
	code, err := model.NewConnectionCode(req.Code)
	if err != nil {
		return nil, err
	}
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		inv, gerr := s.invites.GetByCode(txCtx, code, s.clock.Now())
		if gerr != nil {
			return gerr
		}
		if inv.UserID.Equal(userID) {
			return &errs.ValidationError{Msg: "Inviting yourself?", MsgCode: errs.CodeConnectionInvitingYourself}
		}
		if cerr := s.access.ConnectUsers(txCtx, inv.UserID, userID); cerr != nil {
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
	return &model.AcceptInviteResult{Items: list.Items}, nil
}

// DeleteConnection disconnects the requesting user from a connected user: it
// revokes every account-access grant shared between them (both directions, via
// the account feature's port), drops any budget access between them (both
// directions), and removes the symmetric users_connections link.
func (s *Service) DeleteConnection(ctx context.Context, userID vo.Id, req model.DeleteConnectionRequest) (*model.DeleteConnectionResult, error) {
	connectedID, err := parseID("id", req.Id)
	if err != nil {
		return nil, err
	}
	if connectedID.Equal(userID) {
		return nil, &errs.ValidationError{Msg: "Deleting yourself?", MsgCode: errs.CodeConnectionDeletingYourself}
	}

	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if s.accountAccess != nil {
			if aerr := s.accountAccess.RevokeAccessBetween(txCtx, userID, connectedID); aerr != nil {
				return aerr
			}
		}
		if s.budgetAccess != nil {
			if berr := s.budgetAccess.RevokeBetween(txCtx, userID, connectedID); berr != nil {
				return berr
			}
		}
		return s.access.DeleteConnection(txCtx, userID, connectedID)
	}); err != nil {
		return nil, err
	}
	return &model.DeleteConnectionResult{}, nil
}
