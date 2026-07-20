// Adapters bridging the admin feature to the user and connection features. They
// live here because features must not import each other; only the composition
// root may join them.
package server

import (
	"context"
	"time"

	appadmin "github.com/econumo/econumo/internal/admin"
	appconnection "github.com/econumo/econumo/internal/connection"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	appuser "github.com/econumo/econumo/internal/user"
)

type AdminUserAccess struct{ users *appuser.Service }

var _ appadmin.UserLookup = (*AdminUserAccess)(nil)

func NewAdminUserAccess(users *appuser.Service) *AdminUserAccess {
	return &AdminUserAccess{users: users}
}

func (a *AdminUserAccess) GetUser(ctx context.Context, id vo.Id) (appadmin.UserRecord, error) {
	u, email, err := a.users.AdminUserByID(ctx, id)
	if err != nil {
		return appadmin.UserRecord{}, err
	}
	return appadmin.UserRecord{
		ID:          u.ID.String(),
		Name:        u.Name,
		Email:       email,
		AccessLevel: u.AccessLevel,
		AccessUntil: u.AccessUntil,
	}, nil
}

func (a *AdminUserAccess) SetAccess(ctx context.Context, id vo.Id, level model.AccessLevel, until *time.Time) error {
	return a.users.AdminSetAccessByID(ctx, id, level, until)
}

type AdminConnections struct{ conns *appconnection.Service }

var _ appadmin.ConnectionLookup = (*AdminConnections)(nil)

func NewAdminConnections(conns *appconnection.Service) *AdminConnections {
	return &AdminConnections{conns: conns}
}

func (a *AdminConnections) ConnectedUserIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error) {
	list, err := a.conns.GetConnectionList(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]vo.Id, 0, len(list.Items))
	for _, item := range list.Items {
		id, perr := vo.ParseId(item.User.Id)
		if perr != nil {
			return nil, perr
		}
		ids = append(ids, id)
	}
	return ids, nil
}
