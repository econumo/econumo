// Service wiring for the connection module: the use-case orchestrator, its
// dependency seams (the AccountAccessRepository, the account/budget access
// revokers needed for delete-connection's unwind, and the user-embed lookup),
// the constructor, and shared helpers.
package connection

import (
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/port"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Service is the connection write+read orchestrator. It owns the tx boundary and
// builds response-shaped *Result structs directly.
type Service struct {
	access        AccountAccessRepository
	invites       InviteRepository
	users         UserLookup
	accountAccess AccountAccessRevoker
	budgetAccess  BudgetAccessRevoker
	tx            port.TxRunner
	clock         port.Clock
}

// NewService wires the connection service. invites backs the generate/accept/
// delete-invite + delete-connection flows (the cloud-edition endpoints).
// accountAccess unwinds account sharing on delete-connection; budgetAccess
// drops budget sharing on delete-connection. Both may be nil (delete-connection
// then only unwinds whatever port(s) are wired plus the connection link).
func NewService(
	access AccountAccessRepository,
	invites InviteRepository,
	users UserLookup,
	accountAccess AccountAccessRevoker,
	budgetAccess BudgetAccessRevoker,
	tx port.TxRunner,
	clock port.Clock,
) *Service {
	return &Service{
		access: access, invites: invites,
		users: users, accountAccess: accountAccess, budgetAccess: budgetAccess, tx: tx, clock: clock,
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
