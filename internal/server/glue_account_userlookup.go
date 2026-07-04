// AccountUserLookup satisfies the account service's UserLookup port (owner
// embed) by delegating to the user repository. It lives here, not in
// internal/account, because account and user are separate features that must
// not import each other (see archtest); only the composition root may bridge
// them.
package server

import (
	"context"

	appaccount "github.com/econumo/econumo/internal/account"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// accountUserByID is the minimal user-repo surface this adapter needs.
type accountUserByID interface {
	GetHeaderByID(ctx context.Context, id vo.Id) (model.Header, error)
}

// AccountUserLookup adapts the user repository to account.UserLookup.
type AccountUserLookup struct {
	users accountUserByID
}

var _ appaccount.UserLookup = (*AccountUserLookup)(nil)

// NewAccountUserLookup wraps a user repository (anything exposing GetHeaderByID).
func NewAccountUserLookup(users accountUserByID) *AccountUserLookup {
	return &AccountUserLookup{users: users}
}

// GetOwner resolves the owner (id, name, avatar) for the account-result embed.
func (l *AccountUserLookup) GetOwner(ctx context.Context, userID string) (model.OwnerView, error) {
	id, err := vo.ParseId(userID)
	if err != nil {
		return model.OwnerView{}, err
	}
	h, err := l.users.GetHeaderByID(ctx, id)
	if err != nil {
		return model.OwnerView{}, err
	}
	return model.OwnerView{ID: h.ID, Name: h.Name, Avatar: h.AvatarURL}, nil
}
