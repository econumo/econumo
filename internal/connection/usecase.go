// Service wiring for the connection module: the use-case orchestrator, its
// dependency seams (the AccountAccessRepository, the folder/options ports needed
// for the access side effects, and the user-embed lookup), the constructor, and
// shared helpers.
package connection

import (
	"context"
	"errors"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Service is the connection write+read orchestrator. It owns the tx boundary and
// builds response-shaped *Result structs directly.
type Service struct {
	access       AccountAccessRepository
	invites      InviteRepository
	folders      FolderPort
	options      OptionPort
	users        UserLookup
	budgetAccess BudgetAccessRevoker
	tx           port.TxRunner
	clock        port.Clock
}

// NewService wires the connection service. invites backs the generate/accept/
// delete-invite + delete-connection flows (the cloud-edition endpoints).
// budgetAccess drops budget sharing on delete-connection; it may be nil
// (delete-connection then only unwinds account access + the connection link).
func NewService(
	access AccountAccessRepository,
	invites InviteRepository,
	folders FolderPort,
	options OptionPort,
	users UserLookup,
	budgetAccess BudgetAccessRevoker,
	tx port.TxRunner,
	clock port.Clock,
) *Service {
	return &Service{
		access: access, invites: invites, folders: folders, options: options,
		users: users, budgetAccess: budgetAccess, tx: tx, clock: clock,
	}
}

// parseID converts a primitive id string to a vo.Id, surfacing a validation
// error on an invalid UUID.
func parseID(field, s string) (vo.Id, error) {
	id, err := vo.ParseId(s)
	if err != nil {
		return vo.Id{}, errs.NewValidation("Validation failed", errs.FieldError{
			Key: field, Message: "This is not a valid UUID.", Code: "INVALID_UUID_ERROR",
		})
	}
	return id, nil
}

// requireOwnerAdmin checks the requesting user may UPDATE the account: they own
// it, or they hold an admin grant on it. Returns AccessDenied otherwise.
func (s *Service) requireOwnerAdmin(ctx context.Context, userID, accountID vo.Id) error {
	owner, err := s.access.AccountOwner(ctx, accountID)
	if err != nil {
		return err
	}
	if owner.Equal(userID) {
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
	if grant.Role == model.RoleAdmin {
		return nil
	}
	return errs.NewAccessDenied("Access denied")
}
