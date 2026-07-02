// Service wiring for the connection module: the use-case orchestrator, its
// dependency seams (the AccountAccessRepository, the folder/options ports needed
// for the access side effects, and the user-embed lookup), the constructor, and
// shared helpers.
package connection

import (
	"context"
	"errors"
	"time"

	domconnection "github.com/econumo/econumo/internal/domain/connection"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// Clock supplies the current time (seam for deterministic tests).
type Clock interface {
	Now() time.Time
}

// TxRunner is the transaction boundary the service owns.
type TxRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// OwnerView is the minimal user shape the connection list embeds.
type OwnerView struct {
	ID     string
	Name   string
	Avatar string
}

// UserLookup resolves the connected-user embed (id, name, avatar).
type UserLookup interface {
	GetOwner(ctx context.Context, userID string) (OwnerView, error)
}

// FolderPort is the slice of folder behavior the connection side effects need:
// finding the guest's last folder (by position) and mutating membership. Backed
// by the account module's FolderRepository via an adapter.
type FolderPort interface {
	// LastFolderID returns the user's last folder (highest position). ok=false if
	// the user has no folders.
	LastFolderID(ctx context.Context, userID vo.Id) (folderID vo.Id, ok bool, err error)
	// FoldersContaining returns the user's folder ids that contain the account.
	FoldersContaining(ctx context.Context, userID, accountID vo.Id) ([]vo.Id, error)
	// AddAccount adds the account to the folder (idempotent).
	AddAccount(ctx context.Context, folderID, accountID vo.Id) error
	// RemoveAccount removes the account from the folder.
	RemoveAccount(ctx context.Context, folderID, accountID vo.Id) error
}

// OptionPort is the slice of accounts_options behavior the side effects need.
type OptionPort interface {
	// MaxPosition returns the user's highest accounts_options.position (0 if none).
	MaxPosition(ctx context.Context, userID vo.Id) (int16, error)
	// SavePosition upserts the user's accounts_options row.
	SavePosition(ctx context.Context, accountID, userID vo.Id, position int16, now time.Time) error
}

// Service is the connection write+read orchestrator. It owns the tx boundary and
// builds response-shaped *Result structs directly.
type Service struct {
	access       domconnection.AccountAccessRepository
	invites      domconnection.InviteRepository
	folders      FolderPort
	options      OptionPort
	users        UserLookup
	budgetAccess BudgetAccessRevoker
	tx           TxRunner
	clock        Clock
}

// NewService wires the connection service. invites backs the generate/accept/
// delete-invite + delete-connection flows (the cloud-edition endpoints).
// budgetAccess drops budget sharing on delete-connection; it may be nil
// (delete-connection then only unwinds account access + the connection link).
func NewService(
	access domconnection.AccountAccessRepository,
	invites domconnection.InviteRepository,
	folders FolderPort,
	options OptionPort,
	users UserLookup,
	budgetAccess BudgetAccessRevoker,
	tx TxRunner,
	clock Clock,
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
	if grant.Role() == domconnection.RoleAdmin {
		return nil
	}
	return errs.NewAccessDenied("Access denied")
}
