// AccountUserLookup satisfies the account service's UserLookup port (owner
// embed) by delegating to the user repository. It lives here, not in
// internal/infra/repo/account, because it needs the user feature's Header
// type and an infra package must not import a feature (see archtest).
package server

import (
	"context"

	appaccount "github.com/econumo/econumo/internal/app/account"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/user"
)

// accountUserByID is the minimal user-repo surface this adapter needs.
type accountUserByID interface {
	GetHeaderByID(ctx context.Context, id vo.Id) (user.Header, error)
}

// AccountUserLookup adapts the user repository to app/account.UserLookup.
type AccountUserLookup struct {
	users accountUserByID
}

var _ appaccount.UserLookup = (*AccountUserLookup)(nil)

// NewAccountUserLookup wraps a user repository (anything exposing GetHeaderByID).
func NewAccountUserLookup(users accountUserByID) *AccountUserLookup {
	return &AccountUserLookup{users: users}
}

// GetOwner resolves the owner (id, name, avatar) for the account-result embed.
func (l *AccountUserLookup) GetOwner(ctx context.Context, userID string) (appaccount.OwnerView, error) {
	id, err := vo.ParseId(userID)
	if err != nil {
		return appaccount.OwnerView{}, err
	}
	h, err := l.users.GetHeaderByID(ctx, id)
	if err != nil {
		return appaccount.OwnerView{}, err
	}
	return appaccount.OwnerView{ID: h.ID, Name: h.Name, Avatar: h.AvatarURL}, nil
}
