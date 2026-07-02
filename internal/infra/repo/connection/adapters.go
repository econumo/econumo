// Adapters that satisfy the connection service's OptionPort by delegating to
// the account repository. Lives here (infra) so app/connection depends only
// on its own small interfaces. The UserLookup, FolderPort, SharedAccessLookup
// and AccessRevoker counterparts live in internal/server: they need the user
// and account features' types, which an infra package must not import.
package connectionrepo

import (
	"context"
	"errors"
	"time"

	appconnection "github.com/econumo/econumo/internal/app/connection"
	domconnection "github.com/econumo/econumo/internal/domain/connection"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// accountAccessFull is the subset of the connection AccountAccess repo used to
// resolve an account's owner and a connected user's grant role for the
// create-for-account path (category/tag create with an accountId).
type accountAccessFull interface {
	AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error)
	Get(ctx context.Context, accountID, userID vo.Id) (*domconnection.AccountAccess, error)
}

// AccountAccessResolver answers "who owns this account" and "what role does this
// user hold on it" — the two questions the category and tag create-for-account
// paths need: the add check requires an admin grant, and ownership of the new
// entity goes to the account owner. It structurally satisfies the AccountAccess
// port that the category and tag app services declare.
type AccountAccessResolver struct{ access accountAccessFull }

// NewAccountAccessResolver wraps the connection AccountAccess repo.
func NewAccountAccessResolver(access accountAccessFull) *AccountAccessResolver {
	return &AccountAccessResolver{access: access}
}

// AccountOwner returns the owner user id of an account.
func (r *AccountAccessResolver) AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error) {
	return r.access.AccountOwner(ctx, accountID)
}

// GrantRole returns the user's granted role on the account. A missing grant
// (the repo's *errs.NotFoundError) is reported as ok=false, nil error; other
// errors propagate.
func (r *AccountAccessResolver) GrantRole(ctx context.Context, accountID, userID vo.Id) (domconnection.Role, bool, error) {
	grant, err := r.access.Get(ctx, accountID, userID)
	if err != nil {
		var nf *errs.NotFoundError
		if errors.As(err, &nf) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return grant.Role(), true, nil
}

// HasAdminGrant reports whether the user holds an admin grant on the account —
// the category feature's own AccountAccess port shape (it must not import
// domain/connection directly, so the Role comparison happens here instead of
// at the call site). A missing grant or a non-admin grant is false, nil error;
// other errors propagate.
func (r *AccountAccessResolver) HasAdminGrant(ctx context.Context, accountID, userID vo.Id) (bool, error) {
	grant, err := r.access.Get(ctx, accountID, userID)
	if err != nil {
		var nf *errs.NotFoundError
		if errors.As(err, &nf) {
			return false, nil
		}
		return false, err
	}
	return grant.Role() == domconnection.RoleAdmin, nil
}

// optionRepo is the slice of the account Repository the connection side effects
// need (accounts_options position).
type optionRepo interface {
	MaxPosition(ctx context.Context, userID vo.Id) (int16, error)
	SavePosition(ctx context.Context, accountID, userID vo.Id, position int16, now time.Time) error
}

// OptionPort adapts the account Repository to app/connection.OptionPort.
type OptionPort struct{ accounts optionRepo }

var _ appconnection.OptionPort = (*OptionPort)(nil)

// NewOptionPort wraps an account Repository (anything exposing the position ops).
func NewOptionPort(accounts optionRepo) *OptionPort { return &OptionPort{accounts: accounts} }

// MaxPosition returns the user's highest accounts_options.position.
func (p *OptionPort) MaxPosition(ctx context.Context, userID vo.Id) (int16, error) {
	return p.accounts.MaxPosition(ctx, userID)
}

// SavePosition upserts the user's accounts_options row.
func (p *OptionPort) SavePosition(ctx context.Context, accountID, userID vo.Id, position int16, now time.Time) error {
	return p.accounts.SavePosition(ctx, accountID, userID, position, now)
}
