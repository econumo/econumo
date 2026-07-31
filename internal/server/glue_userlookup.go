// UserOwnerLookup resolves a user's public header (id, name, avatar) for the
// account, connection, and transaction features — after the shared model
// unified their view types, all three declare the identical consumer-side
// UserLookup port, so one adapter serves them. It lives here, not in a
// feature, because features must not import each other (see archtest); only
// the composition root may bridge them.
package server

import (
	"context"

	appaccount "github.com/econumo/econumo/internal/account"
	appconnection "github.com/econumo/econumo/internal/connection"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	apptransaction "github.com/econumo/econumo/internal/transaction"
)

// userHeaderByID is the minimal user-repo surface this adapter needs.
type userHeaderByID interface {
	GetHeaderByID(ctx context.Context, id vo.Id) (model.Header, error)
}

// UserOwnerLookup adapts the user repository to the three UserLookup ports.
type UserOwnerLookup struct {
	users userHeaderByID
}

var (
	_ appaccount.UserLookup     = (*UserOwnerLookup)(nil)
	_ appconnection.UserLookup  = (*UserOwnerLookup)(nil)
	_ apptransaction.UserLookup = (*UserOwnerLookup)(nil)
)

// NewUserOwnerLookup wraps a user repository (anything exposing GetHeaderByID).
func NewUserOwnerLookup(users userHeaderByID) *UserOwnerLookup {
	return &UserOwnerLookup{users: users}
}

// GetOwner resolves the owner (id, name, avatar) for result embeds, plus the
// raw access columns the connection list needs (account/transaction author
// embeds ignore them).
func (l *UserOwnerLookup) GetOwner(ctx context.Context, userID string) (model.OwnerView, error) {
	id, err := vo.ParseId(userID)
	if err != nil {
		return model.OwnerView{}, err
	}
	h, err := l.users.GetHeaderByID(ctx, id)
	if err != nil {
		return model.OwnerView{}, err
	}
	return model.OwnerView{
		ID: h.ID, Name: h.Name, Avatar: h.Avatar,
		AccessLevel: h.AccessLevel, AccessUntil: h.AccessUntil,
	}, nil
}
