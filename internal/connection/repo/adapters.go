// AccountAccessResolver satisfies the category/tag AccountAccess port by
// delegating to the connection AccountAccess repo. Lives here (infra) so
// connection depends only on its own small interfaces. The UserLookup,
// FolderPort, SharedAccessLookup and AccessRevoker counterparts live in
// internal/server: they need the user and account features' types, which a
// feature package must not import.
package repo

import (
	"context"
	"errors"

	domconnection "github.com/econumo/econumo/internal/connection"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// AccountAccessResolver answers "who owns this account" and "what role does this
// user hold on it" — the two questions the category and tag create-for-account
// paths need: the add check requires an admin grant, and ownership of the new
// entity goes to the account owner. It structurally satisfies the AccountAccess
// port that the category and tag app services declare.
type AccountAccessResolver struct{ access *Repo }

// NewAccountAccessResolver wraps the connection AccountAccess repo.
func NewAccountAccessResolver(access *Repo) *AccountAccessResolver {
	return &AccountAccessResolver{access: access}
}

// AccountOwner returns the owner user id of an account.
func (r *AccountAccessResolver) AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error) {
	return r.access.AccountOwner(ctx, accountID)
}

// HasWriteGrant reports whether the user holds an admin OR user grant on the
// account — the transaction feature's write-access check (a guest grant or no
// grant is denied). A missing grant or a guest grant is false, nil error;
// other errors propagate.
func (r *AccountAccessResolver) HasWriteGrant(ctx context.Context, accountID, userID vo.Id) (bool, error) {
	grant, err := r.access.Get(ctx, accountID, userID)
	if err != nil {
		var nf *errs.NotFoundError
		if errors.As(err, &nf) {
			return false, nil
		}
		return false, err
	}
	role := grant.Role()
	return role == domconnection.RoleAdmin || role == domconnection.RoleUser, nil
}

// HasAdminGrant reports whether the user holds an admin grant on the account —
// the category feature's own AccountAccess port shape (it must not import
// connection directly, so the Role comparison happens here instead of
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
