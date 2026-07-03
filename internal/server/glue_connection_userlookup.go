// ConnectionUserLookup satisfies the connection service's UserLookup port
// (connected-user embed) by delegating to the user repository. It lives here,
// not in internal/connection/repo, because it needs the model package's
// Header type and an infra package must not import a feature (see archtest).
package server

import (
	"context"

	appconnection "github.com/econumo/econumo/internal/connection"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// connectionUserByID is the minimal user-repo surface this adapter needs.
type connectionUserByID interface {
	GetHeaderByID(ctx context.Context, id vo.Id) (model.Header, error)
}

// ConnectionUserLookup adapts the user repository to connection.UserLookup.
type ConnectionUserLookup struct{ users connectionUserByID }

var _ appconnection.UserLookup = (*ConnectionUserLookup)(nil)

// NewConnectionUserLookup wraps a user repository.
func NewConnectionUserLookup(users connectionUserByID) *ConnectionUserLookup {
	return &ConnectionUserLookup{users: users}
}

// GetOwner resolves the connected-user embed (id, name, avatar).
func (l *ConnectionUserLookup) GetOwner(ctx context.Context, userID string) (appconnection.OwnerView, error) {
	id, err := vo.ParseId(userID)
	if err != nil {
		return appconnection.OwnerView{}, err
	}
	h, err := l.users.GetHeaderByID(ctx, id)
	if err != nil {
		return appconnection.OwnerView{}, err
	}
	return appconnection.OwnerView{ID: h.ID, Name: h.Name, Avatar: h.AvatarURL}, nil
}
